# Security & Observability Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the DocScout-MCP server against input abuse and improve operator observability through five targeted improvements: entity name validation, search input sanitization, per-repo scan timeouts, indexer observability, and HTTP security + startup warnings.

**Architecture:** All changes are surgical modifications to existing files — no new packages. Each task is independent and produces working, tested code. Tasks 1–2 are pure input validation (parser/content/tools). Task 3 adds resilience to the scanner. Task 4 migrates indexer logging to `slog` and adds phase/failure observability. Task 5 hardens HTTP auth and adds startup-time operator warnings in `main.go`.

**Tech Stack:** Go 1.26.1, `crypto/subtle` (stdlib), `log/slog` (stdlib), GORM + SQLite/PostgreSQL, `github.com/modelcontextprotocol/go-sdk`

---

## File Map

| Action | Path                             | Responsibility                                                                  |
| ------ | -------------------------------- | ------------------------------------------------------------------------------- |
| Modify | `scanner/parser/catalog.go`      | Add `isValidEntityName` — reject names with dangerous chars                     |
| Modify | `scanner/parser/catalog_test.go` | Tests for invalid entity names                                                  |
| Modify | `memory/content.go`              | Escape LIKE wildcards; reject whitespace-only queries                           |
| Modify | `memory/content_test.go`         | Tests for wildcard and whitespace inputs                                        |
| Modify | `tools/search_docs.go`           | Trim whitespace on query args for `search_docs`                                 |
| Modify | `tools/search_content.go`        | Trim whitespace on query args for `search_content`                              |
| Modify | `tools/tools_test.go`            | Tests for whitespace-only queries                                               |
| Modify | `scanner/scanner.go`             | Add 30s per-repo context timeout in scan goroutines                             |
| Modify | `scanner/scanner_test.go`        | Test that scan completes when a repo context is pre-cancelled                   |
| Modify | `indexer/indexer.go`             | Replace `log.Printf` with `slog`; phase logging; failure summary                |
| Modify | `indexer/indexer_test.go`        | Verify no regression                                                            |
| Modify | `main.go`                        | Constant-time bearer token; OnScanComplete timing; filter/SCAN_CONTENT warnings |

---

## Task 1: Entity Name Validation in Catalog Parser (S3)

**Files:**

- Modify: `scanner/parser/catalog.go`
- Modify: `scanner/parser/catalog_test.go`

**Context:** The parser currently accepts any non-empty string as `metadata.name`. A catalog-info.yaml from a compromised repo could inject null bytes, newlines, or extremely long strings that corrupt graph queries or logs. Backstage's own spec constrains names to `[a-zA-Z0-9_.-]` with optional `namespace/` prefix. We'll enforce that.

- [ ] **Step 1: Add failing tests**

Add to the bottom of `scanner/parser/catalog_test.go`:

```go
func TestParseCatalog_InvalidEntityNames(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"null byte", "payment\x00service", true},
		{"newline", "payment\nservice", true},
		{"too long", "a" + strings.Repeat("b", 253), true},
		{"empty after trim", "  ", true},
		{"valid with dash", "payment-service", false},
		{"valid with dot", "payment.v2", false},
		{"valid with namespace", "default/payment-service", false},
		{"valid with underscore", "payment_service", false},
		{"valid with number", "svc-1", false},
	}
	for _, tc := range cases {
		yaml := []byte("apiVersion: backstage.io/v1alpha1\nkind: Component\nmetadata:\n  name: " + tc.input + "\nspec:\n  type: service\n")
		_, err := parser.ParseCatalog(yaml)
		if tc.wantErr && err == nil {
			t.Errorf("name=%q: expected error but got none", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("name=%q: unexpected error: %v", tc.name, err)
		}
	}
}
```

Note: add `"strings"` to the import block in `catalog_test.go`.

