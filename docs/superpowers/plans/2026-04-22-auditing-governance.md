# Auditing — Governance & Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist graph mutations with agent identity to a `audit_events` DB table and expose `query_audit_log` / `get_audit_summary` MCP tools and `/audit` HTTP endpoints.

**Architecture:** New `memory.AuditStore` interface + `DBAuditStore` GORM implementation. `GraphAuditLogger` gains an `agentFn func() string` (resolved from `AGENT_ID` env → MCP handshake → `"unknown"`) and a nullable `AuditStore`. Two new MCP tools delegate to `AuditReader` (read-only interface in `tools/ports.go`). HTTP endpoints mirror the tools. Graceful no-op when no persistent DB.

**Tech Stack:** Go 1.26, `github.com/google/uuid` v1.6.0 (UUIDv7), GORM, `github.com/modelcontextprotocol/go-sdk/mcp`.

---

## File Map

| File                         | Action | Responsibility                                                                                      |
| ---------------------------- | ------ | --------------------------------------------------------------------------------------------------- |
| `memory/audit_store.go`      | Create | `AuditEvent` model, `AuditFilter`, `AuditSummary`, `AuditStore` interface, `DBAuditStore` impl      |
| `memory/audit_store_test.go` | Create | Unit: write + query round-trip, UUIDv7 order, nil-safe no-op                                        |
| `tools/audit.go`             | Modify | Add `agentFn func() string` + `store memory.AuditStore`; call `writeAuditEvent` after each mutation |
| `tools/ports.go`             | Modify | Add `AuditReader` read-only interface                                                               |
| `tools/query_audit_log.go`   | Create | `query_audit_log` MCP tool handler                                                                  |
| `tools/get_audit_summary.go` | Create | `get_audit_summary` MCP tool handler                                                                |
| `tools/tools.go`             | Modify | `Register` gains `auditReader AuditReader` param; register two new tools                            |
| `tests/testutils/utils.go`   | Modify | Update `SetupTestServer` call to pass `nil` audit reader                                            |
| `tests/audit/audit_test.go`  | Create | Integration: mutation → query round-trip, risky events, HTTP endpoints                              |
| `main.go`                    | Modify | Agent identity resolution; wire `AuditStore`; add `/audit` + `/audit/summary` routes                |

---

### Task 1: AuditStore — data model + write + query + summary

**Files:**

- Create: `memory/audit_store.go`
- Create: `memory/audit_store_test.go`

- [ ] **Step 1: Write failing tests**

```go
// memory/audit_store_test.go
package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/doc-scout/mcp-server/memory"
)

func TestAuditStore_WriteAndQuery(t *testing.T) {
	ctx := t.Context()
	db, err := memory.OpenDB("")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	store, err := memory.NewAuditStore(db)
	if err != nil {
		t.Fatalf("NewAuditStore: %v", err)
	}

	err = store.Write(ctx, memory.AuditEvent{
		Agent: "test-agent", Tool: "create_entities", Operation: "create",
		Targets: memory.MarshalTargets([]string{"svc-a"}), Count: 1, Outcome: "ok",
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	events, total, err := store.Query(ctx, memory.AuditFilter{Agent: "test-agent"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Fatalf("want total=1, got %d", total)
	}
	if len(events) != 1 || events[0].Agent != "test-agent" {
		t.Fatalf("unexpected events: %v", events)
	}
	if events[0].ID == "" {
		t.Fatal("ID must be set (UUIDv7)")
	}
}

func TestAuditStore_UUIDv7Order(t *testing.T) {
	ctx := t.Context()
	db, _ := memory.OpenDB("")
	store, _ := memory.NewAuditStore(db)

	for range 3 {
		_ = store.Write(ctx, memory.AuditEvent{
			Agent: "a", Tool: "create_entities", Operation: "create",
			Targets: memory.MarshalTargets([]string{"e"}), Count: 1, Outcome: "ok",
		})
		time.Sleep(time.Millisecond) // ensure distinct UUIDv7 timestamps
	}

	events, _, _ := store.Query(ctx, memory.AuditFilter{Limit: 10})
	for i := 1; i < len(events); i++ {
		if events[i].ID <= events[i-1].ID {
			t.Fatalf("events not in UUIDv7 (chronological) order: %s <= %s", events[i].ID, events[i-1].ID)
		}
	}
}

func TestAuditStore_SummaryRiskyMassDelete(t *testing.T) {
	ctx := t.Context()
	db, _ := memory.OpenDB("")
	store, _ := memory.NewAuditStore(db)

	_ = store.Write(ctx, memory.AuditEvent{
		Agent: "bot", Tool: "delete_entities", Operation: "delete",
		Targets: memory.MarshalTargets([]string{}), Count: 15, Outcome: "ok",
	})

	summary, err := store.Summary(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if len(summary.RiskyEvents) == 0 {
		t.Fatal("expected mass delete to appear in risky_events")
	}
}
```

