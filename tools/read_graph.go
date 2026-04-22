// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

func readGraphHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, memory.KnowledgeGraph, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, memory.KnowledgeGraph, error) {

		g, err := graph.ReadGraph()

		if err != nil {

			return nil, memory.KnowledgeGraph{}, err

		}

		return nil, g, nil

	}

}
