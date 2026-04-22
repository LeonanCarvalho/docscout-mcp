// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package open_nodes_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_OpenNodes(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// Setup: create entity

	_, _ = session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "payment-api", "entityType": "Service", "observations": []string{}},
			},
		},
	})

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "open_nodes",

		Arguments: map[string]any{"names": []string{"payment-api"}},
	})

	if err != nil {

		t.Fatalf("open_nodes: %v", err)

	}

	if res.IsError {

		t.Fatalf("open_nodes returned error: %v", res.Content)

	}

}
