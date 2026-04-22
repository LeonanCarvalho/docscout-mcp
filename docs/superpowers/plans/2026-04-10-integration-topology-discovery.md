# Integration Topology Discovery (#15) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Prerequisite:** Branch `feat/custom-parser-extension` must be merged (or stacked — see Task 1).

**Goal:** Automatically populate the knowledge graph with producer/consumer and API dependency relationships during each scan, and expose a `get_integration_map` MCP tool that returns the complete integration picture of a service in a single call.

**Architecture:** Five new `FileParser` implementations (AsyncAPI, Spring Kafka, OpenAPI, Proto, K8s env heuristic) feed integration edges into the knowledge graph via the registry added in #13. A new `memory/integration.go` provides targeted SQL queries per relation type. The `get_integration_map` MCP tool assembles the result and computes a `graph_coverage` confidence field.

**Tech Stack:** Go 1.26+, GORM, `gopkg.in/yaml.v3` (already in go.mod), no new external dependencies. Builds on `FileParser` interface from #13.

**Branch:** `feat/integration-topology-discovery` (stacked on `feat/custom-parser-extension`)

---

## File Map

| File                                            | Action     | Responsibility                                                           |
| ----------------------------------------------- | ---------- | ------------------------------------------------------------------------ |
| `scanner/parser/asyncapi.go`                    | **Create** | Parse AsyncAPI channels → `publishes_event`/`subscribes_event` relations |
| `scanner/parser/asyncapi_test.go`               | **Create** | Unit tests                                                               |
| `scanner/parser/springkafka.go`                 | **Create** | Parse `application.yml`/`.properties` Kafka config → topic relations     |
| `scanner/parser/springkafka_test.go`            | **Create** | Unit tests                                                               |
| `scanner/parser/openapi.go`                     | **Create** | Parse OpenAPI/Swagger `info` → `exposes_api` relation                    |
| `scanner/parser/openapi_test.go`                | **Create** | Unit tests                                                               |
| `scanner/parser/proto.go`                       | **Create** | Line-scan `.proto` files → `provides_grpc`/`depends_on_grpc` relations   |
| `scanner/parser/proto_test.go`                  | **Create** | Unit tests                                                               |
| `scanner/parser/k8sintegration.go`              | **Create** | Scan K8s Deployment env vars → `calls_service` relations                 |
| `scanner/parser/k8sintegration_test.go`         | **Create** | Unit tests                                                               |
| `scanner/scanner.go`                            | **Modify** | Add `.proto` to `infraExtensions`                                        |
| `memory/integration.go`                         | **Create** | `GetIntegrationMap` SQL queries + `IntegrationMap` types                 |
| `memory/integration_test.go`                    | **Create** | Unit tests for `GetIntegrationMap`                                       |
| `memory/memory.go`                              | **Modify** | Expose `GetIntegrationMap` on `MemoryService`                            |
| `tools/ports.go`                                | **Modify** | Add `GetIntegrationMap` to `GraphStore` interface                        |
| `tools/audit.go`                                | **Modify** | Add `GetIntegrationMap` read-only pass-through                           |
| `tools/get_integration_map.go`                  | **Create** | MCP tool handler                                                         |
| `tools/tools.go`                                | **Modify** | Register `get_integration_map` tool                                      |
| `main.go`                                       | **Modify** | Register 5 new parsers in `parser.Default`                               |
| `tests/integration_map/integration_map_test.go` | **Create** | E2E tests                                                                |
| `AGENTS.md`                                     | **Modify** | Update §7 with new relation types and tool usage                         |

---

## Task 1: Create stacked branch

- [ ] **Step 1: Create branch from feat/custom-parser-extension**

```bash
cd /mnt/e/DEV/mcpdocs
git checkout feat/custom-parser-extension
git checkout -b feat/integration-topology-discovery
```

Expected: `Switched to a new branch 'feat/integration-topology-discovery'`

---

## Task 2: AsyncAPIParser

**Files:**

- Create: `scanner/parser/asyncapi.go`
- Create: `scanner/parser/asyncapi_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

Save to: `/mnt/e/DEV/mcpdocs/scanner/parser/asyncapi_test.go`

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -run TestAsyncAPI -v 2>&1 | tail -5
```

Expected: FAIL (file not created yet)

- [ ] **Step 3: Write `asyncapi.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// asyncAPIDoc is the minimal shape we need from an AsyncAPI document.
type asyncAPIDoc struct {
	Info struct {
		Title string `yaml:"title"`
	} `yaml:"info"`
	Channels map[string]asyncAPIChannel `yaml:"channels"`
}

type asyncAPIChannel struct {
	Publish   *asyncAPIOperation `yaml:"publish"`
	Subscribe *asyncAPIOperation `yaml:"subscribe"`
}

type asyncAPIOperation struct {
	Message struct {
		Name string `yaml:"name"`
	} `yaml:"message"`
}

// asyncAPIParser implements FileParser for AsyncAPI documents.
type asyncAPIParser struct{}

// AsyncAPIParser returns the FileParser for asyncapi.yaml and asyncapi.json files.
func AsyncAPIParser() FileParser { return &asyncAPIParser{} }

func (*asyncAPIParser) FileType() string    { return "asyncapi" }
func (*asyncAPIParser) Filenames() []string { return []string{"asyncapi.yaml", "asyncapi.json"} }

func (p *asyncAPIParser) Parse(data []byte) (ParsedFile, error) {
	var doc asyncAPIDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return ParsedFile{}, fmt.Errorf("asyncapi: yaml parse error: %w", err)
	}
	title := strings.TrimSpace(doc.Info.Title)
	if title == "" {
		return ParsedFile{}, fmt.Errorf("asyncapi: missing info.title")
	}

	var aux []AuxEntity
	var rels []ParsedRelation

	for channelName, ch := range doc.Channels {
		var schemaObs []string
		if ch.Publish != nil && ch.Publish.Message.Name != "" {
			schemaObs = append(schemaObs, "schema:"+ch.Publish.Message.Name)
		} else if ch.Subscribe != nil && ch.Subscribe.Message.Name != "" {
			schemaObs = append(schemaObs, "schema:"+ch.Subscribe.Message.Name)
		}
		aux = append(aux, AuxEntity{
			Name:         channelName,
			EntityType:   "event-topic",
			Observations: schemaObs,
		})

		if ch.Publish != nil {
			rels = append(rels, ParsedRelation{
				From:         title,
				To:           channelName,
				RelationType: "publishes_event",
			})
		}
		if ch.Subscribe != nil {
			rels = append(rels, ParsedRelation{
				From:         title,
				To:           channelName,
				RelationType: "subscribes_event",
			})
		}
	}

	return ParsedFile{
		EntityName:   title,
		EntityType:   "service",
		Observations: []string{"_integration_source:asyncapi"},
		AuxEntities:  aux,
		Relations:    rels,
	}, nil
}
```

Save to: `/mnt/e/DEV/mcpdocs/scanner/parser/asyncapi.go`

- [ ] **Step 4: Run tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -run TestAsyncAPI -v 2>&1 | tail -15
```

Expected: all `TestAsyncAPI*` tests PASS

- [ ] **Step 5: Commit**

```bash
git add scanner/parser/asyncapi.go scanner/parser/asyncapi_test.go
git commit -m "feat(parser): add AsyncAPIParser for publishes_event/subscribes_event relations"
```

---

## Task 3: SpringKafkaParser

**Files:**

- Create: `scanner/parser/springkafka.go`
- Create: `scanner/parser/springkafka_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

