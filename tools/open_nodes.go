// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

type OpenNodesArgs struct {
	Names []string `json:"names" jsonschema:"names of nodes to open"`

	IncludeArchived bool `json:"include_archived,omitempty" jsonschema:"When true, includes entities marked as archived (_status:archived). Default false — archived entities are hidden from results."`
}

func openNodesHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args OpenNodesArgs) (*mcp.CallToolResult, memory.KnowledgeGraph, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args OpenNodesArgs) (*mcp.CallToolResult, memory.KnowledgeGraph, error) {

		g, err := graph.OpenNodesFiltered(args.Names, args.IncludeArchived)

		if err != nil {

			return nil, memory.KnowledgeGraph{}, err

		}

		return nil, g, nil

	}

}
