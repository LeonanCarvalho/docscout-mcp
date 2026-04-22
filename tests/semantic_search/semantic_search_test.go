// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package semantic_search_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/tests/testutils"
	"github.com/doc-scout/mcp-server/tools"
)

var testCounter atomic.Int64

// keywordProvider returns vectors based on keyword presence (3 dims: payment, auth, notification).

type keywordProvider struct{}

func (k *keywordProvider) ModelKey() string { return "mock:keyword" }

func (k *keywordProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {

	out := make([][]float32, len(texts))

	for i, t := range texts {

		lower := strings.ToLower(t)

		v := []float32{0.1, 0.1, 0.1}

		if strings.Contains(lower, "payment") || strings.Contains(lower, "stripe") {

			v[0] = 0.9

		}

		if strings.Contains(lower, "auth") || strings.Contains(lower, "jwt") || strings.Contains(lower, "token") {

			v[1] = 0.9

		}

		if strings.Contains(lower, "notification") || strings.Contains(lower, "email") {

			v[2] = 0.9

		}

		out[i] = v

	}

	return out, nil

}

func setupServer(t *testing.T) *mcp.ClientSession {

	t.Helper()

	ctx := t.Context()

	dsn := fmt.Sprintf("file:semantic_e2e_%d?mode=memory&cache=shared", testCounter.Add(1))

	db, err := memory.OpenDB(dsn)

	if err != nil {

		t.Fatalf("OpenDB: %v", err)

	}

	memorySrv := memory.NewMemoryService(db)

	cc := memory.NewContentCache(db, true, 1024*1024)

	// Pre-populate docs

	cc.Upsert("org/payment-svc", "README.md", "sha1", "Payment service handles Stripe transactions and refunds.", "readme")

	cc.Upsert("org/auth-svc", "README.md", "sha2", "Auth service manages JWT tokens and OAuth2 flows.", "readme")

	cc.Upsert("org/notification-svc", "README.md", "sha3", "Notification service sends emails and SMS alerts.", "readme")

	// Pre-populate entities

	memorySrv.CreateEntities([]memory.Entity{

		{Name: "payment-svc", EntityType: "service", Observations: []string{"lang:go", "handles stripe payments"}},

		{Name: "auth-svc", EntityType: "service", Observations: []string{"lang:go", "manages jwt tokens"}},
	})

	// Build semantic service

	provider := &keywordProvider{}

	store, err := embeddings.NewVectorStore(db)

	if err != nil {

		t.Fatalf("NewVectorStore: %v", err)

	}

	indexer := embeddings.NewIndexer(provider, store, cc, memorySrv)

	searcher := embeddings.NewSemanticSearcher(provider, store, indexer, cc, memorySrv)

	// Index everything

	indexer.IndexDocs(ctx, "org/payment-svc")

	indexer.IndexDocs(ctx, "org/auth-svc")

	indexer.IndexDocs(ctx, "org/notification-svc")

	indexer.IndexEntities(ctx, []string{"payment-svc", "auth-svc"})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v1"}, nil)

	tools.Register(server, &testutils.MockScanner{}, memorySrv, cc, searcher, tools.NewToolMetrics(), tools.NewDocMetrics(), false)

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

func callSemantic(t *testing.T, session *mcp.ClientSession, args map[string]any) map[string]any {

	t.Helper()

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{

		Name: "semantic_search",

		Arguments: args,
	})

	if err != nil {

		t.Fatalf("CallTool: %v", err)

	}

	if res.IsError {

		t.Fatalf("tool returned error: %v", res.Content)

	}

	var out map[string]any

	raw, _ := json.Marshal(res.Content[0])

	var wrapper struct {
		Text string `json:"text"`
	}

	json.Unmarshal(raw, &wrapper)

	json.Unmarshal([]byte(wrapper.Text), &out)

	return out

}

func TestSemanticSearch_ContentTarget_ReturnsPaymentFirst(t *testing.T) {

	session := setupServer(t)

	result := callSemantic(t, session, map[string]any{

		"query": "stripe payment billing",

		"target": "content",

		"top_k": 3,
	})

	contentResults, ok := result["content_results"].([]any)

	if !ok || len(contentResults) == 0 {

		t.Fatalf("expected content_results, got %v", result)

	}

	first := contentResults[0].(map[string]any)

	repo, _ := first["repo"].(string)

	if repo != "org/payment-svc" {

		t.Errorf("expected payment-svc first, got %s", repo)

	}

}

func TestSemanticSearch_EntitiesTarget_ReturnsAuthFirst(t *testing.T) {

	session := setupServer(t)

	result := callSemantic(t, session, map[string]any{

		"query": "jwt authentication tokens",

		"target": "entities",

		"top_k": 5,
	})

	entityResults, ok := result["entity_results"].([]any)

	if !ok || len(entityResults) == 0 {

		t.Fatalf("expected entity_results, got %v", result)

	}

	first := entityResults[0].(map[string]any)

	name, _ := first["name"].(string)

	if name != "auth-svc" {

		t.Errorf("expected auth-svc first, got %s", name)

	}

}

func TestSemanticSearch_BothTarget_ReturnsBothSections(t *testing.T) {

	session := setupServer(t)

	result := callSemantic(t, session, map[string]any{

		"query": "payment",

		"target": "both",
	})

	if _, ok := result["content_results"]; !ok {

		t.Error("expected content_results in 'both' target")

	}

	if _, ok := result["entity_results"]; !ok {

		t.Error("expected entity_results in 'both' target")

	}

}

func TestSemanticSearch_EmptyQuery_ReturnsError(t *testing.T) {

	session := setupServer(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{

		Name: "semantic_search",

		Arguments: map[string]any{"query": ""},
	})

	if err != nil {

		t.Fatalf("unexpected transport error: %v", err)

	}

	if !res.IsError {

		t.Error("expected IsError=true for empty query")

	}

}

func TestSemanticSearch_ToolIsListed(t *testing.T) {

	session := setupServer(t)

	resp, err := session.ListTools(t.Context(), &mcp.ListToolsParams{})

	if err != nil {

		t.Fatalf("ListTools: %v", err)

	}

	for _, tool := range resp.Tools {

		if tool.Name == "semantic_search" {

			return

		}

	}

	t.Error("semantic_search not found in tool list")

}
