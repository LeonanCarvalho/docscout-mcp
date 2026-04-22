// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package create_relations_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_CreateRelations(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// Setup: create entities first

	_, _ = session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "api-gateway", "entityType": "Component", "observations": []string{}},

				{"name": "user-service", "entityType": "Component", "observations": []string{}},
			},
		},
	})

	// Test creating relations

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_relations",

		Arguments: map[string]any{

			"relations": []map[string]any{

				{"from": "api-gateway", "to": "user-service", "relationType": "proxies"},
			},
		},
	})

	if err != nil {

		t.Fatalf("create_relations: %v", err)

	}

	if res.IsError {

		t.Fatalf("create_relations returned error: %v", res.Content)

	}

}