- [ ] **Step 2: Run failing tests**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go test ./scanner/parser/... -run TestParseCatalog_InvalidEntityNames -v"
```

Expected: FAIL — invalid names currently pass through.

- [ ] **Step 3: Add validation to `scanner/parser/catalog.go`**

Add the `regexp` import and the `isValidEntityName` function, then call it inside `ParseCatalog`.

**Add to the import block** (add `"regexp"` and `"strings"`):

```go
import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)
```

**Add after the `backstageCatalog` struct definition** (before `kindToEntityType`):

```go
// validEntityName matches Backstage-compatible entity names:
// optional "namespace/" prefix, then 1–253 chars of [a-zA-Z0-9._-].
var validEntityName = regexp.MustCompile(`^([a-zA-Z0-9._-]+/)?[a-zA-Z0-9._-]{1,253}$`)

// isValidEntityName returns false for names containing dangerous characters
// (null bytes, newlines, control characters) or exceeding length limits.
func isValidEntityName(name string) bool {
	// Reject any control characters (including null bytes and newlines).
	for _, r := range name {
		if r < 0x20 {
			return false
		}
	}
	return validEntityName.MatchString(strings.TrimSpace(name))
}
```

**In `ParseCatalog`, after the `raw.Metadata.Name == ""` check**, add:

```go
	if !isValidEntityName(raw.Metadata.Name) {
		return ParsedCatalog{}, fmt.Errorf("catalog-info.yaml: invalid metadata.name %q (must match [a-zA-Z0-9._-]{1,253} with optional namespace/ prefix)", raw.Metadata.Name)
	}
```

- [ ] **Step 4: Run all parser tests**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go test ./scanner/parser/... -v"
```

Expected: all tests PASS (5 existing + new `TestParseCatalog_InvalidEntityNames`).

- [ ] **Step 5: Commit**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "git add scanner/parser/catalog.go scanner/parser/catalog_test.go; git commit -m 'fix: validate entity names in catalog parser to reject dangerous characters'"
```

---

## Task 2: Search Input Sanitization — Wildcard Escaping + Whitespace (S4 + M3)

**Files:**

- Modify: `memory/content.go`
- Modify: `memory/content_test.go`
- Modify: `tools/search_docs.go`
- Modify: `tools/search_content.go`
- Modify: `tools/tools_test.go`

**Context:** Two related issues. (1) In `content.go`, a query like `50%_off` is passed directly into a SQL LIKE pattern, so `%` becomes a wildcard and `_` matches any single character — the search returns wrong results. Fix: escape before concatenating. (2) In `tools/search_docs.go` and `tools/search_content.go`, the handlers check `args.Query == ""` but a query of `"   "` passes that check and produces a useless (or infinite) LIKE `"%   %"` scan. Fix: trim and re-check.

- [ ] **Step 1: Add failing tests for wildcard escaping**

Add to the bottom of `memory/content_test.go`:

```go
func TestContentCache_Search_WildcardInQuery(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/svc", "README.md", "sha1", "discount: 50% off all items")
	cache.Upsert("org/svc2", "README.md", "sha2", "discount: anything off all items")

	// A literal "50%" should only match files that contain the literal string "50%".
	// Without escaping, "50%" is a valid LIKE pattern and would match any string
	// starting with "50" — which is wrong behaviour.
	matches, err := cache.Search("50%", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("expected exactly 1 match for literal '50%%', got %d", len(matches))
	}
}

func TestContentCache_Search_WhitespaceOnlyQuery(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)
	cache.Upsert("org/svc", "README.md", "sha1", "some content")

	_, err := cache.Search("   ", "")
	if err == nil {
		t.Error("expected error for whitespace-only query")
	}
}
```

- [ ] **Step 2: Run failing tests**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go test ./memory/... -run 'TestContentCache_Search_Wildcard|TestContentCache_Search_Whitespace' -v"
```

Expected: `TestContentCache_Search_WildcardInQuery` — FAIL (returns 2 matches instead of 1). `TestContentCache_Search_WhitespaceOnlyQuery` — FAIL (no error returned).

- [ ] **Step 3: Fix `memory/content.go`**

In the `Search` method, replace:

```go
	if query == "" {
		return nil, fmt.Errorf("query must not be empty")
	}

	var rows []dbDocContent
	q := cc.db.Where("LOWER(content) LIKE LOWER(?)", "%"+query+"%")
```

With:

