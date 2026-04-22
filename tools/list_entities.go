// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

type ListEntitiesArgs struct {
	EntityType string `json:"entity_type,omitempty" jsonschema:"optional filter: only return entities of this type (e.g. 'service', 'team', 'event-topic', 'grpc-service', 'api', 'person'). Leave empty to return all entities."`
}

func listEntitiesHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args ListEntitiesArgs) (*mcp.CallToolResult, memory.KnowledgeGraph, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args ListEntitiesArgs) (*mcp.CallToolResult, memory.KnowledgeGraph, error) {

		g, err := graph.ListEntities(args.EntityType)

		if err != nil {

			return nil, memory.KnowledgeGraph{}, err

		}

		return nil, g, nil

	}

}
