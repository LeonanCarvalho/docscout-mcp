// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory_test

import (
	"context"
	"testing"

	"github.com/doc-scout/mcp-server/memory"
)

func setupIntegrationTestDB(t *testing.T) *memory.MemoryService {
	t.Helper()
	// Use a unique in-memory SQLite name per test to avoid state pollution across tests.
	db, err := memory.OpenDB("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	return memory.NewMemoryService(db)
}

func TestGetIntegrationMap_None(t *testing.T) {
	svc := setupIntegrationTestDB(t)
	result, err := svc.GetIntegrationMap(context.Background(), "unknown-service", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Coverage != "none" {
		t.Errorf("Coverage = %q, want %q", result.Coverage, "none")
	}
	if len(result.Publishes) != 0 || len(result.Subscribes) != 0 || len(result.Calls) != 0 {
		t.Error("expected empty integration map for unknown service")
	}
}

func TestGetIntegrationMap_AuthoritativeSource(t *testing.T) {
	svc := setupIntegrationTestDB(t)

	// Create service entity with authoritative integration source.
	_, err := svc.CreateEntities([]memory.Entity{
		{Name: "checkout-service", EntityType: "service", Observations: []string{"_integration_source:asyncapi"}},
		{Name: "order.created", EntityType: "event-topic"},
		{Name: "payment.approved", EntityType: "event-topic"},
	})
	if err != nil {
		t.Fatalf("CreateEntities: %v", err)
	}

	_, err = svc.CreateRelations([]memory.Relation{
		{From: "checkout-service", To: "order.created", RelationType: "publishes_event"},
		{From: "checkout-service", To: "payment.approved", RelationType: "subscribes_event"},
	})
	if err != nil {
		t.Fatalf("CreateRelations: %v", err)
	}

	result, err := svc.GetIntegrationMap(context.Background(), "checkout-service", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Coverage != "full" {
		t.Errorf("Coverage = %q, want %q (authoritative source present)", result.Coverage, "full")
	}
	if len(result.Publishes) != 1 || result.Publishes[0].Target != "order.created" {
		t.Errorf("Publishes = %v", result.Publishes)
	}
	if len(result.Subscribes) != 1 || result.Subscribes[0].Target != "payment.approved" {
		t.Errorf("Subscribes = %v", result.Subscribes)
	}
	if result.Publishes[0].Confidence != "authoritative" {
		t.Errorf("Confidence = %q, want authoritative", result.Publishes[0].Confidence)
	}
}

func TestGetIntegrationMap_InferredSource(t *testing.T) {
	svc := setupIntegrationTestDB(t)

	_, err := svc.CreateEntities([]memory.Entity{
		{Name: "checkout-service", EntityType: "service", Observations: []string{"_integration_source:k8s-env"}},
		{Name: "payment-service", EntityType: "service"},
	})
	if err != nil {
		t.Fatalf("CreateEntities: %v", err)
	}

	_, err = svc.CreateRelations([]memory.Relation{
		{From: "checkout-service", To: "payment-service", RelationType: "calls_service"},
	})
	if err != nil {
		t.Fatalf("CreateRelations: %v", err)
	}

	result, err := svc.GetIntegrationMap(context.Background(), "checkout-service", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Coverage != "inferred" {
		t.Errorf("Coverage = %q, want inferred", result.Coverage)
	}
	if len(result.Calls) != 1 || result.Calls[0].Target != "payment-service" {
		t.Errorf("Calls = %v", result.Calls)
	}
	if result.Calls[0].Confidence != "inferred" {
		t.Errorf("Confidence = %q, want inferred", result.Calls[0].Confidence)
	}
}

func TestGetIntegrationMap_PartialCoverage(t *testing.T) {
	svc := setupIntegrationTestDB(t)

	// Mix of authoritative (asyncapi) and inferred (k8s-env) sources.
	_, err := svc.CreateEntities([]memory.Entity{
		{Name: "checkout-service", EntityType: "service", Observations: []string{
			"_integration_source:asyncapi",
			"_integration_source:k8s-env",
		}},
		{Name: "order.created", EntityType: "event-topic"},
		{Name: "payment-service", EntityType: "service"},
	})
	if err != nil {
		t.Fatalf("CreateEntities: %v", err)
	}

	_, err = svc.CreateRelations([]memory.Relation{
		{From: "checkout-service", To: "order.created", RelationType: "publishes_event"},
		{From: "checkout-service", To: "payment-service", RelationType: "calls_service"},
	})
	if err != nil {
		t.Fatalf("CreateRelations: %v", err)
	}

	result, err := svc.GetIntegrationMap(context.Background(), "checkout-service", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Coverage != "partial" {
		t.Errorf("Coverage = %q, want partial", result.Coverage)
	}
}
