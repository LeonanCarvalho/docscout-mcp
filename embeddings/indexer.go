// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/doc-scout/mcp-server/memory"
)

const indexBatchSize = 50

// DocStore lists cached documents for a repo.

type DocStore interface {
	ListDocs(repoName string) ([]memory.DocRecord, error)
}

// EntityOpener fetches entities with their observations by name.

type EntityOpener interface {
	OpenNodes(names []string) (memory.KnowledgeGraph, error)
}

// Indexer embeds and stores document and entity vectors after scans/mutations.

type Indexer struct {
	provider EmbeddingProvider

	store *VectorStore

	docs DocStore

	entities EntityOpener

	mu sync.Mutex // serialises concurrent IndexDocs / IndexEntities calls

	debounce struct {
		mu sync.Mutex

		timer *time.Timer

		names map[string]bool
	}
}

// NewIndexer creates an Indexer. docs and entities may be nil (indexing of that type is skipped).

func NewIndexer(provider EmbeddingProvider, store *VectorStore, docs DocStore, entities EntityOpener) *Indexer {

	return &Indexer{provider: provider, store: store, docs: docs, entities: entities}

}

// IndexDocs re-indexes all documents for repoFullName. Skips docs whose stored

// content_hash already matches. Safe to call from a background goroutine.

func (idx *Indexer) IndexDocs(ctx context.Context, repoFullName string) {

	if idx.docs == nil {

		return

	}

	idx.mu.Lock()

	defer idx.mu.Unlock()

	docs, err := idx.docs.ListDocs(repoFullName)

	if err != nil {

		slog.Error("[embeddings] IndexDocs: list docs", "repo", repoFullName, "error", err)

		return

	}

	// TODO(perf): loads all doc embeddings for this model key; for large deployments consider

	// loading only the rows matching this repo to reduce memory pressure.

	existing, err := idx.store.LoadDocEmbeddings(idx.provider.ModelKey())

	if err != nil {

		slog.Error("[embeddings] IndexDocs: load existing", "error", err)

		return

	}

	storedHash := make(map[string]string, len(existing))

	for _, e := range existing {

		storedHash[e.DocID] = e.ContentHash

	}

	var toEmbed []memory.DocRecord

	for _, d := range docs {

		if storedHash[d.DocID] == sha256hex(d.Content) {

			continue

		}

		toEmbed = append(toEmbed, d)

	}

	for i := 0; i < len(toEmbed); i += indexBatchSize {

		end := min(i+indexBatchSize, len(toEmbed))

		batch := toEmbed[i:end]

		texts := make([]string, len(batch))

		for j, d := range batch {

			texts[j] = d.Content

		}

		vectors, err := idx.provider.Embed(ctx, texts)

		if err != nil {

			if errors.Is(err, ErrRateLimit) {

				slog.Warn("[embeddings] IndexDocs: rate limit hit; remaining docs will be retried on next scan", "repo", repoFullName)

			} else {

				slog.Error("[embeddings] IndexDocs: embed batch", "repo", repoFullName, "error", err)

			}

			continue

		}

		for j, d := range batch {

			if err := idx.store.UpsertDoc(d.DocID, sha256hex(d.Content), idx.provider.ModelKey(), vectors[j]); err != nil {

				slog.Error("[embeddings] IndexDocs: upsert", "docID", d.DocID, "error", err)

			}

		}

	}

	// Remove stale rows for docs that no longer exist in this repo.

	// Only consider embeddings that belong to this repo (DocID prefix = repoFullName+"#")

	// to avoid deleting embeddings for other repos.

	repoPrefix := repoFullName + "#"

	currentIDs := make(map[string]bool, len(docs))

	for _, d := range docs {

		currentIDs[d.DocID] = true

	}

	for _, e := range existing {

		if !strings.HasPrefix(e.DocID, repoPrefix) {

			continue // belongs to a different repo — leave it alone

		}

		if !currentIDs[e.DocID] {

			if err := idx.store.DeleteDocByID(e.DocID); err != nil {

				slog.Error("[embeddings] IndexDocs: delete stale", "docID", e.DocID, "error", err)

			}

		}

	}

}

