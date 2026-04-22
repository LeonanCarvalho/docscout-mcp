// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package list_tools_test

import (
	"testing"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_ListTools(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	// 3 scanner tools + 9 memory tools + 1 get_scan_status + 1 traverse_graph + 1 get_usage_stats = 15
	// search_content is not registered because cache is nil
	if len(result.Tools) < 15 {
		t.Fatalf("expected at least 15 tools, got %d", len(result.Tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	expected := []string{
		"list_repos", "search_docs", "get_file_content", "get_scan_status",
		"create_entities", "create_relations", "add_observations",
		"delete_entities", "delete_observations", "delete_relations",
		"read_graph", "search_nodes", "open_nodes",
		"traverse_graph", "get_usage_stats",
	}
	for _, name := range expected {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}
