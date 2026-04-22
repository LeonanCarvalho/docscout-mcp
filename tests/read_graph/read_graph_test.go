// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package read_graph_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_ReadGraph(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// Add data

	_, _ = session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "test-node", "entityType": "Testing", "observations": []string{"unit testing"}},
			},
		},
	})

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "read_graph"})

	if err != nil {

		t.Fatalf("read_graph: %v", err)

	}

	if res.IsError {

		t.Fatalf("read_graph returned error: %v", res.Content)

	}

}
