# Custom Parser Extension (#13) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a `FileParser` interface and `ParserRegistry` so users can plug in custom manifest parsers without touching core code, while replacing the five hardcoded phase loops and five duplicate upsert methods in the indexer with a single generic loop.

**Architecture:** A new `scanner/parser/extension.go` defines the `FileParser` interface and `ParsedFile` struct. A `ParserRegistry` in `scanner/parser/registry.go` holds all registered parsers (built-in and custom). The scanner computes `targetFiles` from the registry; the indexer iterates registered parsers via a single `runParsers()` loop and one `upsertParsedFile()` method.

**Tech Stack:** Go 1.26+, `sync.RWMutex`, existing `scanner`, `indexer`, `memory` packages. No new external dependencies.

**Branch:** `feat/custom-parser-extension` (from `main`)

---

## File Map

| File                              | Action     | Responsibility                                                                                                       |
| --------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------------- |
| `scanner/parser/extension.go`     | **Create** | `FileParser` interface, `ParsedFile`, `ParsedRelation`, `AuxEntity`, `MergeMode`                                     |
| `scanner/parser/registry.go`      | **Create** | `ParserRegistry`, `Default`, `Register()` convenience func                                                           |
| `scanner/parser/registry_test.go` | **Create** | Unit tests for registry                                                                                              |
| `scanner/parser/gomod.go`         | **Modify** | Add `Parser` struct implementing `FileParser`; keep `ParseGoMod()` helper                                            |
| `scanner/parser/packagejson.go`   | **Modify** | Add `Parser` struct; keep `ParsePackageJSON()` helper                                                                |
| `scanner/parser/pom.go`           | **Modify** | Add `Parser` struct; keep `ParsePom()` helper                                                                        |
| `scanner/parser/codeowners.go`    | **Modify** | Add `Parser` struct; keep `ParseCodeowners()` helper                                                                 |
| `scanner/parser/catalog.go`       | **Modify** | Add `Parser` struct; move `ParsedRelation` to `extension.go`                                                         |
| `scanner/scanner.go`              | **Modify** | `New()` accepts `*parser.ParserRegistry`; `DefaultTargetFiles` computed from registry; `classifyFile` takes registry |
| `indexer/indexer.go`              | **Modify** | `New()` accepts `*parser.ParserRegistry`; `runParsers()` + `upsertParsedFile()` replace 5 phases                     |
| `main.go`                         | **Modify** | Register built-in parsers; pass `parser.Default` to scanner + indexer                                                |
| `AGENTS.md`                       | **Modify** | Update §7 with `FileParser` contract and registration guide                                                          |

---

## Task 1: Create branch

**Files:** none

- [ ] **Step 1: Create and checkout branch**

```bash
git checkout -b feat/custom-parser-extension
```

Expected: `Switched to a new branch 'feat/custom-parser-extension'`

---

## Task 2: Create `extension.go` — core types

**Files:**

- Create: `scanner/parser/extension.go`

- [ ] **Step 1: Write the file**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

// ParsedRelation is a directed edge produced by a parser.
// If To is empty (""), the indexer fills it with the derived repo service name —
// used by CodeownersParser where the target service is context-dependent.
type ParsedRelation struct {
	From         string
	To           string
	RelationType string // e.g. "depends_on", "owns", "provides_api"
}

// AuxEntity is an additional graph entity produced alongside the primary entity.
// Used by parsers that create multiple entities (e.g. CodeownersParser creates one
// team/person entity per owner).
type AuxEntity struct {
	Name         string
	EntityType   string
	Observations []string
}

// MergeMode controls how upsertParsedFile() handles existing graph entities.
type MergeMode int

const (
	// MergeModeUpsert (default) — create entity if absent, add observations if present.
	MergeModeUpsert MergeMode = iota
	// MergeModeCatalog — same as MergeModeUpsert in the current implementation;
	// reserved to allow catalog-specific merge semantics in a future iteration.
	MergeModeCatalog
)

// ParsedFile is the normalized, graph-ready output every FileParser must return.
// EntityName must be non-empty unless AuxEntities is non-empty (codeowners pattern).
// EntityType defaults to "service" if blank.
// Observations and Relations may be nil or empty.
// MergeMode defaults to MergeModeUpsert if zero.
type ParsedFile struct {
	EntityName   string
	EntityType   string
	Observations []string
	Relations    []ParsedRelation
	MergeMode    MergeMode
	// AuxEntities are created/updated before Relations. Used when a single file
	// produces multiple graph entities (e.g. CODEOWNERS produces one per owner).
	AuxEntities []AuxEntity
}