Save to: `/mnt/e/DEV/mcpdocs/scanner/parser/springkafka_test.go`

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -run TestSpringKafka -v 2>&1 | tail -5
```

- [ ] **Step 3: Write `springkafka.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"bufio"
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

// springKafkaParser implements FileParser for Spring Boot Kafka configuration files.
type springKafkaParser struct{}

// SpringKafkaParser returns the FileParser for application.yml/yaml/properties files.
func SpringKafkaParser() FileParser { return &springKafkaParser{} }

func (*springKafkaParser) FileType() string { return "spring-kafka" }
func (*springKafkaParser) Filenames() []string {
	return []string{"application.yml", "application.yaml", "application.properties"}
}

func (p *springKafkaParser) Parse(data []byte) (ParsedFile, error) {
	// Try YAML first; fall back to properties format.
	var rels []ParsedRelation

	if looksLikeYAML(data) {
		rels = parseSpringKafkaYAML(data)
	} else {
		rels = parseSpringKafkaProperties(data)
	}

	if len(rels) == 0 {
		return ParsedFile{}, nil
	}

	return ParsedFile{
		// EntityName is empty: the indexer fills Relations[i].From with the repo
		// service name (via the empty-From sentinel convention).
		Observations: []string{"_integration_source:spring-kafka"},
		Relations:    rels,
	}, nil
}

func looksLikeYAML(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// If the first meaningful line contains ": " or ends with ":", it's YAML.
		return strings.Contains(line, ": ") || strings.HasSuffix(line, ":")
	}
	return false
}

// kafkaYAML is the minimal nested structure we extract from application.yml.
type kafkaYAML struct {
	Spring struct {
		Kafka struct {
			Producer struct {
				Topic string `yaml:"topic"`
			} `yaml:"producer"`
			Consumer struct {
				Topics string `yaml:"topics"`
			} `yaml:"consumer"`
		} `yaml:"kafka"`
	} `yaml:"spring"`
}

func parseSpringKafkaYAML(data []byte) []ParsedRelation {
	var cfg kafkaYAML
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	var rels []ParsedRelation
	if t := strings.TrimSpace(cfg.Spring.Kafka.Producer.Topic); t != "" && !isPlaceholder(t) {
		rels = append(rels, ParsedRelation{From: "", To: t, RelationType: "publishes_event"})
	}
	for _, topic := range splitTopics(cfg.Spring.Kafka.Consumer.Topics) {
		rels = append(rels, ParsedRelation{From: "", To: topic, RelationType: "subscribes_event"})
	}
	return rels
}

func parseSpringKafkaProperties(data []byte) []ParsedRelation {
	var rels []ParsedRelation
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "spring.kafka.producer.topic":
			if !isPlaceholder(value) {
				rels = append(rels, ParsedRelation{From: "", To: value, RelationType: "publishes_event"})
			}
		case "spring.kafka.consumer.topics":
			for _, topic := range splitTopics(value) {
				rels = append(rels, ParsedRelation{From: "", To: topic, RelationType: "subscribes_event"})
			}
		}
	}
	return rels
}

// splitTopics splits a comma-separated topic list, trimming whitespace and skipping placeholders.
func splitTopics(raw string) []string {
	if raw == "" {
		return nil
	}
	var topics []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t != "" && !isPlaceholder(t) {
			topics = append(topics, t)
		}
	}
	return topics
}

// isPlaceholder returns true for Spring ${ENV_VAR} expressions that cannot be resolved.
func isPlaceholder(s string) bool {
	return strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}")
}
```

Save to: `/mnt/e/DEV/mcpdocs/scanner/parser/springkafka.go`

**Important**: In `upsertParsedFile` in `indexer/indexer.go`, the empty-`From` convention (from #13) fills `To` with the repo service name. For SpringKafkaParser, the relations have `From = ""` (the service that publishes/subscribes is the repo itself). We need to also fill empty `From` in the indexer.

**Update `indexer/indexer.go` `upsertParsedFile`** — change the relation fill-in logic to handle empty `From` as well:

```go
svcName := repoServiceName(repo)
rels := make([]memory.Relation, 0, len(parsed.Relations))
for _, r := range parsed.Relations {
    from := r.From
    if from == "" {
        from = svcName
    }
    to := r.To
    if to == "" {
        to = svcName
    }
    rels = append(rels, memory.Relation{
        From:         from,
        To:           to,
        RelationType: r.RelationType,
    })
}
```

Also update the `ParsedFile` doc comment in `extension.go`:

```go
// Relations with From == "" have their From field filled with the derived repo service name.
// Relations with To == "" have their To field filled with the derived repo service name.
```

- [ ] **Step 4: Apply the indexer and extension.go updates**

In `/mnt/e/DEV/mcpdocs/indexer/indexer.go`, find the `svcName := repoServiceName(repo)` block and update it to fill both empty `From` and empty `To`.

In `/mnt/e/DEV/mcpdocs/scanner/parser/extension.go`, update the `ParsedFile` doc comment for `Relations`.

- [ ] **Step 5: Run tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -run TestSpringKafka -v 2>&1 | tail -15
```

Expected: all `TestSpringKafka*` tests PASS

- [ ] **Step 6: Run full suite to confirm no regressions**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./... 2>&1 | tail -20
```

- [ ] **Step 7: Commit**

```bash
git add scanner/parser/springkafka.go scanner/parser/springkafka_test.go indexer/indexer.go scanner/parser/extension.go
git commit -m "feat(parser): add SpringKafkaParser; indexer fills empty From in relations"
```

---

## Task 4: OpenAPIParser

**Files:**

- Create: `scanner/parser/openapi.go`
- Create: `scanner/parser/openapi_test.go`

- [ ] **Step 1: Write the failing test**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

func TestOpenAPIParser_FileTypeAndFilenames(t *testing.T) {
	p := parser.OpenAPIParser()
	if p.FileType() != "openapi" {
		t.Errorf("FileType = %q, want %q", p.FileType(), "openapi")
	}
	wantNames := map[string]bool{
		"openapi.yaml": true,
		"openapi.json": true,
		"swagger.json": true,
		"swagger.yaml": true,
	}
	for _, fn := range p.Filenames() {
		if !wantNames[fn] {
			t.Errorf("unexpected filename %q", fn)
		}
	}
}

func TestOpenAPIParser_Parse_YAML(t *testing.T) {
	input := []byte(`
openapi: "3.0.0"
info:
  title: checkout-api
  version: "2.1.0"
servers:
  - url: https://api.example.com/checkout
paths:
  /orders:
    get: {}
  /orders/{id}:
    get: {}
