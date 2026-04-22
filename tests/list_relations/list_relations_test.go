// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package list_relations_test

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

type listRelationsResp struct {
	Relations []struct {
		From string `json:"from"`

		To string `json:"to"`

		RelationType string `json:"relationType"`
	} `json:"relations"`

	Count int `json:"count"`
}

func decodeRelations(t *testing.T, result *mcp.CallToolResult) listRelationsResp {

	t.Helper()

	if len(result.Content) == 0 {

		return listRelationsResp{}

	}

	raw, _ := json.Marshal(result.Content[0])

	var tc struct {
		Text string `json:"text"`
	}

	_ = json.Unmarshal(raw, &tc)

	var resp listRelationsResp

	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {

		t.Fatalf("decode relations: %v (body=%s)", err, tc.Text)

	}

	return resp

}

func seedGraph(t *testing.T, session *mcp.ClientSession) {

	t.Helper()

	ctx := t.Context()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "order-svc", "entityType": "service", "observations": []string{}},

				{"name": "payment-svc", "entityType": "service", "observations": []string{}},

				{"name": "order-placed", "entityType": "event-topic", "observations": []string{}},
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

				{"from": "order-svc", "to": "payment-svc", "relationType": "calls_service"},

				{"from": "order-svc", "to": "order-placed", "relationType": "publishes_event"},

				{"from": "payment-svc", "to": "order-placed", "relationType": "subscribes_event"},
			},
		},
	})

	if err != nil {

		t.Fatalf("create_relations: %v", err)

	}

}

func TestE2E_ListRelations_FilterByType(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	seedGraph(t, session)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "list_relations",

		Arguments: map[string]any{"relation_type": "calls_service"},
	})

	if err != nil {

		t.Fatalf("list_relations: %v", err)

	}

	if result.IsError {

		t.Fatalf("list_relations returned error: %v", result.Content)

	}

	resp := decodeRelations(t, result)

	if resp.Count != 1 {

		t.Fatalf("expected 1 calls_service relation, got %d", resp.Count)

	}

	if resp.Relations[0].From != "order-svc" || resp.Relations[0].To != "payment-svc" {

		t.Errorf("unexpected relation: %+v", resp.Relations[0])

	}

}

func TestE2E_ListRelations_FilterByFromEntity(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	seedGraph(t, session)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "list_relations",

		Arguments: map[string]any{"from_entity": "order-svc"},
	})

	if err != nil {

		t.Fatalf("list_relations: %v", err)

	}

	if result.IsError {

		t.Fatalf("list_relations returned error: %v", result.Content)

	}

	resp := decodeRelations(t, result)

	if resp.Count != 2 {

		t.Fatalf("expected 2 relations from order-svc, got %d", resp.Count)

	}

}

func TestE2E_ListRelations_CombinedFilter(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	seedGraph(t, session)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "list_relations",

		Arguments: map[string]any{

			"relation_type": "publishes_event",

			"from_entity": "order-svc",
		},
	})

	if err != nil {

		t.Fatalf("list_relations: %v", err)

	}

	if result.IsError {

		t.Fatalf("list_relations returned error: %v", result.Content)

	}

	resp := decodeRelations(t, result)

	if resp.Count != 1 {

		t.Fatalf("expected 1 relation, got %d", resp.Count)

	}

	if resp.Relations[0].To != "order-placed" {

		t.Errorf("expected to=order-placed, got %s", resp.Relations[0].To)

	}

}

func TestE2E_ListRelations_NoFilter_ReturnsAll(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	seedGraph(t, session)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "list_relations",
	})

	if err != nil {

		t.Fatalf("list_relations: %v", err)

	}

	if result.IsError {

		t.Fatalf("list_relations returned error: %v", result.Content)

	}

	resp := decodeRelations(t, result)

	if resp.Count != 3 {

		t.Fatalf("expected 3 relations with no filter, got %d", resp.Count)

	}

}
