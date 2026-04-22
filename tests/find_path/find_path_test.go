// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package find_path_test

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/tests/testutils"
	"github.com/doc-scout/mcp-server/tools"
)

var testCounter atomic.Int64

// newSession creates an isolated MCP test session with its own SQLite DB.

func newSession(t *testing.T) *mcp.ClientSession {

	t.Helper()

	ctx := t.Context()

	server := mcp.NewServer(&mcp.Implementation{Name: "docscout-mcp-test", Version: "test"}, nil)

	dsn := fmt.Sprintf("file:memdb_findpath_%d?mode=memory&cache=shared", testCounter.Add(1))

	db, err := memory.OpenDB(dsn)

	if err != nil {

		t.Fatalf("memory.OpenDB: %v", err)

	}

	memorySrv := memory.NewMemoryService(db)

	tools.Register(server, &testutils.MockScanner{}, memorySrv, nil, nil, tools.NewToolMetrics(), tools.NewDocMetrics(), false)

	t1, t2 := mcp.NewInMemoryTransports()

	if _, err := server.Connect(ctx, t1, nil); err != nil {

		t.Fatalf("server connect: %v", err)

	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)

	session, err := client.Connect(ctx, t2, nil)

	if err != nil {

		t.Fatalf("client connect: %v", err)

	}

	return session

}

// callFindPath calls the find_path tool and returns a parsed FindPathResult.

func callFindPath(t *testing.T, session *mcp.ClientSession, args map[string]any) tools.FindPathResult {

	t.Helper()

	ctx := t.Context()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "find_path",

		Arguments: args,
	})

	if err != nil {

		t.Fatalf("find_path call failed: %v", err)

	}

	if res.IsError {

		t.Fatalf("find_path returned MCP error: %v", res.Content)

	}

	if len(res.Content) == 0 {

		t.Fatal("find_path: empty response content")

	}

	text, ok := res.Content[0].(*mcp.TextContent)

	if !ok {

		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])

	}

	var result tools.FindPathResult

	if err := json.Unmarshal([]byte(text.Text), &result); err != nil {

		t.Fatalf("unmarshal FindPathResult: %v — raw: %s", err, text.Text)

	}

	return result

}

// seedLinearChain creates: frontend →depends_on→ backend →depends_on→ database

func seedLinearChain(t *testing.T, session *mcp.ClientSession) {

	t.Helper()

	ctx := t.Context()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "frontend", "entityType": "service", "observations": []string{}},

				{"name": "backend", "entityType": "service", "observations": []string{}},

				{"name": "database", "entityType": "service", "observations": []string{}},
			},
		},
	})

	if err != nil {

		t.Fatalf("create_entities: %v", err)

	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_relations",

		Arguments: map[string]any{

			"relations": []map[string]any{

				{"from": "frontend", "to": "backend", "relationType": "depends_on"},

				{"from": "backend", "to": "database", "relationType": "depends_on"},
			},
		},
	})

	if err != nil {

		t.Fatalf("create_relations: %v", err)

	}

}

// TestFindPath_DirectConnection verifies a single-hop path is found.

func TestFindPath_DirectConnection(t *testing.T) {

	session := newSession(t)

	seedLinearChain(t, session)

	result := callFindPath(t, session, map[string]any{

		"from": "frontend",

		"to": "backend",
	})

	if !result.Found {

		t.Fatal("expected path to be found")

	}

	if result.Hops != 1 {

		t.Errorf("expected 1 hop, got %d", result.Hops)

	}

	if len(result.Path) != 1 {

		t.Fatalf("expected 1 edge, got %d", len(result.Path))

	}

	edge := result.Path[0]

	if edge.From != "frontend" || edge.To != "backend" || edge.RelationType != "depends_on" {

		t.Errorf("unexpected edge: %+v", edge)

	}

}

// TestFindPath_MultiHop verifies a two-hop path is found and correctly ordered.