- [ ] **Step 2: Run tests to see them fail**

```bash
go test ./memory/... -run TestAuditStore -v
```

Expected: `FAIL` — `memory.AuditStore` and `memory.NewAuditStore` do not exist.

- [ ] **Step 3: Create `memory/audit_store.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuditEvent is one row in the audit_events table.
// ID is a UUIDv7 string — time-sortable, so ORDER BY id = chronological order.
type AuditEvent struct {
	ID        string    `gorm:"primaryKey"        json:"id"`
	CreatedAt time.Time `                         json:"created_at"`
	Agent     string    `                         json:"agent"`
	Tool      string    `                         json:"tool"`
	Operation string    `                         json:"operation"`
	Targets   string    `                         json:"targets"`   // JSON array
	Count     int       `                         json:"count"`
	Outcome   string    `                         json:"outcome"`   // "ok" | "error"
	ErrorMsg  string    `                         json:"error_msg,omitzero"`
}

// AuditFilter restricts Query results. Empty string fields are ignored.
type AuditFilter struct {
	Agent     string
	Tool      string
	Operation string
	Outcome   string
	Since     time.Time
	Limit     int // 0 → default 50; capped at 500
}

// AuditSummary aggregates mutations over a time window.
type AuditSummary struct {
	TotalMutations int            `json:"total_mutations"`
	ByAgent        map[string]int `json:"by_agent"`
	ByOperation    map[string]int `json:"by_operation"`
	ErrorRate      float64        `json:"error_rate"`
	RiskyEvents    []AuditEvent   `json:"risky_events"`
}

// AuditStore persists and queries audit events.
type AuditStore interface {
	Write(ctx context.Context, event AuditEvent) error
	Query(ctx context.Context, filter AuditFilter) ([]AuditEvent, int64, error)
	Summary(ctx context.Context, window time.Duration) (AuditSummary, error)
}

// DBAuditStore is the GORM-backed AuditStore implementation.
type DBAuditStore struct {
	db *gorm.DB
}

// NewAuditStore auto-migrates the audit_events table and returns a DBAuditStore.
func NewAuditStore(db *gorm.DB) (*DBAuditStore, error) {
	if err := db.AutoMigrate(&AuditEvent{}); err != nil {
		return nil, err
	}
	return &DBAuditStore{db: db}, nil
}

func (s *DBAuditStore) Write(ctx context.Context, event AuditEvent) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	event.ID = id.String()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	return s.db.WithContext(ctx).Create(&event).Error
}

func (s *DBAuditStore) Query(ctx context.Context, f AuditFilter) ([]AuditEvent, int64, error) {
	q := s.db.WithContext(ctx).Model(&AuditEvent{})
	if f.Agent != "" {
		q = q.Where("agent = ?", f.Agent)
	}
	if f.Tool != "" {
		q = q.Where("tool = ?", f.Tool)
	}
	if f.Operation != "" {
		q = q.Where("operation = ?", f.Operation)
	}
	if f.Outcome != "" {
		q = q.Where("outcome = ?", f.Outcome)
	}
	if !f.Since.IsZero() {
		q = q.Where("created_at >= ?", f.Since)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	var events []AuditEvent
	if err := q.Order("id ASC").Limit(limit).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

func (s *DBAuditStore) Summary(ctx context.Context, window time.Duration) (AuditSummary, error) {
	since := time.Now().UTC().Add(-window)
	var events []AuditEvent
	if err := s.db.WithContext(ctx).Where("created_at >= ?", since).Find(&events).Error; err != nil {
		return AuditSummary{}, err
	}

	sum := AuditSummary{
		ByAgent:     make(map[string]int),
		ByOperation: make(map[string]int),
	}
	agentErrorTimes := make(map[string][]time.Time)
	var totalErrors int

	for _, e := range events {
		sum.TotalMutations++
		sum.ByAgent[e.Agent]++
		sum.ByOperation[e.Operation]++

		if e.Outcome == "error" {
			totalErrors++
			agentErrorTimes[e.Agent] = append(agentErrorTimes[e.Agent], e.CreatedAt)
		}
		if e.Operation == "delete" && e.Count > 10 {
			sum.RiskyEvents = append(sum.RiskyEvents, e)
		}
		if e.Agent == "unknown" {
			sum.RiskyEvents = append(sum.RiskyEvents, e)
		}
	}

	if sum.TotalMutations > 0 {
		sum.ErrorRate = float64(totalErrors) / float64(sum.TotalMutations)
	}

	// Detect error bursts: >5 errors from same agent within any 1h window.
	for agent, times := range agentErrorTimes {
		slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
		for i := range times {
			cutoff := times[i].Add(time.Hour)
			count := 0
			for j := i; j < len(times) && times[j].Before(cutoff); j++ {
				count++
			}
			if count > 5 {
				for _, e := range events {
					if e.Agent == agent && e.Outcome == "error" &&
						!e.CreatedAt.Before(times[i]) && e.CreatedAt.Before(cutoff) {
						sum.RiskyEvents = append(sum.RiskyEvents, e)
					}
				}
				break
			}
		}
	}

	return sum, nil
}

// MarshalTargets serializes a slice of entity names to a JSON string for storage.
func MarshalTargets(names []string) string {
	b, _ := json.Marshal(names)
	return string(b)
}
```

