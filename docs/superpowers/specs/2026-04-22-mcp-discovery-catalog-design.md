# MCP Discovery & Catalog Design

**Date:** 2026-04-22
**Status:** Approved

---

## Overview

Extend DocScout-MCP to automatically discover MCP server configurations from GitHub repos and catalog them as `mcp-server` graph entities. Agents can then query the catalog for inventory, capability search ("which server can do X?"), and service dependency edges (`uses_mcp`).

Two complementary modes:
- **Auto-discovery**: `McpConfigParser` scans all known MCP config file formats and indexes servers into the knowledge graph.
- **Manual registration**: agents can register servers directly via existing `create_entities` + `create_relations` tools.

---

## Approach

**Option C — Static parsing + known-server enrichment registry.**

Static parsing extracts server metadata (name, transport, command/URL) from config files. A small embedded registry maps well-known server names to their tool descriptions, enriching observations without any subprocess execution or live connections.

Unknown servers are still indexed — they receive transport/command/url observations only. The registry degrades gracefully and is trivially extensible.

---

## Data Model

New entity type: `mcp-server`

**Observations per server:**

| Observation | Example |
|---|---|
| `_source:mcp-config` | added by indexer |
| `_scan_repo:<repo>` | added by indexer |
| `transport:<type>` | `transport:stdio` |
| `command:<cmd>` | `command:npx -y @modelcontextprotocol/server-github` |
| `url:<url>` | `url:https://mcp.example.com` |
| `config_file:<filename>` | `config_file:.mcp.json` |
| `tool:<name>: <description>` | `tool:search_repositories: Search GitHub repositories` |

**New relation type:**

| Relation | From | To | Source |
|---|---|---|---|
| `uses_mcp` | service | mcp-server | mcp-config parser |

