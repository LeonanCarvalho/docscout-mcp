// Copyright 2026 Doc Scout
// SPDX-License-Identifier: AGPL-3.0-only

// embeddings/searcher_test.go

package embeddings_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
)

var searchCounter atomic.Int64

// keywordProvider returns a 3-dim vector based on keyword presence.

// dim0=payment, dim1=auth, dim2=notification

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

		if strings.Contains(lower, "auth") || strings.Contains(lower, "jwt") {

			v[1] = 0.9

		}

		if strings.Contains(lower, "notification") || strings.Contains(lower, "email") {

			v[2] = 0.9

		}

		out[i] = v

	}

	return out, nil

}

func newSearchEnv(t *testing.T) (*embeddings.VectorStore, *memory.ContentCache, *memory.MemoryService) {

	t.Helper()

	dsn := fmt.Sprintf("file:search_test_%d?mode=memory&cache=shared", searchCounter.Add(1))

	db, err := memory.OpenDB(dsn)

	if err != nil {

		t.Fatalf("OpenDB: %v", err)

	}

	vs, err := embeddings.NewVectorStore(db)

	if err != nil {

		t.Fatalf("NewVectorStore: %v", err)

	}

	return vs, memory.NewContentCache(db, true, 1024*1024), memory.NewMemoryService(db)

}

func TestSemanticSearcher_SearchDocs_TopResult(t *testing.T) {

	vs, cc, ms := newSearchEnv(t)

	provider := &keywordProvider{}

	cc.Upsert("org/pay", "README.md", "sha1", "Payment service handles Stripe transactions.", "readme")

	cc.Upsert("org/auth", "README.md", "sha2", "Auth service manages JWT tokens.", "readme")

	idx := embeddings.NewIndexer(provider, vs, cc, ms)

	idx.IndexDocs(context.Background(), "org/pay")

	idx.IndexDocs(context.Background(), "org/auth")

	searcher := embeddings.NewSemanticSearcher(provider, vs, idx, cc, ms)

	results, stale, err := searcher.SearchDocs(context.Background(), "payment billing stripe", "", 5)

	if err != nil {

		t.Fatalf("SearchDocs: %v", err)

	}

	if stale != 0 {

		t.Errorf("expected 0 stale docs, got %d", stale)

	}

	if len(results) == 0 {

		t.Fatal("expected at least 1 result")

	}

	if results[0].Repo != "org/pay" {

		t.Errorf("expected payment doc first, got %s", results[0].Repo)

	}

}

func TestSemanticSearcher_SearchDocs_Disabled(t *testing.T) {

	vs, cc, ms := newSearchEnv(t)

	searcher := embeddings.NewSemanticSearcher(nil, vs, nil, cc, ms)

	_, _, err := searcher.SearchDocs(context.Background(), "query", "", 5)

	if err == nil {

		t.Fatal("expected error when provider is nil")

	}

}

func TestSemanticSearcher_SearchEntities_TopResult(t *testing.T) {

	vs, cc, ms := newSearchEnv(t)

	provider := &keywordProvider{}

	ms.CreateEntities([]memory.Entity{

		{Name: "payment-svc", EntityType: "service", Observations: []string{"handles stripe payments"}},

		{Name: "auth-svc", EntityType: "service", Observations: []string{"manages jwt tokens"}},
	})

	idx := embeddings.NewIndexer(provider, vs, cc, ms)

	idx.IndexEntities(context.Background(), []string{"payment-svc", "auth-svc"})

	searcher := embeddings.NewSemanticSearcher(provider, vs, idx, cc, ms)

	results, _, err := searcher.SearchEntities(context.Background(), "payment stripe", 5)

	if err != nil {

		t.Fatalf("SearchEntities: %v", err)

	}

	if len(results) == 0 {

		t.Fatal("expected at least 1 result")

	}

	if results[0].Name != "payment-svc" {

		t.Errorf("expected payment-svc first, got %s", results[0].Name)

	}

}