- [ ] **Step 4: Run tests to see them pass**

```bash
go test ./memory/... -run TestAuditStore -v
```

Expected: all three tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add memory/audit_store.go memory/audit_store_test.go
git -c commit.gpgsign=false commit -m "feat: add AuditStore — UUIDv7-keyed audit_events table with write/query/summary"
```

---

### Task 2: GraphAuditLogger — agent identity + write to store

**Files:**

- Modify: `tools/audit.go`
- Modify: `tools/ports.go`

- [ ] **Step 1: Add `AuditReader` to `tools/ports.go`**

Append to the end of `tools/ports.go`:

```go
// AuditReader is the read-only view of the audit store used by MCP tools and HTTP handlers.
// memory.DBAuditStore satisfies this interface.
type AuditReader interface {
	Query(ctx context.Context, filter memory.AuditFilter) ([]memory.AuditEvent, int64, error)
	Summary(ctx context.Context, window time.Duration) (memory.AuditSummary, error)
}
```

Add `"time"` and `"github.com/doc-scout/mcp-server/memory"` to the imports if not already present.

- [ ] **Step 2: Update `GraphAuditLogger` in `tools/audit.go`**

Replace the struct definition and constructor:

```go
// GraphAuditLogger is a GraphStore decorator that logs every mutation to slog
// and, when a store is provided, persists an AuditEvent row.
type GraphAuditLogger struct {
	inner   GraphStore
	agentFn func() string      // called per event; never nil
	store   memory.AuditStore  // nil = no-op (in-memory deployments)
}