// FileParser is the extension point for manifest parsers.
// All methods must be safe for concurrent use (implementations are typically stateless).
type FileParser interface {
	// FileType returns the classifier key used by classifyFile() and the indexer.
	// Must be unique across the registry. Examples: "gomod", "catalog-info".
	FileType() string

	// Filenames returns the exact filenames (or suffix sentinels starting with ".")
	// this parser handles. The scanner looks for these at the repo root.
	// Examples: ["go.mod"], ["catalog-info.yaml"], [".proto"] (suffix match).
	Filenames() []string

	// Parse converts raw file bytes into a normalized graph-ready result.
	Parse(data []byte) (ParsedFile, error)
}
```

- [ ] **Step 2: Build to verify it compiles**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./scanner/parser/...
```

Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add scanner/parser/extension.go
git commit -m "feat(parser): add FileParser interface and ParsedFile types"
```

---

## Task 3: Create `registry.go` + registry tests

**Files:**

- Create: `scanner/parser/registry.go`
- Create: `scanner/parser/registry_test.go`

- [ ] **Step 1: Write `registry_test.go` (failing)**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"sync"
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

// stubParser is a minimal FileParser for testing.
type stubParser struct {
	fileType  string
	filenames []string
}

func (s *stubParser) FileType() string        { return s.fileType }
func (s *stubParser) Filenames() []string     { return s.filenames }
func (s *stubParser) Parse(_ []byte) (parser.ParsedFile, error) {
	return parser.ParsedFile{EntityName: "stub"}, nil
}

func TestRegistry_GetReturnsRegistered(t *testing.T) {
	reg := parser.NewRegistry()
	p := &stubParser{fileType: "mytype", filenames: []string{"myfile"}}
	reg.Register(p)

	got, ok := reg.Get("mytype")
	if !ok {
		t.Fatal("expected Get to return true")
	}
	if got.FileType() != "mytype" {
		t.Errorf("got FileType %q, want %q", got.FileType(), "mytype")
	}
}

func TestRegistry_GetUnknownReturnsFalse(t *testing.T) {
	reg := parser.NewRegistry()
	_, ok := reg.Get("nope")
	if ok {
		t.Fatal("expected Get to return false for unknown type")
	}
}

func TestRegistry_TargetFilenames(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubParser{fileType: "a", filenames: []string{"file-a", "alt-a"}})
	reg.Register(&stubParser{fileType: "b", filenames: []string{"file-b"}})

	names := reg.TargetFilenames()
	want := map[string]bool{"file-a": true, "alt-a": true, "file-b": true}
	if len(names) != len(want) {
		t.Fatalf("TargetFilenames len=%d, want %d: %v", len(names), len(want), names)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected filename %q", n)
		}
	}
}

func TestRegistry_DuplicateFileTypePanics(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubParser{fileType: "dup", filenames: []string{"f1"}})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate FileType")
		}
	}()
	reg.Register(&stubParser{fileType: "dup", filenames: []string{"f2"}})
}

func TestRegistry_DuplicateFilenamePanics(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubParser{fileType: "a", filenames: []string{"shared"}})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate filename")
		}
	}()
	reg.Register(&stubParser{fileType: "b", filenames: []string{"shared"}})
}

func TestRegistry_AllReturnsAll(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubParser{fileType: "x", filenames: []string{"fx"}})
	reg.Register(&stubParser{fileType: "y", filenames: []string{"fy"}})

	all := reg.All()
	if len(all) != 2 {
		t.Errorf("All() len=%d, want 2", len(all))
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := parser.NewRegistry()
	var wg sync.WaitGroup
	// Pre-register a few parsers so Get has something to find.
	for i := range 5 {
		ft := "type" + string(rune('a'+i))
		fn := "file" + string(rune('a'+i))
		reg.Register(&stubParser{fileType: ft, filenames: []string{fn}})
	}
	// Concurrent reads.
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.Get("typea")
			reg.All()
			reg.TargetFilenames()
		}()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -run TestRegistry -v 2>&1 | tail -5
```

Expected: `FAIL` (registry not yet implemented)

- [ ] **Step 3: Write `registry.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"fmt"
	"sync"
)

// Default is the global registry. Built-in parsers register here from main.go.
// Custom parsers may register here from their own init() functions.
var Default = NewRegistry()

// Register adds p to the global Default registry.
// Panics on duplicate FileType() or Filenames() overlap — fail-fast at startup.
func Register(p FileParser) { Default.Register(p) }

// ParserRegistry is a thread-safe map of FileParser implementations keyed by FileType().
type ParserRegistry struct {
	mu        sync.RWMutex
	parsers   map[string]FileParser // keyed by FileType()
	filenames map[string]string     // filename → FileType(), for duplicate detection
}

// NewRegistry returns an empty ParserRegistry.
func NewRegistry() *ParserRegistry {
	return &ParserRegistry{
		parsers:   make(map[string]FileParser),
		filenames: make(map[string]string),
	}
}

// Register adds p to the registry.
// Panics if p.FileType() is already registered.
// Panics if any entry in p.Filenames() is already claimed by another parser.
func (r *ParserRegistry) Register(p FileParser) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ft := p.FileType()
	if _, exists := r.parsers[ft]; exists {
		panic(fmt.Sprintf("parser registry: duplicate FileType %q", ft))
	}
	for _, fn := range p.Filenames() {
		if existing, clash := r.filenames[fn]; clash {
			panic(fmt.Sprintf("parser registry: filename %q already claimed by %q", fn, existing))
		}
	}
	r.parsers[ft] = p
	for _, fn := range p.Filenames() {
		r.filenames[fn] = ft
	}
}

// Get returns the FileParser for the given fileType, or (nil, false) if not registered.
func (r *ParserRegistry) Get(fileType string) (FileParser, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.parsers[fileType]
	return p, ok
}

// All returns a snapshot of all registered parsers in an unspecified order.
func (r *ParserRegistry) All() []FileParser {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]FileParser, 0, len(r.parsers))
	for _, p := range r.parsers {
		result = append(result, p)
	}
	return result
}

// TargetFilenames returns the union of all Filenames() across all registered parsers.
func (r *ParserRegistry) TargetFilenames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, 0, len(r.filenames))
	for fn := range r.filenames {
		result = append(result, fn)
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -run TestRegistry -v -race 2>&1 | tail -20
```

Expected: all `TestRegistry_*` tests PASS, `-race` reports no races

- [ ] **Step 5: Commit**

```bash
git add scanner/parser/registry.go scanner/parser/registry_test.go
git commit -m "feat(parser): add ParserRegistry with duplicate-detection and thread-safety"
```

---

## Task 4: Migrate `gomod.go` to implement `FileParser`

**Files:**

- Modify: `scanner/parser/gomod.go`

