// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

func TestSpringKafkaParser_FileTypeAndFilenames(t *testing.T) {
	p := parser.SpringKafkaParser()
	if p.FileType() != "spring-kafka" {
		t.Errorf("FileType = %q, want %q", p.FileType(), "spring-kafka")
	}
	wantNames := map[string]bool{
		"application.yml":        true,
		"application.yaml":       true,
		"application.properties": true,
	}
	for _, fn := range p.Filenames() {
		if !wantNames[fn] {
			t.Errorf("unexpected filename %q", fn)
		}
	}
}

func TestSpringKafkaParser_Parse_YAML(t *testing.T) {
	input := []byte(`
spring:
  kafka:
    producer:
      topic: order.created
    consumer:
      topics: payment.approved, fraud.checked
`)
	p := parser.SpringKafkaParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Produces only relations, no primary entity (entity name filled by indexer from repo).
	relByType := make(map[string][]string)
	for _, r := range got.Relations {
		relByType[r.RelationType] = append(relByType[r.RelationType], r.To)
	}
	if len(relByType["publishes_event"]) != 1 || relByType["publishes_event"][0] != "order.created" {
		t.Errorf("publishes_event = %v, want [order.created]", relByType["publishes_event"])
	}
	if len(relByType["subscribes_event"]) != 2 {
		t.Errorf("subscribes_event count = %d, want 2: %v", len(relByType["subscribes_event"]), relByType["subscribes_event"])
	}

	// Relations From should be empty (indexer fills with repo service name).
	for _, r := range got.Relations {
		if r.From != "" {
			t.Errorf("From should be empty (filled by indexer), got %q", r.From)
		}
	}
}

func TestSpringKafkaParser_Parse_Properties(t *testing.T) {
	input := []byte(`
spring.kafka.producer.topic=order.created
spring.kafka.consumer.topics=payment.approved,fraud.checked
`)
	p := parser.SpringKafkaParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	relByType := make(map[string][]string)
	for _, r := range got.Relations {
		relByType[r.RelationType] = append(relByType[r.RelationType], r.To)
	}
	if len(relByType["publishes_event"]) != 1 {
		t.Errorf("publishes_event = %v, want 1", relByType["publishes_event"])
	}
	if len(relByType["subscribes_event"]) != 2 {
		t.Errorf("subscribes_event count = %d, want 2", len(relByType["subscribes_event"]))
	}
}

func TestSpringKafkaParser_Parse_SkipsPlaceholders(t *testing.T) {
	input := []byte(`
spring.kafka.producer.topic=${TOPIC_ENV}
spring.kafka.consumer.topics=real.topic
`)
	p := parser.SpringKafkaParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range got.Relations {
		if r.RelationType == "publishes_event" {
			t.Errorf("placeholder topic should be skipped, got relation to %q", r.To)
		}
	}
}

func TestSpringKafkaParser_Parse_NoKafkaConfig(t *testing.T) {
	input := []byte(`
server:
  port: 8080
`)
	p := parser.SpringKafkaParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Relations) != 0 {
		t.Errorf("expected no relations for non-kafka config, got %d", len(got.Relations))
	}
}