`)
	p := parser.OpenAPIParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.EntityName != "checkout-api" {
		t.Errorf("EntityName = %q, want %q", got.EntityName, "checkout-api")
	}
	if got.EntityType != "api" {
		t.Errorf("EntityType = %q, want %q", got.EntityType, "api")
	}

	obsMap := make(map[string]bool)
	for _, o := range got.Observations {
		obsMap[o] = true
	}
	if !obsMap["version:2.1.0"] {
		t.Error("missing version observation")
	}
	if !obsMap["paths:2"] {
		t.Errorf("missing paths:2 observation, got obs: %v", got.Observations)
	}
	if !obsMap["server_url:https://api.example.com/checkout"] {
		t.Errorf("missing server_url observation, got obs: %v", got.Observations)
	}

	// Should produce exposes_api relation from service (empty From) to API entity.
	if len(got.Relations) != 1 || got.Relations[0].RelationType != "exposes_api" {
		t.Errorf("expected one exposes_api relation, got %v", got.Relations)
	}
	if got.Relations[0].From != "" {
		t.Errorf("From should be empty (filled by indexer), got %q", got.Relations[0].From)
	}
	if got.Relations[0].To != "checkout-api" {
		t.Errorf("To = %q, want %q", got.Relations[0].To, "checkout-api")
	}
}

func TestOpenAPIParser_Parse_MissingTitle(t *testing.T) {
	input := []byte(`openapi: "3.0.0"
info:
  version: "1.0"`)
	p := parser.OpenAPIParser()
	_, err := p.Parse(input)
	if err == nil {
		t.Error("expected error for missing info.title")
	}
}
```

Save to: `/mnt/e/DEV/mcpdocs/scanner/parser/openapi_test.go`

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -run TestOpenAPI -v 2>&1 | tail -5
```

- [ ] **Step 3: Write `openapi.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// openAPIDoc is the minimal shape we need from OpenAPI / Swagger documents.
type openAPIDoc struct {
	Info struct {
		Title   string `yaml:"title"   json:"title"`
		Version string `yaml:"version" json:"version"`
	} `yaml:"info" json:"info"`
	Servers []struct {
		URL string `yaml:"url" json:"url"`
	} `yaml:"servers" json:"servers"`
	// OpenAPI 3.x uses "paths"; Swagger 2.x also uses "paths".
	Paths map[string]interface{} `yaml:"paths" json:"paths"`
}

// openAPIParser implements FileParser for OpenAPI/Swagger documents.
type openAPIParser struct{}

// OpenAPIParser returns the FileParser for openapi.yaml/json and swagger.json/yaml files.
func OpenAPIParser() FileParser { return &openAPIParser{} }

func (*openAPIParser) FileType() string { return "openapi" }
func (*openAPIParser) Filenames() []string {
	return []string{"openapi.yaml", "openapi.json", "swagger.json", "swagger.yaml"}
}

func (p *openAPIParser) Parse(data []byte) (ParsedFile, error) {
	var doc openAPIDoc
	// Try JSON first (swagger.json), fall back to YAML.
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal(data, &doc); err != nil {
			return ParsedFile{}, fmt.Errorf("openapi: json parse error: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ParsedFile{}, fmt.Errorf("openapi: yaml parse error: %w", err)
		}
	}

	title := strings.TrimSpace(doc.Info.Title)
	if title == "" {
		return ParsedFile{}, fmt.Errorf("openapi: missing info.title")
	}

	obs := []string{"_integration_source:openapi"}
	if doc.Info.Version != "" {
		obs = append(obs, "version:"+doc.Info.Version)
	}
	if len(doc.Paths) > 0 {
		obs = append(obs, fmt.Sprintf("paths:%d", len(doc.Paths)))
	}
	for _, srv := range doc.Servers {
		if srv.URL != "" {
			obs = append(obs, "server_url:"+srv.URL)
		}
	}

	return ParsedFile{
		EntityName:   title,
		EntityType:   "api",
		Observations: obs,
		Relations: []ParsedRelation{
			// From is empty: filled by indexer with repo service name.
			{From: "", To: title, RelationType: "exposes_api"},
		},
	}, nil
}
```

Save to: `/mnt/e/DEV/mcpdocs/scanner/parser/openapi.go`

- [ ] **Step 4: Run tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -run TestOpenAPI -v 2>&1 | tail -15
```

Expected: all `TestOpenAPI*` tests PASS

- [ ] **Step 5: Commit**

```bash
git add scanner/parser/openapi.go scanner/parser/openapi_test.go
git commit -m "feat(parser): add OpenAPIParser for exposes_api relations"
```

---

## Task 5: ProtoParser

**Files:**

- Create: `scanner/parser/proto.go`
- Create: `scanner/parser/proto_test.go`
- Modify: `scanner/scanner.go` (add `.proto` to `infraExtensions`)

- [ ] **Step 1: Write the failing test**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

func TestProtoParser_FileTypeAndFilenames(t *testing.T) {
	p := parser.ProtoParser()
	if p.FileType() != "proto" {
		t.Errorf("FileType = %q, want %q", p.FileType(), "proto")
	}
	if len(p.Filenames()) != 1 || p.Filenames()[0] != ".proto" {
		t.Errorf("Filenames = %v, want [.proto]", p.Filenames())
	}
}

func TestProtoParser_Parse_ServiceDefinition(t *testing.T) {
	input := []byte(`
syntax = "proto3";
package com.example.payment;

import "google/protobuf/empty.proto";
import "fraud/fraud_service.proto";

service PaymentService {
  rpc ProcessPayment(PaymentRequest) returns (PaymentResponse);
}

message PaymentRequest {}
message PaymentResponse {}
`)
	p := parser.ProtoParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have a grpc-service aux entity for PaymentService.
	var foundGRPC bool
	for _, a := range got.AuxEntities {
		if a.Name == "PaymentService" && a.EntityType == "grpc-service" {
			foundGRPC = true
		}
	}
	if !foundGRPC {
		t.Errorf("expected grpc-service aux entity for PaymentService, got %v", got.AuxEntities)
	}

	relByType := make(map[string][]parser.ParsedRelation)
	for _, r := range got.Relations {
		relByType[r.RelationType] = append(relByType[r.RelationType], r)
	}

	// provides_grpc from repo service (empty From) to PaymentService.
	if len(relByType["provides_grpc"]) != 1 || relByType["provides_grpc"][0].To != "PaymentService" {
		t.Errorf("provides_grpc = %v", relByType["provides_grpc"])
	}

	// depends_on_grpc for internal import fraud_service (skip google/protobuf).
	var foundFraud bool
	for _, r := range relByType["depends_on_grpc"] {
		if r.To == "fraud_service" {
			foundFraud = true
		}
	}
	if !foundFraud {
		t.Errorf("expected depends_on_grpc for fraud_service, got %v", relByType["depends_on_grpc"])
	}
	// google/protobuf import is external — must be skipped.
	for _, r := range relByType["depends_on_grpc"] {
		if r.To == "empty" {
			t.Error("google/protobuf import should be skipped")
		}
	}
}

func TestProtoParser_Parse_NoService(t *testing.T) {
	input := []byte(`
syntax = "proto3";
message Foo {}
`)
	p := parser.ProtoParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.AuxEntities) != 0 {
		t.Errorf("expected no aux entities, got %v", got.AuxEntities)
	}
}
```

Save to: `/mnt/e/DEV/mcpdocs/scanner/parser/proto_test.go`

- [ ] **Step 2: Write `proto.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"strings"
)

// externalProtoPackages are well-known external proto packages whose imports
// should not produce internal depends_on_grpc relations.
var externalProtoPackages = []string{
	"google/",
	"github.com/",
	"grpc/",
	"protoc-gen-",
}

