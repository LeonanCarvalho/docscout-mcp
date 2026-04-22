// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package traverse_graph_test

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/tests/testutils"
	"github.com/doc-scout/mcp-server/tools"
)

// callTraverse is a helper that calls traverse_graph and unmarshals the result.
func callTraverse(t *testing.T, session *mcp.ClientSession, args map[string]any) tools.TraverseGraphResult {
	t.Helper()
	ctx := t.Context()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "traverse_graph",
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("traverse_graph call failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("traverse_graph returned MCP error: %v", res.Content)
	}

	// The SDK encodes the result struct as a text content block.
	if len(res.Content) == 0 {
		t.Fatal("traverse_graph: empty response content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])
	}

	var result tools.TraverseGraphResult
	if err := json.Unmarshal([]byte(text.Text), &result); err != nil {
		t.Fatalf("unmarshal TraverseGraphResult: %v — raw: %s", err, text.Text)
	}
	return result
}

// setupGraph creates entities A→B→C (depends_on) and A→D (owns).
func setupGraph(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	ctx := t.Context()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_entities",
		Arguments: map[string]any{
			"entities": []map[string]any{
				{"name": "svc-a", "entityType": "service", "observations": []string{"root node"}},
				{"name": "svc-b", "entityType": "service", "observations": []string{"middle node"}},
				{"name": "svc-c", "entityType": "service", "observations": []string{"leaf node"}},
				{"name": "team-x", "entityType": "team", "observations": []string{"owner"}},
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
				{"from": "svc-a", "to": "svc-b", "relationType": "depends_on"},
				{"from": "svc-b", "to": "svc-c", "relationType": "depends_on"},
				{"from": "team-x", "to": "svc-a", "relationType": "owns"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create_relations: %v", err)
	}
}

func TestE2E_TraverseGraph_OutgoingDepth1(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })
	setupGraph(t, session)

	result := callTraverse(t, session, map[string]any{
		"entity":        "svc-a",
		"relation_type": "depends_on",
		"direction":     "outgoing",
		"depth":         1,
	})

	if result.TotalFound != 1 {
		t.Errorf("depth=1: expected 1 node, got %d: %+v", result.TotalFound, result.Nodes)
	}
	if result.Nodes[0].Name != "svc-b" {
		t.Errorf("depth=1: expected svc-b, got %q", result.Nodes[0].Name)
	}
	if result.Nodes[0].Distance != 1 {
		t.Errorf("depth=1: expected distance 1, got %d", result.Nodes[0].Distance)
	}
}

func TestE2E_TraverseGraph_OutgoingDepth2(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })
	setupGraph(t, session)

	result := callTraverse(t, session, map[string]any{
		"entity":        "svc-a",
		"relation_type": "depends_on",
		"direction":     "outgoing",
		"depth":         2,
	})

	if result.TotalFound != 2 {
		t.Errorf("depth=2: expected 2 nodes, got %d: %+v", result.TotalFound, result.Nodes)
	}

	byName := make(map[string]memory.TraverseNode)
	for _, n := range result.Nodes {
		byName[n.Name] = n
	}

	if b, ok := byName["svc-b"]; !ok || b.Distance != 1 {
		t.Errorf("expected svc-b at distance 1, got %+v", byName["svc-b"])
	}
	if c, ok := byName["svc-c"]; !ok || c.Distance != 2 {
		t.Errorf("expected svc-c at distance 2, got %+v", byName["svc-c"])
	}
}

func TestE2E_TraverseGraph_Incoming(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })
	setupGraph(t, session)

	result := callTraverse(t, session, map[string]any{
		"entity":    "svc-a",
		"direction": "incoming",
		"depth":     1,
	})

	if result.TotalFound != 1 {
		t.Errorf("incoming: expected 1 node (team-x), got %d: %+v", result.TotalFound, result.Nodes)
	}
	if result.Nodes[0].Name != "team-x" {
		t.Errorf("incoming: expected team-x, got %q", result.Nodes[0].Name)
	}
}

func TestE2E_TraverseGraph_RelationTypeFilter(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })
	setupGraph(t, session)

	// svc-a has outgoing depends_on (→svc-b) only; team-x has outgoing owns (→svc-a).
	// Traversing from team-x with relation_type=owns should return svc-a only.
	result := callTraverse(t, session, map[string]any{
		"entity":        "team-x",
		"relation_type": "owns",
		"direction":     "outgoing",
		"depth":         1,
	})

	if result.TotalFound != 1 {
		t.Errorf("filter: expected 1 node, got %d: %+v", result.TotalFound, result.Nodes)
	}
	if result.Nodes[0].Name != "svc-a" {
		t.Errorf("filter: expected svc-a, got %q", result.Nodes[0].Name)
	}
}

func TestE2E_TraverseGraph_UnknownEntityReturnsEmpty(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })

	result := callTraverse(t, session, map[string]any{
		"entity":    "does-not-exist",
		"direction": "outgoing",
		"depth":     1,
	})

	if result.TotalFound != 0 {
		t.Errorf("unknown entity: expected 0 nodes, got %d", result.TotalFound)
	}
}

func TestE2E_TraverseGraph_CycleDoesNotLoop(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })
	ctx := t.Context()

	// Build a cycle: A → B → A
	_, _ = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_entities",
		Arguments: map[string]any{
			"entities": []map[string]any{
				{"name": "cycle-a", "entityType": "service", "observations": []string{"cycle"}},
				{"name": "cycle-b", "entityType": "service", "observations": []string{"cycle"}},
			},
		},
	})
	_, _ = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_relations",
		Arguments: map[string]any{
			"relations": []map[string]any{
				{"from": "cycle-a", "to": "cycle-b", "relationType": "depends_on"},
				{"from": "cycle-b", "to": "cycle-a", "relationType": "depends_on"},
			},
		},
	})

	result := callTraverse(t, session, map[string]any{
		"entity":    "cycle-a",
		"direction": "outgoing",
		"depth":     10,
	})

	// Should find only cycle-b (cycle-a is the start, already visited)
	if result.TotalFound != 1 {
		t.Errorf("cycle: expected 1 unique node, got %d: %+v", result.TotalFound, result.Nodes)
	}
}

func TestE2E_TraverseGraph_InvalidEntityError(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })
	ctx := t.Context()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "traverse_graph",
		Arguments: map[string]any{"entity": "   "},
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !res.IsError {
		t.Error("expected MCP error for empty entity, got success")
	}
}

func TestE2E_TraverseGraph_InvalidDirectionError(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })
	ctx := t.Context()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "traverse_graph",
		Arguments: map[string]any{
			"entity":    "svc-a",
			"direction": "sideways",
		},
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !res.IsError {
		t.Error("expected MCP error for invalid direction, got success")
	}
}