- [ ] **Step 1: Add `Parser` struct and methods at the bottom of `gomod.go`**

Append to `/mnt/e/DEV/mcpdocs/scanner/parser/gomod.go` after the existing `moduleEntityName` function:

```go
// Parser implements FileParser for go.mod files.
type goModParser struct{}

func (*goModParser) FileType() string        { return "gomod" }
func (*goModParser) Filenames() []string     { return []string{"go.mod"} }
func (p *goModParser) Parse(data []byte) (ParsedFile, error) {
	parsed, err := ParseGoMod(data)
	if err != nil {
		return ParsedFile{}, err
	}

	obs := []string{
		"go_module:" + parsed.ModulePath,
	}
	if parsed.GoVersion != "" {
		obs = append(obs, "go_version:"+parsed.GoVersion)
	}

	rels := make([]ParsedRelation, 0, len(parsed.DirectDeps))
	for _, dep := range parsed.DirectDeps {
		rels = append(rels, ParsedRelation{
			From:         parsed.EntityName,
			To:           moduleEntityName(dep),
			RelationType: "depends_on",
		})
	}

	return ParsedFile{
		EntityName:   parsed.EntityName,
		EntityType:   "service",
		Observations: obs,
		Relations:    rels,
	}, nil
}

// GoModParser returns the FileParser for go.mod files.
func GoModParser() FileParser { return &goModParser{} }
```

- [ ] **Step 2: Add `FileType` + `Filenames` tests to `gomod_test.go`**

Append to `/mnt/e/DEV/mcpdocs/scanner/parser/gomod_test.go`:

```go
func TestGoModParser_FileTypeAndFilenames(t *testing.T) {
	p := parser.GoModParser()
	if p.FileType() != "gomod" {
		t.Errorf("FileType = %q, want %q", p.FileType(), "gomod")
	}
	if len(p.Filenames()) != 1 || p.Filenames()[0] != "go.mod" {
		t.Errorf("Filenames = %v, want [go.mod]", p.Filenames())
	}
}

func TestGoModParser_Parse(t *testing.T) {
	input := []byte(`module github.com/myorg/my-service

go 1.22

require (
	github.com/foo/bar v1.2.3
	github.com/baz/qux v0.1.0 // indirect
)
`)
	p := parser.GoModParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.EntityName != "my-service" {
		t.Errorf("EntityName = %q, want %q", got.EntityName, "my-service")
	}
	if got.EntityType != "service" {
		t.Errorf("EntityType = %q, want %q", got.EntityType, "service")
	}
	// Observations should include go_module and go_version.
	obsMap := make(map[string]bool)
	for _, o := range got.Observations {
		obsMap[o] = true
	}
	if !obsMap["go_module:github.com/myorg/my-service"] {
		t.Error("missing go_module observation")
	}
	if !obsMap["go_version:1.22"] {
		t.Error("missing go_version observation")
	}
	// Direct dep bar should produce a depends_on relation.
	var foundRel bool
	for _, r := range got.Relations {
		if r.From == "my-service" && r.To == "bar" && r.RelationType == "depends_on" {
			foundRel = true
		}
	}
	if !foundRel {
		t.Errorf("expected depends_on relation to 'bar', got %v", got.Relations)
	}
	// Indirect dep qux must NOT appear.
	for _, r := range got.Relations {
		if r.To == "qux" {
			t.Error("indirect dep qux should not produce a relation")
		}
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -run TestGoMod -v 2>&1 | tail -15
```

Expected: all `TestGoMod*` tests PASS

- [ ] **Step 4: Commit**

```bash
git add scanner/parser/gomod.go scanner/parser/gomod_test.go
git commit -m "feat(parser): gomod implements FileParser interface"
```

---

## Task 5: Migrate `packagejson.go`, `pom.go`, `codeowners.go`, `catalog.go`

**Files:**

- Modify: `scanner/parser/packagejson.go`
- Modify: `scanner/parser/pom.go`
- Modify: `scanner/parser/codeowners.go`
- Modify: `scanner/parser/catalog.go`
- Modify: `scanner/parser/extension.go` (move `ParsedRelation` here — already done in Task 2)

Note: `ParsedRelation` is already defined in `extension.go` (Task 2). The existing definition in `catalog.go` must be **removed** to avoid a duplicate symbol error.

- [ ] **Step 1: Remove `ParsedRelation` from `catalog.go`**

In `/mnt/e/DEV/mcpdocs/scanner/parser/catalog.go`, delete the `ParsedRelation` struct definition (lines 22-26 in the current file):

```go
// ParsedRelation is a directed edge extracted from catalog-info.yaml.
type ParsedRelation struct {
	From         string
	To           string
	RelationType string
}
```

The `ParsedCatalog` struct at line 15-18 already references `[]ParsedRelation` — this will now resolve to the definition in `extension.go` (same package).

- [ ] **Step 2: Add `CatalogParser` to `catalog.go`**

Append to `/mnt/e/DEV/mcpdocs/scanner/parser/catalog.go` after the final closing brace of `ParseCatalog`:

```go
// catalogParser implements FileParser for catalog-info.yaml files.
type catalogParser struct{}

func (*catalogParser) FileType() string        { return "catalog-info" }
func (*catalogParser) Filenames() []string     { return []string{"catalog-info.yaml"} }
func (p *catalogParser) Parse(data []byte) (ParsedFile, error) {
	parsed, err := ParseCatalog(data)
	if err != nil {
		return ParsedFile{}, err
	}
	return ParsedFile{
		EntityName:   parsed.EntityName,
		EntityType:   parsed.EntityType,
		Observations: parsed.Observations,
		Relations:    parsed.Relations,
		MergeMode:    MergeModeCatalog,
	}, nil
}

// CatalogParser returns the FileParser for catalog-info.yaml files.
func CatalogParser() FileParser { return &catalogParser{} }
```