```go
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query must not be empty or whitespace-only")
	}

	// Escape SQL LIKE special characters so the query is treated as a literal string.
	escaped := strings.ReplaceAll(query, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, "%", `\%`)
	escaped = strings.ReplaceAll(escaped, "_", `\_`)

	var rows []dbDocContent
	q := cc.db.Where("LOWER(content) LIKE LOWER(?) ESCAPE ?", "%"+escaped+"%", `\`)
```

- [ ] **Step 4: Run memory tests**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go test ./memory/... -v"
```

Expected: all tests PASS.

- [ ] **Step 5: Add failing tool handler tests**

Add to the bottom of `tools/tools_test.go`:

```go
func TestSearchDocsHandler_WhitespaceQuery(t *testing.T) {
	sc := &mockScanner{}
	handler := searchDocsHandler(sc)
	req := &mcp.CallToolRequest{}

	_, _, err := handler(context.Background(), req, SearchDocsArgs{Query: "   "})
	if err == nil {
		t.Error("expected error for whitespace-only query in search_docs")
	}
}

func TestSearchContentHandler_WhitespaceQuery(t *testing.T) {
	searcher := &mockContentSearcher{enabled: true}
	handler := searchContentHandler(searcher)
	req := &mcp.CallToolRequest{}

	_, _, err := handler(context.Background(), req, SearchContentArgs{Query: "\t\n"})
	if err == nil {
		t.Error("expected error for whitespace-only query in search_content")
	}
}
```

- [ ] **Step 6: Run failing tool tests**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go test ./tools/... -run 'TestSearchDocs.*Whitespace|TestSearchContent.*Whitespace' -v"
```

Expected: both FAIL — whitespace queries currently pass through.

- [ ] **Step 7: Fix `tools/search_docs.go` and `tools/search_content.go`**

In `tools/search_docs.go` (inside `searchDocsHandler`), replace:

```go
		if args.Query == "" {
			return nil, SearchDocsResult{}, fmt.Errorf("parameter 'query' is required")
		}
```

With:

```go
		if strings.TrimSpace(args.Query) == "" {
			return nil, SearchDocsResult{}, fmt.Errorf("parameter 'query' must not be empty or whitespace-only")
		}
```

In `searchContentHandler`, replace:

```go
		if args.Query == "" {
			return nil, SearchContentResult{}, fmt.Errorf("parameter 'query' is required")
		}
```

With:

```go
		if strings.TrimSpace(args.Query) == "" {
			return nil, SearchContentResult{}, fmt.Errorf("parameter 'query' must not be empty or whitespace-only")
		}
```

Also add `"strings"` to the import blocks if it's not already there.

- [ ] **Step 8: Run all tool tests**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go test ./tools/... -v"
```

Expected: all tests PASS.

- [ ] **Step 9: Commit**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "git add memory/content.go memory/content_test.go tools/search_docs.go tools/search_content.go tools/tools_test.go; git commit -m 'fix: escape LIKE wildcards in content search and reject whitespace-only queries'"
```

---

## Task 3: Per-Repo Scan Timeout (M1)

**Files:**

- Modify: `scanner/scanner.go`
- Modify: `scanner/scanner_test.go`

**Context:** The scanner spawns up to 5 concurrent goroutines to scan repos. Each goroutine calls the GitHub API (potentially multiple times for dirs). A single slow/hanging GitHub response blocks that goroutine indefinitely. Adding a 30-second per-repo context prevents one bad API call from stalling the whole scan cycle.

- [ ] **Step 1: Add a failing test**

Add to `scanner/scanner_test.go`:

```go
func TestScanner_RepoScanRespectsContext(t *testing.T) {
	ts, client := setupMockGitHub()
	defer ts.Close()

	s := New(client, "test-org", 0, []string{"README.md"}, []string{"docs"}, nil, nil, nil)

	// Run with a pre-cancelled context — scanOrg should return without blocking.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	done := make(chan struct{})
	go func() {
		s.scanOrg(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good: completed without blocking
	case <-time.After(3 * time.Second):
		t.Fatal("scanOrg did not complete within 3 seconds with a cancelled context")
	}
}
```

