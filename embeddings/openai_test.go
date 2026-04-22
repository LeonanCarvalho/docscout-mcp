// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package embeddings_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/doc-scout/mcp-server/embeddings"
)

func TestOpenAIProvider_Embed(t *testing.T) {

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.Header.Get("Authorization") != "Bearer test-key" {

			http.Error(w, "unauthorized", http.StatusUnauthorized)

			return

		}

		json.NewEncoder(w).Encode(map[string]any{

			"data": []map[string]any{

				{"embedding": []float32{0.1, 0.2, 0.3}},

				{"embedding": []float32{0.4, 0.5, 0.6}},
			},
		})

	}))

	defer srv.Close()

	p := embeddings.NewOpenAIProviderWithURL("test-key", "text-embedding-3-small", srv.URL)

	vecs, err := p.Embed(context.Background(), []string{"hello", "world"})

	if err != nil {

		t.Fatalf("Embed: %v", err)

	}

	if len(vecs) != 2 {

		t.Fatalf("want 2 vectors, got %d", len(vecs))

	}

	if abs32(vecs[0][0]-0.1) > 1e-6 {

		t.Errorf("vecs[0][0]: want 0.1, got %f", vecs[0][0])

	}

}

func TestOpenAIProvider_RateLimit(t *testing.T) {

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		http.Error(w, "rate limited", http.StatusTooManyRequests)

	}))

	defer srv.Close()

	p := embeddings.NewOpenAIProviderWithURL("key", "text-embedding-3-small", srv.URL)

	_, err := p.Embed(context.Background(), []string{"text"})

	if err == nil {

		t.Fatal("expected ErrRateLimit, got nil")

	}

}

func TestOpenAIProvider_ModelKey(t *testing.T) {

	p := embeddings.NewOpenAIProvider("key", "text-embedding-3-small")

	if p.ModelKey() != "openai:text-embedding-3-small" {

		t.Errorf("wrong model key: %s", p.ModelKey())

	}

}