- [ ] **Step 3: Build to confirm no duplicate ParsedRelation**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./scanner/parser/...
```

Expected: no output (success)

- [ ] **Step 4: Add `PackageJSONParser` to `packagejson.go`**

Append to `/mnt/e/DEV/mcpdocs/scanner/parser/packagejson.go` after the final closing brace of `PackageEntityName`:

```go
// packageJSONParser implements FileParser for package.json files.
type packageJSONParser struct{}

func (*packageJSONParser) FileType() string        { return "packagejson" }
func (*packageJSONParser) Filenames() []string     { return []string{"package.json"} }
func (p *packageJSONParser) Parse(data []byte) (ParsedFile, error) {
	parsed, err := ParsePackageJSON(data)
	if err != nil {
		return ParsedFile{}, err
	}

	obs := []string{"npm_package:" + parsed.Name}
	if parsed.Version != "" {
		obs = append(obs, "version:"+parsed.Version)
	}

	rels := make([]ParsedRelation, 0, len(parsed.DirectDeps))
	for _, dep := range parsed.DirectDeps {
		rels = append(rels, ParsedRelation{
			From:         parsed.EntityName,
			To:           PackageEntityName(dep),
			RelationType: "depends_on",
		})
	}

	return ParsedFile{
		EntityName:   parsed.EntityName,
		EntityType:   "service",
		Observations: obs,
		Relations:    rels,
	}, nil
}

// PackageJSONParser returns the FileParser for package.json files.
func PackageJSONParser() FileParser { return &packageJSONParser{} }
```

- [ ] **Step 5: Add `PomParser` to `pom.go`**

Append to `/mnt/e/DEV/mcpdocs/scanner/parser/pom.go` after the final closing brace of `ParsePom`:

```go
// pomParser implements FileParser for pom.xml files.
type pomParser struct{}

func (*pomParser) FileType() string        { return "pomxml" }
func (*pomParser) Filenames() []string     { return []string{"pom.xml"} }
func (p *pomParser) Parse(data []byte) (ParsedFile, error) {
	parsed, err := ParsePom(data)
	if err != nil {
		return ParsedFile{}, err
	}

	obs := []string{
		"maven_artifact:" + parsed.GroupID + ":" + parsed.ArtifactID,
	}
	if parsed.GroupID != "" {
		obs = append(obs, "java_group:"+parsed.GroupID)
	}
	if parsed.Version != "" {
		obs = append(obs, "version:"+parsed.Version)
	}

	rels := make([]ParsedRelation, 0, len(parsed.DirectDeps))
	for _, dep := range parsed.DirectDeps {
		rels = append(rels, ParsedRelation{
			From:         parsed.EntityName,
			To:           dep,
			RelationType: "depends_on",
		})
	}

	return ParsedFile{
		EntityName:   parsed.EntityName,
		EntityType:   "service",
		Observations: obs,
		Relations:    rels,
	}, nil
}

// PomParser returns the FileParser for pom.xml files.
func PomParser() FileParser { return &pomParser{} }
```

- [ ] **Step 6: Add `CodeownersParser` to `codeowners.go`**

The codeowners parser creates multiple entities (one per owner). It returns an empty `EntityName` with owner entities in `AuxEntities` and `owns` relations with `To = ""` (the indexer fills in the repo service name).

Append to `/mnt/e/DEV/mcpdocs/scanner/parser/codeowners.go` after `classifyOwner`:

```go
// codeownersParser implements FileParser for CODEOWNERS files.
type codeownersParser struct{}

func (*codeownersParser) FileType() string { return "codeowners" }
func (*codeownersParser) Filenames() []string {
	return []string{"CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"}
}
func (p *codeownersParser) Parse(data []byte) (ParsedFile, error) {
	parsed := ParseCodeowners(data)
	if len(parsed.UniqueOwners) == 0 {
		return ParsedFile{}, nil
	}

	aux := make([]AuxEntity, 0, len(parsed.UniqueOwners))
	rels := make([]ParsedRelation, 0, len(parsed.UniqueOwners))

	for _, owner := range parsed.UniqueOwners {
		aux = append(aux, AuxEntity{
			Name:       owner.EntityName,
			EntityType: owner.EntityType,
			Observations: []string{
				"github_handle:" + owner.Raw,
			},
		})
		// To is intentionally empty: the indexer replaces "" with the derived
		// repo service name (last segment of repo.FullName).
		rels = append(rels, ParsedRelation{
			From:         owner.EntityName,
			To:           "",
			RelationType: "owns",
		})
	}

	return ParsedFile{
		AuxEntities: aux,
		Relations:   rels,
	}, nil
}

// CodeownersParser returns the FileParser for CODEOWNERS files.
func CodeownersParser() FileParser { return &codeownersParser{} }
```

- [ ] **Step 7: Build**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./scanner/parser/...
```

Expected: no output

