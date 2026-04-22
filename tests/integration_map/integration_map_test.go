// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package integration_map_test

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
	"github.com/doc-scout/mcp-server/tools"
)

// callIntegrationMap is a helper that calls get_integration_map and returns the parsed result.
func callIntegrationMap(t *testing.T, session *mcp.ClientSession, args map[string]any) (tools.IntegrationMapResult, bool) {
	t.Helper()
	ctx := t.Context()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_integration_map",
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("get_integration_map call failed: %v", err)
	}
	if res.IsError {
		return tools.IntegrationMapResult{}, true
	}
	if len(res.Content) == 0 {
		t.Fatal("get_integration_map: empty response content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])
	}
	var result tools.IntegrationMapResult
	if err := json.Unmarshal([]byte(text.Text), &result); err != nil {
		t.Fatalf("unmarshal IntegrationMapResult: %v — raw: %s", err, text.Text)
	}
	return result, false
}

// setupIntegrationGraph creates a checkout-service with asyncapi source, event topics, and a calls_service relation.
func setupIntegrationGraph(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	ctx := t.Context()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_entities",
		Arguments: map[string]any{
			"entities": []map[string]any{
				{
					"name":         "checkout-service",
					"entityType":   "service",
					"observations": []string{"_integration_source:asyncapi"},
				},
				{"name": "order.created", "entityType": "event-topic", "observations": []string{}},
				{"name": "payment.approved", "entityType": "event-topic", "observations": []string{}},
				{"name": "payment-service", "entityType": "service", "observations": []string{}},
			},
		},
	})
	if err != nil {
		t.Fatalf("create_entities: %v", err)
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_relations",
		Arguments: map[string]any{
			"relations": []map[string]any{
				{"from": "checkout-service", "to": "order.created", "relationType": "publishes_event"},
				{"from": "checkout-service", "to": "payment.approved", "relationType": "subscribes_event"},
				{"from": "checkout-service", "to": "payment-service", "relationType": "calls_service"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create_relations: %v", err)
	}
}

func TestGetIntegrationMapTool_E2E(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })
	setupIntegrationGraph(t, session)

	got, isError := callIntegrationMap(t, session, map[string]any{
		"service": "checkout-service",
		"depth":   1,
	})
	if isError {
		t.Fatal("expected successful response, got error")
	}

	if got.Service != "checkout-service" {
		t.Errorf("Service = %q, want %q", got.Service, "checkout-service")
	}
	if len(got.Publishes) != 1 || got.Publishes[0].Target != "order.created" {
		t.Errorf("Publishes = %v, want [{order.created}]", got.Publishes)
	}
	if len(got.Subscribes) != 1 || got.Subscribes[0].Target != "payment.approved" {
		t.Errorf("Subscribes = %v, want [{payment.approved}]", got.Subscribes)
	}
	if len(got.Calls) != 1 || got.Calls[0].Target != "payment-service" {
		t.Errorf("Calls = %v, want [{payment-service}]", got.Calls)
	}
	// AsyncAPI is authoritative + k8s inferred → partial (or at minimum not "none")
	if got.Coverage == "none" {
		t.Error("Coverage should not be 'none' when integration data is present")
	}
}

func TestGetIntegrationMapTool_UnknownService(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })

	got, isError := callIntegrationMap(t, session, map[string]any{
		"service": "does-not-exist",
	})
	if isError {
		t.Fatal("CallTool error unexpected for unknown service")
	}
	if got.Coverage != "none" {
		t.Errorf("Coverage = %q, want %q for unknown service", got.Coverage, "none")
	}
}

func TestGetIntegrationMapTool_MissingService(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(func() { _ = session.Close() })

	ctx := t.Context()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_integration_map",
		Arguments: map[string]any{},
	})
	// The SDK may validate required fields and return either a call error or an MCP error response.
	if err == nil && !res.IsError {
		t.Error("expected error response when service is missing")
	}
}
