// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package create_entities_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_CreateEntities(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "create_entities",

		Arguments: map[string]any{

			"entities": []map[string]any{

				{"name": "api-gateway", "entityType": "Component", "observations": []string{"routes traffic"}},
			},
		},
	})

	if err != nil {

		t.Fatalf("create_entities: %v", err)

	}

	if res.IsError {

		t.Fatalf("create_entities returned error: %v", res.Content)

	}

}