- [ ] **Step 8: Run all parser tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -v 2>&1 | grep -E "^(=== RUN|--- PASS|--- FAIL|FAIL|ok)"
```

Expected: all tests PASS

- [ ] **Step 9: Commit**

```bash
git add scanner/parser/catalog.go scanner/parser/packagejson.go scanner/parser/pom.go scanner/parser/codeowners.go
git commit -m "feat(parser): all 5 built-in parsers implement FileParser interface"
```

---

## Task 6: Refactor `indexer/indexer.go`

Replace the 5 hardcoded phase loops and 5 upsert methods with `runParsers()` + `upsertParsedFile()`.

**Files:**

- Modify: `indexer/indexer.go`

The full replacement for `indexer.go` is shown below. Key changes:

- `AutoIndexer` gains `registry *parser.ParserRegistry` field
- `New()` gains `registry *parser.ParserRegistry` parameter
- `Run()` calls `runParsers()` instead of phases 2a-2e
- `upsertParsedFile()` handles all parsers generically
- `upsertGoMod`, `upsertPackageJSON`, `upsertPom`, `upsertCodeowners`, `upsertCatalog` are deleted
- `refreshContent`, `archiveStale`, `moduleEntityName` are unchanged

- [ ] **Step 1: Write the new `indexer.go`**

Replace the entire file `/mnt/e/DEV/mcpdocs/indexer/indexer.go` with:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
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

// AutoIndexer automatically populates the knowledge graph from manifest files
// and optionally refreshes the content cache after each scan.
type AutoIndexer struct {
	sc       FileGetter
	graph    GraphWriter
	cache    *memory.ContentCache // nil if content caching disabled
	registry *parser.ParserRegistry
}

// New creates an AutoIndexer. cache may be nil if SCAN_CONTENT is disabled.
func New(sc FileGetter, graph GraphWriter, cache *memory.ContentCache, registry *parser.ParserRegistry) *AutoIndexer {
	return &AutoIndexer{sc: sc, graph: graph, cache: cache, registry: registry}
}

// Run is the OnScanComplete callback. It:
//  1. Refreshes the content cache for all indexed files (if enabled).
//  2. Runs all registered parsers and upserts entities/relations into the graph.
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

	// Phase 2: Run all registered parsers.
	slog.Info("[indexer] Phase 2: running parsers", "repos", len(repos), "parsers", len(ai.registry.All()))
	if err := ai.runParsers(ctx, repos); err != nil {
		slog.Error("[indexer] Phase 2 failed", "error", err)
	}

	// Phase 3: Soft-delete stale entities.
	slog.Info("[indexer] Phase 3: archiving stale entities")
	ai.archiveStale(ctx, activeRepos)

	slog.Info("[indexer] Indexing complete", "active_repos", len(repos))
}

// runParsers iterates all repos and files, routes each file to its registered parser,
// and upserts the resulting graph entities and relations.
func (ai *AutoIndexer) runParsers(ctx context.Context, repos []scanner.RepoInfo) error {
	// Track which CODEOWNERS file we've processed per repo (first one wins).
	processedCodeowners := make(map[string]bool)

	for _, repo := range repos {
		for _, file := range repo.Files {
			p, ok := ai.registry.Get(file.Type)
			if !ok {
				continue // infra/docs file — no graph entity
			}

			// Only process the first CODEOWNERS per repo to avoid duplicate owns relations.
			if file.Type == "codeowners" {
				if processedCodeowners[repo.Name] {
					continue
				}
				processedCodeowners[repo.Name] = true
			}

			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)
			if err != nil {
				slog.Warn("[indexer] Failed to fetch file", "repo", repo.Name, "path", file.Path, "error", err)
				continue
			}

			parsed, err := p.Parse([]byte(content))
			if err != nil {
				slog.Warn("[indexer] Parser failed", "type", file.Type, "repo", repo.Name, "path", file.Path, "error", err)
				continue
			}

			if err := ai.upsertParsedFile(ctx, repo, p.FileType(), parsed); err != nil {
				slog.Error("[indexer] upsertParsedFile failed", "type", file.Type, "repo", repo.Name, "error", err)
			}
		}
	}
	return nil
}

// repoServiceName returns the short service entity name derived from a repo FullName.
// "myorg/my-service" → "my-service". Falls back to repo.Name if FullName is not set.
func repoServiceName(repo scanner.RepoInfo) string {
	if _, name, ok := strings.Cut(repo.FullName, "/"); ok {
		return name
	}
	return repo.Name
}

// upsertParsedFile writes a ParsedFile's entities, observations, and relations to the graph.
// Auto-observations _source:<fileType> and _scan_repo:<repo.FullName> are always added.
// If parsed.EntityName is empty but AuxEntities are present (codeowners pattern), only
// aux entities and relations are created.
// Relations with To == "" have their To field filled with the derived repo service name.
func (ai *AutoIndexer) upsertParsedFile(ctx context.Context, repo scanner.RepoInfo, fileType string, parsed parser.ParsedFile) error {
	autoObs := []string{
		"_source:" + fileType,
		"_scan_repo:" + repo.FullName,
	}

	// Create / update primary entity (if named).
	if parsed.EntityName != "" {
		entityType := parsed.EntityType
		if entityType == "" {
			entityType = "service"
		}

		obs := append(autoObs, parsed.Observations...)

		graph, err := ai.graph.SearchNodes(parsed.EntityName)
		if err != nil {
			return fmt.Errorf("SearchNodes %q: %w", parsed.EntityName, err)
		}

		entityExists := slices.ContainsFunc(graph.Entities, func(e memory.Entity) bool {
			return e.Name == parsed.EntityName
		})

		if !entityExists {
			if _, err := ai.graph.CreateEntities([]memory.Entity{
				{Name: parsed.EntityName, EntityType: entityType, Observations: obs},
			}); err != nil {
				return fmt.Errorf("CreateEntities %q: %w", parsed.EntityName, err)
			}
		} else {
			if _, err := ai.graph.AddObservations([]memory.Observation{
				{EntityName: parsed.EntityName, Contents: obs},
			}); err != nil {
				return fmt.Errorf("AddObservations %q: %w", parsed.EntityName, err)
			}
		}
	}

	// Create / update auxiliary entities (e.g. owners from CODEOWNERS).
	for _, aux := range parsed.AuxEntities {
		auxObs := append(autoObs, aux.Observations...)

		graph, err := ai.graph.SearchNodes(aux.Name)
		if err != nil {
			slog.Error("[indexer] SearchNodes failed for aux entity", "entity", aux.Name, "error", err)
			continue
		}

		auxExists := slices.ContainsFunc(graph.Entities, func(e memory.Entity) bool {
			return e.Name == aux.Name
		})

		if !auxExists {
			if _, err := ai.graph.CreateEntities([]memory.Entity{
				{Name: aux.Name, EntityType: aux.EntityType, Observations: auxObs},
			}); err != nil {
				slog.Error("[indexer] CreateEntities failed for aux entity", "entity", aux.Name, "error", err)
				continue
			}
		} else {
			if _, err := ai.graph.AddObservations([]memory.Observation{
				{EntityName: aux.Name, Contents: auxObs},
			}); err != nil {
				slog.Error("[indexer] AddObservations failed for aux entity", "entity", aux.Name, "error", err)
			}
		}
	}

	// Create relations. Empty To is filled with the repo service name.
	if len(parsed.Relations) == 0 {
		return nil
	}

	svcName := repoServiceName(repo)
	rels := make([]memory.Relation, 0, len(parsed.Relations))
	for _, r := range parsed.Relations {
		to := r.To
		if to == "" {
			to = svcName
		}
		rels = append(rels, memory.Relation{
			From:         r.From,
			To:           to,
			RelationType: r.RelationType,
		})
	}
	if _, err := ai.graph.CreateRelations(rels); err != nil {
		return fmt.Errorf("CreateRelations: %w", err)
	}
	return nil
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

// archiveStale adds _status:archived to entities whose source repo is no longer active.
func (ai *AutoIndexer) archiveStale(ctx context.Context, activeRepos map[string]bool) {
	graph, err := ai.graph.SearchNodes("_scan_repo:")
	if err != nil {
		slog.Error("[indexer] SearchNodes for stale check failed", "error", err)
		return
	}

	for _, entity := range graph.Entities {
		repoName := ""
		for _, obs := range entity.Observations {
			if name, ok := strings.CutPrefix(obs, "_scan_repo:"); ok {
				repoName = name
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

- [ ] **Step 2: Build**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./indexer/...
```

