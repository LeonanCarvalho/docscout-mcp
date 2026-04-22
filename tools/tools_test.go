// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/scanner"
)

// newTestServer creates a fresh MCP server for tool registration tests.

func newTestServer() *mcp.Server {

	return mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, nil)

}

// listToolNames connects an in-memory client to s and returns the names of all registered tools.

func listToolNames(s *mcp.Server) []string {

	ctx := context.Background()

	t1, t2 := mcp.NewInMemoryTransports()

	if _, err := s.Connect(ctx, t1, nil); err != nil {

		return nil

	}

	client := mcp.NewClient(&mcp.Implementation{Name: "tc", Version: "v0"}, nil)

	session, err := client.Connect(ctx, t2, nil)

	if err != nil {

		return nil

	}

	res, err := session.ListTools(ctx, nil)

	if err != nil {

		return nil

	}

	names := make([]string, 0, len(res.Tools))

	for _, tool := range res.Tools {

		names = append(names, tool.Name)

	}

	return names

}

// mockScanner implements DocumentScanner for testing

type mockScanner struct {
	repos []scanner.RepoInfo

	files []scanner.FileEntry

	content map[string]string // key: repoName/path

}

func (m *mockScanner) ListRepos() []scanner.RepoInfo {

	return m.repos

}

func (m *mockScanner) SearchDocs(query string) []scanner.FileEntry {

	return m.files

}

func (m *mockScanner) GetFileContent(ctx context.Context, repo, path string) (string, error) {

	key := fmt.Sprintf("%s/%s", repo, path)

	if content, ok := m.content[key]; ok {

		return content, nil

	}

	return "", fmt.Errorf("security policy: path '%s' is not indexed as a documentation file", path)

}

func (m *mockScanner) Status() (bool, time.Time, int) {

	return false, time.Time{}, len(m.repos)

}

func (m *mockScanner) TriggerScan() bool { return true }

func TestListReposHandler(t *testing.T) {

	mock := &mockScanner{

		repos: []scanner.RepoInfo{

			{

				Name: "test-org/test-repo",

				FullName: "test-org/test-repo",

				Description: "A test repo",

				HTMLURL: "https://github.com/test-org/test-repo",

				Files: []scanner.FileEntry{

					{RepoName: "test-repo", Path: "README.md", Type: "readme"},
				},
			},
		},
	}

	handler := listReposHandler(mock)

	req := &mcp.CallToolRequest{}

	// The new SDK generic API signature

	res, output, err := handler(t.Context(), req, ListReposArgs{})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	// We expect SDK auto-marshaling, so CallToolResult is nil

	if res != nil && res.IsError {

		t.Fatalf("expected IsError false")

	}

	summaries := output.Repos

	if len(summaries) != 1 {

		t.Fatalf("expected 1 repo summary, got %d", len(summaries))

	}

	if summaries[0].Name != "test-org/test-repo" {

		t.Errorf("expected repo name 'test-org/test-repo', got %s", summaries[0].Name)

	}

	if len(summaries[0].FileTypes) != 1 || summaries[0].FileTypes[0] != "readme" {

		t.Errorf("expected FileTypes ['readme'], got %v", summaries[0].FileTypes)

	}

}

func TestListReposHandler_FileTypeFilter(t *testing.T) {

	mock := &mockScanner{

		repos: []scanner.RepoInfo{

			{

				Name: "org/svc-api",

				FullName: "org/svc-api",

				Files: []scanner.FileEntry{

					{RepoName: "svc-api", Path: "openapi.yaml", Type: "openapi"},
				},
			},

			{

				Name: "org/svc-docs",

				FullName: "org/svc-docs",

				Files: []scanner.FileEntry{

					{RepoName: "svc-docs", Path: "README.md", Type: "readme"},
				},
			},
		},
	}

	handler := listReposHandler(mock)

	req := &mcp.CallToolRequest{}

	// filter to openapi only

	_, output, err := handler(t.Context(), req, ListReposArgs{FileType: "openapi"})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if len(output.Repos) != 1 {

		t.Fatalf("expected 1 repo after filter, got %d", len(output.Repos))

	}

	if output.Repos[0].Name != "org/svc-api" {

		t.Errorf("expected org/svc-api, got %s", output.Repos[0].Name)

	}

	// no filter returns all

	_, all, err := handler(t.Context(), req, ListReposArgs{})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if len(all.Repos) != 2 {

		t.Errorf("expected 2 repos with no filter, got %d", len(all.Repos))

	}

	// filter that matches nothing

	_, none, err := handler(t.Context(), req, ListReposArgs{FileType: "proto"})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if len(none.Repos) != 0 {

		t.Errorf("expected 0 repos for unmatched filter, got %d", len(none.Repos))

	}

}

