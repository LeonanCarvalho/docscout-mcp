// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/scanner"
	"github.com/doc-scout/mcp-server/scanner/parser"
)

// FileGetter fetches the raw content of an indexed documentation file.

type FileGetter interface {
	GetFileContent(ctx context.Context, repo, path string) (string, error)
}

// GraphWriter writes entities and relations to the knowledge graph.

type GraphWriter interface {
	CreateEntities([]memory.Entity) ([]memory.Entity, error)

	CreateRelations([]memory.Relation) ([]memory.Relation, error)

	AddObservations([]memory.Observation) ([]memory.Observation, error)

	SearchNodes(query string) (memory.KnowledgeGraph, error)

	EntityCount() (int64, error)
}

// AutoIndexer automatically populates the knowledge graph from manifest files

// and optionally refreshes the content cache after each scan.

type AutoIndexer struct {
	sc FileGetter

	graph GraphWriter

	cache *memory.ContentCache // nil if content caching disabled

	registry *parser.ParserRegistry
}

// New creates an AutoIndexer. cache may be nil if SCAN_CONTENT is disabled.

func New(sc FileGetter, graph GraphWriter, cache *memory.ContentCache, registry *parser.ParserRegistry) *AutoIndexer {

	return &AutoIndexer{sc: sc, graph: graph, cache: cache, registry: registry}

}

// Run is the OnScanComplete callback. It:

//  1. Refreshes the content cache for all indexed files (if enabled).

//  2. Runs all registered parsers and upserts entities/relations into the graph.

//  3. Soft-deletes entities from repos no longer in the scan.

func (ai *AutoIndexer) Run(ctx context.Context, repos []scanner.RepoInfo) {

	activeRepos := make(map[string]bool, len(repos))

	for _, r := range repos {

		activeRepos[r.Name] = true

	}

	// Phase 1: Refresh content cache.

	if ai.cache != nil {

		slog.Info("[indexer] Phase 1: refreshing content cache", "repos", len(repos))

		ai.refreshContent(ctx, repos)

		activeRepoList := make([]string, 0, len(activeRepos))

		for name := range activeRepos {

			activeRepoList = append(activeRepoList, name)

		}

		if err := ai.cache.DeleteOrphanedContent(activeRepoList); err != nil {

			slog.Error("[indexer] Failed to delete orphaned content", "error", err)

		}

	}

	// Phase 2: Run all registered parsers.

	slog.Info("[indexer] Phase 2: running parsers", "repos", len(repos), "parsers", len(ai.registry.All()))

	if err := ai.runParsers(ctx, repos); err != nil {

		slog.Error("[indexer] Phase 2 failed", "error", err)

	}

	// Phase 3: Soft-delete stale entities.

	slog.Info("[indexer] Phase 3: archiving stale entities")

	ai.archiveStale(ctx, activeRepos)

	slog.Info("[indexer] Indexing complete", "active_repos", len(repos))

}

// runParsers iterates all repos and files, routes each file to its registered parser,

// and upserts the resulting graph entities and relations.

func (ai *AutoIndexer) runParsers(ctx context.Context, repos []scanner.RepoInfo) error {

	// Track which CODEOWNERS file we've processed per repo (first one wins).

	processedCodeowners := make(map[string]bool)

	for _, repo := range repos {

		for _, file := range repo.Files {

			p, ok := ai.registry.Get(file.Type)

			if !ok {

				continue // infra/docs file — no graph entity

			}

			// Only process the first CODEOWNERS per repo to avoid duplicate owns relations.

			if file.Type == "codeowners" {

				if processedCodeowners[repo.Name] {

					continue

				}

				processedCodeowners[repo.Name] = true

			}

			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)

			if err != nil {

				slog.Warn("[indexer] Failed to fetch file", "repo", repo.Name, "path", file.Path, "error", err)

				continue

			}

			parsed, err := p.Parse([]byte(content))

			if err != nil {

				slog.Warn("[indexer] Parser failed", "type", file.Type, "repo", repo.Name, "path", file.Path, "error", err)

				continue

			}

			if err := ai.upsertParsedFile(ctx, repo, p.FileType(), parsed); err != nil {

				slog.Error("[indexer] upsertParsedFile failed", "type", file.Type, "repo", repo.Name, "error", err)

			}

		}

	}

	return nil

}

// repoServiceName returns the short service entity name derived from a repo FullName.

// "myorg/my-service" → "my-service". Falls back to repo.Name if FullName is not set.

func repoServiceName(repo scanner.RepoInfo) string {

	if _, name, ok := strings.Cut(repo.FullName, "/"); ok {

		return name

	}

	return repo.Name

}

// upsertParsedFile writes a ParsedFile's entities, observations, and relations to the graph.

// Auto-observations _source:<fileType> and _scan_repo:<repo.FullName> are always added.

// If parsed.EntityName is empty but AuxEntities are present (codeowners pattern), only

// aux entities and relations are created.