Expected: compilation error in `main.go` because `indexer.New()` signature changed — fix in Task 7.

- [ ] **Step 3: Commit**

```bash
git add indexer/indexer.go
git commit -m "refactor(indexer): replace 5 phase loops with generic runParsers + upsertParsedFile"
```

---

## Task 7: Refactor `scanner/scanner.go` and update `main.go`

**Files:**

- Modify: `scanner/scanner.go`
- Modify: `main.go`

- [ ] **Step 1: Update `scanner.go` — add registry to `New()` and update `classifyFile()`**

The scanner needs two changes:

1. `New()` accepts a `*parser.ParserRegistry` parameter and computes `targetFiles` from it
2. `classifyFile()` checks the registry before its hardcoded switch

In `/mnt/e/DEV/mcpdocs/scanner/scanner.go`:

**a)** Add import for the parser package. Locate the import block and add:

```go
"github.com/doc-scout/mcp-server/scanner/parser"
```

**b)** Replace the `Scanner` struct definition to add a `registry` field:

```go
// Scanner manages GitHub org scanning and caching.
type Scanner struct {
	client       *github.Client
	org          string
	scanInterval time.Duration
	targetFiles  []string       // files to look for at repo root
	scanDirs     []string       // directories to scan recursively for .md files
	infraDirs    []string       // directories to scan recursively for infra files (.yaml, .tf, .hcl, .toml)
	extraRepos   []string       // extra explicit repos formatted as "owner/repo"
	repoTopics   []string       // filter org repos by topics
	repoRegex    *regexp.Regexp // filter org repos by name using regex
	registry     *parser.ParserRegistry

	mu    sync.RWMutex
	repos map[string]*RepoInfo // keyed by repo name

	scanning   bool
	lastScanAt time.Time

	onScanComplete func([]RepoInfo) // called after each full scan completes
}
```

**c)** Replace the `New()` function:

```go
// staticTargetFiles are files indexed as documentation or infra assets that have no
// registered FileParser (they don't produce graph entities).
var staticTargetFiles = []string{
	"README.md",
	"mkdocs.yml",
	"openapi.yaml",
	"swagger.json",
	"SKILLS.md",
	"AGENTS.md",
	// Infrastructure / tooling files at repo root.
	"Dockerfile",
	"docker-compose.yml",
	"docker-compose.yaml",
	"Makefile",
	".mise.toml",
	"mise.toml",
}

// New creates a new Scanner instance. registry must be non-nil.
// targetFiles are computed from the registry (parser filenames) plus staticTargetFiles.
// Pass non-nil targetFiles override to add extra root-level files beyond the defaults.
func New(client *github.Client, org string, scanInterval time.Duration, targetFilesOverride, scanDirs, infraDirs, extraRepos, repoTopics []string, repoRegex *regexp.Regexp, registry *parser.ParserRegistry) *Scanner {
	// Compute default target files from registry + static set.
	seen := make(map[string]bool)
	var targetFiles []string
	for _, fn := range registry.TargetFilenames() {
		if !seen[fn] {
			seen[fn] = true
			targetFiles = append(targetFiles, fn)
		}
	}
	for _, fn := range staticTargetFiles {
		if !seen[fn] {
			seen[fn] = true
			targetFiles = append(targetFiles, fn)
		}
	}
	// Apply user override if supplied.
	if len(targetFilesOverride) > 0 {
		for _, fn := range targetFilesOverride {
			if !seen[fn] {
				seen[fn] = true
				targetFiles = append(targetFiles, fn)
			}
		}
	}

	if len(scanDirs) == 0 {
		scanDirs = DefaultScanDirs
	}
	if len(infraDirs) == 0 {
		infraDirs = DefaultInfraDirs
	}
	return &Scanner{
		client:       client,
		org:          org,
		scanInterval: scanInterval,
		targetFiles:  targetFiles,
		scanDirs:     scanDirs,
		infraDirs:    infraDirs,
		extraRepos:   extraRepos,
		repoTopics:   repoTopics,
		repoRegex:    repoRegex,
		registry:     registry,
		repos:        make(map[string]*RepoInfo),
	}
}
```