func TestSearchDocsHandler(t *testing.T) {

	mock := &mockScanner{

		files: []scanner.FileEntry{

			{RepoName: "test-repo", Path: "docs/guide.md", Type: "docs"},
		},
	}

	handler := searchDocsHandler(mock)

	req := &mcp.CallToolRequest{}

	res, output, err := handler(t.Context(), req, SearchDocsArgs{Query: "guide"})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if res != nil && res.IsError {

		t.Fatalf("expected IsError false")

	}

	results := output.Files

	if len(results) != 1 {

		t.Fatalf("expected 1 result, got %d", len(results))

	}

	if results[0].Path != "docs/guide.md" {

		t.Errorf("expected path 'docs/guide.md', got %s", results[0].Path)

	}

}

func TestSearchDocsHandler_FileTypeFilter(t *testing.T) {

	mock := &mockScanner{

		files: []scanner.FileEntry{

			{RepoName: "svc-api", Path: "openapi.yaml", Type: "openapi"},

			{RepoName: "svc-docs", Path: "README.md", Type: "readme"},
		},
	}

	handler := searchDocsHandler(mock)

	req := &mcp.CallToolRequest{}

	// Filter to openapi only.

	_, output, err := handler(t.Context(), req, SearchDocsArgs{Query: "svc", FileType: "openapi"})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if len(output.Files) != 1 || output.Files[0].Type != "openapi" {

		t.Errorf("expected 1 openapi file, got %v", output.Files)

	}

	// No filter returns both files.

	_, all, err := handler(t.Context(), req, SearchDocsArgs{Query: "svc"})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if len(all.Files) != 2 {

		t.Errorf("expected 2 files with no filter, got %d", len(all.Files))

	}

}

func TestGetFileContentHandler(t *testing.T) {

	mock := &mockScanner{

		content: map[string]string{

			"test-repo/docs/guide.md": "# Guide\nThis is a guide.",
		},
	}

	handler := getFileContentHandler(mock, NewDocMetrics())

	// Test successful case

	req := &mcp.CallToolRequest{}

	res, output, err := handler(t.Context(), req, GetFileContentArgs{Repo: "test-repo", Path: "docs/guide.md"})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if res != nil && res.IsError {

		t.Fatalf("expected IsError false")

	}

	if output.Content != "# Guide\nThis is a guide." {

		t.Errorf("unexpected content: %s", output.Content)

	}

	// Test error case (file not indexed)

	req2 := &mcp.CallToolRequest{}

	_, _, err2 := handler(t.Context(), req2, GetFileContentArgs{Repo: "test-repo", Path: "docs/secret.txt"})

	// The new behavior returns a standard Go compilation error handled by the wrapper.

	if err2 == nil {

		t.Errorf("expected error for unindexed file")

	}

}

type mockContentSearcher struct {
	matches []memory.ContentMatch

	count int64

	enabled bool
}

func (m *mockContentSearcher) Search(query, repo, fileType string) ([]memory.ContentMatch, error) {

	if !m.enabled {

		return nil, fmt.Errorf("content search is disabled")

	}

	return m.matches, nil

}

func (m *mockContentSearcher) Count() (int64, error) {

	return m.count, nil

}

func (m *mockContentSearcher) SearchMode() string {

	return "like"

}

type mockGraphStore struct {
	count int64
}

func (m *mockGraphStore) EntityCount() (int64, error) {

	return m.count, nil

}

