// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package get_scan_status_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_ScanStatus(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_scan_status"})

	if err != nil {

		t.Fatalf("CallTool get_scan_status: %v", err)

	}

	if result.IsError {

		t.Fatalf("get_scan_status returned error: %v", result.Content)

	}

	if len(result.Content) == 0 {

		t.Fatal("expected content from get_scan_status")

	}

}
