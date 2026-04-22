// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/doc-scout/mcp-server/memory"
)

const disabledMsg = "semantic search not enabled: set DOCSCOUT_EMBED_OPENAI_KEY or DOCSCOUT_EMBED_OLLAMA_URL"

// DocResult is a single semantic search result for a document.

type DocResult struct {
	DocID string `json:"doc_id"`

	Repo string `json:"repo"`

	Path string `json:"path"`

	Score float64 `json:"score"`

	Snippet string `json:"snippet"`
}

// EntityResult is a single semantic search result for a knowledge graph entity.

type EntityResult struct {
	Name string `json:"name"`

	EntityType string `json:"entity_type"`

	Score float64 `json:"score"`

	Observations []string `json:"observations"`
}

// GraphReader reads the full knowledge graph.

type GraphReader interface {
	ReadGraph() (memory.KnowledgeGraph, error)
}

// SemanticSearcher runs semantic similarity search over docs and entities.

// It also implements ScheduleIndexEntities and IndexDocs as a facade over Indexer.

type SemanticSearcher struct {
	provider EmbeddingProvider

	store *VectorStore

	indexer *Indexer

	docs DocStore

	entities GraphReader
}

// NewSemanticSearcher creates a SemanticSearcher.

// provider may be nil — all Search* methods return a "not enabled" error.

func NewSemanticSearcher(provider EmbeddingProvider, store *VectorStore, indexer *Indexer, docs DocStore, entities GraphReader) *SemanticSearcher {

	return &SemanticSearcher{

		provider: provider,

		store: store,

		indexer: indexer,

		docs: docs,

		entities: entities,
	}

}

// Enabled returns true when a provider is configured.

func (ss *SemanticSearcher) Enabled() bool { return ss.provider != nil }

// ScheduleIndexEntities delegates to the Indexer's debounced scheduler.

func (ss *SemanticSearcher) ScheduleIndexEntities(names []string) {

	if ss.indexer != nil {

		ss.indexer.ScheduleEntities(names)

	}

}

// IndexDocs delegates to the Indexer for post-scan wiring in main.go.

func (ss *SemanticSearcher) IndexDocs(ctx context.Context, repo string) {

	if ss.indexer != nil {

		ss.indexer.IndexDocs(ctx, repo)

	}

}

// SearchDocs returns the top-k semantically similar documents.

// repo may be empty to search all repos. Returns stale count alongside results.

func (ss *SemanticSearcher) SearchDocs(ctx context.Context, query, repo string, topK int) ([]DocResult, int, error) {

	if ss.provider == nil {

		return nil, 0, errors.New(disabledMsg)

	}

	vecs, err := ss.provider.Embed(ctx, []string{query})

	if err != nil {

		return nil, 0, fmt.Errorf("embed query: %w", err)

	}

	queryVec := vecs[0]

	stored, err := ss.store.LoadDocEmbeddings(ss.provider.ModelKey())

	if err != nil {

		return nil, 0, fmt.Errorf("load doc embeddings: %w", err)

	}

	// TODO(perf): when repo is "", this loads the entire content cache for hash-freshness

	// checking. For large deployments, consider loading only the doc IDs present in `stored`.

	current, err := ss.docs.ListDocs(repo)

	if err != nil {

		return nil, 0, fmt.Errorf("list docs: %w", err)

	}

	type docInfo struct {
		hash string

		snippet string
	}

	currentMap := make(map[string]docInfo, len(current))

	for _, d := range current {

		snip := d.Content

		if len(snip) > 300 {

			snip = snip[:300]

		}

		currentMap[d.DocID] = docInfo{hash: sha256hex(d.Content), snippet: snip}

	}

	type candidate struct {
		e DocEmbedding

		score float64
	}

	var candidates []candidate

	stale := 0

	for _, e := range stored {

		if repo != "" && !strings.HasPrefix(e.DocID, repo+"#") {

			continue

		}

		info, ok := currentMap[e.DocID]

		if !ok || info.hash != e.ContentHash {

			stale++

			continue

		}

		candidates = append(candidates, candidate{e: e, score: CosineSimilarity(queryVec, e.Vector)})

	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })

	if topK > 0 && len(candidates) > topK {

		candidates = candidates[:topK]

	}

	results := make([]DocResult, len(candidates))

	for i, c := range candidates {

		parts := strings.SplitN(c.e.DocID, "#", 2)

		repoName, path := c.e.DocID, ""

		if len(parts) == 2 {

			repoName, path = parts[0], parts[1]

		}

		results[i] = DocResult{

			DocID: c.e.DocID,

			Repo: repoName,

			Path: path,

			Score: c.score,

			Snippet: currentMap[c.e.DocID].snippet,
		}

	}

	return results, stale, nil

}

// SearchEntities returns the top-k semantically similar knowledge graph entities.

func (ss *SemanticSearcher) SearchEntities(ctx context.Context, query string, topK int) ([]EntityResult, int, error) {

	if ss.provider == nil {

		return nil, 0, errors.New(disabledMsg)

	}

	vecs, err := ss.provider.Embed(ctx, []string{query})

	if err != nil {

		return nil, 0, fmt.Errorf("embed query: %w", err)

	}

	queryVec := vecs[0]

	stored, err := ss.store.LoadEntityEmbeddings(ss.provider.ModelKey())

	if err != nil {

		return nil, 0, fmt.Errorf("load entity embeddings: %w", err)

	}

	kg, err := ss.entities.ReadGraph()

	if err != nil {

		return nil, 0, fmt.Errorf("read graph: %w", err)

	}

	type entityInfo struct {
		hash string

		entityType string

		observations []string
	}

	currentMap := make(map[string]entityInfo, len(kg.Entities))

	for _, e := range kg.Entities {

		currentMap[e.Name] = entityInfo{

			hash: sha256hex(EntityText(e)),

			entityType: e.EntityType,

			observations: e.Observations,
		}

	}

	type candidate struct {
		e EntityEmbedding

		score float64
	}

	var candidates []candidate

	stale := 0

	for _, e := range stored {

		info, ok := currentMap[e.EntityName]

		if !ok || info.hash != e.ObsHash {

			stale++

			continue

		}

		candidates = append(candidates, candidate{e: e, score: CosineSimilarity(queryVec, e.Vector)})

	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })

	if topK > 0 && len(candidates) > topK {

		candidates = candidates[:topK]

	}

	results := make([]EntityResult, len(candidates))

	for i, c := range candidates {

		info := currentMap[c.e.EntityName]

		results[i] = EntityResult{

			Name: c.e.EntityName,

			EntityType: info.entityType,

			Score: c.score,

			Observations: info.observations,
		}

	}

	return results, stale, nil

}
