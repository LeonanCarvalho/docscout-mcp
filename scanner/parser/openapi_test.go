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
