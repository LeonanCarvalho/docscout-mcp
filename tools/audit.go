// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

// Package tools — GraphAuditLogger wraps any GraphStore and emits a structured

// slog line for every mutation (create / add / delete). Read-only operations

// (ReadGraph, SearchNodes, OpenNodes, EntityCount) are delegated silently.

package tools

import (
	"context"
	"log/slog"

	"github.com/doc-scout/mcp-server/memory"
)

// GraphAuditLogger is a GraphStore decorator that logs every mutation operation.

// Instantiate with NewGraphAuditLogger and use in place of the underlying store.

type GraphAuditLogger struct {
	inner GraphStore
}

// NewGraphAuditLogger wraps inner with audit logging. inner must not be nil.

func NewGraphAuditLogger(inner GraphStore) *GraphAuditLogger {

	return &GraphAuditLogger{inner: inner}

}

// ── Mutations ─────────────────────────────────────────────────────────────────

func (a *GraphAuditLogger) CreateEntities(entities []memory.Entity) ([]memory.Entity, error) {

	names := entityNames(entities)

	result, err := a.inner.CreateEntities(entities)

	if err != nil {

		slog.Warn("[graph:audit] create_entities failed", "names", names, "error", err)

	} else {

		slog.Info("[graph:audit] create_entities", "names", names, "count", len(result))

	}

	return result, err

}

func (a *GraphAuditLogger) CreateRelations(relations []memory.Relation) ([]memory.Relation, error) {

	result, err := a.inner.CreateRelations(relations)

	if err != nil {

		slog.Warn("[graph:audit] create_relations failed", "count", len(relations), "error", err)

	} else {

		slog.Info("[graph:audit] create_relations", "count", len(result))

	}

	return result, err

}

func (a *GraphAuditLogger) AddObservations(observations []memory.Observation) ([]memory.Observation, error) {

	entities := observationEntityNames(observations)

	totalObs := countObservations(observations)

	result, err := a.inner.AddObservations(observations)

	if err != nil {

		slog.Warn("[graph:audit] add_observations failed", "entities", entities, "total_obs", totalObs, "error", err)

	} else {

		slog.Info("[graph:audit] add_observations", "entities", entities, "total_obs", totalObs)

	}

	return result, err

}

func (a *GraphAuditLogger) DeleteEntities(entityNames []string) error {

	err := a.inner.DeleteEntities(entityNames)

	if err != nil {

		slog.Warn("[graph:audit] delete_entities failed", "names", entityNames, "count", len(entityNames), "error", err)

	} else {

		slog.Info("[graph:audit] delete_entities", "names", entityNames, "count", len(entityNames))

	}

	return err

}

func (a *GraphAuditLogger) DeleteObservations(deletions []memory.Observation) error {

	entities := observationEntityNames(deletions)

	err := a.inner.DeleteObservations(deletions)

	if err != nil {

		slog.Warn("[graph:audit] delete_observations failed", "entities", entities, "error", err)

	} else {

		slog.Info("[graph:audit] delete_observations", "entities", entities, "count", len(deletions))

	}

	return err

}

func (a *GraphAuditLogger) DeleteRelations(relations []memory.Relation) error {

	err := a.inner.DeleteRelations(relations)

	if err != nil {

		slog.Warn("[graph:audit] delete_relations failed", "count", len(relations), "error", err)

	} else {

		slog.Info("[graph:audit] delete_relations", "count", len(relations))

	}

	return err

}

func (a *GraphAuditLogger) UpdateEntity(oldName, newName, newType string) error {

	err := a.inner.UpdateEntity(oldName, newName, newType)

	if err != nil {

		slog.Warn("[graph:audit] update_entity failed", "old_name", oldName, "new_name", newName, "new_type", newType, "error", err)

	} else {

		slog.Info("[graph:audit] update_entity", "old_name", oldName, "new_name", newName, "new_type", newType)

	}

	return err

}

// ── Read-only pass-throughs ───────────────────────────────────────────────────

func (a *GraphAuditLogger) GetIntegrationMap(ctx context.Context, service string, depth int) (memory.IntegrationMap, error) {

	return a.inner.GetIntegrationMap(ctx, service, depth)

}

func (a *GraphAuditLogger) ListEntities(entityType string) (memory.KnowledgeGraph, error) {

	return a.inner.ListEntities(entityType)

}

func (a *GraphAuditLogger) ListRelations(relationType, fromEntity string) ([]memory.Relation, error) {

	return a.inner.ListRelations(relationType, fromEntity)

}

func (a *GraphAuditLogger) ReadGraph() (memory.KnowledgeGraph, error) {

	return a.inner.ReadGraph()

}

func (a *GraphAuditLogger) SearchNodes(query string) (memory.KnowledgeGraph, error) {

	return a.inner.SearchNodes(query)

}

func (a *GraphAuditLogger) SearchNodesFiltered(query string, includeArchived bool) (memory.KnowledgeGraph, error) {

	return a.inner.SearchNodesFiltered(query, includeArchived)

}

func (a *GraphAuditLogger) OpenNodes(names []string) (memory.KnowledgeGraph, error) {

	return a.inner.OpenNodes(names)

}

func (a *GraphAuditLogger) OpenNodesFiltered(names []string, includeArchived bool) (memory.KnowledgeGraph, error) {

	return a.inner.OpenNodesFiltered(names, includeArchived)

}

func (a *GraphAuditLogger) EntityCount() (int64, error) {

	return a.inner.EntityCount()

}

func (a *GraphAuditLogger) EntityTypeCounts() (map[string]int64, error) {

	return a.inner.EntityTypeCounts()

}

func (a *GraphAuditLogger) TraverseGraph(entity, relationType, direction string, maxDepth int) ([]memory.TraverseNode, error) {

	return a.inner.TraverseGraph(entity, relationType, direction, maxDepth)

}

func (a *GraphAuditLogger) FindPath(from, to string, maxDepth int) ([]memory.PathEdge, error) {

	return a.inner.FindPath(from, to, maxDepth)

}

// ── Helpers ───────────────────────────────────────────────────────────────────

func entityNames(entities []memory.Entity) []string {

	names := make([]string, len(entities))

	for i, e := range entities {

		names[i] = e.Name

	}

	return names

}

func observationEntityNames(obs []memory.Observation) []string {

	seen := make(map[string]struct{}, len(obs))

	var names []string

	for _, o := range obs {

		if _, ok := seen[o.EntityName]; !ok {

			seen[o.EntityName] = struct{}{}

			names = append(names, o.EntityName)

		}

	}

	return names

}

func countObservations(obs []memory.Observation) int {

	total := 0

	for _, o := range obs {

		total += len(o.Contents)

	}

	return total

}
