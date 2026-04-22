// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package testutils

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/scanner"
	"github.com/doc-scout/mcp-server/tools"
)

type MockScanner struct{}

func (m *MockScanner) ListRepos() []scanner.RepoInfo {

	return []scanner.RepoInfo{

		{

			Name: "test-repo",

			FullName: "test-org/test-repo",

			Description: "A test repository",

			HTMLURL: "https://github.com/test-org/test-repo",

			Files: []scanner.FileEntry{

				{RepoName: "test-repo", Path: "README.md", Type: "readme"},

				{RepoName: "test-repo", Path: "docs/guide.md", Type: "docs"},
			},
		},
	}

}

func (m *MockScanner) SearchDocs(query string) []scanner.FileEntry {

	if query == "guide" {

		return []scanner.FileEntry{

			{RepoName: "test-repo", Path: "docs/guide.md", Type: "docs"},
		}

	}

	return nil

}

func (m *MockScanner) GetFileContent(ctx context.Context, repo, path string) (string, error) {

	if repo == "test-repo" && path == "README.md" {

		return "# Test Repo\nThis is a test.", nil

	}

	return "", nil

}

func (m *MockScanner) Status() (bool, time.Time, int) {

	return false, time.Now(), 1

}

func (m *MockScanner) TriggerScan() bool {

	return true

}

func SetupTestServer(t *testing.T) *mcp.ClientSession {

	t.Helper()

	ctx := t.Context()

	server := mcp.NewServer(&mcp.Implementation{

		Name: "docscout-mcp-test",

		Version: "test",
	}, nil)

	db, err := memory.OpenDB("")

	if err != nil {

		t.Fatalf("memory.OpenDB: %v", err)

	}

	memorySrv := memory.NewMemoryService(db)

	// Register scanner tools and memory tools

	tools.Register(server, &MockScanner{}, memorySrv, nil, nil, tools.NewToolMetrics(), tools.NewDocMetrics(), false)

	t1, t2 := mcp.NewInMemoryTransports()

	if _, err := server.Connect(ctx, t1, nil); err != nil {

		t.Fatalf("server connect: %v", err)

	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)

	session, err := client.Connect(ctx, t2, nil)

	if err != nil {

		t.Fatalf("client connect: %v", err)

	}

	return session

}
