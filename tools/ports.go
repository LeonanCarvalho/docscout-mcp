// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"time"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/scanner"
)

// DocumentScanner defines the interface for interacting with the documentation scanner.

type DocumentScanner interface {
	ListRepos() []scanner.RepoInfo

	SearchDocs(query string) []scanner.FileEntry

	GetFileContent(ctx context.Context, repo string, path string) (string, error)

	Status() (scanning bool, lastScan time.Time, repoCount int)

	// TriggerScan queues an immediate full scan. Returns true when successfully

	// queued, false when a scan was already queued (duplicates are coalesced).

	TriggerScan() bool
}

// GraphStore provides full access to the Knowledge Graph domain layer.

type GraphStore interface {
	CreateEntities(entities []memory.Entity) ([]memory.Entity, error)

	CreateRelations(relations []memory.Relation) ([]memory.Relation, error)

	AddObservations(observations []memory.Observation) ([]memory.Observation, error)

	DeleteEntities(entityNames []string) error

	DeleteObservations(deletions []memory.Observation) error

	DeleteRelations(relations []memory.Relation) error

	ReadGraph() (memory.KnowledgeGraph, error)

	SearchNodes(query string) (memory.KnowledgeGraph, error)

	SearchNodesFiltered(query string, includeArchived bool) (memory.KnowledgeGraph, error)

	OpenNodes(names []string) (memory.KnowledgeGraph, error)

	OpenNodesFiltered(names []string, includeArchived bool) (memory.KnowledgeGraph, error)

	EntityCount() (int64, error)

	// EntityTypeCounts returns a map of entity_type → count for all entities in the graph.

	EntityTypeCounts() (map[string]int64, error)

	TraverseGraph(entity, relationType, direction string, maxDepth int) ([]memory.TraverseNode, error)

	GetIntegrationMap(ctx context.Context, service string, depth int) (memory.IntegrationMap, error)

	// ListEntities returns all entities matching entityType (case-insensitive).

	// When entityType is empty, all entities are returned.

	ListEntities(entityType string) (memory.KnowledgeGraph, error)

	// ListRelations returns all relations, optionally filtered by relationType and/or fromEntity.

	// Empty string parameters act as wildcards (match all).

	ListRelations(relationType, fromEntity string) ([]memory.Relation, error)

	FindPath(from, to string, maxDepth int) ([]memory.PathEdge, error)

	UpdateEntity(oldName, newName, newType string) error
}

// ContentSearcher provides full-text search over cached documentation content.

type ContentSearcher interface {

	// Search performs full-text search. Pass "" for repo or fileType to skip those filters.

	Search(query, repo, fileType string) ([]memory.ContentMatch, error)

	Count() (int64, error)

	// SearchMode returns the active search backend: "fts5" (SQLite FTS5) or "like" (LIKE fallback).

	SearchMode() string
}

// SemanticSearch gates the semantic search Plus feature.

// Pass nil to Register to disable semantic search entirely.

type SemanticSearch interface {
	Enabled() bool

	SearchDocs(ctx context.Context, query, repo string, topK int) ([]embeddings.DocResult, int, error)

	SearchEntities(ctx context.Context, query string, topK int) ([]embeddings.EntityResult, int, error)

	// ScheduleIndexEntities queues entities for debounced re-indexing after graph mutations.

	ScheduleIndexEntities(names []string)

	// IndexDocs synchronously re-indexes docs for a repo. Call from a background goroutine.

	IndexDocs(ctx context.Context, repo string)
}