// protoParser implements FileParser for Protocol Buffer files.
type protoParser struct{}

// ProtoParser returns the FileParser for *.proto files (suffix-matched by scanner).
func ProtoParser() FileParser { return &protoParser{} }

func (*protoParser) FileType() string    { return "proto" }
func (*protoParser) Filenames() []string { return []string{".proto"} }

func (p *protoParser) Parse(data []byte) (ParsedFile, error) {
	var aux []AuxEntity
	var rels []ParsedRelation
	var packageName string

	for rawLine := range strings.SplitSeq(string(data), "\n") {
		line := strings.TrimSpace(rawLine)

		if strings.HasPrefix(line, "package ") {
			packageName = strings.TrimSuffix(strings.TrimPrefix(line, "package "), ";")
			packageName = strings.TrimSpace(packageName)
			continue
		}

		if strings.HasPrefix(line, "service ") {
			// "service Foo {" → extract "Foo"
			rest := strings.TrimPrefix(line, "service ")
			name := strings.Fields(rest)[0]
			name = strings.TrimSuffix(name, "{")
			name = strings.TrimSpace(name)
			if name != "" {
				aux = append(aux, AuxEntity{
					Name:       name,
					EntityType: "grpc-service",
				})
				// From is empty: indexer fills with repo service name.
				rels = append(rels, ParsedRelation{
					From:         "",
					To:           name,
					RelationType: "provides_grpc",
				})
			}
			continue
		}

		if strings.HasPrefix(line, `import "`) {
			// `import "path/to/service.proto";` → extract base name without extension.
			importPath := strings.TrimPrefix(line, `import "`)
			importPath = strings.TrimSuffix(importPath, `";`)
			importPath = strings.TrimSpace(importPath)
			if isExternalProtoImport(importPath) {
				continue
			}
			// Extract base name: "fraud/fraud_service.proto" → "fraud_service"
			base := importPath
			if idx := strings.LastIndex(base, "/"); idx >= 0 {
				base = base[idx+1:]
			}
			base = strings.TrimSuffix(base, ".proto")
			if base != "" {
				rels = append(rels, ParsedRelation{
					From:         "",
					To:           base,
					RelationType: "depends_on_grpc",
				})
			}
		}
	}

	if len(aux) == 0 && len(rels) == 0 {
		return ParsedFile{}, nil
	}

	obs := []string{"_integration_source:proto"}
	if packageName != "" {
		obs = append(obs, "proto_package:"+packageName)
	}

	return ParsedFile{
		Observations: obs,
		AuxEntities:  aux,
		Relations:    rels,
	}, nil
}

func isExternalProtoImport(path string) bool {
	for _, prefix := range externalProtoPackages {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
```

Save to: `/mnt/e/DEV/mcpdocs/scanner/parser/proto.go`

- [ ] **Step 3: Add `.proto` to `infraExtensions` in scanner.go**

In `/mnt/e/DEV/mcpdocs/scanner/scanner.go`, find the `infraExtensions` map and add `.proto`:

```go
var infraExtensions = map[string]bool{
    ".yaml":  true,
    ".yml":   true,
    ".tf":    true,
    ".hcl":   true,
    ".toml":  true,
    ".proto": true,
}
```

- [ ] **Step 4: Run tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -run TestProto -v 2>&1 | tail -15
```

Expected: all `TestProto*` tests PASS

- [ ] **Step 5: Commit**

```bash
git add scanner/parser/proto.go scanner/parser/proto_test.go scanner/scanner.go
git commit -m "feat(parser): add ProtoParser for provides_grpc/depends_on_grpc relations"
```

---

## Task 6: K8sServiceParser

**Files:**

- Create: `scanner/parser/k8sintegration.go`
- Create: `scanner/parser/k8sintegration_test.go`

Note: This parser registers `FileType() = "k8s"` — the same type emitted by `classifyFile` for K8s manifests. The indexer's `runParsers()` will route k8s-typed files through this parser automatically after registration. Existing k8s files that are infra assets (but not Deployment specs) will be processed and return an empty `ParsedFile` gracefully.

- [ ] **Step 1: Write the failing test**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

func TestK8sServiceParser_FileTypeAndFilenames(t *testing.T) {
	p := parser.K8sServiceParser()
	if p.FileType() != "k8s" {
		t.Errorf("FileType = %q, want %q", p.FileType(), "k8s")
	}
	// k8s files are discovered by the infra scanner, not by root-level filename.
	// Filenames returns sentinel values that classifyFile uses for path-based routing.
	if len(p.Filenames()) == 0 {
		t.Error("Filenames should not be empty")
	}
}

func TestK8sServiceParser_Parse_DeploymentEnvVars(t *testing.T) {
	input := []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-service
spec:
  template:
    spec:
      containers:
      - name: checkout
        env:
        - name: PAYMENT_SERVICE_HOST
          value: payment-service
        - name: FRAUD_API_URL
          value: http://fraud-service:8080
        - name: LOG_LEVEL
          value: info
`)
	p := parser.K8sServiceParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should produce calls_service relations for PAYMENT_SERVICE_HOST and FRAUD_API_URL.
	callsTargets := make(map[string]bool)
	for _, r := range got.Relations {
		if r.RelationType == "calls_service" {
			callsTargets[r.To] = true
		}
	}
	if !callsTargets["payment-service"] {
		t.Errorf("expected calls_service to payment-service, got %v", callsTargets)
	}
	if !callsTargets["fraud-service"] {
		t.Errorf("expected calls_service to fraud-service, got %v", callsTargets)
	}
	// LOG_LEVEL should not produce a calls_service relation.
	if callsTargets["log"] {
		t.Error("LOG_LEVEL should not produce a calls_service relation")
	}
	// From should be empty (indexer fills with repo service name).
	for _, r := range got.Relations {
		if r.From != "" {
			t.Errorf("From should be empty, got %q", r.From)
		}
	}
}

func TestK8sServiceParser_Parse_NonDeployment(t *testing.T) {
	input := []byte(`
apiVersion: v1
kind: Service
metadata:
  name: checkout-svc
spec:
  selector:
    app: checkout
`)
	p := parser.K8sServiceParser()
	got, err := p.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Relations) != 0 {
		t.Errorf("expected no relations for non-Deployment kind, got %d", len(got.Relations))
	}
}
```

Save to: `/mnt/e/DEV/mcpdocs/scanner/parser/k8sintegration_test.go`

- [ ] **Step 2: Write `k8sintegration.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// serviceEnvPatterns matches env variable names that suggest a service dependency.
var serviceEnvPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(.+)_SERVICE_HOST$`),
	regexp.MustCompile(`^(.+)_SERVICE_URL$`),
	regexp.MustCompile(`^(.+)_API_URL$`),
	regexp.MustCompile(`^(.+)_BASE_URL$`),
}

// k8sDeployment is the minimal shape we need from a K8s Deployment manifest.
type k8sDeployment struct {
	Kind string `yaml:"kind"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers []struct {
					Env []struct {
						Name string `yaml:"name"`
					} `yaml:"env"`
				} `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

// k8sServiceParser implements FileParser for Kubernetes Deployment manifests.
type k8sServiceParser struct{}

// K8sServiceParser returns the FileParser for K8s-classified YAML files (k8s type).
func K8sServiceParser() FileParser { return &k8sServiceParser{} }

func (*k8sServiceParser) FileType() string { return "k8s" }
// Filenames returns path-suffix sentinels. K8s files are discovered by the infra
// scanner and classified as "k8s" by classifyFile; these sentinels ensure the
// registry lookup finds this parser for files of that type.
func (*k8sServiceParser) Filenames() []string { return []string{"/k8s/", "/kubernetes/"} }

func (p *k8sServiceParser) Parse(data []byte) (ParsedFile, error) {
	var doc k8sDeployment
	if err := yaml.Unmarshal(data, &doc); err != nil {
		// Non-parseable YAML is silently skipped.
		return ParsedFile{}, nil
	}
	if doc.Kind != "Deployment" {
		return ParsedFile{}, nil
	}

	seen := make(map[string]bool)
	var rels []ParsedRelation

	for _, container := range doc.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			target := extractServiceTarget(env.Name)
			if target == "" || seen[target] {
				continue
			}
			seen[target] = true
			rels = append(rels, ParsedRelation{
				From:         "", // indexer fills with repo service name
				To:           target,
				RelationType: "calls_service",
			})
		}
	}

	if len(rels) == 0 {
		return ParsedFile{}, nil
	}

	return ParsedFile{
		Observations: []string{"_integration_source:k8s-env"},
		Relations:    rels,
	}, nil
}