**d)** Remove (or keep as deprecated alias) `DefaultTargetFiles` var, replacing with a comment pointing to the registry. Since external callers may reference it, keep it but empty:

Replace the old `DefaultTargetFiles` var with:

```go
// DefaultTargetFiles is kept for backward compatibility but is no longer used by New().
// Target files are now computed from the ParserRegistry at construction time.
// Deprecated: pass a ParserRegistry to New() instead.
var DefaultTargetFiles = []string{}
```

**e)** Replace `classifyFile(name string) string` with a version that checks the registry first:

```go
// classifyFile returns a type label for a given file path.
// It checks the scanner's parser registry first, then falls back to hardcoded rules
// for infra/docs types that have no associated FileParser.
func (s *Scanner) classifyFile(name string) string {
	base := filepath.Base(strings.ToLower(name))

	// Registry-based classification (exact filename match).
	for _, p := range s.registry.All() {
		for _, fn := range p.Filenames() {
			if strings.HasPrefix(fn, ".") && strings.HasSuffix(base, fn) {
				// Suffix match for extension-based parsers (e.g. ".proto").
				return p.FileType()
			}
			if base == strings.ToLower(fn) {
				return p.FileType()
			}
		}
	}

	lowerPath := strings.ToLower(name)
	switch base {
	// Documentation / catalog types not backed by a FileParser.
	case "mkdocs.yml":
		return "mkdocs"
	case "openapi.yaml":
		return "openapi"
	case "swagger.json":
		return "swagger"
	case "readme.md":
		return "readme"
	case "skills.md":
		return "skills"
	case "agents.md":
		return "agents"
	// Infrastructure / tooling
	case "dockerfile":
		return "dockerfile"
	case "makefile":
		return "makefile"
	case "docker-compose.yml", "docker-compose.yaml":
		return "compose"
	case ".mise.toml", "mise.toml":
		return "mise"
	// Helm
	case "chart.yaml":
		return "helm"
	case "values.yaml":
		if strings.Contains(lowerPath, "/helm/") {
			return "helm"
		}
		return "infra"
	}

	ext := filepath.Ext(base)
	switch ext {
	case ".tf", ".hcl":
		return "terraform"
	case ".yaml", ".yml":
		if strings.Contains(lowerPath, "/helm/") {
			return "helm"
		}
		if strings.Contains(lowerPath, "/k8s/") || strings.Contains(lowerPath, "/kubernetes/") {
			return "k8s"
		}
		if strings.Contains(lowerPath, "/workflows/") {
			return "workflow"
		}
		return "infra"
	case ".toml":
		return "toml"
	}

	return "docs"
}
```

**f)** Update every call site of `classifyFile(...)` to `s.classifyFile(...)`.

Search for `classifyFile(` in scanner.go and replace each occurrence:

```bash
grep -n "classifyFile(" /mnt/e/DEV/mcpdocs/scanner/scanner.go
```

Each call like `classifyFile(target)` → `s.classifyFile(target)` and `classifyFile(itemPath)` → `s.classifyFile(itemPath)`.

- [ ] **Step 2: Update `main.go`**

In `/mnt/e/DEV/mcpdocs/main.go`:

**a)** Add import for parser package:

```go
"github.com/doc-scout/mcp-server/scanner/parser"
```

**b)** Before the `// --- Scanner ---` section, add parser registration:

```go
// --- Parser Registry ---
parser.Register(parser.GoModParser())
parser.Register(parser.PackageJSONParser())
parser.Register(parser.PomParser())
parser.Register(parser.CodeownersParser())
parser.Register(parser.CatalogParser())
```

**c)** Update the scanner constructor call (add `parser.Default` as last arg):

```go
sc := scanner.New(ghClient, org, scanInterval, targetFiles, scanDirs, infraDirs, extraRepos, repoTopics, repoRegex, parser.Default)
```

**d)** Update the indexer constructor call (add `parser.Default` as last arg):

```go
ai := indexer.New(sc, auditedGraph, contentCache, parser.Default)
```

- [ ] **Step 3: Build the entire project**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./...
```

Expected: no output (success)

- [ ] **Step 4: Run all tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./... 2>&1 | tail -30
```

Expected: all packages pass. If any integration tests reference the old `indexer.New(sc, graph, cache)` signature, update them to `indexer.New(sc, graph, cache, parser.Default)` (or a `parser.NewRegistry()` for isolation).

- [ ] **Step 5: Fix any test compilation errors**

Check for tests that call `indexer.New` or `scanner.New` with the old signature:

```bash
grep -rn "indexer\.New\|scanner\.New" /mnt/e/DEV/mcpdocs/tests/
```

Update any found call sites to pass a registry. For tests, use a test-isolated registry:

```go
reg := parser.NewRegistry()
reg.Register(parser.GoModParser())
// ... etc
ai := indexer.New(mockFetcher, mockGraph, nil, reg)
```