- [ ] **Step 2: Run the failing test**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go test ./scanner/... -run TestScanner_RepoScanRespectsContext -v -timeout 10s"
```

Expected: the test should pass already (GitHub client returns errors on cancelled context), but verify no hang.

- [ ] **Step 3: Add per-repo timeout in `scanner/scanner.go`**

In `scanOrg`, inside the goroutine launched per repo, replace:

```go
		go func(repoName string, repo *github.Repository) {
			defer wg.Done()
			defer func() { <-sem }()

			repoOwner := repo.GetOwner().GetLogin()

			files := s.scanRepo(ctx, repoOwner, repoName)
```

With:

```go
		go func(repoName string, repo *github.Repository) {
			defer wg.Done()
			defer func() { <-sem }()

			repoOwner := repo.GetOwner().GetLogin()

			// Per-repo timeout prevents a single slow GitHub response from stalling the scan.
			repoCtx, repoCancel := context.WithTimeout(ctx, 30*time.Second)
			defer repoCancel()

			files := s.scanRepo(repoCtx, repoOwner, repoName)
```

- [ ] **Step 4: Run all scanner tests**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go test ./scanner/... -v -timeout 30s"
```

Expected: all tests PASS (the mock server responds instantly so the 30s timeout is never hit).

- [ ] **Step 5: Commit**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "git add scanner/scanner.go scanner/scanner_test.go; git commit -m 'fix: add 30s per-repo context timeout to prevent scan goroutine stalls'"
```

---

## Task 4: Indexer Observability — slog Migration + Phase Logging + Failure Summary (M2, M4, M5, M6)

**Files:**

- Modify: `indexer/indexer.go`
- Modify: `indexer/indexer_test.go`

**Context:** `indexer.go` uses the old `log` package while the rest of the app uses `slog`. More importantly, when multiple catalog files fail to parse, each failure is logged individually with no aggregate summary. Operators end up grepping through logs to count failures. This task: (1) migrates to `slog`, (2) adds phase-level `slog.Info` markers, (3) emits a single summary line if any failures occurred, (4) adds `slog.Debug` when a file is skipped due to size in `content.go`.

- [ ] **Step 1: Run existing indexer tests (baseline)**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go test ./indexer/... -v"
```

Expected: all 4 tests PASS. Note the output for comparison.

- [ ] **Step 2: Rewrite `indexer/indexer.go`**

Replace the entire file with:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package indexer

import (
	"context"
	"log/slog"
	"strings"

	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/scanner"
	"github.com/doc-scout/mcp-server/scanner/parser"
)

// FileGetter fetches the raw content of an indexed documentation file.
type FileGetter interface {
	GetFileContent(ctx context.Context, repo, path string) (string, error)
}

// GraphWriter writes entities and relations to the knowledge graph.
type GraphWriter interface {
	CreateEntities([]memory.Entity) ([]memory.Entity, error)
	CreateRelations([]memory.Relation) ([]memory.Relation, error)
	AddObservations([]memory.Observation) ([]memory.Observation, error)
	SearchNodes(query string) (memory.KnowledgeGraph, error)
	EntityCount() (int64, error)
}

// AutoIndexer automatically populates the knowledge graph from catalog-info.yaml files
// and optionally refreshes the content cache after each scan.
type AutoIndexer struct {
	sc    FileGetter
	graph GraphWriter
	cache *memory.ContentCache // nil if content caching disabled
}

// New creates an AutoIndexer. cache may be nil if SCAN_CONTENT is disabled.
func New(sc FileGetter, graph GraphWriter, cache *memory.ContentCache) *AutoIndexer {
	return &AutoIndexer{sc: sc, graph: graph, cache: cache}
}

// Run is the OnScanComplete callback. It:
//  1. Refreshes the content cache for all indexed files (if enabled).
//  2. Parses catalog-info.yaml files and upserts entities/relations in the graph.
//  3. Soft-deletes entities from repos no longer in the scan.
func (ai *AutoIndexer) Run(ctx context.Context, repos []scanner.RepoInfo) {
	activeRepos := make(map[string]bool, len(repos))
	for _, r := range repos {
		activeRepos[r.Name] = true
	}

	// Phase 1: Refresh content cache.
	if ai.cache != nil {
		slog.Info("[indexer] Phase 1: refreshing content cache", "repos", len(repos))
		ai.refreshContent(ctx, repos)
		activeRepoList := make([]string, 0, len(activeRepos))
		for name := range activeRepos {
			activeRepoList = append(activeRepoList, name)
		}
		if err := ai.cache.DeleteOrphanedContent(activeRepoList); err != nil {
			slog.Error("[indexer] Failed to delete orphaned content", "error", err)
		}
	}

	// Phase 2: Auto-graph from catalog-info.yaml.
	slog.Info("[indexer] Phase 2: parsing catalog-info.yaml files", "repos", len(repos))
	var fetchFailures, parseFailures int
	for _, repo := range repos {
		for _, file := range repo.Files {
			if file.Type != "catalog-info" {
				continue
			}
			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)
			if err != nil {
				fetchFailures++
				slog.Warn("[indexer] Failed to fetch catalog file", "repo", repo.Name, "path", file.Path, "error", err)
				continue
			}
			parsed, err := parser.ParseCatalog([]byte(content))
			if err != nil {
				parseFailures++
				slog.Warn("[indexer] Failed to parse catalog", "repo", repo.Name, "error", err)
				continue
			}
			ai.upsertCatalog(ctx, parsed, repo.Name)
		}
	}
	if fetchFailures > 0 || parseFailures > 0 {
		slog.Warn("[indexer] Phase 2 completed with errors", "fetch_failures", fetchFailures, "parse_failures", parseFailures)
	}

	// Phase 3: Soft-delete stale entities.
	slog.Info("[indexer] Phase 3: archiving stale entities")
	ai.archiveStale(ctx, activeRepos)

	slog.Info("[indexer] Indexing complete", "active_repos", len(repos))
}

// refreshContent fetches and caches content for files whose SHA has changed.
func (ai *AutoIndexer) refreshContent(ctx context.Context, repos []scanner.RepoInfo) {
	for _, repo := range repos {
		for _, file := range repo.Files {
			if !ai.cache.NeedsUpdate(repo.Name, file.Path, file.SHA) {
				continue
			}
			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)
			if err != nil {
				slog.Warn("[indexer] Content fetch failed", "repo", repo.Name, "path", file.Path, "error", err)
				continue
			}
			if err := ai.cache.Upsert(repo.Name, file.Path, file.SHA, content); err != nil {
				slog.Warn("[indexer] Content store failed", "repo", repo.Name, "path", file.Path, "error", err)
			}
		}
	}
}

