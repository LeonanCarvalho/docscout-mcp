# MCP Discovery & Catalog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically discover MCP server configs from GitHub repos, index them as `mcp-server` graph entities with `uses_mcp` relation edges, and expose a `discover_mcp_servers` MCP tool for inventory and capability search.

**Architecture:** `McpConfigParser` implements the existing `FileParser` interface and is registered in `main.go` alongside other parsers. A `KnownServerRegistry` enriches well-known servers with tool observation strings. Each discovered server becomes an `AuxEntity` (type `mcp-server`) with a `uses_mcp` relation (`To: ""`) filled by the indexer. `discover_mcp_servers` queries the graph via `SearchNodes` and `OpenNodes` with no live connections or subprocess execution.

**Tech Stack:** Go 1.26, existing `scanner/parser` `FileParser` interface, `cmp.Or`, `slices`, `strings.ToLower`, GORM-backed knowledge graph.

---

## File Map

| File                                           | Action | Responsibility                                                                        |
| ---------------------------------------------- | ------ | ------------------------------------------------------------------------------------- |
| `scanner/parser/mcp/known_servers.go`          | Create | `KnownServerRegistry` type + `DefaultKnownServers()`                                  |
| `scanner/parser/mcp/mcp_config_parser.go`      | Create | `McpConfigParser` implementing `FileParser` — parses 5 config formats                 |
| `scanner/parser/mcp/mcp_config_parser_test.go` | Create | Unit tests: parse formats, known/unknown servers, transport inference, malformed JSON |
| `tools/discover_mcp_servers.go`                | Create | `discover_mcp_servers` MCP tool handler                                               |
| `tools/tools.go`                               | Modify | Register `discover_mcp_servers`                                                       |
| `main.go`                                      | Modify | `parser.Register(mcp.NewMcpConfigParser(mcp.DefaultKnownServers()))`                  |
| `AGENTS.md`                                    | Modify | Add `mcp-server` entity type and `uses_mcp` relation to §7 tables                     |
| `tests/mcp_discovery/mcp_discovery_test.go`    | Create | Integration: scan→index→query round-trip, capability search, traverse edge            |

---

### Task 1: KnownServerRegistry

**Files:**

- Create: `scanner/parser/mcp/known_servers.go`

- [ ] **Step 1: Create the package directory and file**

```bash
mkdir -p /mnt/e/DEV/mcpdocs/scanner/parser/mcp
```

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import "strings"

// KnownServerRegistry maps lowercase MCP server names to their tool observation strings.
// Matched servers get these observations merged in addition to transport/command metadata.
type KnownServerRegistry map[string][]string

// DefaultKnownServers returns the built-in registry of well-known public MCP servers.
func DefaultKnownServers() KnownServerRegistry {
	return KnownServerRegistry{
		"github": {
			"tool:search_repositories: Search GitHub repositories by query",
			"tool:get_file_contents: Read the contents of a file from a GitHub repo",
			"tool:create_issue: Create a new GitHub issue",
			"tool:list_pull_requests: List pull requests for a repository",
		},
		"filesystem": {
			"tool:read_file: Read the contents of a file",
			"tool:write_file: Write content to a file",
			"tool:list_directory: List the contents of a directory",
			"tool:delete_file: Delete a file",
		},
		"postgres": {
			"tool:query: Execute a read-only SQL query",
			"tool:list_tables: List all tables in the database",
			"tool:describe_table: Describe the schema of a specific table",
		},
		"fetch": {
			"tool:fetch: Fetch a URL and return its content as text or markdown",
		},
		"brave-search": {
			"tool:brave_web_search: Run a web search using the Brave Search API",
		},
		"slack": {
			"tool:send_message: Send a message to a Slack channel",
			"tool:list_channels: List available Slack channels",
			"tool:get_channel_history: Retrieve message history from a channel",
		},
	}
}

