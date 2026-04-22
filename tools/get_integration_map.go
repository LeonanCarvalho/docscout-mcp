// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

// IntegrationMapArgs are the inputs for get_integration_map.

type IntegrationMapArgs struct {
	Service string `json:"service"         jsonschema:"required,Entity name of the service in the knowledge graph (e.g. 'checkout-service')."`

	Depth int `json:"depth,omitempty" jsonschema:"Number of integration hops to include (1-3, default 1). depth=1 returns direct integrations only."`
}

// IntegrationEdgeJSON is a single integration edge in the MCP response.

type IntegrationEdgeJSON struct {
	Target string `json:"target"`

	Schema string `json:"schema,omitempty"`

	Version string `json:"version,omitempty"`

	Paths int `json:"paths,omitempty"`

	Confidence string `json:"confidence"`

	SourceRepo string `json:"source_repo,omitempty"`
}

// IntegrationMapResult is the output of get_integration_map.

type IntegrationMapResult struct {
	Service string `json:"service"`

	Publishes []IntegrationEdgeJSON `json:"publishes"`

	Subscribes []IntegrationEdgeJSON `json:"subscribes"`

	ExposesAPI []IntegrationEdgeJSON `json:"exposes_api"`

	ProvidesGRPC []IntegrationEdgeJSON `json:"provides_grpc"`

	GRPCDeps []IntegrationEdgeJSON `json:"grpc_deps"`

	Calls []IntegrationEdgeJSON `json:"calls"`

	Coverage string `json:"graph_coverage" jsonschema:"Confidence level: full (authoritative), partial (mixed), inferred (heuristics only), or none (no data)."`
}

func convertEdges(edges []memory.IntegrationEdge) []IntegrationEdgeJSON {

	out := make([]IntegrationEdgeJSON, len(edges))

	for i, e := range edges {

		out[i] = IntegrationEdgeJSON{

			Target: e.Target,

			Schema: e.Schema,

			Version: e.Version,

			Paths: e.Paths,

			Confidence: e.Confidence,

			SourceRepo: e.SourceRepo,
		}

	}

	return out

}

func getIntegrationMapHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args IntegrationMapArgs) (*mcp.CallToolResult, IntegrationMapResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args IntegrationMapArgs) (*mcp.CallToolResult, IntegrationMapResult, error) {

		if args.Service == "" {

			return nil, IntegrationMapResult{}, fmt.Errorf("service is required")

		}

		depth := args.Depth

		if depth < 1 {

			depth = 1

		}

		if depth > 3 {

			depth = 3

		}

		m, err := graph.GetIntegrationMap(ctx, args.Service, depth)

		if err != nil {

			return nil, IntegrationMapResult{}, fmt.Errorf("GetIntegrationMap: %w", err)

		}

		return nil, IntegrationMapResult{

			Service: m.Service,

			Publishes: convertEdges(m.Publishes),

			Subscribes: convertEdges(m.Subscribes),

			ExposesAPI: convertEdges(m.ExposesAPI),

			ProvidesGRPC: convertEdges(m.ProvidesGRPC),

			GRPCDeps: convertEdges(m.GRPCDeps),

			Calls: convertEdges(m.Calls),

			Coverage: m.Coverage,
		}, nil

	}

}