// upsertCatalog writes parsed catalog data to the knowledge graph.
// Upsert rules:
//   - New entity → create with auto observations.
//   - Existing entity → add missing observations only, never overwrite.
//
// Note: between SearchNodes and CreateEntities, a concurrent scan could create the
// same entity. The store's COUNT(*) deduplication handles this gracefully.
func (ai *AutoIndexer) upsertCatalog(ctx context.Context, parsed parser.ParsedCatalog, repoFullName string) {
	autoObs := []string{
		"_source:catalog-info",
		"_scan_repo:" + repoFullName,
	}
	autoObs = append(autoObs, parsed.Observations...)

	graph, err := ai.graph.SearchNodes(parsed.EntityName)
	if err != nil {
		slog.Error("[indexer] SearchNodes failed", "entity", parsed.EntityName, "error", err)
		return
	}

	entityExists := false
	for _, e := range graph.Entities {
		if e.Name == parsed.EntityName {
			entityExists = true
			break
		}
	}

	if !entityExists {
		_, err := ai.graph.CreateEntities([]memory.Entity{
			{
				Name:         parsed.EntityName,
				EntityType:   parsed.EntityType,
				Observations: autoObs,
			},
		})
		if err != nil {
			slog.Error("[indexer] CreateEntities failed", "entity", parsed.EntityName, "error", err)
			return
		}
	} else {
		_, err := ai.graph.AddObservations([]memory.Observation{
			{EntityName: parsed.EntityName, Contents: autoObs},
		})
		if err != nil {
			slog.Error("[indexer] AddObservations failed", "entity", parsed.EntityName, "error", err)
		}
	}

	if len(parsed.Relations) > 0 {
		rels := make([]memory.Relation, 0, len(parsed.Relations))
		for _, r := range parsed.Relations {
			rels = append(rels, memory.Relation{
				From:         r.From,
				To:           r.To,
				RelationType: r.RelationType,
			})
		}
		if _, err := ai.graph.CreateRelations(rels); err != nil {
			slog.Error("[indexer] CreateRelations failed", "entity", parsed.EntityName, "error", err)
		}
	}
}