// Lookup returns the tool observations for a given server name (case-insensitive).
// Returns nil if the server is not in the registry.
func (r KnownServerRegistry) Lookup(name string) []string {
	return r[strings.ToLower(name)]
}
```

Add `"strings"` to the import block.

- [ ] **Step 2: Verify it compiles**

```bash
go build ./scanner/parser/mcp/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add scanner/parser/mcp/known_servers.go
git -c commit.gpgsign=false commit -m "feat: add KnownServerRegistry for MCP tool observation enrichment"
```

---

### Task 2: McpConfigParser

**Files:**

- Create: `scanner/parser/mcp/mcp_config_parser.go`
- Create: `scanner/parser/mcp/mcp_config_parser_test.go`

- [ ] **Step 1: Write failing tests**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package mcp_test

import (
	"testing"

	mcpparser "github.com/doc-scout/mcp-server/scanner/parser/mcp"
)

func TestMcpConfigParser_FileType(t *testing.T) {
	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	if p.FileType() != "mcp-config" {
		t.Fatalf("want FileType=mcp-config, got %q", p.FileType())
	}
}

func TestMcpConfigParser_ParseDotMcpJSON(t *testing.T) {
	input := []byte(`{
		"mcpServers": {
			"github": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-github"]
			}
		}
	}`)

	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	result, err := p.Parse(input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.AuxEntities) != 1 {
		t.Fatalf("want 1 AuxEntity, got %d", len(result.AuxEntities))
	}
	ent := result.AuxEntities[0]
	if ent.Name != "github" {
		t.Fatalf("want name=github, got %q", ent.Name)
	}
	if ent.EntityType != "mcp-server" {
		t.Fatalf("want entityType=mcp-server, got %q", ent.EntityType)
	}
	// Should have transport observation
	hasTransport := false
	hasToolObs := false
	for _, obs := range ent.Observations {
		if obs == "transport:stdio" {
			hasTransport = true
		}
		if len(obs) > 5 && obs[:5] == "tool:" {
			hasToolObs = true
		}
	}
	if !hasTransport {
		t.Fatalf("expected transport:stdio observation, got: %v", ent.Observations)
	}
	if !hasToolObs {
		t.Fatalf("expected tool: observations from known registry, got: %v", ent.Observations)
	}
}

func TestMcpConfigParser_UnknownServerNoToolObs(t *testing.T) {
	input := []byte(`{
		"mcpServers": {
			"my-custom-server": {
				"command": "my-server",
				"args": ["--port", "3000"]
			}
		}
	}`)

	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	result, err := p.Parse(input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.AuxEntities) != 1 {
		t.Fatalf("want 1 AuxEntity, got %d", len(result.AuxEntities))
	}
	for _, obs := range result.AuxEntities[0].Observations {
		if len(obs) > 5 && obs[:5] == "tool:" {
			t.Fatalf("unexpected tool: observation for unknown server: %q", obs)
		}
	}
}

func TestMcpConfigParser_TransportInference(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		wantTrans string
	}{
		{
			name: "stdio from command",
			input: []byte(`{"mcpServers":{"srv":{"command":"node","args":["server.js"]}}}`),
			wantTrans: "stdio",
		},
		{
			name: "http from url",
			input: []byte(`{"mcpServers":{"srv":{"url":"http://localhost:3000/mcp"}}}`),
			wantTrans: "http",
		},
		{
			name: "sse from sse url",
			input: []byte(`{"mcpServers":{"srv":{"url":"http://localhost:3000/sse","transport":"sse"}}}`),
			wantTrans: "sse",
		},
	}

	p := mcpparser.NewMcpConfigParser(mcpparser.KnownServerRegistry{})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := p.Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			found := false
			for _, obs := range result.AuxEntities[0].Observations {
				if obs == "transport:"+tc.wantTrans {
					found = true
				}
			}
			if !found {
				t.Fatalf("want transport:%s in observations, got: %v", tc.wantTrans, result.AuxEntities[0].Observations)
			}
		})
	}
}

func TestMcpConfigParser_MalformedJSON(t *testing.T) {
	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	_, err := p.Parse([]byte(`{not valid json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestMcpConfigParser_EmptyMcpServers(t *testing.T) {
	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	result, err := p.Parse([]byte(`{"mcpServers":{}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.AuxEntities) != 0 {
		t.Fatalf("want 0 AuxEntities for empty mcpServers, got %d", len(result.AuxEntities))
	}
}

func TestMcpConfigParser_UsesRelationEmitted(t *testing.T) {
	input := []byte(`{"mcpServers":{"github":{"command":"npx","args":["-y","@modelcontextprotocol/server-github"]}}}`)
	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	result, err := p.Parse(input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.AuxEntities) == 0 {
		t.Fatal("no AuxEntities")
	}
	found := false
	for _, rel := range result.AuxEntities[0].Relations {
		if rel.RelationType == "uses_mcp" && rel.To == "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected uses_mcp relation with To='', got: %v", result.AuxEntities[0].Relations)
	}
}
```

- [ ] **Step 2: Run tests to see them fail**

```bash
go test ./scanner/parser/mcp/... -v
```

Expected: `FAIL` — `mcpparser.NewMcpConfigParser` does not exist.

- [ ] **Step 3: Create `scanner/parser/mcp/mcp_config_parser.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

// McpConfigParser discovers MCP server definitions from well-known config file formats
// and indexes each server as an mcp-server entity with a uses_mcp relation edge.
type McpConfigParser struct {
	known KnownServerRegistry
}

// NewMcpConfigParser creates a parser pre-loaded with the given known-server registry.
func NewMcpConfigParser(known KnownServerRegistry) *McpConfigParser {
	return &McpConfigParser{known: known}
}

func (p *McpConfigParser) FileType() string { return "mcp-config" }

func (p *McpConfigParser) Filenames() []string {
	return []string{
		".mcp.json",
		"mcp.json",
		".cursor/mcp.json",
		"claude_desktop_config.json",
		".vscode/mcp.json",
	}
}

// rawConfig is the shared envelope all MCP config formats use.
type rawConfig struct {
	McpServers map[string]rawServerEntry `json:"mcpServers"`
}

// rawServerEntry normalises across config formats.
type rawServerEntry struct {
	Command   string   `json:"command,omitzero"`
	Args      []string `json:"args,omitzero"`
	URL       string   `json:"url,omitzero"`
	Transport string   `json:"transport,omitzero"`
	// Some formats embed tool definitions inline.
	Tools []rawToolEntry `json:"tools,omitzero"`
}

type rawToolEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitzero"`
}

func (p *McpConfigParser) Parse(data []byte) (parser.ParsedFile, error) {
	var cfg rawConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return parser.ParsedFile{}, fmt.Errorf("mcp-config: %w", err)
	}

	var auxEntities []parser.AuxEntity
	for name, entry := range cfg.McpServers {
		obs := p.buildObservations(name, entry)
		rel := parser.ParsedRelation{RelationType: "uses_mcp", To: ""}
		auxEntities = append(auxEntities, parser.AuxEntity{
			Name:       name,
			EntityType: "mcp-server",
			Observations: obs,
			Relations:  []parser.ParsedRelation{rel},
		})
	}

	return parser.ParsedFile{AuxEntities: auxEntities}, nil
}

func (p *McpConfigParser) buildObservations(name string, entry rawServerEntry) []string {
	var obs []string

	transport := inferTransport(entry)
	obs = append(obs, "transport:"+transport)

	if entry.Command != "" {
		cmd := entry.Command
		if len(entry.Args) > 0 {
			cmd += " " + strings.Join(entry.Args, " ")
		}
		obs = append(obs, "command:"+cmd)
	}
	if entry.URL != "" {
		obs = append(obs, "url:"+entry.URL)
	}

	// Tool observations from inline config (some formats embed them).
	for _, t := range entry.Tools {
		if t.Name != "" {
			obs = append(obs, "tool:"+t.Name+": "+t.Description)
		}
	}

	// Enrich from known registry (case-insensitive lookup).
	if knownTools := p.known.Lookup(name); len(knownTools) > 0 {
		obs = append(obs, knownTools...)
	}

	return obs
}

// inferTransport determines the transport type from the entry fields.
// Uses cmp.Or pattern: explicit transport field wins, then infer from URL/command.
func inferTransport(entry rawServerEntry) string {
	if entry.Transport != "" {
		return strings.ToLower(entry.Transport)
	}
	if entry.URL != "" {
		if strings.Contains(entry.URL, "/sse") {
			return "sse"
		}
		return "http"
	}
	if entry.Command != "" {
		return "stdio"
	}
	return "unknown"
}
```

Note: `cmp.Or` from the Go standard library requires comparable zero-values. Since we need string-present logic (not just zero-check), `inferTransport` uses explicit conditions instead — this is the correct approach for non-zero-value semantics.

- [ ] **Step 4: Run tests**

```bash
go test ./scanner/parser/mcp/... -v
```

Expected: all tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add scanner/parser/mcp/mcp_config_parser.go scanner/parser/mcp/mcp_config_parser_test.go
git -c commit.gpgsign=false commit -m "feat: add McpConfigParser — discovers mcp-server entities from 5 config formats"
```

---

### Task 3: `discover_mcp_servers` MCP tool

**Files:**

- Create: `tools/discover_mcp_servers.go`
- Modify: `tools/tools.go` (register the tool)

- [ ] **Step 1: Create `tools/discover_mcp_servers.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

// DiscoverMCPServersArgs are the input parameters for discover_mcp_servers.
type DiscoverMCPServersArgs struct {
	Repo      string `json:"repo,omitempty"       jsonschema:"Filter results to a specific repository name."`
	ToolName  string `json:"tool_name,omitempty"  jsonschema:"Return only MCP servers that have a tool matching this name (capability search). Matched against tool observation prefixes."`
	Transport string `json:"transport,omitempty"  jsonschema:"Filter by transport type: stdio, http, or sse."`
	Limit     int    `json:"limit,omitempty"      jsonschema:"Maximum number of servers to return (default 20, max 100)."`
}

// MCPServerResult is one discovered MCP server.
type MCPServerResult struct {
	Name       string   `json:"name"`
	Repo       string   `json:"repo"`
	Transport  string   `json:"transport,omitzero"`
	Command    string   `json:"command,omitzero"`
	URL        string   `json:"url,omitzero"`
	Tools      []string `json:"tools"`
	ConfigFile string   `json:"config_file,omitzero"`
}

// DiscoverMCPServersResult is the structured output of discover_mcp_servers.
type DiscoverMCPServersResult struct {
	Servers []MCPServerResult `json:"servers"`
	Total   int               `json:"total"`
}

func discoverMCPServersHandler(g GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args DiscoverMCPServersArgs) (*mcp.CallToolResult, DiscoverMCPServersResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args DiscoverMCPServersArgs) (*mcp.CallToolResult, DiscoverMCPServersResult, error) {
		limit := args.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}

		// Search for all mcp-server entities.
		kg, err := g.ListEntities("mcp-server")
		if err != nil {
			return nil, DiscoverMCPServersResult{}, fmt.Errorf("discover_mcp_servers: %w", err)
		}

		var servers []MCPServerResult
		for _, entity := range kg.Entities {
			srv := extractMCPServer(entity)

			// Apply filters.
			if args.Repo != "" && srv.Repo != args.Repo {
				continue
			}
			if args.Transport != "" && srv.Transport != args.Transport {
				continue
			}
			if args.ToolName != "" && !hasMatchingTool(srv.Tools, args.ToolName) {
				continue
			}

			servers = append(servers, srv)
			if len(servers) >= limit {
				break
			}
		}

		if servers == nil {
			servers = []MCPServerResult{}
		}
		return nil, DiscoverMCPServersResult{Servers: servers, Total: len(servers)}, nil
	}
}

// extractMCPServer converts a graph entity into an MCPServerResult by parsing observations.
func extractMCPServer(entity memory.Entity) MCPServerResult {
	srv := MCPServerResult{Name: entity.Name}
	for _, obs := range entity.Observations {
		switch {
		case strings.HasPrefix(obs, "transport:"):
			srv.Transport = strings.TrimPrefix(obs, "transport:")
		case strings.HasPrefix(obs, "command:"):
			srv.Command = strings.TrimPrefix(obs, "command:")
		case strings.HasPrefix(obs, "url:"):
			srv.URL = strings.TrimPrefix(obs, "url:")
		case strings.HasPrefix(obs, "config_file:"):
			srv.ConfigFile = strings.TrimPrefix(obs, "config_file:")
		case strings.HasPrefix(obs, "_scan_repo:"):
			srv.Repo = strings.TrimPrefix(obs, "_scan_repo:")
		case strings.HasPrefix(obs, "tool:"):
			// Extract tool name from "tool:<name>: <description>"
			rest := strings.TrimPrefix(obs, "tool:")
			toolName, _, _ := strings.Cut(rest, ":")
			if toolName != "" {
				srv.Tools = append(srv.Tools, strings.TrimSpace(toolName))
			}
		}
	}
	if srv.Tools == nil {
		srv.Tools = []string{}
	}
	return srv
}

// hasMatchingTool returns true if any tool name contains the query (case-insensitive).
func hasMatchingTool(tools []string, query string) bool {
	q := strings.ToLower(query)
	for _, t := range tools {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

```

Imports for `tools/discover_mcp_servers.go` (no `encoding/json` needed):

```go
import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)
```

- [ ] **Step 2: Register `discover_mcp_servers` in `tools/tools.go`**

After all existing graph tools (before the closing brace of the `if graph != nil` block), add:

```go
		mcp.AddTool(s, &mcp.Tool{
			Name:        "discover_mcp_servers",
			Description: "Discover and catalog MCP servers found in indexed GitHub repositories. Supports three query modes: (1) inventory — list all known MCP servers; (2) capability search — find servers that expose a specific tool (tool_name filter); (3) dependency lookup — combine with traverse_graph on a service entity to follow uses_mcp edges. Filter by repo, transport (stdio/http/sse), or tool name. Only returns servers discovered from indexed config files (.mcp.json, claude_desktop_config.json, .cursor/mcp.json, mcp.json, .vscode/mcp.json).",
		}, withMetrics("discover_mcp_servers", metrics, withRecovery("discover_mcp_servers", discoverMCPServersHandler(graph))))
```

- [ ] **Step 3: Build and verify**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add tools/discover_mcp_servers.go tools/tools.go
git -c commit.gpgsign=false commit -m "feat: add discover_mcp_servers MCP tool — inventory, capability search, uses_mcp edges"
```

---

### Task 4: Wire parser in `main.go` + update `AGENTS.md`

**Files:**

- Modify: `main.go`
- Modify: `AGENTS.md`

- [ ] **Step 1: Register `McpConfigParser` in `main.go`**

In `main.go`, after the existing parser registrations (after `parser.Register(parser.K8sServiceParser())`), add:

```go
	// MCP Discovery parser (#N)
	parser.Register(mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers()))
```

Add the import:

```go
	mcpparser "github.com/doc-scout/mcp-server/scanner/parser/mcp"
```

- [ ] **Step 2: Update `AGENTS.md` §7 relation types table**

In `AGENTS.md`, find the Integration Relation Types table under `## Integration Relation Types` and add a new row:

```markdown
| `uses_mcp` | service | mcp-server | mcp-config parser |
```

Under the `New entity types:` line, append `mcp-server`:

```markdown
New entity types: `event-topic`, `grpc-service`, `mcp-server` (in addition to existing `api`, `service`, `team`, `person`).
```

- [ ] **Step 3: Build and verify**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add main.go AGENTS.md
git -c commit.gpgsign=false commit -m "feat: register McpConfigParser in main.go; update AGENTS.md with mcp-server entity type"
```

---

### Task 5: Integration tests

**Files:**

- Create: `tests/mcp_discovery/mcp_discovery_test.go`

- [ ] **Step 1: Create `tests/mcp_discovery/mcp_discovery_test.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package mcp_discovery_test

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
	"github.com/doc-scout/mcp-server/tools"
)

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s returned MCP error: %v", name, res.Content)
	}
	if len(res.Content) == 0 {
		return "{}"
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])
	}
	return text.Text
}