// extractServiceTarget extracts a normalized service name from a K8s env var name.
// Returns "" if the env var doesn't match a known service pattern.
func extractServiceTarget(envName string) string {
	for _, pat := range serviceEnvPatterns {
		matches := pat.FindStringSubmatch(envName)
		if len(matches) < 2 {
			continue
		}
		// PAYMENT_SERVICE → payment-service
		raw := strings.ToLower(matches[1])
		// Strip trailing _SERVICE suffix from patterns like PAYMENT_SERVICE_HOST
		raw = strings.TrimSuffix(raw, "_service")
		name := strings.ReplaceAll(raw, "_", "-")
		if name != "" {
			return name
		}
	}
	return ""
}
```

Save to: `/mnt/e/DEV/mcpdocs/scanner/parser/k8sintegration.go`

**Note about K8sServiceParser registration:** Since `FileType() = "k8s"` and `Filenames()` returns path sentinels (`/k8s/`, `/kubernetes/`), the `classifyFile` registry lookup needs to handle this. The existing `classifyFile` (from #13) routes k8s files by path-contains logic in the hardcoded switch. The K8sServiceParser's `Filenames()` contains path patterns, not filenames, so the registry exact-match loop won't claim them.

To avoid a conflicting registration, the K8sServiceParser should register with its `FileType() = "k8s"` so the indexer can find it via `registry.Get("k8s")`. The `classifyFile` hardcoded switch still handles the "k8s" classification; K8sServiceParser doesn't need to participate in discovery — it only needs to be findable by the indexer. No change to `classifyFile` needed.

However, `registry.Register()` checks for duplicate filenames. Since K8sServiceParser's `Filenames()` returns path patterns that no other parser claims, there's no conflict.

- [ ] **Step 3: Run tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -run TestK8sService -v 2>&1 | tail -15
```

Expected: all `TestK8sService*` tests PASS

- [ ] **Step 4: Run full parser test suite**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./scanner/parser/... -race -v 2>&1 | grep -E "^(=== RUN|--- PASS|--- FAIL|FAIL|ok)"
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add scanner/parser/k8sintegration.go scanner/parser/k8sintegration_test.go
git commit -m "feat(parser): add K8sServiceParser for calls_service relations from env vars"
```

---

## Task 7: Register 5 new parsers in `main.go`

- [ ] **Step 1: Add parser registrations to `main.go`**

In `/mnt/e/DEV/mcpdocs/main.go`, find the `// --- Parser Registry ---` section (added in #13) and append the 5 new parsers:

```go
// --- Parser Registry ---
parser.Register(parser.GoModParser())
parser.Register(parser.PackageJSONParser())
parser.Register(parser.PomParser())
parser.Register(parser.CodeownersParser())
parser.Register(parser.CatalogParser())
// Integration topology parsers (#15)
parser.Register(parser.AsyncAPIParser())
parser.Register(parser.SpringKafkaParser())
parser.Register(parser.OpenAPIParser())
parser.Register(parser.ProtoParser())
parser.Register(parser.K8sServiceParser())
```

- [ ] **Step 2: Build**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./...
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat(main): register 5 integration topology parsers"
```

---

## Task 8: Memory layer — `GetIntegrationMap`

**Files:**

- Create: `memory/integration.go`
- Create: `memory/integration_test.go`
- Modify: `memory/memory.go`

- [ ] **Step 1: Write the failing test for `GetIntegrationMap`**

```go
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
	db, err := memory.OpenDB("") // in-memory SQLite
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
```

Save to: `/mnt/e/DEV/mcpdocs/memory/integration_test.go`

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./memory/... -run TestGetIntegrationMap -v 2>&1 | tail -5
```

- [ ] **Step 3: Write `memory/integration.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"context"
	"strings"
)

// IntegrationEdge is a single integration relationship entry in an IntegrationMap.
type IntegrationEdge struct {
	Target     string `json:"target"`
	Schema     string `json:"schema,omitempty"`      // event-topic schema name
	Version    string `json:"version,omitempty"`     // api version
	Paths      int    `json:"paths,omitempty"`       // api path count
	Confidence string `json:"confidence"`            // "authoritative" | "inferred"
	SourceRepo string `json:"source_repo,omitempty"` // originating repo
}

// IntegrationMap aggregates all integration relationships for a service.
type IntegrationMap struct {
	Service      string            `json:"service"`
	Publishes    []IntegrationEdge `json:"publishes"`
	Subscribes   []IntegrationEdge `json:"subscribes"`
	ExposesAPI   []IntegrationEdge `json:"exposes_api"`
	ProvidesGRPC []IntegrationEdge `json:"provides_grpc"`
	GRPCDeps     []IntegrationEdge `json:"grpc_deps"`
	Calls        []IntegrationEdge `json:"calls"`
	Coverage     string            `json:"graph_coverage"` // "full" | "partial" | "inferred" | "none"
}

// authoritativeSources are integration sources whose data comes from explicit contracts.
var authoritativeSources = map[string]bool{
	"asyncapi": true,
	"proto":    true,
	"openapi":  true,
}

// getIntegrationMap queries the graph for all integration edges of the given service
// up to the specified depth.
func (s store) getIntegrationMap(_ context.Context, service string, depth int) (IntegrationMap, error) {
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	result := IntegrationMap{Service: service}

	// Load the service entity's observations to determine integration sources.
	var obs []dbObservation
	if err := s.db.Where("entity_name = ?", service).Find(&obs).Error; err != nil {
		return result, err
	}

	integrationSources := make(map[string]bool)
	for _, o := range obs {
		if src, ok := strings.CutPrefix(o.Content, "_integration_source:"); ok {
			integrationSources[src] = true
		}
	}

	// Helper: determine confidence for an edge based on integration sources.
	confidence := func(source string) string {
		if authoritativeSources[source] {
			return "authoritative"
		}
		return "inferred"
	}

	// Determine overall confidence from sources on the service entity.
	hasAuthoritative := false
	hasInferred := false
	for src := range integrationSources {
		if authoritativeSources[src] {
			hasAuthoritative = true
		} else {
			hasInferred = true
		}
	}

	// Query each integration relation type.
	integrationRelTypes := []struct {
		relType    string
		assignTo   func([]IntegrationEdge)
		source     string
	}{
		{"publishes_event", func(e []IntegrationEdge) { result.Publishes = e }, "asyncapi"},
		{"subscribes_event", func(e []IntegrationEdge) { result.Subscribes = e }, "asyncapi"},
		{"exposes_api", func(e []IntegrationEdge) { result.ExposesAPI = e }, "openapi"},
		{"provides_grpc", func(e []IntegrationEdge) { result.ProvidesGRPC = e }, "proto"},
		{"depends_on_grpc", func(e []IntegrationEdge) { result.GRPCDeps = e }, "proto"},
		{"calls_service", func(e []IntegrationEdge) { result.Calls = e }, "k8s-env"},
	}

	hasAnyRelations := false
	for _, rt := range integrationRelTypes {
		var rels []dbRelation
		if err := s.db.Where("from_node = ? AND relation_type = ?", service, rt.relType).Find(&rels).Error; err != nil {
			return result, err
		}
		if len(rels) == 0 {
			continue
		}
		hasAnyRelations = true
		conf := confidence(rt.source)
		edges := make([]IntegrationEdge, 0, len(rels))
		for _, r := range rels {
			edges = append(edges, IntegrationEdge{
				Target:     r.ToEntity,
				Confidence: conf,
			})
		}
		rt.assignTo(edges)
	}

	// Compute graph_coverage.
	switch {
	case !hasAnyRelations && len(integrationSources) == 0:
		result.Coverage = "none"
	case hasAuthoritative && !hasInferred:
		result.Coverage = "full"
	case hasAuthoritative && hasInferred:
		result.Coverage = "partial"
	case !hasAuthoritative && hasInferred:
		result.Coverage = "inferred"
	default:
		result.Coverage = "none"
	}

	return result, nil
}
```

