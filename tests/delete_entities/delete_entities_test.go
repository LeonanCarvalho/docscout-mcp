// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package delete_entities_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_DeleteEntities(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// Setup: create entity

	_, _ = session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "obsolete-service", "entityType": "Service", "observations": []string{}},
			},
		},
	})

	// Test deleting below threshold — should succeed.

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "delete_entities",

		Arguments: map[string]any{"entityNames": []string{"obsolete-service"}},
	})

	if err != nil {

		t.Fatalf("delete_entities: %v", err)

	}

	if res.IsError {

		t.Fatalf("delete_entities returned error: %v", res.Content)

	}

}

func TestE2E_DeleteEntities_MassDeleteGuard(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// Build a list that exceeds the mass-delete threshold (11 entities).

	names := make([]string, 11)

	for i := range names {

		names[i] = "entity-" + string(rune('a'+i))

	}

	// Without confirm=true, the guard should refuse.

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "delete_entities",

		Arguments: map[string]any{"entityNames": names},
	})

	if err != nil {

		t.Fatalf("unexpected transport error: %v", err)

	}

	if !res.IsError {

		t.Fatal("expected mass-delete guard to return an error, but got success")

	}

	// With confirm=true, the guard passes (actual delete is a no-op since entities don't exist).

	res2, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "delete_entities",

		Arguments: map[string]any{"entityNames": names, "confirm": true},
	})

	if err != nil {

		t.Fatalf("unexpected transport error: %v", err)

	}

	if res2.IsError {

		t.Fatalf("expected success with confirm=true, got error: %v", res2.Content)

	}

}
