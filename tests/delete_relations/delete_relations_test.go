// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package delete_relations_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_DeleteRelations(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// Setup: create entities and relations

	_, _ = session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "A", "entityType": "Node", "observations": []string{}},

				{"name": "B", "entityType": "Node", "observations": []string{}},
			},
		},
	})

	_, _ = session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_relations",

		Arguments: map[string]any{

			"relations": []map[string]any{

				{"from": "A", "to": "B", "relationType": "calls"},
			},
		},
	})

	// Test deleting relations

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "delete_relations",

		Arguments: map[string]any{

			"relations": []map[string]any{

				{"from": "A", "to": "B", "relationType": "calls"},
			},
		},
	})

	if err != nil {

		t.Fatalf("delete_relations: %v", err)

	}

	if res.IsError {

		t.Fatalf("delete_relations returned error: %v", res.Content)

	}

}
