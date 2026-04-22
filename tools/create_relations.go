// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

type CreateRelationsArgs struct {
	Relations []memory.Relation `json:"relations" jsonschema:"relations to create"`
}

type CreateRelationsResult struct {
	Relations []memory.Relation `json:"relations"`
}

func createRelationsHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args CreateRelationsArgs) (*mcp.CallToolResult, CreateRelationsResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args CreateRelationsArgs) (*mcp.CallToolResult, CreateRelationsResult, error) {

		relations, err := graph.CreateRelations(args.Relations)

		if err != nil {

			return nil, CreateRelationsResult{}, err

		}

		return nil, CreateRelationsResult{Relations: relations}, nil

	}

}