func (m *mockGraphStore) CreateEntities(entities []memory.Entity) ([]memory.Entity, error) {

	return nil, nil

}

func (m *mockGraphStore) CreateRelations(relations []memory.Relation) ([]memory.Relation, error) {

	return nil, nil

}

func (m *mockGraphStore) AddObservations(observations []memory.Observation) ([]memory.Observation, error) {

	return nil, nil

}

func (m *mockGraphStore) DeleteEntities(entityNames []string) error { return nil }

func (m *mockGraphStore) DeleteObservations(deletions []memory.Observation) error { return nil }

func (m *mockGraphStore) DeleteRelations(relations []memory.Relation) error { return nil }

func (m *mockGraphStore) ReadGraph() (memory.KnowledgeGraph, error) {

	return memory.KnowledgeGraph{}, nil

}

func (m *mockGraphStore) SearchNodes(query string) (memory.KnowledgeGraph, error) {

	return memory.KnowledgeGraph{}, nil

}

func (m *mockGraphStore) SearchNodesFiltered(query string, includeArchived bool) (memory.KnowledgeGraph, error) {

	return memory.KnowledgeGraph{}, nil

}

func (m *mockGraphStore) OpenNodes(names []string) (memory.KnowledgeGraph, error) {

	return memory.KnowledgeGraph{}, nil

}

func (m *mockGraphStore) OpenNodesFiltered(names []string, includeArchived bool) (memory.KnowledgeGraph, error) {

	return memory.KnowledgeGraph{}, nil

}

func (m *mockGraphStore) TraverseGraph(entity, relationType, direction string, maxDepth int) ([]memory.TraverseNode, error) {

	return nil, nil

}

func (m *mockGraphStore) GetIntegrationMap(ctx context.Context, service string, depth int) (memory.IntegrationMap, error) {

	return memory.IntegrationMap{}, nil

}

func (m *mockGraphStore) ListEntities(entityType string) (memory.KnowledgeGraph, error) {

	return memory.KnowledgeGraph{}, nil

}

func (m *mockGraphStore) EntityTypeCounts() (map[string]int64, error) {

	return map[string]int64{"service": m.count}, nil

}

func (m *mockGraphStore) ListRelations(relationType, fromEntity string) ([]memory.Relation, error) {

	return nil, nil

}

func (m *mockGraphStore) FindPath(from, to string, maxDepth int) ([]memory.PathEdge, error) {

	return nil, nil

}

func (m *mockGraphStore) UpdateEntity(oldName, newName, newType string) error { return nil }

func TestGetScanStatusHandler(t *testing.T) {

	sc := &mockScanner{

		repos: []scanner.RepoInfo{

			{Name: "org/svc-a"},

			{Name: "org/svc-b"},
		},
	}

	counter := &mockGraphStore{count: 5}

	searcher := &mockContentSearcher{count: 10, enabled: true}

	handler := getScanStatusHandler(sc, counter, searcher, false)

	req := &mcp.CallToolRequest{}

	_, result, err := handler(t.Context(), req, ScanStatusArgs{})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if result.RepoCount != 2 {

		t.Errorf("expected RepoCount=2, got %d", result.RepoCount)

	}

	if result.GraphEntities != 5 {

		t.Errorf("expected GraphEntities=5, got %d", result.GraphEntities)

	}

	if result.ContentIndexed != 10 {

		t.Errorf("expected ContentIndexed=10, got %d", result.ContentIndexed)

	}

	if !result.ContentEnabled {

		t.Error("expected ContentEnabled=true")

	}

	if result.EntityBreakdown == nil {

		t.Error("expected EntityBreakdown to be populated")

	}

	if result.EntityBreakdown["service"] != 5 {

		t.Errorf("expected EntityBreakdown[service]=5, got %d", result.EntityBreakdown["service"])

	}

}

func TestGetScanStatusHandler_ReadOnly(t *testing.T) {

	sc := &mockScanner{}

	handler := getScanStatusHandler(sc, nil, nil, true)

	req := &mcp.CallToolRequest{}

	_, result, err := handler(t.Context(), req, ScanStatusArgs{})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if !result.ReadOnly {

		t.Error("expected ReadOnly=true when read-only mode is active")

	}

}

