// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/doc-scout/mcp-server/memory"
)

func TestAuditStore_WriteAndQuery(t *testing.T) {
	ctx := t.Context()
	db, err := memory.OpenDB("")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	store, err := memory.NewAuditStore(db)
	if err != nil {
		t.Fatalf("NewAuditStore: %v", err)
	}

	err = store.Write(ctx, memory.AuditEvent{
		Agent: "test-agent", Tool: "create_entities", Operation: "create",
		Targets: memory.MarshalTargets([]string{"svc-a"}), Count: 1, Outcome: "ok",
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	events, total, err := store.Query(ctx, memory.AuditFilter{Agent: "test-agent"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Fatalf("want total=1, got %d", total)
	}
	if len(events) != 1 || events[0].Agent != "test-agent" {
		t.Fatalf("unexpected events: %v", events)
	}
	if events[0].ID == "" {
		t.Fatal("ID must be set (UUIDv7)")
	}
}

func TestAuditStore_UUIDv7Order(t *testing.T) {
	ctx := t.Context()
	db, _ := memory.OpenDB("")
	store, _ := memory.NewAuditStore(db)

	for range 3 {
		_ = store.Write(ctx, memory.AuditEvent{
			Agent: "a", Tool: "create_entities", Operation: "create",
			Targets: memory.MarshalTargets([]string{"e"}), Count: 1, Outcome: "ok",
		})
		time.Sleep(time.Millisecond)
	}

	events, _, _ := store.Query(ctx, memory.AuditFilter{Limit: 10})
	for i := 1; i < len(events); i++ {
		if events[i].ID <= events[i-1].ID {
			t.Fatalf("events not in UUIDv7 (chronological) order: %s <= %s", events[i].ID, events[i-1].ID)
		}
	}
}

func TestAuditStore_SummaryRiskyMassDelete(t *testing.T) {
	ctx := t.Context()
	db, _ := memory.OpenDB("")
	store, _ := memory.NewAuditStore(db)

	_ = store.Write(ctx, memory.AuditEvent{
		Agent: "bot", Tool: "delete_entities", Operation: "delete",
		Targets: memory.MarshalTargets([]string{}), Count: 15, Outcome: "ok",
	})

	summary, err := store.Summary(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if len(summary.RiskyEvents) == 0 {
		t.Fatal("expected mass delete to appear in risky_events")
	}
}

// Ensure *DBAuditStore satisfies the AuditStore interface at compile time.
var _ memory.AuditStore = (*memory.DBAuditStore)(nil)

// Suppress unused import warning — context is used via t.Context().
var _ context.Context