`uses_mcp` edges are emitted with `To: ""` (filled by the indexer with the repo's service name, consistent with existing parser conventions).

---

## Config Parser

`McpConfigParser` implements the `FileParser` interface in `scanner/parser/mcp/mcp_config_parser.go`:

```go
func (p *McpConfigParser) FileType()  string   { return "mcp-config" }
func (p *McpConfigParser) Filenames() []string {
    return []string{
        ".mcp.json",
        "mcp.json",
        ".cursor/mcp.json",
        "claude_desktop_config.json",
        ".vscode/mcp.json",
    }
}
```

### Intermediate struct

```go
// omitzero used throughout (Go 1.24+) — never omitempty
type mcpServerEntry struct {
    Name      string         `json:"name"`
    Command   string         `json:"command,omitzero"`
    Args      []string       `json:"args,omitzero"`
    URL       string         `json:"url,omitzero"`
    Transport string         // inferred, not from JSON
    Tools     []mcpToolEntry `json:"tools,omitzero"`
}
```

### Transport inference

Uses `cmp.Or` (Go 1.22+) — no if/else chains:

```go
entry.Transport = cmp.Or(
    inferFromURL(entry.URL),      // "http" or "sse"
    inferFromCommand(entry.Command), // "stdio"
    "unknown",
)
```

### Output

Each `mcpServerEntry` becomes one `AuxEntity` (type: `mcp-server`) in the `ParsedFile`, plus one `ParsedRelation` (`uses_mcp`, `To: ""`).

### Registration

```go
// main.go — at startup, before scanner construction
parser.Register(mcp.NewMcpConfigParser(mcp.DefaultKnownServers()))
```

---

## Known Server Registry

Embedded in `scanner/parser/mcp/known_servers.go`:

```go
type KnownServerRegistry map[string][]string // lowercase name → tool observation strings

func DefaultKnownServers() KnownServerRegistry {
    return KnownServerRegistry{
        "github":      {"tool:search_repositories: Search GitHub repositories", "tool:get_file_contents: Read file from a repo", "tool:create_issue: Create a GitHub issue"},
        "filesystem":  {"tool:read_file: Read file contents", "tool:write_file: Write file contents", "tool:list_directory: List directory entries"},
        "postgres":    {"tool:query: Execute a SQL query", "tool:list_tables: List all tables", "tool:describe_table: Describe table schema"},
        "fetch":       {"tool:fetch: Fetch a URL and return its content"},
        "brave-search":{"tool:brave_web_search: Run a web search via Brave"},
        "slack":       {"tool:send_message: Send a Slack message", "tool:list_channels: List Slack channels"},
    }
}
```

- Lookup is case-insensitive (server name normalised to lowercase before lookup).
- Matching tool observations are merged into the `AuxEntity`.
- Unknown servers are indexed without tool observations — not an error.
- Tests inject a minimal registry for isolation.

---

## MCP Tool

### `discover_mcp_servers`

```
Args:
  repo        string   optional — filter by repo name
  tool_name   string   optional — capability search: servers with a tool matching this name
  transport   string   optional — "stdio" | "http" | "sse"
  limit       int      optional — default 20, max 100

Result:
  servers  []MCPServerResult
  total    int

MCPServerResult:
  name         string
  repo         string
  transport    string
  command      string    empty for http/sse
  url          string    empty for stdio
  tools        []string  tool names from observations
  config_file  string    which config file discovered this server
```

`tool_name` search matches against the `tool:<name>:` observation prefix — capability search with no live connections required.

`traverse_graph` already handles "which services use this MCP server?" via `uses_mcp` edges — no additional tool needed.

---

## Error Handling & Degradation

| Scenario | Behavior |
|---|---|
| Unparseable JSON config file | `Parse()` returns error → file skipped with slog warning; scan continues |
| Config file with no `mcpServers` key | Returns empty `ParsedFile` — not an error |
| Server name collides with existing entity | `MergeModeUpsert` — observations merged, no duplicate |
| Unknown server (not in registry) | Indexed with transport/command/url only; no tool observations |
| `discover_mcp_servers` with no results | `{ "servers": [], "total": 0 }` — not an error |
| `tool_name` filter matches nothing | Same as above |
| Subdirectory config (e.g. `.cursor/mcp.json`) | `scanInfraDir` already handles subdirectory traversal |

**Security**: `command` values are stored as observations only — never executed. No subprocess execution. No local filesystem reads — parsing operates on pre-fetched GitHub API content.

---

## Go 1.26 Compliance

All implementation must apply modern Go patterns per project guidelines:

| Pattern | Usage |
|---|---|
| `omitzero` | All JSON struct tags on optional fields (not `omitempty`) |
| `cmp.Or(...)` | Transport inference; agent identity fallback chain in `main.go` |
| `for i := range n` | Iteration over slice indices |
| `slices` package | Deduplication, sorting of observations |
| `errors.AsType[T](err)` | Any error type assertions |
| `wg.Go(fn)` | Any goroutine spawning |
| `t.Context()` | All test context setup |
| `b.Loop()` | All benchmark main loops |
| `new(val)` | Pointer fields in structs |

---

## Testing

### Unit (`scanner/parser/mcp/mcp_config_parser_test.go`)
- `.mcp.json` with stdio server → correct `AuxEntity` + `uses_mcp` relation
- `claude_desktop_config.json` → normalises `mcpServers` block correctly
- Known server name → tool observations merged from registry
- Unknown server → transport/command only, no tool observations
- Malformed JSON → `Parse()` returns error
- Empty `mcpServers` key → empty `ParsedFile`, no error
- Transport inference: command → `"stdio"`, URL → `"http"`/`"sse"`, via `cmp.Or`

### Integration (`tests/mcp_discovery/mcp_discovery_test.go`)
- Repo with `.mcp.json` → `discover_mcp_servers` returns server with correct observations
- `tool_name` filter → only servers with matching tool observation returned
- `repo` filter → scoped correctly
- `traverse_graph` on service entity → `uses_mcp` edge reaches `mcp-server` entity
- Multiple config formats in same repo → all servers indexed, no duplicates

Follows the same E2E harness pattern as `tests/integration_map/`.

---

## Files Affected

| File | Change |
|---|---|
| `scanner/parser/mcp/mcp_config_parser.go` | New — `McpConfigParser` implementing `FileParser` |
| `scanner/parser/mcp/known_servers.go` | New — `KnownServerRegistry` + `DefaultKnownServers()` |
| `tools/discover_mcp_servers.go` | New — MCP tool handler |
| `tools/tools.go` | Register `discover_mcp_servers` |
| `tools/ports.go` | No change needed (uses existing `GraphStore` via `SearchNodes`) |
| `main.go` | Register `McpConfigParser`; `cmp.Or` agent identity chain |
| `scanner/parser/mcp/mcp_config_parser_test.go` | New — unit tests |
| `tests/mcp_discovery/mcp_discovery_test.go` | New — integration tests |
| `AGENTS.md` | Add `mcp-server` entity type and `uses_mcp` relation to §7 tables |