// seedMCPServer inserts an mcp-server entity directly into the graph via MCP tools.
func seedMCPServer(t *testing.T, session *mcp.ClientSession, name, repo, transport string, toolObs []string) {
	t.Helper()
	obs := []string{
		"transport:" + transport,
		"_scan_repo:" + repo,
		"config_file:.mcp.json",
	}
	obs = append(obs, toolObs...)

	callTool(t, session, "create_entities", map[string]any{
		"entities": []map[string]any{
			{"name": name, "entityType": "mcp-server", "observations": obs},
		},
	})
}

func TestDiscoverMCPServers_Inventory(t *testing.T) {
	session := testutils.SetupTestServer(t)
	seedMCPServer(t, session, "github", "test-org/test-repo", "stdio",
		[]string{"tool:search_repositories: Search GitHub repos"})

	raw := callTool(t, session, "discover_mcp_servers", map[string]any{})
	var result tools.DiscoverMCPServersResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal: %v — raw: %s", err, raw)
	}
	if result.Total == 0 {
		t.Fatal("expected at least one mcp-server")
	}
	if result.Servers[0].Name != "github" {
		t.Fatalf("want name=github, got %q", result.Servers[0].Name)
	}
}

func TestDiscoverMCPServers_CapabilitySearch(t *testing.T) {
	session := testutils.SetupTestServer(t)
	seedMCPServer(t, session, "github", "org/repo-a", "stdio",
		[]string{"tool:search_repositories: Search GitHub repos"})
	seedMCPServer(t, session, "postgres", "org/repo-b", "stdio",
		[]string{"tool:query: Execute a SQL query"})

	raw := callTool(t, session, "discover_mcp_servers", map[string]any{
		"tool_name": "search",
	})
	var result tools.DiscoverMCPServersResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("want total=1 (github only), got %d", result.Total)
	}
	if result.Servers[0].Name != "github" {
		t.Fatalf("want github, got %q", result.Servers[0].Name)
	}
}

