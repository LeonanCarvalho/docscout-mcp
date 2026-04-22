// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package get_file_content_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_GetFileContent(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "get_file_content",

		Arguments: map[string]any{"repo": "test-repo", "path": "README.md"},
	})

	if err != nil {

		t.Fatalf("CallTool get_file_content: %v", err)

	}

	if result.IsError {

		t.Fatalf("get_file_content returned error")

	}

	text := result.Content[0].(*mcp.TextContent).Text

	if text == "" {

		t.Fatal("expected non-empty file content")

	}

}