// NewGraphAuditLogger wraps inner with audit logging.
// agentFn is called on each write to resolve the current agent identity.
// store may be nil — audit persistence is skipped silently.
func NewGraphAuditLogger(inner GraphStore, agentFn func() string, store memory.AuditStore) *GraphAuditLogger {
	return &GraphAuditLogger{inner: inner, agentFn: agentFn, store: store}
}
```

Add the `writeAuditEvent` helper directly after the struct definition:

```go
func (a *GraphAuditLogger) writeAuditEvent(ctx context.Context, tool, operation string, targets []string, count int, outcome, errorMsg string) {
	if a.store == nil {
		return
	}
	event := memory.AuditEvent{
		Agent:     a.agentFn(),
		Tool:      tool,
		Operation: operation,
		Targets:   memory.MarshalTargets(targets),
		Count:     count,
		Outcome:   outcome,
		ErrorMsg:  errorMsg,
	}
	if err := a.store.Write(ctx, event); err != nil {
		slog.Warn("[graph:audit] failed to persist audit event", "tool", tool, "error", err)
	}
}
```

Update every mutation method to call `writeAuditEvent`. Example for `CreateEntities`:

```go
func (a *GraphAuditLogger) CreateEntities(entities []memory.Entity) ([]memory.Entity, error) {
	names := entityNames(entities)
	result, err := a.inner.CreateEntities(entities)
	outcome, errMsg := "ok", ""
	if err != nil {
		slog.Warn("[graph:audit] create_entities failed", "names", names, "error", err)
		outcome, errMsg = "error", err.Error()
	} else {
		slog.Info("[graph:audit] create_entities", "names", names, "count", len(result))
	}
	a.writeAuditEvent(context.Background(), "create_entities", "create", names, len(entities), outcome, errMsg)
	return result, err
}
```

Apply the same pattern to all six mutation methods (`CreateRelations`, `AddObservations`, `DeleteEntities`, `DeleteObservations`, `DeleteRelations`, `UpdateEntity`), adjusting `tool`, `operation`, `targets`, and `count` for each:

| Method               | tool                    | operation  | targets                                                 | count                             |
| -------------------- | ----------------------- | ---------- | ------------------------------------------------------- | --------------------------------- |
| `CreateRelations`    | `"create_relations"`    | `"create"` | `[]string{fmt.Sprintf("%d relations", len(relations))}` | `len(relations)`                  |
| `AddObservations`    | `"add_observations"`    | `"add"`    | `observationEntityNames(observations)`                  | `countObservations(observations)` |
| `DeleteEntities`     | `"delete_entities"`     | `"delete"` | `entityNames`                                           | `len(entityNames)`                |
| `DeleteObservations` | `"delete_observations"` | `"delete"` | `observationEntityNames(deletions)`                     | `len(deletions)`                  |
| `DeleteRelations`    | `"delete_relations"`    | `"delete"` | `[]string{fmt.Sprintf("%d relations", len(relations))}` | `len(relations)`                  |
| `UpdateEntity`       | `"update_entity"`       | `"update"` | `[]string{oldName}`                                     | `1`                               |

Add `"context"` and `"fmt"` to imports in `tools/audit.go` if not already present.

- [ ] **Step 3: Fix the `NewGraphAuditLogger` call in `main.go` (temporary stub)**

Locate this line in `main.go`:

```go
auditedGraph := tools.NewGraphAuditLogger(memorySrv)
```

Replace with a stub that keeps it compiling (full wiring happens in Task 5):

```go
auditedGraph := tools.NewGraphAuditLogger(memorySrv, func() string { return "unknown" }, nil)
```

- [ ] **Step 4: Verify it compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add tools/audit.go tools/ports.go main.go
git -c commit.gpgsign=false commit -m "feat: GraphAuditLogger — agent identity + AuditStore write on each mutation"
```

---

### Task 3: `query_audit_log` MCP tool

**Files:**

- Create: `tools/query_audit_log.go`
- Modify: `tools/tools.go` (Register signature + tool registration)
- Modify: `tests/testutils/utils.go` (fix Register call)

