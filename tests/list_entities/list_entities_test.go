// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package list_entities_test

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

// decodeKGResponse extracts entities from an MCP tool JSON response.

func decodeKGResponse(t *testing.T, result *mcp.CallToolResult) []map[string]any {

	t.Helper()

	if len(result.Content) == 0 {

		return nil

	}

	raw, _ := json.Marshal(result.Content[0])

	var tc struct {
		Text string `json:"text"`
	}

	_ = json.Unmarshal(raw, &tc)

	var kg struct {
		Entities []map[string]any `json:"entities"`
	}

	if err := json.Unmarshal([]byte(tc.Text), &kg); err != nil {

		t.Fatalf("decode KG response: %v (body=%s)", err, tc.Text)

	}

	return kg.Entities

}

func TestE2E_ListEntities_Empty(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// No entities seeded — expect empty list, no error.

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_entities"})

	if err != nil {

		t.Fatalf("list_entities: %v", err)

	}

	if result.IsError {

		t.Fatalf("list_entities returned error: %v", result.Content)

	}

}

func TestE2E_ListEntities_AllEntities(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// Seed two entities of different types.

	_, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "payment-svc", "entityType": "service", "observations": []string{}},

				{"name": "platform-team", "entityType": "team", "observations": []string{}},
			},
		},
	})

	if err != nil {

		t.Fatalf("create_entities: %v", err)

	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_entities"})

	if err != nil {

		t.Fatalf("list_entities: %v", err)

	}

	if result.IsError {

		t.Fatalf("list_entities returned error: %v", result.Content)

	}

	entities := decodeKGResponse(t, result)

	if len(entities) != 2 {

		t.Fatalf("expected 2 entities, got %d", len(entities))

	}

}

func TestE2E_ListEntities_FilterByType(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// Seed a service and a team.

	_, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "order-svc", "entityType": "service", "observations": []string{}},

				{"name": "backend-team", "entityType": "team", "observations": []string{}},
			},
		},
	})

	if err != nil {

		t.Fatalf("create_entities: %v", err)

	}

	// Filter to services only.

	result, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "list_entities",

		Arguments: map[string]any{"entity_type": "service"},
	})

	if err != nil {

		t.Fatalf("list_entities: %v", err)

	}

	if result.IsError {

		t.Fatalf("list_entities returned error: %v", result.Content)

	}

	entities := decodeKGResponse(t, result)

	if len(entities) != 1 {

		t.Fatalf("expected 1 service entity, got %d", len(entities))

	}

	if entities[0]["name"] != "order-svc" {

		t.Errorf("expected order-svc, got %v", entities[0]["name"])

	}

}

func TestE2E_ListEntities_FilterNoMatch(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// Seed a service; filter for event-topic should return nothing.

	_, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "inventory-svc", "entityType": "service", "observations": []string{}},
			},
		},
	})

	if err != nil {

		t.Fatalf("create_entities: %v", err)

	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "list_entities",

		Arguments: map[string]any{"entity_type": "event-topic"},
	})

	if err != nil {

		t.Fatalf("list_entities: %v", err)

	}

	if result.IsError {

		t.Fatalf("list_entities returned error: %v", result.Content)

	}

	entities := decodeKGResponse(t, result)

	if len(entities) != 0 {

		t.Fatalf("expected 0 event-topic entities, got %d", len(entities))

	}

}