// IndexEntities re-indexes the named entities. Skips those whose stored obs_hash matches.

func (idx *Indexer) IndexEntities(ctx context.Context, names []string) {

	if idx.entities == nil || len(names) == 0 {

		return

	}

	idx.mu.Lock()

	defer idx.mu.Unlock()

	kg, err := idx.entities.OpenNodes(names)

	if err != nil {

		slog.Error("[embeddings] IndexEntities: open nodes", "error", err)

		return

	}

	// TODO(perf): loads all entity embeddings for this model key; for large deployments consider

	// loading only the named entities to reduce memory pressure.

	existing, err := idx.store.LoadEntityEmbeddings(idx.provider.ModelKey())

	if err != nil {

		slog.Error("[embeddings] IndexEntities: load existing", "error", err)

		return

	}

	storedHash := make(map[string]string, len(existing))

	for _, e := range existing {

		storedHash[e.EntityName] = e.ObsHash

	}

	type item struct {
		name string

		text string
	}

	var toEmbed []item

	for _, e := range kg.Entities {

		text := EntityText(e)

		if storedHash[e.Name] == sha256hex(text) {

			continue

		}

		toEmbed = append(toEmbed, item{name: e.Name, text: text})

	}

	for i := 0; i < len(toEmbed); i += indexBatchSize {

		end := min(i+indexBatchSize, len(toEmbed))

		batch := toEmbed[i:end]

		texts := make([]string, len(batch))

		for j, it := range batch {

			texts[j] = it.text

		}

		vectors, err := idx.provider.Embed(ctx, texts)

		if err != nil {

			if errors.Is(err, ErrRateLimit) {

				slog.Warn("[embeddings] IndexEntities: rate limit hit; remaining entities will be retried on next mutation")

			} else {

				slog.Error("[embeddings] IndexEntities: embed batch", "error", err)

			}

			continue

		}

		for j, it := range batch {

			if err := idx.store.UpsertEntity(it.name, sha256hex(it.text), idx.provider.ModelKey(), vectors[j]); err != nil {

				slog.Error("[embeddings] IndexEntities: upsert", "entity", it.name, "error", err)

			}

		}

	}

	// Remove embedding rows for entities that were requested but no longer exist in the graph.

	// This handles the delete_entities case: deleted entities won't appear in OpenNodes results.

	foundInGraph := make(map[string]bool, len(kg.Entities))

	for _, e := range kg.Entities {

		foundInGraph[e.Name] = true

	}

	for _, name := range names {

		if !foundInGraph[name] {

			if err := idx.store.DeleteEntityByName(name); err != nil {

				slog.Error("[embeddings] IndexEntities: delete stale", "entity", name, "error", err)

			}

		}

	}

}

// ScheduleEntities queues entity names for re-indexing with a 2-second debounce.

// Burst calls are coalesced: all accumulated names are indexed in one pass.

func (idx *Indexer) ScheduleEntities(names []string) {

	if len(names) == 0 {

		return

	}

	idx.debounce.mu.Lock()

	defer idx.debounce.mu.Unlock()

	if idx.debounce.names == nil {

		idx.debounce.names = make(map[string]bool)

	}

	for _, n := range names {

		idx.debounce.names[n] = true

	}

	if idx.debounce.timer != nil {

		idx.debounce.timer.Reset(2 * time.Second)

		return

	}

	idx.debounce.timer = time.AfterFunc(2*time.Second, func() {

		idx.debounce.mu.Lock()

		names := make([]string, 0, len(idx.debounce.names))

		for n := range idx.debounce.names {

			names = append(names, n)

		}

		idx.debounce.names = nil

		idx.debounce.timer = nil

		idx.debounce.mu.Unlock()

		idx.IndexEntities(context.Background(), names)

	})

}
