// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package trigger_scan_test

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
	"github.com/doc-scout/mcp-server/tools"
)

// TestTriggerScan_Queued verifies that a trigger_scan call returns triggered=true.

func TestTriggerScan_Queued(t *testing.T) {

	session := testutils.SetupTestServer(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{

		Name: "trigger_scan",

		Arguments: map[string]any{},
	})

	if err != nil {

		t.Fatalf("trigger_scan call failed: %v", err)

	}

	if res.IsError {

		t.Fatalf("trigger_scan returned MCP error: %v", res.Content)

	}

	if len(res.Content) == 0 {

		t.Fatal("empty response")

	}

	text, ok := res.Content[0].(*mcp.TextContent)

	if !ok {

		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])

	}

	var result tools.TriggerScanResult

	if err := json.Unmarshal([]byte(text.Text), &result); err != nil {

		t.Fatalf("unmarshal: %v", err)

	}

	// MockScanner.TriggerScan always returns true.

	if !result.Triggered {

		t.Error("expected triggered=true")

	}

	if result.AlreadyQueued {

		t.Error("expected already_queued=false on first call")

	}

}

// TestTriggerScan_ToolRegistered verifies trigger_scan appears in the tool list.

func TestTriggerScan_ToolRegistered(t *testing.T) {

	session := testutils.SetupTestServer(t)

	resp, err := session.ListTools(t.Context(), &mcp.ListToolsParams{})

	if err != nil {

		t.Fatalf("list_tools: %v", err)

	}

	for _, tool := range resp.Tools {

		if tool.Name == "trigger_scan" {

			return

		}

	}

	t.Error("trigger_scan not found in tool list")

}
