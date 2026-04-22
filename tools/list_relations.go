// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

type ListRelationsArgs struct {
	RelationType string `json:"relation_type,omitempty" jsonschema:"optional filter: only return relations of this type (e.g. 'depends_on', 'publishes_event', 'subscribes_event', 'exposes_api', 'provides_grpc', 'calls_service', 'owns', 'part_of'). Leave empty to return all relation types."`

	FromEntity string `json:"from_entity,omitempty" jsonschema:"optional filter: only return relations originating from this entity name. Leave empty to return relations from all entities."`
}

type ListRelationsResult struct {
	Relations []memory.Relation `json:"relations"`

	Count int `json:"count"`
}

func listRelationsHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args ListRelationsArgs) (*mcp.CallToolResult, ListRelationsResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args ListRelationsArgs) (*mcp.CallToolResult, ListRelationsResult, error) {

		relations, err := graph.ListRelations(args.RelationType, args.FromEntity)

		if err != nil {

			return nil, ListRelationsResult{}, err

		}

		return nil, ListRelationsResult{Relations: relations, Count: len(relations)}, nil

	}

}