Save to: `/mnt/e/DEV/mcpdocs/memory/integration.go`

- [ ] **Step 4: Expose `GetIntegrationMap` on `MemoryService` in `memory/memory.go`**

Append to `/mnt/e/DEV/mcpdocs/memory/memory.go` after the `EntityCount` method:

```go
// GetIntegrationMap returns all integration edges for service up to depth hops.
// depth is clamped to [1, 3].
func (srv *MemoryService) GetIntegrationMap(ctx context.Context, service string, depth int) (IntegrationMap, error) {
	return srv.s.getIntegrationMap(ctx, service, depth)
}
```

- [ ] **Step 5: Run tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./memory/... -run TestGetIntegrationMap -v 2>&1 | tail -20
```

Expected: all `TestGetIntegrationMap*` tests PASS

- [ ] **Step 6: Commit**

```bash
git add memory/integration.go memory/integration_test.go memory/memory.go
git commit -m "feat(memory): add GetIntegrationMap for integration topology queries"
```

---

## Task 9: Wire `GetIntegrationMap` into tools layer

**Files:**

- Modify: `tools/ports.go`
- Modify: `tools/audit.go`
- Create: `tools/get_integration_map.go`
- Modify: `tools/tools.go`

- [ ] **Step 1: Add `GetIntegrationMap` to `GraphStore` interface in `tools/ports.go`**

Append to the `GraphStore` interface in `/mnt/e/DEV/mcpdocs/tools/ports.go`:

```go
GetIntegrationMap(ctx context.Context, service string, depth int) (memory.IntegrationMap, error)
```

The import for `"context"` should already be present.

- [ ] **Step 2: Add pass-through to `tools/audit.go`**

Append to `/mnt/e/DEV/mcpdocs/tools/audit.go` (in the read-only pass-throughs section):

```go
func (a *GraphAuditLogger) GetIntegrationMap(ctx context.Context, service string, depth int) (memory.IntegrationMap, error) {
	return a.inner.GetIntegrationMap(ctx, service, depth)
}
```

Add `"context"` to the imports if not present.

- [ ] **Step 3: Build to verify interface satisfaction**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./tools/...
```

Expected: no errors (GraphAuditLogger now satisfies GraphStore)

- [ ] **Step 4: Write `tools/get_integration_map.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// IntegrationMapArgs are the inputs for get_integration_map.
type IntegrationMapArgs struct {
	Service string `json:"service" jsonschema:"required,description=Entity name of the service in the knowledge graph (e.g. 'checkout-service')."`
	Depth   int    `json:"depth,omitempty" jsonschema:"description=Number of integration hops to include (1–3\\, default 1). depth=1 returns direct integrations only."`
}

// IntegrationMapResult is the output of get_integration_map.
// It is identical to memory.IntegrationMap but re-exported for MCP schema generation.
type IntegrationMapResult struct {
	Service      string                 `json:"service"`
	Publishes    []IntegrationEdgeJSON  `json:"publishes"`
	Subscribes   []IntegrationEdgeJSON  `json:"subscribes"`
	ExposesAPI   []IntegrationEdgeJSON  `json:"exposes_api"`
	ProvidesGRPC []IntegrationEdgeJSON  `json:"provides_grpc"`
	GRPCDeps     []IntegrationEdgeJSON  `json:"grpc_deps"`
	Calls        []IntegrationEdgeJSON  `json:"calls"`
	Coverage     string                 `json:"graph_coverage" jsonschema:"description=Confidence level: 'full' (authoritative source covers all directions)\\, 'partial' (mix of authoritative and inferred)\\, 'inferred' (heuristics only)\\, 'none' (no data found)."`
}

// IntegrationEdgeJSON is a single integration edge in the MCP response.
type IntegrationEdgeJSON struct {
	Target     string `json:"target"`
	Schema     string `json:"schema,omitempty"`
	Version    string `json:"version,omitempty"`
	Paths      int    `json:"paths,omitempty"`
	Confidence string `json:"confidence"`
	SourceRepo string `json:"source_repo,omitempty"`
}

func getIntegrationMapHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args IntegrationMapArgs) (*mcp.CallToolResult, IntegrationMapResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args IntegrationMapArgs) (*mcp.CallToolResult, IntegrationMapResult, error) {
		if args.Service == "" {
			return nil, IntegrationMapResult{}, errorf("service is required")
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
			return nil, IntegrationMapResult{}, errorf("GetIntegrationMap: %w", err)
		}

		toEdges := func(edges []struct {
			Target, Schema, Version, Confidence, SourceRepo string
			Paths                                           int
		}) []IntegrationEdgeJSON {
			result := make([]IntegrationEdgeJSON, 0, len(edges))
			for _, e := range edges {
				result = append(result, IntegrationEdgeJSON{
					Target:     e.Target,
					Schema:     e.Schema,
					Version:    e.Version,
					Paths:      e.Paths,
					Confidence: e.Confidence,
					SourceRepo: e.SourceRepo,
				})
			}
			return result
		}
		_ = toEdges // replaced below with direct conversion

		mapEdges := func(in []interface{ getFields() IntegrationEdgeJSON }) []IntegrationEdgeJSON {
			out := make([]IntegrationEdgeJSON, len(in))
			for i, e := range in {
				out[i] = e.getFields()
			}
			return out
		}
		_ = mapEdges

		// Direct field mapping from memory.IntegrationEdge to IntegrationEdgeJSON.
		convert := func(edges interface{}) []IntegrationEdgeJSON { return nil }
		_ = convert

		pub := make([]IntegrationEdgeJSON, len(m.Publishes))
		for i, e := range m.Publishes {
			pub[i] = IntegrationEdgeJSON{Target: e.Target, Schema: e.Schema, Confidence: e.Confidence, SourceRepo: e.SourceRepo}
		}
		sub := make([]IntegrationEdgeJSON, len(m.Subscribes))
		for i, e := range m.Subscribes {
			sub[i] = IntegrationEdgeJSON{Target: e.Target, Confidence: e.Confidence, SourceRepo: e.SourceRepo}
		}
		api := make([]IntegrationEdgeJSON, len(m.ExposesAPI))
		for i, e := range m.ExposesAPI {
			api[i] = IntegrationEdgeJSON{Target: e.Target, Version: e.Version, Paths: e.Paths, Confidence: e.Confidence}
		}
		grpc := make([]IntegrationEdgeJSON, len(m.ProvidesGRPC))
		for i, e := range m.ProvidesGRPC {
			grpc[i] = IntegrationEdgeJSON{Target: e.Target, Confidence: e.Confidence}
		}
		grpcDeps := make([]IntegrationEdgeJSON, len(m.GRPCDeps))
		for i, e := range m.GRPCDeps {
			grpcDeps[i] = IntegrationEdgeJSON{Target: e.Target, Confidence: e.Confidence, SourceRepo: e.SourceRepo}
		}
		calls := make([]IntegrationEdgeJSON, len(m.Calls))
		for i, e := range m.Calls {
			calls[i] = IntegrationEdgeJSON{Target: e.Target, Confidence: e.Confidence}
		}

		return nil, IntegrationMapResult{
			Service:      m.Service,
			Publishes:    pub,
			Subscribes:   sub,
			ExposesAPI:   api,
			ProvidesGRPC: grpc,
			GRPCDeps:     grpcDeps,
			Calls:        calls,
			Coverage:     m.Coverage,
		}, nil
	}
}
```

