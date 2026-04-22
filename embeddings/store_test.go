// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package embeddings_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
)

var storeCounter atomic.Int64

func newTestStore(t *testing.T) *embeddings.VectorStore {

	t.Helper()

	dsn := fmt.Sprintf("file:store_test_%d?mode=memory&cache=shared", storeCounter.Add(1))

	db, err := memory.OpenDB(dsn)

	if err != nil {

		t.Fatalf("OpenDB: %v", err)

	}

	vs, err := embeddings.NewVectorStore(db)

	if err != nil {

		t.Fatalf("NewVectorStore: %v", err)

	}

	return vs

}

func TestVectorStore_UpsertAndLoadDoc(t *testing.T) {

	vs := newTestStore(t)

	vec := []float32{0.1, 0.2, 0.3, 0.4}

	if err := vs.UpsertDoc("org/svc#README.md", "hash1", "openai:text-embedding-3-small", vec); err != nil {

		t.Fatalf("UpsertDoc: %v", err)

	}

	rows, err := vs.LoadDocEmbeddings("openai:text-embedding-3-small")

	if err != nil {

		t.Fatalf("LoadDocEmbeddings: %v", err)

	}

	if len(rows) != 1 {

		t.Fatalf("want 1 row, got %d", len(rows))

	}

	if rows[0].DocID != "org/svc#README.md" {

		t.Errorf("wrong docID: %s", rows[0].DocID)

	}

	if rows[0].ContentHash != "hash1" {

		t.Errorf("wrong hash: %s", rows[0].ContentHash)

	}

	for i, v := range rows[0].Vector {

		if abs32(v-vec[i]) > 1e-6 {

			t.Errorf("vector[%d]: want %f, got %f", i, vec[i], v)

		}

	}

}

func TestVectorStore_UpsertDocUpdates(t *testing.T) {

	vs := newTestStore(t)

	vs.UpsertDoc("org/svc#README.md", "hash1", "openai:text-embedding-3-small", []float32{0.1, 0.2})

	vs.UpsertDoc("org/svc#README.md", "hash2", "openai:text-embedding-3-small", []float32{0.9, 0.8})

	rows, _ := vs.LoadDocEmbeddings("openai:text-embedding-3-small")

	if len(rows) != 1 {

		t.Fatalf("upsert should not duplicate; got %d rows", len(rows))

	}

	if rows[0].ContentHash != "hash2" {

		t.Errorf("expected updated hash2, got %s", rows[0].ContentHash)

	}

}

func TestVectorStore_UpsertAndLoadEntity(t *testing.T) {

	vs := newTestStore(t)

	vec := []float32{0.5, 0.5}

	if err := vs.UpsertEntity("payment-svc", "obshash1", "ollama:nomic-embed-text", vec); err != nil {

		t.Fatalf("UpsertEntity: %v", err)

	}

	rows, err := vs.LoadEntityEmbeddings("ollama:nomic-embed-text")

	if err != nil {

		t.Fatalf("LoadEntityEmbeddings: %v", err)

	}

	if len(rows) != 1 {

		t.Fatalf("want 1 row, got %d", len(rows))

	}

	if rows[0].EntityName != "payment-svc" {

		t.Errorf("wrong name: %s", rows[0].EntityName)

	}

}

func TestVectorStore_DeleteDocByID(t *testing.T) {

	vs := newTestStore(t)

	vs.UpsertDoc("org/svc#a.md", "h1", "openai:text-embedding-3-small", []float32{0.1})

	vs.UpsertDoc("org/svc#b.md", "h2", "openai:text-embedding-3-small", []float32{0.2})

	if err := vs.DeleteDocByID("org/svc#a.md"); err != nil {

		t.Fatalf("DeleteDocByID: %v", err)

	}

	rows, _ := vs.LoadDocEmbeddings("openai:text-embedding-3-small")

	if len(rows) != 1 || rows[0].DocID != "org/svc#b.md" {

		t.Errorf("expected only b.md to remain, got %v", rows)

	}

}

func abs32(x float32) float32 {

	if x < 0 {

		return -x

	}

	return x

}