- [ ] **Step 1: Create `tools/query_audit_log.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

const auditDisabledMsg = "audit persistence not enabled — set DATABASE_URL to a persistent store"

// QueryAuditLogArgs are the input parameters for query_audit_log.
type QueryAuditLogArgs struct {
	Agent     string `json:"agent,omitempty"     jsonschema:"Filter events by agent name (e.g. claude-desktop, indexer-bot)."`
	Tool      string `json:"tool,omitempty"      jsonschema:"Filter events by MCP tool name (e.g. create_entities, delete_entities)."`
	Operation string `json:"operation,omitempty" jsonschema:"Filter by operation type: create, delete, update, or add."`
	Outcome   string `json:"outcome,omitempty"   jsonschema:"Filter by outcome: ok or error."`
	Since     string `json:"since,omitempty"     jsonschema:"Return only events after this RFC3339 timestamp (e.g. 2026-04-21T00:00:00Z)."`
	Limit     int    `json:"limit,omitempty"     jsonschema:"Maximum events to return (default 50, max 500)."`
}

// QueryAuditLogResult is the structured output of query_audit_log.
type QueryAuditLogResult struct {
	Events []memory.AuditEvent `json:"events"`
	Total  int64               `json:"total"`
}

func queryAuditLogHandler(r AuditReader) func(ctx context.Context, req *mcp.CallToolRequest, args QueryAuditLogArgs) (*mcp.CallToolResult, QueryAuditLogResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args QueryAuditLogArgs) (*mcp.CallToolResult, QueryAuditLogResult, error) {
		if r == nil {
			return nil, QueryAuditLogResult{}, fmt.Errorf(auditDisabledMsg)
		}

		filter := memory.AuditFilter{
			Agent:     args.Agent,
			Tool:      args.Tool,
			Operation: args.Operation,
			Outcome:   args.Outcome,
			Limit:     args.Limit,
		}
		if args.Since != "" {
			t, err := time.Parse(time.RFC3339, args.Since)
			if err != nil {
				return nil, QueryAuditLogResult{}, fmt.Errorf("invalid 'since' timestamp: %w", err)
			}
			filter.Since = t
		}

		events, total, err := r.Query(ctx, filter)
		if err != nil {
			return nil, QueryAuditLogResult{}, fmt.Errorf("query_audit_log: %w", err)
		}
		if events == nil {
			events = []memory.AuditEvent{}
		}
		return nil, QueryAuditLogResult{Events: events, Total: total}, nil
	}
}
```

- [ ] **Step 2: Update `tools/tools.go` — Register signature**

Change the `Register` function signature from:

```go
func Register(s *mcp.Server, sc DocumentScanner, graph GraphStore, search ContentSearcher, semantic SemanticSearch, metrics *ToolMetrics, docMetrics *DocMetrics, readOnly bool) {
```

To:

```go
func Register(s *mcp.Server, sc DocumentScanner, graph GraphStore, search ContentSearcher, semantic SemanticSearch, metrics *ToolMetrics, docMetrics *DocMetrics, readOnly bool, auditReader AuditReader) {
```

At the bottom of `Register`, after all existing tool registrations, add:

```go
	// --- Audit Tools ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "query_audit_log",
		Description: "Query the persistent audit log of graph mutations. Filter by agent, tool name, operation type (create/delete/update/add), outcome (ok/error), or time window. Returns raw audit events and total count. Only available when DATABASE_URL is set to a persistent store.",
	}, withMetrics("query_audit_log", metrics, withRecovery("query_audit_log", queryAuditLogHandler(auditReader))))
```

- [ ] **Step 3: Fix the two `Register` calls in `main.go`**

Both calls (initial registration and the one inside `SetOnScanComplete`) must add `nil` as the last argument temporarily:

```go
tools.Register(mcpServer, sc, auditedGraph, searcher, semanticSrv, toolMetrics, docMetrics, graphReadOnly, nil)
```

- [ ] **Step 4: Fix `tests/testutils/utils.go`**

