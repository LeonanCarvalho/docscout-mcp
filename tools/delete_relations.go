// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

type DeleteRelationsArgs struct {
	Relations []memory.Relation `json:"relations" jsonschema:"relations to delete"`
}

func deleteRelationsHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args DeleteRelationsArgs) (*mcp.CallToolResult, struct{}, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args DeleteRelationsArgs) (*mcp.CallToolResult, struct{}, error) {

		err := graph.DeleteRelations(args.Relations)

		return nil, struct{}{}, err

	}

}