// archiveStale adds _status:archived to entities whose source repo is no longer active.
func (ai *AutoIndexer) archiveStale(ctx context.Context, activeRepos map[string]bool) {
	graph, err := ai.graph.SearchNodes("_source:catalog-info")
	if err != nil {
		slog.Error("[indexer] SearchNodes for stale check failed", "error", err)
		return
	}

	for _, entity := range graph.Entities {
		repoName := ""
		for _, obs := range entity.Observations {
			if strings.HasPrefix(obs, "_scan_repo:") {
				repoName = strings.TrimPrefix(obs, "_scan_repo:")
				break
			}
		}
		if repoName == "" || activeRepos[repoName] {
			continue
		}
		_, err := ai.graph.AddObservations([]memory.Observation{
			{EntityName: entity.Name, Contents: []string{"_status:archived"}},
		})
		if err != nil {
			slog.Error("[indexer] Failed to archive entity", "entity", entity.Name, "error", err)
		}
	}
}
```

- [ ] **Step 3: Add `slog.Debug` for skipped files in `memory/content.go`**

In the `Upsert` method, replace the silent skip:

```go
	if len(content) > cc.maxSize {
		return nil // skip oversized files silently
	}
```

With:

```go
	if len(content) > cc.maxSize {
		slog.Debug("[content] Skipping oversized file", "repo", repoName, "path", path, "size", len(content), "max", cc.maxSize)
		return nil
	}
```

Also add `"log/slog"` to `memory/content.go`'s import block.

- [ ] **Step 4: Run all indexer and memory tests**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go test ./indexer/... ./memory/... -v"
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "git add indexer/indexer.go memory/content.go; git commit -m 'feat: migrate indexer to slog, add phase logging and failure summary'"
```

---

## Task 5: HTTP Security + Startup Observability (S1, M7, U1, U3)

**Files:**

- Modify: `main.go`

**Context:** Four small improvements to `main.go`:

1. **(S1)** The bearer token comparison `authHeader != "Bearer "+expectedToken` uses `!=` which short-circuits on length mismatch — a timing oracle. Replace with `crypto/subtle.ConstantTimeCompare`.
2. **(M7)** The `OnScanComplete` callback runs indexing synchronously with no timing logged — operators can't tell if indexing is fast or slow.
3. **(U1)** When `REPO_REGEX` or `REPO_TOPICS` are set, repos excluded by the filter will cause their entities to be marked `_status:archived` — this is silent today.
4. **(U3)** The `SCAN_CONTENT=true` + in-memory DB warning uses `slog.Warn` — this is a misconfiguration that silently disables a feature the operator explicitly requested. Bump to `slog.Error` with a clear explanation.

- [ ] **Step 1: Verify current main.go compiles**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go build ./..."
```

Expected: success.

- [ ] **Step 2: Apply all four changes to `main.go`**

**a) Add `"crypto/subtle"` to the import block:**

```go
import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/indexer"
	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/scanner"
	"github.com/doc-scout/mcp-server/tools"
)
```

**b) Replace the SCAN_CONTENT warning** (U3). Change:

```go
	// Disable content caching silently when using in-memory SQLite.
	if scanContent && isInMemoryDB(dbURL) {
		slog.Warn("SCAN_CONTENT=true requires a persistent DATABASE_URL; content caching disabled.")
		scanContent = false
	}
```

With:

```go
	// Disable content caching when using in-memory SQLite — data would be lost on restart.
	if scanContent && isInMemoryDB(dbURL) {
		slog.Error("SCAN_CONTENT=true requires a persistent DATABASE_URL. Content caching has been disabled. " +
			"Set DATABASE_URL to a SQLite file path (e.g. sqlite:///data/docs.db) or a PostgreSQL URL to enable full-text search.")
		scanContent = false
	}