Update the `Register` call:

```go
tools.Register(server, &MockScanner{}, memorySrv, nil, nil, tools.NewToolMetrics(), tools.NewDocMetrics(), false, nil)
```

- [ ] **Step 5: Verify it compiles and existing tests pass**

```bash
go build ./... && go test ./... -count=1 -timeout 120s
```

Expected: all existing tests `PASS`.

- [ ] **Step 6: Commit**

```bash
git add tools/query_audit_log.go tools/tools.go tests/testutils/utils.go main.go
git -c commit.gpgsign=false commit -m "feat: add query_audit_log MCP tool"
```

---

### Task 4: `get_audit_summary` MCP tool

**Files:**

- Create: `tools/get_audit_summary.go`
- Modify: `tools/tools.go` (register the new tool)

- [ ] **Step 1: Create `tools/get_audit_summary.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

// GetAuditSummaryArgs are the input parameters for get_audit_summary.
type GetAuditSummaryArgs struct {
	Window string `json:"window,omitempty" jsonschema:"Time window for the summary: 1h, 24h, or 7d. Defaults to 24h."`
}

// GetAuditSummaryResult is the structured output of get_audit_summary.
type GetAuditSummaryResult struct {
	memory.AuditSummary
}

func getAuditSummaryHandler(r AuditReader) func(ctx context.Context, req *mcp.CallToolRequest, args GetAuditSummaryArgs) (*mcp.CallToolResult, GetAuditSummaryResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args GetAuditSummaryArgs) (*mcp.CallToolResult, GetAuditSummaryResult, error) {
		if r == nil {
			return nil, GetAuditSummaryResult{}, fmt.Errorf(auditDisabledMsg)
		}

		windows := map[string]time.Duration{
			"1h":  time.Hour,
			"24h": 24 * time.Hour,
			"7d":  7 * 24 * time.Hour,
		}
		window := windows[args.Window]
		if window == 0 {
			window = 24 * time.Hour
		}

		summary, err := r.Summary(ctx, window)
		if err != nil {
			return nil, GetAuditSummaryResult{}, fmt.Errorf("get_audit_summary: %w", err)
		}
		return nil, GetAuditSummaryResult{AuditSummary: summary}, nil
	}
}
```

- [ ] **Step 2: Register `get_audit_summary` in `tools/tools.go`**

After the `query_audit_log` registration, add:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_audit_summary",
		Description: "Returns an anomaly-focused audit summary over a rolling time window (1h, 24h, or 7d). Highlights risky events: mass deletes (count > 10), unknown agents, and error bursts (>5 errors from same agent within 1h). Includes total mutations, breakdown by agent and operation type, and error rate. Only available when DATABASE_URL is set.",
	}, withMetrics("get_audit_summary", metrics, withRecovery("get_audit_summary", getAuditSummaryHandler(auditReader))))
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add tools/get_audit_summary.go tools/tools.go
git -c commit.gpgsign=false commit -m "feat: add get_audit_summary MCP tool with risky event detection"
```

---

### Task 5: Wire `AuditStore` + HTTP endpoints in `main.go`

**Files:**

- Modify: `main.go`

- [ ] **Step 1: Add agent identity resolution and AuditStore wiring to `main.go`**

After the `--- Database ---` block (after `memory.OpenDB`), add:

```go
	// --- Audit Store ---
	// Only enabled for persistent (non-in-memory) deployments.
	var auditStore memory.AuditStore
	if !isInMemoryDB(dbURL) {
		as, err := memory.NewAuditStore(db)
		if err != nil {
			slog.Error("Failed to initialise audit store", "error", err)
			os.Exit(1)
		}
		auditStore = as
		slog.Info("Audit persistence enabled")
	} else {
		slog.Info("Audit persistence disabled (no persistent DATABASE_URL)")
	}

	// --- Agent Identity ---
	agentID := os.Getenv("AGENT_ID")
	var capturedClient atomic.Value
	capturedClient.Store("") // default: empty string