**Note:** The above code has some unused variables that need cleanup. Use this clean version instead:

```go
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
	Service string `json:"service" jsonschema:"required,description=Entity name of the service in the knowledge graph (e.g. 'checkout-service')."`
	Depth   int    `json:"depth,omitempty" jsonschema:"description=Number of integration hops to include (1–3\\, default 1). depth=1 returns direct integrations only."`
}

// IntegrationEdgeJSON is a single integration edge in the MCP response.
type IntegrationEdgeJSON struct {
	Target     string `json:"target"`
	Schema     string `json:"schema,omitempty"`
	Version    string `json:"version,omitempty"`
	Paths      int    `json:"paths,omitempty"`
	Confidence string `json:"confidence"`
	SourceRepo string `json:"source_repo,omitempty"`
}

// IntegrationMapResult is the output of get_integration_map.
type IntegrationMapResult struct {
	Service      string                `json:"service"`
	Publishes    []IntegrationEdgeJSON `json:"publishes"`
	Subscribes   []IntegrationEdgeJSON `json:"subscribes"`
	ExposesAPI   []IntegrationEdgeJSON `json:"exposes_api"`
	ProvidesGRPC []IntegrationEdgeJSON `json:"provides_grpc"`
	GRPCDeps     []IntegrationEdgeJSON `json:"grpc_deps"`
	Calls        []IntegrationEdgeJSON `json:"calls"`
	Coverage     string                `json:"graph_coverage" jsonschema:"description=Confidence level: 'full'\\, 'partial'\\, 'inferred'\\, or 'none'."`
}

func convertEdges(edges []memory.IntegrationEdge) []IntegrationEdgeJSON {
	out := make([]IntegrationEdgeJSON, len(edges))
	for i, e := range edges {
		out[i] = IntegrationEdgeJSON{
			Target:     e.Target,
			Schema:     e.Schema,
			Version:    e.Version,
			Paths:      e.Paths,
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
			Service:      m.Service,
			Publishes:    convertEdges(m.Publishes),
			Subscribes:   convertEdges(m.Subscribes),
			ExposesAPI:   convertEdges(m.ExposesAPI),
			ProvidesGRPC: convertEdges(m.ProvidesGRPC),
			GRPCDeps:     convertEdges(m.GRPCDeps),
			Calls:        convertEdges(m.Calls),
			Coverage:     m.Coverage,
		}, nil
	}
}
```

Save to: `/mnt/e/DEV/mcpdocs/tools/get_integration_map.go`

- [ ] **Step 5: Register the tool in `tools/tools.go`**

In `/mnt/e/DEV/mcpdocs/tools/tools.go`, find the `Register` function and add the new tool registration at the end (before the closing brace):

```go
mcp.AddTool(s, &mcp.Tool{
    Name: "get_integration_map",
    Description: "Returns the complete integration topology of a service in a single call: " +
        "which events it publishes and subscribes to, which APIs and gRPC services it exposes or depends on, " +
        "and which services it calls directly. Each entry includes a confidence level so the AI agent can " +
        "distinguish authoritative contract declarations (AsyncAPI, proto, OpenAPI) from inferred config values " +
        "(Spring Kafka, K8s env vars). Use this tool before any architecture, impact analysis, or documentation " +
        "task involving a specific service — it eliminates the need to read raw config files across multiple repos. " +
        "Check graph_coverage to know how much to trust the result.",
}, withMetrics("get_integration_map", metrics, withRecovery("get_integration_map", getIntegrationMapHandler(graph))))
```

