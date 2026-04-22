// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

type SearchContentArgs struct {
	Query string `json:"query" jsonschema:"The term to search for inside documentation content. Use natural language terms like 'payment', 'authentication', 'event sourcing'."`

	Repo string `json:"repo,omitempty" jsonschema:"Optional: filter results to a single repository name (e.g. 'org/payment-service')."`

	FileType string `json:"file_type,omitempty" jsonschema:"Optional: filter by file classification. Common values: 'readme', 'docs', 'openapi', 'catalog', 'proto', 'asyncapi', 'helm', 'terraform', 'workflow'. Leave empty to search all file types."`
}

type SearchContentResult struct {
	Matches []memory.ContentMatch `json:"matches" jsonschema:"List of files containing the query term, with a snippet showing the matched context."`
}

func searchContentHandler(search ContentSearcher, docMetrics *DocMetrics) func(ctx context.Context, req *mcp.CallToolRequest, args SearchContentArgs) (*mcp.CallToolResult, SearchContentResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args SearchContentArgs) (*mcp.CallToolResult, SearchContentResult, error) {

		if strings.TrimSpace(args.Query) == "" {

			return nil, SearchContentResult{}, fmt.Errorf("parameter 'query' must not be empty or whitespace-only")

		}

		matches, err := search.Search(args.Query, args.Repo, args.FileType)

		if err != nil {

			return nil, SearchContentResult{}, err

		}

		for _, m := range matches {

			docMetrics.Record(m.RepoName, m.Path)

		}

		return nil, SearchContentResult{Matches: matches}, nil

	}

}