```

- [ ] **Step 2: Replace the `mcp.NewServer` call to capture clientInfo**

Replace:

```go
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, nil)
```

With:

```go
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, &mcp.ServerOptions{
		InitializedHandler: func(_ context.Context, req *mcp.InitializedRequest) {
			if agentID != "" {
				return // AGENT_ID env takes priority; ignore clientInfo
			}
			if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil && p.ClientInfo.Name != "" {
				capturedClient.CompareAndSwap("", p.ClientInfo.Name)
			}
		},
	})
```

- [ ] **Step 3: Replace the `GraphAuditLogger` stub with full wiring**

Replace:

```go
	auditedGraph := tools.NewGraphAuditLogger(memorySrv, func() string { return "unknown" }, nil)
```

With:

```go
	agentFn := func() string {
		client, _ := capturedClient.Load().(string)
		return cmp.Or(agentID, client, "unknown")
	}
	auditedGraph := tools.NewGraphAuditLogger(memorySrv, agentFn, auditStore)
```

Add `"cmp"` and `"sync/atomic"` to the import block.

- [ ] **Step 4: Pass `auditStore` to both `Register` calls**

Replace the two `nil` stubs from Task 3:

```go
tools.Register(mcpServer, sc, auditedGraph, searcher, semanticSrv, toolMetrics, docMetrics, graphReadOnly, auditStore)
```

(Do this for both the initial call and the one inside `SetOnScanComplete`.)

- [ ] **Step 5: Add `/audit` and `/audit/summary` HTTP routes**

Inside the `if httpAddr != ""` block, after the `/metrics` handler, add:

```go
			mux.HandleFunc("/audit", func(w http.ResponseWriter, r *http.Request) {
				if auditStore == nil {
					http.Error(w, `{"error":"audit persistence not enabled — set DATABASE_URL to a persistent store"}`, http.StatusServiceUnavailable)
					return
				}
				filter := memory.AuditFilter{
					Agent:     r.URL.Query().Get("agent"),
					Tool:      r.URL.Query().Get("tool"),
					Operation: r.URL.Query().Get("operation"),
					Outcome:   r.URL.Query().Get("outcome"),
				}
				if s := r.URL.Query().Get("since"); s != "" {
					t, err := time.Parse(time.RFC3339, s)
					if err != nil {
						http.Error(w, `{"error":"invalid since timestamp"}`, http.StatusBadRequest)
						return
					}
					filter.Since = t
				}
				if l := r.URL.Query().Get("limit"); l != "" {
					if n, err := strconv.Atoi(l); err == nil {
						filter.Limit = n
					}
				}
				events, total, err := auditStore.Query(r.Context(), filter)
				if err != nil {
					http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
					return
				}
				if events == nil {
					events = []memory.AuditEvent{}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"events": events, "total": total})
			})

			mux.HandleFunc("/audit/summary", func(w http.ResponseWriter, r *http.Request) {
				if auditStore == nil {
					http.Error(w, `{"error":"audit persistence not enabled — set DATABASE_URL to a persistent store"}`, http.StatusServiceUnavailable)
					return
				}
				windows := map[string]time.Duration{
					"1h":  time.Hour,
					"24h": 24 * time.Hour,
					"7d":  7 * 24 * time.Hour,
				}
				window := windows[r.URL.Query().Get("window")]
				if window == 0 {
					window = 24 * time.Hour
				}
				summary, err := auditStore.Summary(r.Context(), window)
				if err != nil {
					http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(summary)
			})
```

Add `"encoding/json"` to imports if not already present (it should be via `fmt.Fprintf` usage — verify).

- [ ] **Step 6: Build and run lint**

```bash
go build ./... && go vet ./...
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add main.go
git -c commit.gpgsign=false commit -m "feat: wire AuditStore to GraphAuditLogger, Register, and HTTP /audit endpoints"
```

---

### Task 6: Integration tests

**Files:**

- Create: `tests/audit/audit_test.go`

- [ ] **Step 1: Create `tests/audit/audit_test.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package audit_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/tests/testutils"
	"github.com/doc-scout/mcp-server/tools"
)

