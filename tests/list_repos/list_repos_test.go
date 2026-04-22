// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package list_repos_test

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
)

func TestE2E_ListRepos(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_repos"})

	if err != nil {

		t.Fatalf("CallTool list_repos: %v", err)

	}

	if result.IsError {

		t.Fatalf("list_repos returned error")

	}

	if len(result.Content) == 0 {

		t.Fatal("expected content from list_repos")

	}

}

func TestE2E_ListRepos_FileTypeFilter_Match(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// The MockScanner exposes a "readme" file — filter should return 1 repo.

	result, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "list_repos",

		Arguments: map[string]any{"file_type": "readme"},
	})

	if err != nil {

		t.Fatalf("CallTool list_repos: %v", err)

	}

	if result.IsError {

		t.Fatalf("list_repos returned error")

	}

	var resp struct {
		Repos []struct {
			Name string `json:"name"`

			FileTypes []string `json:"file_types"`
		} `json:"repos"`
	}

	raw, _ := json.Marshal(result.Content[0])

	// Content[0] is a TextContent with a JSON string body

	var tc struct {
		Text string `json:"text"`
	}

	_ = json.Unmarshal(raw, &tc)

	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {

		t.Fatalf("decode response: %v", err)

	}

	if len(resp.Repos) != 1 {

		t.Fatalf("expected 1 repo for file_type=readme, got %d", len(resp.Repos))

	}

}

func TestE2E_ListRepos_FileTypeFilter_NoMatch(t *testing.T) {

	session := testutils.SetupTestServer(t)

	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()

	// The MockScanner has no "openapi" files — filter should return empty list.

	result, err := session.CallTool(ctx, &mcp.CallToolParams{

		Name: "list_repos",

		Arguments: map[string]any{"file_type": "openapi"},
	})

	if err != nil {

		t.Fatalf("CallTool list_repos: %v", err)

	}

	if result.IsError {

		t.Fatalf("list_repos returned error")

	}

	var resp struct {
		Repos []any `json:"repos"`
	}

	raw, _ := json.Marshal(result.Content[0])

	var tc struct {
		Text string `json:"text"`
	}

	_ = json.Unmarshal(raw, &tc)

	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {

		t.Fatalf("decode response: %v", err)

	}

	if len(resp.Repos) != 0 {

		t.Fatalf("expected 0 repos for file_type=openapi, got %d", len(resp.Repos))

	}

}
