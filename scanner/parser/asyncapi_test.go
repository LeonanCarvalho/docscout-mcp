// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

func TestAsyncAPIParser_FileTypeAndFilenames(t *testing.T) {
	p := parser.AsyncAPIParser()
	if p.FileType() != "asyncapi" {
		t.Errorf("FileType = %q, want %q", p.FileType(), "asyncapi")
	}
	wantNames := map[string]bool{"asyncapi.yaml": true, "asyncapi.json": true}
	for _, fn := range p.Filenames() {
		if !wantNames[fn] {
			t.Errorf("unexpected filename %q", fn)
		}
	}
	if len(p.Filenames()) != len(wantNames) {
		t.Errorf("Filenames len=%d, want %d", len(p.Filenames()), len(wantNames))
	}
}

func TestAsyncAPIParser_Parse_PublishAndSubscribe(t *testing.T) {
	input := []byte(`
asyncapi: "2.6.0"
info:
  title: order-service
channels:
  order.created:
    publish:
      message:
        name: OrderCreatedEvent
  payment.approved:
    subscribe:
      message:
        name: PaymentApprovedEvent
`)
	p := parser.AsyncAPIParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.EntityName != "order-service" {
		t.Errorf("EntityName = %q, want %q", got.EntityName, "order-service")
	}
	if got.EntityType != "service" {
		t.Errorf("EntityType = %q, want %q", got.EntityType, "service")
	}

	// Should produce an event-topic entity for each channel.
	auxByName := make(map[string]parser.AuxEntity)
	for _, a := range got.AuxEntities {
		auxByName[a.Name] = a
	}
	if _, ok := auxByName["order.created"]; !ok {
		t.Error("missing aux entity for order.created")
	}
	if auxByName["order.created"].EntityType != "event-topic" {
		t.Errorf("event-topic type = %q, want %q", auxByName["order.created"].EntityType, "event-topic")
	}

	// Check observations on event-topic entity.
	obsMap := make(map[string]bool)
	for _, o := range auxByName["order.created"].Observations {
		obsMap[o] = true
	}
	if !obsMap["schema:OrderCreatedEvent"] {
		t.Error("missing schema observation on order.created")
	}

	// Should produce publishes_event and subscribes_event relations.
	relByType := make(map[string][]parser.ParsedRelation)
	for _, r := range got.Relations {
		relByType[r.RelationType] = append(relByType[r.RelationType], r)
	}
	if len(relByType["publishes_event"]) != 1 || relByType["publishes_event"][0].To != "order.created" {
		t.Errorf("publishes_event relations = %v", relByType["publishes_event"])
	}
	if len(relByType["subscribes_event"]) != 1 || relByType["subscribes_event"][0].To != "payment.approved" {
		t.Errorf("subscribes_event relations = %v", relByType["subscribes_event"])
	}
}

func TestAsyncAPIParser_Parse_EmptyChannels(t *testing.T) {
	input := []byte(`
asyncapi: "2.6.0"
info:
  title: quiet-service
channels: {}
`)
	p := parser.AsyncAPIParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Relations) != 0 {
		t.Errorf("expected no relations, got %d", len(got.Relations))
	}
}

func TestAsyncAPIParser_Parse_MissingTitle(t *testing.T) {
	input := []byte(`
asyncapi: "2.6.0"
info: {}
channels: {}
`)
	p := parser.AsyncAPIParser()
	_, err := p.Parse(input)
	if err == nil {
		t.Error("expected error when info.title is missing")
	}
}