// setupAuditServer creates a test MCP server with a live AuditStore.
func setupAuditServer(t *testing.T) (*mcp.ClientSession, memory.AuditStore) {
	t.Helper()
	ctx := t.Context()

	db, err := memory.OpenDB("")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	auditStore, err := memory.NewAuditStore(db)
	if err != nil {
		t.Fatalf("NewAuditStore: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	memorySrv := memory.NewMemoryService(db)
	agentFn := func() string { return "test-agent" }
	auditedGraph := tools.NewGraphAuditLogger(memorySrv, agentFn, auditStore)

	tools.Register(server, &testutils.MockScanner{}, auditedGraph, nil, nil,
		tools.NewToolMetrics(), tools.NewDocMetrics(), false, auditStore)

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	return session, auditStore
}

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

func TestAudit_CreateEntitiesAppearsInQueryLog(t *testing.T) {
	session, store := setupAuditServer(t)

	callTool(t, session, "create_entities", map[string]any{
		"entities": []map[string]any{
			{"name": "svc-audit", "entityType": "service", "observations": []string{"audit test"}},
		},
	})

	// Give the write a moment (it's synchronous, but just in case)
	time.Sleep(10 * time.Millisecond)

	events, total, err := store.Query(t.Context(), memory.AuditFilter{Tool: "create_entities"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total == 0 || len(events) == 0 {
		t.Fatal("expected at least one audit event for create_entities")
	}
	if events[0].Agent != "test-agent" {
		t.Fatalf("want agent=test-agent, got %q", events[0].Agent)
	}
	if events[0].Outcome != "ok" {
		t.Fatalf("want outcome=ok, got %q", events[0].Outcome)
	}
}

func TestAudit_QueryAuditLogMCPTool(t *testing.T) {
	session, _ := setupAuditServer(t)

	callTool(t, session, "create_entities", map[string]any{
		"entities": []map[string]any{
			{"name": "svc-x", "entityType": "service", "observations": []string{"x"}},
		},
	})

	raw := callTool(t, session, "query_audit_log", map[string]any{
		"tool": "create_entities",
	})

	var result tools.QueryAuditLogResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal QueryAuditLogResult: %v — raw: %s", err, raw)
	}
	if result.Total == 0 {
		t.Fatal("expected total > 0")
	}
}

func TestAudit_GetAuditSummaryRiskyMassDelete(t *testing.T) {
	session, store := setupAuditServer(t)

	// Seed a mass-delete event directly into the store (simulates a confirmed mass delete).
	_ = store.Write(t.Context(), memory.AuditEvent{
		Agent:     "test-agent",
		Tool:      "delete_entities",
		Operation: "delete",
		Targets:   memory.MarshalTargets([]string{}),
		Count:     15,
		Outcome:   "ok",
	})

	raw := callTool(t, session, "get_audit_summary", map[string]any{"window": "24h"})

	var result tools.GetAuditSummaryResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal GetAuditSummaryResult: %v — raw: %s", err, raw)
	}
	if len(result.RiskyEvents) == 0 {
		t.Fatal("expected mass delete to appear in risky_events")
	}
}

func TestAudit_NilAuditReaderReturnsError(t *testing.T) {
	// Use the standard test server (no AuditStore) — query_audit_log must return an error.
	session := testutils.SetupTestServer(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "query_audit_log",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected MCP error when audit persistence is disabled")
	}
}
```

- [ ] **Step 2: Run integration tests**

```bash
go test ./tests/audit/... -v -count=1 -timeout 60s
```

Expected: all four tests `PASS`.

- [ ] **Step 3: Run full test suite**

```bash
go test ./... -count=1 -timeout 120s
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add tests/audit/audit_test.go
git -c commit.gpgsign=false commit -m "test: add audit integration tests — mutation→query, risky events, nil-store error"
```
