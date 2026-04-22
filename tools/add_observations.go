// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

type AddObservationsArgs struct {
	Observations []memory.Observation `json:"observations" jsonschema:"observations to add"`
}

type AddObservationsResult struct {
	Observations []memory.Observation `json:"observations"`

	// Skipped lists observations rejected by quality guards (empty, too short/long, duplicate).

	// Non-nil only when at least one observation was filtered.

	Skipped []SkippedObservation `json:"skipped,omitempty"`
}

func addObservationsHandler(graph GraphStore, semantic SemanticSearch) func(ctx context.Context, req *mcp.CallToolRequest, args AddObservationsArgs) (*mcp.CallToolResult, AddObservationsResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args AddObservationsArgs) (*mcp.CallToolResult, AddObservationsResult, error) {

		var clean []memory.Observation

		var allSkipped []SkippedObservation

		for _, obs := range args.Observations {

			valid, skipped := sanitizeObservations(obs.EntityName, obs.Contents)

			allSkipped = append(allSkipped, skipped...)

			if len(valid) > 0 {

				clean = append(clean, memory.Observation{

					EntityName: obs.EntityName,

					Contents: valid,
				})

			}

		}

		if len(clean) == 0 {

			return nil, AddObservationsResult{Skipped: allSkipped}, nil

		}

		observations, err := graph.AddObservations(clean)

		if err != nil {

			return nil, AddObservationsResult{}, err

		}

		if semantic != nil {

			seen := make(map[string]bool)

			for _, obs := range observations {

				seen[obs.EntityName] = true

			}

			names := make([]string, 0, len(seen))

			for n := range seen {

				names = append(names, n)

			}

			semantic.ScheduleIndexEntities(names)

		}

		return nil, AddObservationsResult{Observations: observations, Skipped: allSkipped}, nil

	}

}
