// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package delete_observations_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_DeleteObservations(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// Setup: create entity with observations

	_, _ = session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "noisy-service", "entityType": "Service", "observations": []string{"bad log", "good log"}},
			},
		},
	})

	// Test deleting specific observation

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "delete_observations",

		Arguments: map[string]any{

			"deletions": []map[string]any{

				{"entityName": "noisy-service", "contents": []string{}, "observations": []string{"bad log"}},
			},
		},
	})

	if err != nil {

		t.Fatalf("delete_observations: %v", err)

	}

	if res.IsError {

		t.Fatalf("delete_observations returned error: %v", res.Content)

	}

}
