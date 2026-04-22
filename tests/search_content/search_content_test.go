// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package search_content_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/tests/testutils"
	"github.com/doc-scout/mcp-server/tools"
)

// setupContentServer creates an MCP test session with a pre-populated ContentCache.

func setupContentServer(t *testing.T) *mcp.ClientSession {

	t.Helper()

	ctx := t.Context()

	server := mcp.NewServer(&mcp.Implementation{

		Name: "docscout-mcp-test",

		Version: "test",
	}, nil)

	dsn := fmt.Sprintf("file:search_content_e2e_%d?mode=memory&cache=shared", testCounter.Add(1))

	db, err := memory.OpenDB(dsn)

	if err != nil {

		t.Fatalf("memory.OpenDB: %v", err)

	}

	memorySrv := memory.NewMemoryService(db)

	contentCache := memory.NewContentCache(db, true, 1024*1024)

	// Pre-populate the content cache with test fixtures.

	fixtures := []struct {
		repo, path, sha, content, fileType string
	}{

		{"org/payment-svc", "README.md", "sha1",

			"# Payment Service\nHandles Stripe transactions, refunds, and subscription billing.", "readme"},

		{"org/auth-svc", "README.md", "sha2",

			"# Auth Service\nManages JWT tokens, OAuth2 flows, and session handling.", "readme"},

		{"org/payment-svc", "openapi.yaml", "sha3",

			"openapi: 3.0.0\ninfo:\n  title: Payment API\npaths:\n  /charge:\n    post:\n      summary: Charge a card", "openapi"},

		{"org/notification-svc", "README.md", "sha4",

			"# Notification Service\nSends emails and SMS alerts for payment events.", "readme"},
	}

	for _, f := range fixtures {

		if err := contentCache.Upsert(f.repo, f.path, f.sha, f.content, f.fileType); err != nil {

			t.Fatalf("Upsert fixture: %v", err)

		}

	}

	tools.Register(server, &testutils.MockScanner{}, memorySrv, contentCache, nil, tools.NewToolMetrics(), tools.NewDocMetrics(), false)

	t1, t2 := mcp.NewInMemoryTransports()

	if _, err := server.Connect(ctx, t1, nil); err != nil {

		t.Fatalf("server connect: %v", err)

	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)

	session, err := client.Connect(ctx, t2, nil)

	if err != nil {

		t.Fatalf("client connect: %v", err)

	}

	return session

}

// callSearchContent calls the search_content tool and returns parsed matches.

func callSearchContent(t *testing.T, session *mcp.ClientSession, args map[string]any) ([]map[string]any, bool) {

	t.Helper()

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{

		Name: "search_content",

		Arguments: args,
	})

	if err != nil {

		return nil, true // error

	}

	if res.IsError {

		return nil, true

	}

	text, ok := res.Content[0].(*mcp.TextContent)

	if !ok {

		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])

	}

	var out struct {
		Matches []map[string]any `json:"matches"`
	}

	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {

		t.Fatalf("unmarshal: %v — raw: %s", err, text.Text)

	}

	return out.Matches, false

}

var testCounter testCounterType

type testCounterType struct{ n int }

func (c *testCounterType) Add(d int) int {

	c.n += d

	return c.n

}

// TestSearchContent_BasicMatch verifies that a simple keyword search returns relevant results.

func TestSearchContent_BasicMatch(t *testing.T) {

	session := setupContentServer(t)

	matches, isErr := callSearchContent(t, session, map[string]any{"query": "stripe"})

	if isErr {

		t.Fatal("search_content returned error")

	}

	if len(matches) == 0 {

		t.Fatal("expected at least one match for 'stripe'")

	}

	// Only the payment README mentions Stripe.

	found := false

	for _, m := range matches {

		if m["repo_name"] == "org/payment-svc" {

			found = true

		}

	}

	if !found {

		t.Errorf("expected org/payment-svc in results, got: %v", matches)

	}

}

// TestSearchContent_FilterByRepo verifies the repo filter narrows results correctly.

func TestSearchContent_FilterByRepo(t *testing.T) {

	session := setupContentServer(t)

	matches, isErr := callSearchContent(t, session, map[string]any{

		"query": "payment",

		"repo": "org/notification-svc",
	})

	if isErr {

		t.Fatal("search_content returned error")

	}

	if len(matches) != 1 {

		t.Fatalf("expected 1 match from notification-svc, got %d", len(matches))

	}

	if matches[0]["repo_name"] != "org/notification-svc" {

		t.Errorf("wrong repo: %v", matches[0]["repo_name"])

	}

}

// TestSearchContent_SnippetPresent verifies each match has a non-empty snippet.

func TestSearchContent_SnippetPresent(t *testing.T) {

	session := setupContentServer(t)

	matches, isErr := callSearchContent(t, session, map[string]any{"query": "JWT"})

	if isErr {

		t.Fatal("search_content returned error")

	}

	if len(matches) == 0 {

		t.Fatal("expected at least one match for 'JWT'")

	}

	for _, m := range matches {

		if m["snippet"] == "" || m["snippet"] == nil {

			t.Errorf("match %v has empty snippet", m["repo_name"])

		}

	}

}

// TestSearchContent_EmptyQuery verifies that an empty query returns an error.

func TestSearchContent_EmptyQuery(t *testing.T) {

	session := setupContentServer(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{

		Name: "search_content",

		Arguments: map[string]any{"query": ""},
	})

	if err == nil && !res.IsError {

		t.Error("expected error for empty query")

	}

}

// TestSearchContent_WhitespaceQuery verifies that a whitespace-only query returns an error.

func TestSearchContent_WhitespaceQuery(t *testing.T) {

	session := setupContentServer(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{

		Name: "search_content",

		Arguments: map[string]any{"query": "   "},
	})

	if err == nil && !res.IsError {

		t.Error("expected error for whitespace-only query")

	}

}

// TestSearchContent_NoResults verifies that a query with no matches returns an empty list.

func TestSearchContent_NoResults(t *testing.T) {

	session := setupContentServer(t)

	matches, isErr := callSearchContent(t, session, map[string]any{"query": "xyznonexistentterm"})

	if isErr {

		t.Fatal("unexpected error for no-results query")

	}

	if len(matches) != 0 {

		t.Errorf("expected 0 matches, got %d", len(matches))

	}

}

// TestSearchContent_ToolRegistered verifies search_content appears in the tool list.

func TestSearchContent_ToolRegistered(t *testing.T) {

	session := setupContentServer(t)

	resp, err := session.ListTools(t.Context(), &mcp.ListToolsParams{})

	if err != nil {

		t.Fatalf("list_tools: %v", err)

	}

	for _, tool := range resp.Tools {

		if tool.Name == "search_content" {

			return

		}

	}

	t.Error("search_content not found in tool list")

}