- [ ] **Step 6: Build**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./...
```

Expected: no errors

- [ ] **Step 7: Run all tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./... -race 2>&1 | tail -20
```

Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add tools/ports.go tools/audit.go tools/get_integration_map.go tools/tools.go
git commit -m "feat(tools): add get_integration_map MCP tool for integration topology"
```

---

## Task 10: E2E integration test

**Files:**

- Create: `tests/integration_map/integration_map_test.go`

- [ ] **Step 1: Write the E2E test**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package integration_map_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/tests/testutils"
	"github.com/doc-scout/mcp-server/tools"
)

func TestGetIntegrationMapTool_E2E(t *testing.T) {
	ctx := context.Background()

	// Set up test server.
	session := testutils.SetupTestServer(t)

	// Pre-populate graph with integration data via memory service.
	db, err := memory.OpenDB("")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	svc := memory.NewMemoryService(db)

	_, err = svc.CreateEntities([]memory.Entity{
		{Name: "checkout-service", EntityType: "service", Observations: []string{"_integration_source:asyncapi"}},
		{Name: "order.created", EntityType: "event-topic"},
		{Name: "payment.approved", EntityType: "event-topic"},
		{Name: "payment-service", EntityType: "service"},
	})
	if err != nil {
		t.Fatalf("CreateEntities: %v", err)
	}

	_, err = svc.CreateRelations([]memory.Relation{
		{From: "checkout-service", To: "order.created", RelationType: "publishes_event"},
		{From: "checkout-service", To: "payment.approved", RelationType: "subscribes_event"},
		{From: "checkout-service", To: "payment-service", RelationType: "calls_service"},
	})
	if err != nil {
		t.Fatalf("CreateRelations: %v", err)
	}

	// Call get_integration_map via MCP.
	result, err := session.CallTool(ctx, "get_integration_map", map[string]interface{}{
		"service": "checkout-service",
		"depth":   1,
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	// Parse result JSON.
	var got tools.IntegrationMapResult
	if err := json.Unmarshal([]byte(result.Content[0].Text), &got); err != nil {
		t.Fatalf("unmarshal result: %v\nraw: %s", err, result.Content[0].Text)
	}

	if got.Service != "checkout-service" {
		t.Errorf("Service = %q, want %q", got.Service, "checkout-service")
	}
	if len(got.Publishes) != 1 || got.Publishes[0].Target != "order.created" {
		t.Errorf("Publishes = %v", got.Publishes)
	}
	if len(got.Subscribes) != 1 || got.Subscribes[0].Target != "payment.approved" {
		t.Errorf("Subscribes = %v", got.Subscribes)
	}
	if len(got.Calls) != 1 || got.Calls[0].Target != "payment-service" {
		t.Errorf("Calls = %v", got.Calls)
	}
	// AsyncAPI is authoritative — coverage should be "full" or "partial"
	if got.Coverage == "none" {
		t.Error("Coverage should not be 'none' when integration data is present")
	}
}

func TestGetIntegrationMapTool_UnknownService(t *testing.T) {
	ctx := context.Background()
	session := testutils.SetupTestServer(t)

	result, err := session.CallTool(ctx, "get_integration_map", map[string]interface{}{
		"service": "does-not-exist",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	var got tools.IntegrationMapResult
	if err := json.Unmarshal([]byte(result.Content[0].Text), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Coverage != "none" {
		t.Errorf("Coverage = %q, want %q for unknown service", got.Coverage, "none")
	}
}

func TestGetIntegrationMapTool_MissingService(t *testing.T) {
	ctx := context.Background()
	session := testutils.SetupTestServer(t)

	result, err := session.CallTool(ctx, "get_integration_map", map[string]interface{}{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	// Should return an error response (service is required).
	if !result.IsError {
		t.Error("expected error response when service is missing")
	}
}
```

Save to: `/mnt/e/DEV/mcpdocs/tests/integration_map/integration_map_test.go`

Note: The E2E test uses `testutils.SetupTestServer` which creates an MCP server with an in-memory graph. Review `tests/testutils/utils.go` to confirm the graph instance used in `SetupTestServer` — the test pre-populates that same instance.

If `SetupTestServer` creates its own `MemoryService` internally, the test must use the same DB instance. You may need to update `testutils.SetupTestServer` to accept an optional `memory.GraphStore` parameter, or expose the graph from the server fixture. Check the existing test setup pattern in `tests/traverse_graph/` to follow the same pattern.

- [ ] **Step 2: Run the E2E test**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./tests/integration_map/... -v -race 2>&1 | tail -20
```

Fix any compilation or runtime issues by aligning with the `testutils` patterns used in other E2E tests.

- [ ] **Step 3: Commit**

```bash
git add tests/integration_map/integration_map_test.go
git commit -m "test(e2e): add integration_map E2E tests"
```

---

## Task 11: Update `AGENTS.md` and final verification

- [ ] **Step 1: Update `AGENTS.md` §7 to document new relation types**

Find the `## New relation types` or equivalent section in §7 and append:

```markdown
## Integration Relation Types

| Relation           | From    | To           | Source          |
| ------------------ | ------- | ------------ | --------------- |
| `publishes_event`  | service | event-topic  | AsyncAPI        |
| `subscribes_event` | service | event-topic  | AsyncAPI        |
| `exposes_api`      | service | api          | OpenAPI/Swagger |
| `provides_grpc`    | service | grpc-service | .proto          |
| `depends_on_grpc`  | service | grpc-service | .proto imports  |
| `calls_service`    | service | service      | K8s env vars    |

New entity types: `event-topic`, `grpc-service` (in addition to existing `api`, `service`, `team`, `person`).

## `get_integration_map` tool

Use `get_integration_map` to answer architecture, impact, and documentation questions about a specific service's integration topology. It returns all integration edges in a single call including a `graph_coverage` field:

- `"full"` — at least one authoritative source (AsyncAPI, proto, OpenAPI) covers all directions
- `"partial"` — mix of authoritative and inferred, or some directions have no data
- `"inferred"` — all relations come from config heuristics (Spring Kafka, K8s env vars)
- `"none"` — no integration data found for this service
```

- [ ] **Step 2: Final build and test run**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./... && go test ./... -race -count=1 2>&1 | tail -30
```

Expected: build success, all tests PASS

- [ ] **Step 3: Commit AGENTS.md**

```bash
git add AGENTS.md
git commit -m "docs(agents): document integration relation types and get_integration_map"
```

- [ ] **Step 4: Push and create PR (stacked on feat/custom-parser-extension)**

```bash
git push -u origin feat/integration-topology-discovery
gh pr create \
  --title "feat: integration topology discovery (#15)" \
  --base feat/custom-parser-extension \
  --body "$(cat <<'EOF'
## Summary

- Adds 5 new `FileParser` implementations: AsyncAPI, Spring Kafka, OpenAPI, Proto, K8s env heuristic
- Populates new relation types: `publishes_event`, `subscribes_event`, `exposes_api`, `provides_grpc`, `depends_on_grpc`, `calls_service`
- Adds `get_integration_map` MCP tool for single-call integration topology queries with `graph_coverage` confidence field
- `.proto` files added to infra scanner discovery

## Dependencies

Stacked on #feat/custom-parser-extension — requires `FileParser` interface from #13.

## Test plan

- [ ] `go test ./scanner/parser/... -race` — all 5 new parser unit tests pass
- [ ] `go test ./memory/... -race` — GetIntegrationMap unit tests pass
- [ ] `go test ./tests/integration_map/... -race` — E2E tests pass
- [ ] `go test ./... -race` — full suite green

## Spec

`docs/superpowers/specs/2026-04-03-integration-topology-discovery-design.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-Review Checklist

- [x] All 5 parsers implement `FileParser` from #13 — no core code modified
- [x] SpringKafkaParser uses empty `From` sentinel (indexer fills with repo service name) — consistent with codeowners pattern
- [x] OpenAPIParser uses empty `From` sentinel for `exposes_api` relation
- [x] K8sServiceParser `FileType() = "k8s"` matches existing classifier — routes automatically via registry
- [x] K8sServiceParser `Filenames()` returns path sentinels that don't conflict with other parsers
- [x] ProtoParser suffix sentinel `.proto` requires `classifyFile` suffix match (implemented in #13)
- [x] `.proto` added to `infraExtensions` for scanner discovery
- [x] `memory/integration.go` uses `dbRelation` GORM model — consistent with existing `traverse.go` pattern
- [x] `GraphStore` interface extended, `GraphAuditLogger` updated as pass-through
- [x] `graph_coverage` computation handles all 4 states: full/partial/inferred/none
- [x] E2E test note: check testutils.SetupTestServer pattern before writing — may need adjustment
- [x] PR base is `feat/custom-parser-extension` (stacked branch strategy)