func TestFindPath_MultiHop(t *testing.T) {

	session := newSession(t)

	seedLinearChain(t, session)

	result := callFindPath(t, session, map[string]any{

		"from": "frontend",

		"to": "database",
	})

	if !result.Found {

		t.Fatal("expected path to be found")

	}

	if result.Hops != 2 {

		t.Errorf("expected 2 hops, got %d", result.Hops)

	}

	if len(result.Path) != 2 {

		t.Fatalf("expected 2 edges, got %d", len(result.Path))

	}

	// First edge: frontend → backend

	if result.Path[0].From != "frontend" || result.Path[0].To != "backend" {

		t.Errorf("first edge mismatch: %+v", result.Path[0])

	}

	// Second edge: backend → database

	if result.Path[1].From != "backend" || result.Path[1].To != "database" {

		t.Errorf("second edge mismatch: %+v", result.Path[1])

	}

}

// TestFindPath_ReverseDirection verifies that the path is found following edges in reverse.

func TestFindPath_ReverseDirection(t *testing.T) {

	session := newSession(t)

	seedLinearChain(t, session)

	// database has no outgoing edges, but BFS is undirected

	result := callFindPath(t, session, map[string]any{

		"from": "database",

		"to": "frontend",
	})

	if !result.Found {

		t.Fatal("expected path to be found following edges in reverse")

	}

	if result.Hops != 2 {

		t.Errorf("expected 2 hops, got %d", result.Hops)

	}

}

// TestFindPath_NoPath verifies that disconnected entities return found=false.

func TestFindPath_NoPath(t *testing.T) {

	session := newSession(t)

	seedLinearChain(t, session)

	// Create an isolated entity

	ctx := t.Context()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "isolated-svc", "entityType": "service", "observations": []string{}},
			},
		},
	})

	if err != nil {

		t.Fatalf("create_entities: %v", err)

	}

	result := callFindPath(t, session, map[string]any{

		"from": "frontend",

		"to": "isolated-svc",
	})

	if result.Found {

		t.Error("expected no path to isolated entity")

	}

	if len(result.Path) != 0 {

		t.Errorf("expected empty path, got %d edges", len(result.Path))

	}

}

// TestFindPath_SameEntity verifies that from==to returns found=true with 0 hops.

func TestFindPath_SameEntity(t *testing.T) {

	session := newSession(t)

	seedLinearChain(t, session)

	result := callFindPath(t, session, map[string]any{

		"from": "frontend",

		"to": "frontend",
	})

	if !result.Found {

		t.Error("expected found=true when from==to")

	}

	if result.Hops != 0 {

		t.Errorf("expected 0 hops, got %d", result.Hops)

	}

}

// TestFindPath_MaxDepthExceeded verifies that paths beyond max_depth return found=false.

func TestFindPath_MaxDepthExceeded(t *testing.T) {

	session := newSession(t)

	seedLinearChain(t, session)

	// frontend → backend → database is 2 hops; limiting to 1 should fail

	result := callFindPath(t, session, map[string]any{

		"from": "frontend",

		"to": "database",

		"max_depth": 1,
	})

	if result.Found {

		t.Error("expected no path when max_depth=1 for a 2-hop path")

	}

}

// TestFindPath_MissingFrom verifies that an empty 'from' parameter returns an error.

func TestFindPath_MissingFrom(t *testing.T) {

	session := newSession(t)

	ctx := t.Context()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "find_path",

		Arguments: map[string]any{"from": "", "to": "backend"},
	})

	if err == nil && !res.IsError {

		t.Error("expected error for empty 'from'")

	}

}

// TestFindPath_ToolRegistered verifies find_path appears in the tool list.

func TestFindPath_ToolRegistered(t *testing.T) {

	session := newSession(t)

	resp, err := session.ListTools(t.Context(), &mcp.ListToolsParams{})

	if err != nil {

		t.Fatalf("list_tools: %v", err)

	}

	for _, tool := range resp.Tools {

		if tool.Name == "find_path" {

			return

		}

	}

	t.Error("find_path not found in tool list")

}
