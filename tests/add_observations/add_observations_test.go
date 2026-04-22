// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package add_observations_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_AddObservations(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// Setup: create entity

	_, _ = session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "backend-api", "entityType": "Service", "observations": []string{}},
			},
		},
	})

	// Test adding observations

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "add_observations",

		Arguments: map[string]any{

			"observations": []map[string]any{

				{"entityName": "backend-api", "contents": []string{"uses postgresql", "handles auth"}},
			},
		},
	})

	if err != nil {

		t.Fatalf("add_observations: %v", err)

	}

	if res.IsError {

		t.Fatalf("add_observations returned error: %v", res.Content)

	}

}