func TestRegister_ReadOnly_OmitsMutationTools(t *testing.T) {

	s := newTestServer()

	graph := &mockGraphStore{}

	Register(s, &mockScanner{}, graph, nil, nil, NewToolMetrics(), NewDocMetrics(), true)

	tools := listToolNames(s)

	mutationTools := []string{

		"create_entities", "create_relations", "add_observations",

		"delete_entities", "delete_observations", "delete_relations",
	}

	for _, name := range mutationTools {

		for _, got := range tools {

			if got == name {

				t.Errorf("expected mutation tool %q to be absent in read-only mode", name)

			}

		}

	}

	readOnlyTools := []string{"read_graph", "search_nodes", "open_nodes", "traverse_graph", "get_integration_map"}

	for _, name := range readOnlyTools {

		found := false

		for _, got := range tools {

			if got == name {

				found = true

				break

			}

		}

		if !found {

			t.Errorf("expected read-only tool %q to be present", name)

		}

	}

}

func TestRegister_ReadWrite_IncludesMutationTools(t *testing.T) {

	s := newTestServer()

	graph := &mockGraphStore{}

	Register(s, &mockScanner{}, graph, nil, nil, NewToolMetrics(), NewDocMetrics(), false)

	tools := listToolNames(s)

	mutationTools := []string{

		"create_entities", "create_relations", "add_observations",

		"delete_entities", "delete_observations", "delete_relations",
	}

	for _, name := range mutationTools {

		found := false

		for _, got := range tools {

			if got == name {

				found = true

				break

			}

		}

		if !found {

			t.Errorf("expected mutation tool %q to be present in read-write mode", name)

		}

	}

}

func TestSearchContentHandler_Success(t *testing.T) {

	searcher := &mockContentSearcher{

		enabled: true,

		matches: []memory.ContentMatch{

			{RepoName: "org/payment-svc", Path: "README.md", Snippet: "...handles Stripe payments..."},
		},
	}

	handler := searchContentHandler(searcher, NewDocMetrics())

	req := &mcp.CallToolRequest{}

	_, result, err := handler(t.Context(), req, SearchContentArgs{Query: "stripe"})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if len(result.Matches) != 1 {

		t.Fatalf("expected 1 match, got %d", len(result.Matches))

	}

	if result.Matches[0].RepoName != "org/payment-svc" {

		t.Errorf("wrong repo: %s", result.Matches[0].RepoName)

	}

}

func TestSearchContentHandler_Disabled(t *testing.T) {

	searcher := &mockContentSearcher{enabled: false}

	handler := searchContentHandler(searcher, NewDocMetrics())

	req := &mcp.CallToolRequest{}

	_, _, err := handler(t.Context(), req, SearchContentArgs{Query: "anything"})

	if err == nil {

		t.Error("expected error when content search is disabled")

	}

}

func TestSearchContentHandler_EmptyQuery(t *testing.T) {

	searcher := &mockContentSearcher{enabled: true}

	handler := searchContentHandler(searcher, NewDocMetrics())

	req := &mcp.CallToolRequest{}

	_, _, err := handler(t.Context(), req, SearchContentArgs{Query: ""})

	if err == nil {

		t.Error("expected error for empty query")

	}

}

func TestSearchDocsHandler_WhitespaceQuery(t *testing.T) {

	sc := &mockScanner{}

	handler := searchDocsHandler(sc)

	req := &mcp.CallToolRequest{}

	_, _, err := handler(t.Context(), req, SearchDocsArgs{Query: "   "})

	if err == nil {

		t.Error("expected error for whitespace-only query in search_docs")

	}

}

func TestSearchContentHandler_WhitespaceQuery(t *testing.T) {

	searcher := &mockContentSearcher{enabled: true}

	handler := searchContentHandler(searcher, NewDocMetrics())

	req := &mcp.CallToolRequest{}

	_, _, err := handler(t.Context(), req, SearchContentArgs{Query: "\t\n"})

	if err == nil {

		t.Error("expected error for whitespace-only query in search_content")

	}

}