```

**c) Add filter archival warning** (U1). Add after the scanner is created (after `sc := scanner.New(...)`):

```go
	// Warn operators that active repo filters will cause excluded repos' entities to be archived.
	if repoRegex != nil || len(repoTopics) > 0 {
		slog.Warn("Repository filters are active. Entities from repos excluded by these filters will be marked _status:archived on the next scan.",
			"REPO_REGEX", os.Getenv("REPO_REGEX"),
			"REPO_TOPICS", os.Getenv("REPO_TOPICS"))
	}
```

**d) Add timing to the OnScanComplete callback** (M7). Replace:

```go
	sc.SetOnScanComplete(func(repos []scanner.RepoInfo) {
		ai.Run(context.Background(), repos)

		// Re-register tools to implicitly trigger the MCP tools/list_changed notification
		tools.Register(mcpServer, sc, autoWriter, contentCache)
		slog.Info("Triggered tools/list_changed notification")
	})
```

With:

```go
	sc.SetOnScanComplete(func(repos []scanner.RepoInfo) {
		start := time.Now()
		slog.Info("[indexer] Auto-indexing started", "repos", len(repos))
		ai.Run(context.Background(), repos)
		slog.Info("[indexer] Auto-indexing complete", "duration", time.Since(start).String())

		// Re-register tools to implicitly trigger the MCP tools/list_changed notification.
		tools.Register(mcpServer, sc, autoWriter, contentCache)
		slog.Info("Triggered tools/list_changed notification")
	})
```

**e) Replace bearer token comparison with constant-time** (S1). Replace:

```go
		// Basic Bearer Token Auth Middleware
		authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			expectedToken := os.Getenv("MCP_HTTP_BEARER_TOKEN")
			if expectedToken != "" {
				authHeader := r.Header.Get("Authorization")
				if authHeader != "Bearer "+expectedToken {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
			mcpHandler.ServeHTTP(w, r)
		})
```

With:

```go
		// Bearer Token Auth Middleware — uses constant-time comparison to prevent timing attacks.
		authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			expectedToken := os.Getenv("MCP_HTTP_BEARER_TOKEN")
			if expectedToken != "" {
				provided := r.Header.Get("Authorization")
				expected := "Bearer " + expectedToken
				if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
			mcpHandler.ServeHTTP(w, r)
		})
```

- [ ] **Step 3: Build and run full test suite**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "go build ./... && go test ./... -v 2>&1 | Select-String -Pattern 'PASS|FAIL|ok|---'"
```

Expected: all packages PASS, build succeeds.

- [ ] **Step 4: Commit**

```bash
cd e:/DEV/mcpdocs
pwsh -Command "git add main.go; git commit -m 'fix: constant-time bearer token, timing instrumentation, filter archival warning, SCAN_CONTENT error clarity'"
```

---

## Self-Review

| Finding                         | Task   |
| ------------------------------- | ------ |
| S3 — entity name validation     | Task 1 |
| S4 — LIKE wildcard escaping     | Task 2 |
| M3 — whitespace-only queries    | Task 2 |
| M1 — per-repo scan timeout      | Task 3 |
| M2 — indexer failure summary    | Task 4 |
| M4 — slog migration in indexer  | Task 4 |
| M5 — phase logging in indexer   | Task 4 |
| M6 — skipped file debug log     | Task 4 |
| S1 — constant-time bearer token | Task 5 |
| M7 — OnScanComplete timing      | Task 5 |
| U1 — filter archival warning    | Task 5 |
| U3 — SCAN_CONTENT error clarity | Task 5 |

Findings intentionally deferred (out of scope for this plan):

- S2 — rate limiting (requires new dependency, operationally complex)
- S5 — GitHub error token sanitization (speculative, no evidence tokens leak)
- M8 — race condition documentation only (comment, not testable)
- M9 — broader test coverage (separate initiative)
- M10 — API versioning (future consideration)
- U2 — tool description wording (subjective, low impact)
- U4 — tools/list_changed notification log (already implemented in current main.go)
