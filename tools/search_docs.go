// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/scanner"
)

type SearchDocsArgs struct {
	Query string `json:"query" jsonschema:"The search term to match against file paths and repo names."`

	FileType string `json:"file_type,omitempty" jsonschema:"optional filter: only return files of this type (e.g. 'openapi', 'asyncapi', 'proto', 'readme', 'docs', 'helm'). Leave empty to return all matching files."`
}

type SearchDocsResult struct {
	Files []scanner.FileEntry `json:"files" jsonschema:"List of files matching the query"`
}

func searchDocsHandler(sc DocumentScanner) func(ctx context.Context, req *mcp.CallToolRequest, args SearchDocsArgs) (*mcp.CallToolResult, SearchDocsResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args SearchDocsArgs) (*mcp.CallToolResult, SearchDocsResult, error) {

		if strings.TrimSpace(args.Query) == "" {

			return nil, SearchDocsResult{}, fmt.Errorf("parameter 'query' must not be empty or whitespace-only")

		}

		results := sc.SearchDocs(args.Query)

		if args.FileType != "" {

			filtered := results[:0]

			for _, f := range results {

				if f.Type == args.FileType {

					filtered = append(filtered, f)

				}

			}

			results = filtered

		}

		return nil, SearchDocsResult{Files: results}, nil

	}

}
