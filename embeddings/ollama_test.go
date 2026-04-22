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

func TestOllamaProvider_Embed(t *testing.T) {

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.URL.Path != "/api/embed" {

			http.NotFound(w, r)

			return

		}

		json.NewEncoder(w).Encode(map[string]any{

			"embeddings": [][]float32{{0.7, 0.8, 0.9}},
		})

	}))

	defer srv.Close()

	p := embeddings.NewOllamaProvider(srv.URL, "nomic-embed-text")

	vecs, err := p.Embed(context.Background(), []string{"hello"})

	if err != nil {

		t.Fatalf("Embed: %v", err)

	}

	if len(vecs) != 1 {

		t.Fatalf("want 1 vector, got %d", len(vecs))

	}

	if abs32(vecs[0][0]-0.7) > 1e-6 {

		t.Errorf("vecs[0][0]: want 0.7, got %f", vecs[0][0])

	}

}

func TestOllamaProvider_Unreachable(t *testing.T) {

	p := embeddings.NewOllamaProvider("http://127.0.0.1:19999", "nomic-embed-text")

	_, err := p.Embed(context.Background(), []string{"text"})

	if err == nil {

		t.Fatal("expected error for unreachable Ollama, got nil")

	}

}

func TestOllamaProvider_ModelKey(t *testing.T) {

	p := embeddings.NewOllamaProvider("http://localhost:11434", "nomic-embed-text")

	if p.ModelKey() != "ollama:nomic-embed-text" {

		t.Errorf("wrong model key: %s", p.ModelKey())

	}

}