- [ ] **Step 6: Run tests again**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./... -race 2>&1 | tail -20
```

Expected: all PASS, no races

- [ ] **Step 7: Commit**

```bash
git add scanner/scanner.go main.go
git commit -m "refactor(scanner+main): wire ParserRegistry into scanner and indexer constructors"
```

---

## Task 8: Update `AGENTS.md` §7

**Files:**

- Modify: `AGENTS.md`

- [ ] **Step 1: Replace §7 with updated content**

Find the `# 7. Scanner Extension Points` section in `/mnt/e/DEV/mcpdocs/AGENTS.md` and replace it with:

````markdown
# 7. Scanner Extension Points

New manifest parsers implement the `FileParser` interface in `scanner/parser/extension.go` and register with the global `parser.Default` registry via `parser.Register()` in `main.go`.

## FileParser Interface

```go
// FileParser is the extension point for manifest parsers.
type FileParser interface {
    FileType() string        // unique classifier key (e.g. "pipfile")
    Filenames() []string     // root-level filenames (e.g. ["Pipfile"]) or suffix sentinels (e.g. [".proto"])
    Parse(data []byte) (ParsedFile, error)
}
```
````

## ParsedFile

```go
type ParsedFile struct {
    EntityName   string           // primary entity name; empty if only AuxEntities
    EntityType   string           // defaults to "service" if blank
    Observations []string         // parser-specific observations (e.g. "version:1.2.3")
    Relations    []ParsedRelation // directed edges; To="" → filled with repo service name
    MergeMode    MergeMode        // MergeModeUpsert (default) or MergeModeCatalog
    AuxEntities  []AuxEntity      // additional entities (used by codeowners-style parsers)
}
```

## Registration Pattern

```go
// In main.go — register at startup before scanner/indexer construction:
parser.Register(parser.GoModParser())
parser.Register(myorg.NewPipfileParser())

// Custom parser in mypkg/pipfile/parser.go:
type Parser struct{}
func (p *Parser) FileType()  string   { return "pipfile" }
func (p *Parser) Filenames() []string { return []string{"Pipfile"} }
func (p *Parser) Parse(data []byte) (parser.ParsedFile, error) { ... }
```

## Conventions

- `Register()` panics on duplicate `FileType()` or filename — caught at startup, not runtime.
- `Parse()` returning an error causes the file to be **skipped** with a warning log (scan continues).
- Auto-observations `_source:<FileType>` and `_scan_repo:<repo.FullName>` are added by the indexer; parsers must not duplicate them.
- For suffix-based discovery (e.g. `*.proto`), include a sentinel like `".proto"` in `Filenames()` — the scanner matches files whose name ends with that suffix.
- Infra directories (`deploy/`, `.github/workflows/`) are scanned by `scanInfraDir`; root-level filenames are scanned by `scanRepo`. New parsers targeting root-level files only need to add their filenames via `Filenames()`.

````

- [ ] **Step 2: Commit**

```bash
git add AGENTS.md
git commit -m "docs(agents): update §7 with FileParser interface and registration guide"
````

---

## Task 9: Final verification and PR

- [ ] **Step 1: Run full test suite with race detector**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./... -race -count=1 2>&1
```

Expected: all packages PASS, no data races

- [ ] **Step 2: Run linter if available**

```bash
cd /mnt/e/DEV/mcpdocs && golangci-lint run ./... 2>&1 | head -30
```

Expected: no new errors introduced by this PR

- [ ] **Step 3: Build binary**

```bash
cd /mnt/e/DEV/mcpdocs && go build -o /tmp/docscout-mcp . && echo "Build OK"
```

Expected: `Build OK`

- [ ] **Step 4: Push branch and create PR**

```bash
git push -u origin feat/custom-parser-extension
gh pr create \
  --title "feat: custom parser extension (#13)" \
  --body "$(cat <<'EOF'
## Summary

- Introduces `FileParser` interface and `ParserRegistry` in `scanner/parser/` — users can now register custom manifest parsers without modifying core code
- Migrates all 5 built-in parsers (gomod, packagejson, pom, codeowners, catalog) to implement `FileParser`; backward-compatible `Parse*` helpers remain
- Replaces 5 hardcoded phase loops and 5 duplicate upsert methods in `indexer/indexer.go` with a single `runParsers()` loop and one `upsertParsedFile()` method
- Scanner `targetFiles` now computed from the registry at construction time

## Test plan

- [ ] `go test ./scanner/parser/... -race` — registry tests + all parser unit tests
- [ ] `go test ./... -race` — full suite green
- [ ] `go build ./...` — binary compiles clean

## Spec

`docs/superpowers/specs/2026-04-01-custom-parser-extension-design.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-Review Checklist

- [x] `ParsedRelation` moved from `catalog.go` to `extension.go` — no duplicate symbol
- [x] `AuxEntities` handles codeowners multi-entity pattern within the generic loop
- [x] Codeowners `To = ""` sentinel documented and handled in `upsertParsedFile`
- [x] `FileGetter.GetFileContent(ctx, repo, path)` — 3-arg signature preserved
- [x] `scanner.New()` gets `*parser.ParserRegistry` as last parameter to avoid breaking existing positional callers minimally
- [x] `indexer.New()` gets `*parser.ParserRegistry` as last parameter
- [x] `classifyFile` becomes a method `(s *Scanner) classifyFile(name string) string` — all call sites updated
- [x] `DefaultTargetFiles` deprecated but kept as empty slice for backward compatibility
- [x] Registry `Register()` panics on startup — fail-fast, never silent
- [x] All built-in parsers expose factory functions (`GoModParser()`, etc.) for testability
- [x] Integration tests that call `indexer.New` / `scanner.New` must be updated (Task 7 Step 5)
