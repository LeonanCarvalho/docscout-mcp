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

const maxTraverseDepth = 10

// TraverseGraphArgs defines the input parameters for the traverse_graph tool.

type TraverseGraphArgs struct {
	Entity string `json:"entity"                  jsonschema:"required,The name of the entity to start traversal from (must exist in the graph)."`

	RelationType string `json:"relation_type,omitempty" jsonschema:"Filter edges by relation type (e.g. depends_on, owned_by, consumes_api). Omit to follow all edge types."`

	Direction string `json:"direction,omitempty"     jsonschema:"Edge direction to follow: outgoing (default) follows edges from the entity; incoming follows edges pointing to it; both follows all edges."`

	Depth int `json:"depth,omitempty"         jsonschema:"Number of hops to follow (1–10, default 1). depth=1 returns direct neighbours only; depth=2 includes their neighbours, and so on."`
}

// TraverseGraphResult is the structured output of the traverse_graph tool.

type TraverseGraphResult struct {
	StartEntity string `json:"start_entity"`

	Nodes []memory.TraverseNode `json:"nodes"`

	TotalFound int `json:"total_found"`
}

func traverseGraphHandler(g GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args TraverseGraphArgs) (*mcp.CallToolResult, TraverseGraphResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args TraverseGraphArgs) (*mcp.CallToolResult, TraverseGraphResult, error) {

		if strings.TrimSpace(args.Entity) == "" {

			return nil, TraverseGraphResult{}, fmt.Errorf("parameter 'entity' is required")

		}

		direction := args.Direction

		if direction == "" {

			direction = "outgoing"

		}

		switch direction {

		case "outgoing", "incoming", "both":

		default:

			return nil, TraverseGraphResult{}, fmt.Errorf("parameter 'direction' must be 'outgoing', 'incoming', or 'both'; got %q", direction)

		}

		depth := args.Depth

		if depth < 1 {

			depth = 1

		}

		if depth > maxTraverseDepth {

			depth = maxTraverseDepth

		}

		nodes, err := g.TraverseGraph(args.Entity, args.RelationType, direction, depth)

		if err != nil {

			return nil, TraverseGraphResult{}, fmt.Errorf("traverse_graph: %w", err)

		}

		return nil, TraverseGraphResult{

			StartEntity: args.Entity,

			Nodes: nodes,

			TotalFound: len(nodes),
		}, nil

	}

}
