// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/embeddings"
)

type SemanticSearchArgs struct {
	Query string `json:"query"            jsonschema:"Natural-language search query. Required."`

	Target string `json:"target,omitempty" jsonschema:"What to search: 'content' (indexed docs), 'entities' (knowledge graph), or 'both'. Defaults to 'both'."`

	TopK int `json:"top_k,omitempty"  jsonschema:"Maximum number of results per target (default 5, max 20)."`

	Repo string `json:"repo,omitempty"   jsonschema:"Optional: scope content search to a single repository full name (e.g. 'org/payment-service')."`
}

type SemanticSearchResult struct {
	ContentResults []embeddings.DocResult `json:"content_results,omitempty"`

	EntityResults []embeddings.EntityResult `json:"entity_results,omitempty"`

	StaleDocs int `json:"stale_docs"`

	StaleEntities int `json:"stale_entities"`
}

func semanticSearchHandler(semantic SemanticSearch) func(ctx context.Context, req *mcp.CallToolRequest, args SemanticSearchArgs) (*mcp.CallToolResult, SemanticSearchResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args SemanticSearchArgs) (*mcp.CallToolResult, SemanticSearchResult, error) {

		if strings.TrimSpace(args.Query) == "" {

			return nil, SemanticSearchResult{}, fmt.Errorf("parameter 'query' must not be empty")

		}

		if !semantic.Enabled() {

			return nil, SemanticSearchResult{}, fmt.Errorf("semantic search not enabled: set DOCSCOUT_EMBED_OPENAI_KEY or DOCSCOUT_EMBED_OLLAMA_URL")

		}

		target := strings.ToLower(strings.TrimSpace(args.Target))

		if target == "" {

			target = "both"

		}

		if target != "content" && target != "entities" && target != "both" {

			return nil, SemanticSearchResult{}, fmt.Errorf("invalid target %q: must be 'content', 'entities', or 'both'", target)

		}

		topK := args.TopK

		if topK <= 0 {

			topK = 5

		}

		if topK > 20 {

			topK = 20

		}

		var result SemanticSearchResult

		if target == "content" || target == "both" {

			docs, stale, err := semantic.SearchDocs(ctx, args.Query, args.Repo, topK)

			if err != nil {

				return nil, SemanticSearchResult{}, fmt.Errorf("search docs: %w", err)

			}

			result.ContentResults = docs

			result.StaleDocs = stale

		}

		if target == "entities" || target == "both" {

			entities, stale, err := semantic.SearchEntities(ctx, args.Query, topK)

			if err != nil {

				return nil, SemanticSearchResult{}, fmt.Errorf("search entities: %w", err)

			}

			result.EntityResults = entities

			result.StaleEntities = stale

		}

		return nil, result, nil

	}

}