func TestDiscoverMCPServers_RepoFilter(t *testing.T) {
	session := testutils.SetupTestServer(t)
	seedMCPServer(t, session, "github", "org/repo-a", "stdio", nil)
	seedMCPServer(t, session, "fetch", "org/repo-b", "http", nil)

	raw := callTool(t, session, "discover_mcp_servers", map[string]any{
		"repo": "org/repo-a",
	})
	var result tools.DiscoverMCPServersResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Total != 1 || result.Servers[0].Name != "github" {
		t.Fatalf("want only github from repo-a, got %+v", result)
	}
}

func TestDiscoverMCPServers_TraverseUsesMCPEdge(t *testing.T) {
	session := testutils.SetupTestServer(t)

	// Seed a service and an mcp-server with a uses_mcp edge.
	callTool(t, session, "create_entities", map[string]any{
		"entities": []map[string]any{
			{"name": "my-service", "entityType": "service", "observations": []string{"owns github mcp"}},
			{"name": "github", "entityType": "mcp-server", "observations": []string{"transport:stdio", "_scan_repo:org/my-repo"}},
		},
	})
	callTool(t, session, "create_relations", map[string]any{
		"relations": []map[string]any{
			{"from": "my-service", "to": "github", "relationType": "uses_mcp"},
		},
	})

	raw := callTool(t, session, "traverse_graph", map[string]any{
		"entity":        "my-service",
		"relation_type": "uses_mcp",
		"direction":     "outgoing",
		"depth":         1,
	})

	var result tools.TraverseGraphResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal TraverseGraphResult: %v — raw: %s", err, raw)
	}
	if result.TotalFound != 1 {
		t.Fatalf("want 1 node (github), got %d", result.TotalFound)
	}
	if result.Nodes[0].Entity.Name != "github" {
		t.Fatalf("want github, got %q", result.Nodes[0].Entity.Name)
	}
}

func TestDiscoverMCPServers_EmptyResult(t *testing.T) {
	session := testutils.SetupTestServer(t)

	raw := callTool(t, session, "discover_mcp_servers", map[string]any{
		"tool_name": "nonexistent-tool-xyz",
	})
	var result tools.DiscoverMCPServersResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Total != 0 {
		t.Fatalf("want total=0, got %d", result.Total)
	}
	if result.Servers == nil {
		t.Fatal("Servers must be non-nil empty slice (not null in JSON)")
	}
}
```

- [ ] **Step 2: Run integration tests**

```bash
go test ./tests/mcp_discovery/... -v -count=1 -timeout 60s
```

Expected: all five tests `PASS`.

- [ ] **Step 3: Run full test suite**

```bash
go test ./... -count=1 -timeout 120s
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add tests/mcp_discovery/mcp_discovery_test.go
git -c commit.gpgsign=false commit -m "test: add MCP discovery integration tests — inventory, capability search, traverse edge"
```
