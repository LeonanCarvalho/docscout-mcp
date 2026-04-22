// Copyright 2026 Doc Scout
// SPDX-License-Identifier: AGPL-3.0-only

// embeddings/indexer_test.go

package embeddings_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
)

var idxCounter atomic.Int64

type fixedProvider struct {
	dim int
}

func (f *fixedProvider) ModelKey() string { return "mock:v1" }

func (f *fixedProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {

	out := make([][]float32, len(texts))

	for i := range out {

		out[i] = make([]float32, f.dim)

		out[i][0] = 0.9

	}

	return out, nil

}

func newIdxEnv(t *testing.T) (*embeddings.VectorStore, *memory.ContentCache, *memory.MemoryService) {

	t.Helper()

	dsn := fmt.Sprintf("file:idx_test_%d?mode=memory&cache=shared", idxCounter.Add(1))

	db, err := memory.OpenDB(dsn)

	if err != nil {

		t.Fatalf("OpenDB: %v", err)

	}

	vs, err := embeddings.NewVectorStore(db)

	if err != nil {

		t.Fatalf("NewVectorStore: %v", err)

	}

	cc := memory.NewContentCache(db, true, 1024*1024)

	ms := memory.NewMemoryService(db)

	return vs, cc, ms

}

func TestIndexer_IndexDocs_Upserts(t *testing.T) {

	vs, cc, ms := newIdxEnv(t)

	provider := &fixedProvider{dim: 4}

	cc.Upsert("org/svc", "README.md", "sha1", "hello world payment", "readme")

	idx := embeddings.NewIndexer(provider, vs, cc, ms)

	idx.IndexDocs(context.Background(), "org/svc")

	rows, err := vs.LoadDocEmbeddings("mock:v1")

	if err != nil {

		t.Fatalf("LoadDocEmbeddings: %v", err)

	}

	if len(rows) != 1 {

		t.Fatalf("expected 1 embedding, got %d", len(rows))

	}

	if rows[0].DocID != "org/svc#README.md" {

		t.Errorf("wrong docID: %s", rows[0].DocID)

	}

}

func TestIndexer_IndexDocs_SkipsUpToDate(t *testing.T) {

	vs, cc, ms := newIdxEnv(t)

	provider := &fixedProvider{dim: 4}

	cc.Upsert("org/svc", "README.md", "sha1", "content", "readme")

	idx := embeddings.NewIndexer(provider, vs, cc, ms)

	idx.IndexDocs(context.Background(), "org/svc")

	// Second run — content unchanged — should not re-embed (embed count stays 1)

	idx.IndexDocs(context.Background(), "org/svc")

	rows, _ := vs.LoadDocEmbeddings("mock:v1")

	if len(rows) != 1 {

		t.Fatalf("expected still 1 embedding after second run, got %d", len(rows))

	}

}

func TestIndexer_IndexEntities_Upserts(t *testing.T) {

	vs, cc, ms := newIdxEnv(t)

	provider := &fixedProvider{dim: 4}

	ms.CreateEntities([]memory.Entity{{Name: "payment-svc", EntityType: "service", Observations: []string{"lang:go"}}})

	idx := embeddings.NewIndexer(provider, vs, cc, ms)

	idx.IndexEntities(context.Background(), []string{"payment-svc"})

	rows, err := vs.LoadEntityEmbeddings("mock:v1")

	if err != nil {

		t.Fatalf("LoadEntityEmbeddings: %v", err)

	}

	if len(rows) != 1 {

		t.Fatalf("expected 1 entity embedding, got %d", len(rows))

	}

	if rows[0].EntityName != "payment-svc" {

		t.Errorf("wrong entity: %s", rows[0].EntityName)

	}

}
