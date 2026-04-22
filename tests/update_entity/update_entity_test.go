// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package update_entity_test

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

func newSession(t *testing.T) (*mcp.ClientSession, *memory.MemoryService) {

	t.Helper()

	ctx := t.Context()

	server := mcp.NewServer(&mcp.Implementation{Name: "docscout-mcp-test", Version: "test"}, nil)

	dsn := fmt.Sprintf("file:memdb_updateentity_%d?mode=memory&cache=shared", testCounter.Add(1))

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

	return session, memorySrv

}

// callUpdate calls update_entity and returns the parsed result.

func callUpdate(t *testing.T, session *mcp.ClientSession, args map[string]any) tools.UpdateEntityResult {

	t.Helper()

	ctx := t.Context()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "update_entity",

		Arguments: args,
	})

	if err != nil {

		t.Fatalf("update_entity call failed: %v", err)

	}

	if res.IsError {

		t.Fatalf("update_entity returned MCP error: %v", res.Content)

	}

	text, ok := res.Content[0].(*mcp.TextContent)

	if !ok {

		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])

	}

	var result tools.UpdateEntityResult

	if err := json.Unmarshal([]byte(text.Text), &result); err != nil {

		t.Fatalf("unmarshal UpdateEntityResult: %v", err)

	}

	return result

}

// seedEntity creates a single entity with an observation and a relation.

func seedEntity(t *testing.T, session *mcp.ClientSession) {

	t.Helper()

	ctx := t.Context()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "old-svc", "entityType": "service", "observations": []string{"handles payments"}},

				{"name": "dep-svc", "entityType": "service", "observations": []string{"dependency"}},
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

				{"from": "old-svc", "to": "dep-svc", "relationType": "depends_on"},
			},
		},
	})

	if err != nil {

		t.Fatalf("create_relations: %v", err)

	}

}

// TestUpdateEntity_Rename verifies that renaming updates the entity name.

func TestUpdateEntity_Rename(t *testing.T) {

	session, memorySrv := newSession(t)

	seedEntity(t, session)

	result := callUpdate(t, session, map[string]any{

		"name": "old-svc",

		"new_name": "new-svc",
	})

	if !result.Updated {

		t.Fatal("expected updated=true")

	}

	if result.Name != "new-svc" {

		t.Errorf("expected name=new-svc, got %s", result.Name)

	}

	// Verify entity was renamed in the graph.

	graph, err := memorySrv.ReadGraph()

	if err != nil {

		t.Fatalf("ReadGraph: %v", err)

	}

	found := false

	for _, e := range graph.Entities {

		if e.Name == "new-svc" {

			found = true

		}

		if e.Name == "old-svc" {

			t.Error("old-svc should no longer exist")

		}

	}

	if !found {

		t.Error("new-svc not found in graph")

	}

}

// TestUpdateEntity_RelationsCascade verifies relations are updated when renaming.

func TestUpdateEntity_RelationsCascade(t *testing.T) {

	session, memorySrv := newSession(t)

	seedEntity(t, session)

	callUpdate(t, session, map[string]any{

		"name": "old-svc",

		"new_name": "new-svc",
	})

	graph, err := memorySrv.ReadGraph()

	if err != nil {

		t.Fatalf("ReadGraph: %v", err)

	}

	// The relation from "old-svc" to "dep-svc" should now be "new-svc" → "dep-svc".

	found := false

	for _, r := range graph.Relations {

		if r.From == "new-svc" && r.To == "dep-svc" {

			found = true

		}

		if r.From == "old-svc" {

			t.Errorf("relation still references old-svc: %+v", r)

		}

	}

	if !found {

		t.Error("expected relation new-svc → dep-svc after rename")

	}

}

// TestUpdateEntity_ChangeType verifies changing entity type without renaming.

func TestUpdateEntity_ChangeType(t *testing.T) {

	session, memorySrv := newSession(t)

	seedEntity(t, session)

	result := callUpdate(t, session, map[string]any{

		"name": "old-svc",

		"new_type": "api",
	})

	if !result.Updated {

		t.Fatal("expected updated=true")

	}

	if result.Name != "old-svc" {

		t.Errorf("name should be unchanged, got %s", result.Name)

	}

	graph, err := memorySrv.ReadGraph()

	if err != nil {

		t.Fatalf("ReadGraph: %v", err)

	}

	for _, e := range graph.Entities {

		if e.Name == "old-svc" && e.EntityType != "api" {

			t.Errorf("expected entityType=api, got %s", e.EntityType)

		}

	}

}

// TestUpdateEntity_RenameAndChangeType verifies both operations together.

func TestUpdateEntity_RenameAndChangeType(t *testing.T) {

	session, memorySrv := newSession(t)

	seedEntity(t, session)

	callUpdate(t, session, map[string]any{

		"name": "old-svc",

		"new_name": "renamed-svc",

		"new_type": "team",
	})

	graph, err := memorySrv.ReadGraph()

	if err != nil {

		t.Fatalf("ReadGraph: %v", err)

	}

	for _, e := range graph.Entities {

		if e.Name == "renamed-svc" {

			if e.EntityType != "team" {

				t.Errorf("expected entityType=team, got %s", e.EntityType)

			}

			return

		}

	}

	t.Error("renamed-svc not found after update")

}

// TestUpdateEntity_NotFound verifies an error is returned for non-existent entities.

func TestUpdateEntity_NotFound(t *testing.T) {

	session, _ := newSession(t)

	ctx := t.Context()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "update_entity",

		Arguments: map[string]any{"name": "nonexistent", "new_name": "whatever"},
	})

	if err == nil && !res.IsError {

		t.Error("expected error for non-existent entity")

	}

}

// TestUpdateEntity_DuplicateName verifies an error is returned when new_name is already taken.

func TestUpdateEntity_DuplicateName(t *testing.T) {

	session, _ := newSession(t)

	seedEntity(t, session)

	ctx := t.Context()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "update_entity",

		Arguments: map[string]any{"name": "old-svc", "new_name": "dep-svc"},
	})

	if err == nil && !res.IsError {

		t.Error("expected error when renaming to an existing entity name")

	}

}

// TestUpdateEntity_MissingFields verifies validation errors.

func TestUpdateEntity_MissingFields(t *testing.T) {

	session, _ := newSession(t)

	ctx := t.Context()

	// Missing name

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "update_entity",

		Arguments: map[string]any{"name": "", "new_name": "x"},
	})

	if err == nil && !res.IsError {

		t.Error("expected error for empty name")

	}

	// Missing new_name and new_type

	res2, err2 := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "update_entity",

		Arguments: map[string]any{"name": "old-svc"},
	})

	if err2 == nil && !res2.IsError {

		t.Error("expected error when neither new_name nor new_type is provided")

	}

}

// TestUpdateEntity_ToolRegistered verifies update_entity appears in the tool list.

func TestUpdateEntity_ToolRegistered(t *testing.T) {

	session, _ := newSession(t)

	resp, err := session.ListTools(t.Context(), &mcp.ListToolsParams{})

	if err != nil {

		t.Fatalf("list_tools: %v", err)

	}

	for _, tool := range resp.Tools {

		if tool.Name == "update_entity" {

			return

		}

	}

	t.Error("update_entity not found in tool list")

}