// Relations with To == "" have their To field filled with the derived repo service name.

func (ai *AutoIndexer) upsertParsedFile(ctx context.Context, repo scanner.RepoInfo, fileType string, parsed parser.ParsedFile) error {

	autoObs := []string{

		"_source:" + fileType,

		"_scan_repo:" + repo.FullName,
	}

	// Create / update primary entity (if named).

	if parsed.EntityName != "" {

		entityType := parsed.EntityType

		if entityType == "" {

			entityType = "service"

		}

		obs := append(autoObs, parsed.Observations...)

		graph, err := ai.graph.SearchNodes(parsed.EntityName)

		if err != nil {

			return fmt.Errorf("SearchNodes %q: %w", parsed.EntityName, err)

		}

		entityExists := slices.ContainsFunc(graph.Entities, func(e memory.Entity) bool {

			return e.Name == parsed.EntityName

		})

		if !entityExists {

			if _, err := ai.graph.CreateEntities([]memory.Entity{

				{Name: parsed.EntityName, EntityType: entityType, Observations: obs},
			}); err != nil {

				return fmt.Errorf("CreateEntities %q: %w", parsed.EntityName, err)

			}

		} else {

			if _, err := ai.graph.AddObservations([]memory.Observation{

				{EntityName: parsed.EntityName, Contents: obs},
			}); err != nil {

				return fmt.Errorf("AddObservations %q: %w", parsed.EntityName, err)

			}

		}

	}

	// Create / update auxiliary entities (e.g. owners from CODEOWNERS).

	for _, aux := range parsed.AuxEntities {

		auxObs := append(autoObs, aux.Observations...)

		graph, err := ai.graph.SearchNodes(aux.Name)

		if err != nil {

			slog.Error("[indexer] SearchNodes failed for aux entity", "entity", aux.Name, "error", err)

			continue

		}

		auxExists := slices.ContainsFunc(graph.Entities, func(e memory.Entity) bool {

			return e.Name == aux.Name

		})

		if !auxExists {

			if _, err := ai.graph.CreateEntities([]memory.Entity{

				{Name: aux.Name, EntityType: aux.EntityType, Observations: auxObs},
			}); err != nil {

				slog.Error("[indexer] CreateEntities failed for aux entity", "entity", aux.Name, "error", err)

				continue

			}

		} else {

			if _, err := ai.graph.AddObservations([]memory.Observation{

				{EntityName: aux.Name, Contents: auxObs},
			}); err != nil {

				slog.Error("[indexer] AddObservations failed for aux entity", "entity", aux.Name, "error", err)

			}

		}

	}

	// Create relations. Empty To is filled with the repo service name.

	if len(parsed.Relations) == 0 {

		return nil

	}

	svcName := repoServiceName(repo)

	rels := make([]memory.Relation, 0, len(parsed.Relations))

	for _, r := range parsed.Relations {

		from := r.From

		if from == "" {

			from = svcName

		}

		to := r.To

		if to == "" {

			to = svcName

		}

		rels = append(rels, memory.Relation{

			From: from,

			To: to,

			RelationType: r.RelationType,
		})

	}

	if _, err := ai.graph.CreateRelations(rels); err != nil {

		return fmt.Errorf("CreateRelations: %w", err)

	}

	return nil

}

// refreshContent fetches and caches content for files whose SHA has changed.

func (ai *AutoIndexer) refreshContent(ctx context.Context, repos []scanner.RepoInfo) {

	for _, repo := range repos {

		for _, file := range repo.Files {

			if !ai.cache.NeedsUpdate(repo.Name, file.Path, file.SHA) {

				continue

			}

			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)

			if err != nil {

				slog.Warn("[indexer] Content fetch failed", "repo", repo.Name, "path", file.Path, "error", err)

				continue

			}

			if err := ai.cache.Upsert(repo.Name, file.Path, file.SHA, content, file.Type); err != nil {

				slog.Warn("[indexer] Content store failed", "repo", repo.Name, "path", file.Path, "error", err)

			}

		}

	}

}

// archiveStale adds _status:archived to entities whose source repo is no longer active.

func (ai *AutoIndexer) archiveStale(ctx context.Context, activeRepos map[string]bool) {

	graph, err := ai.graph.SearchNodes("_scan_repo:")

	if err != nil {

		slog.Error("[indexer] SearchNodes for stale check failed", "error", err)

		return

	}

	for _, entity := range graph.Entities {

		repoName := ""

		for _, obs := range entity.Observations {

			if name, ok := strings.CutPrefix(obs, "_scan_repo:"); ok {

				repoName = name

				break

			}

		}

		if repoName == "" || activeRepos[repoName] {

			continue

		}

		_, err := ai.graph.AddObservations([]memory.Observation{

			{EntityName: entity.Name, Contents: []string{"_status:archived"}},
		})

		if err != nil {

			slog.Error("[indexer] Failed to archive entity", "entity", entity.Name, "error", err)

		}

	}

}
