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

type FindPathArgs struct {
	From string `json:"from"               jsonschema:"Name of the starting entity."`

	To string `json:"to"                 jsonschema:"Name of the destination entity."`

	MaxDepth int `json:"max_depth,omitempty" jsonschema:"Maximum number of hops to search (1–10). Defaults to 6 when omitted."`
}

type FindPathResult struct {

	// Path is the ordered sequence of directed edges from 'from' to 'to'.

	// Empty when no connection was found within the depth limit.

	Path []memory.PathEdge `json:"path"`

	Found bool `json:"found"`

	Hops int `json:"hops"`
}

func findPathHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args FindPathArgs) (*mcp.CallToolResult, FindPathResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args FindPathArgs) (*mcp.CallToolResult, FindPathResult, error) {

		if strings.TrimSpace(args.From) == "" {

			return nil, FindPathResult{}, fmt.Errorf("parameter 'from' is required")

		}

		if strings.TrimSpace(args.To) == "" {

			return nil, FindPathResult{}, fmt.Errorf("parameter 'to' is required")

		}

		maxDepth := args.MaxDepth

		if maxDepth <= 0 {

			maxDepth = 6

		}

		// Same entity: trivially connected with 0 hops.

		if args.From == args.To {

			return nil, FindPathResult{Path: []memory.PathEdge{}, Found: true, Hops: 0}, nil

		}

		edges, err := graph.FindPath(args.From, args.To, maxDepth)

		if err != nil {

			return nil, FindPathResult{}, fmt.Errorf("find_path: %w", err)

		}

		return nil, FindPathResult{

			Path: edges,

			Found: len(edges) > 0,

			Hops: len(edges),
		}, nil

	}

}
