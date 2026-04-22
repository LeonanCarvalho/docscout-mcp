// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

func TestParsePackageJSON_Basic(t *testing.T) {
	input := []byte(`{
  "name": "my-service",
  "version": "1.2.3",
  "dependencies": {
    "express": "^4.18.0",
    "axios": "^1.6.0"
  },
  "devDependencies": {
    "jest": "^29.0.0"
  }
}`)

	got, err := parser.ParsePackageJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "my-service" {
		t.Errorf("Name = %q, want %q", got.Name, "my-service")
	}
	if got.EntityName != "my-service" {
		t.Errorf("EntityName = %q, want %q", got.EntityName, "my-service")
	}
	if got.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", got.Version, "1.2.3")
	}

	// Only runtime deps — jest (devDep) must be excluded.
	wantDeps := map[string]bool{"express": true, "axios": true}
	if len(got.DirectDeps) != len(wantDeps) {
		t.Errorf("DirectDeps count = %d, want %d: %v", len(got.DirectDeps), len(wantDeps), got.DirectDeps)
	}
	for _, dep := range got.DirectDeps {
		if !wantDeps[dep] {
			t.Errorf("unexpected dep %q", dep)
		}
	}
}

func TestParsePackageJSON_ScopedName(t *testing.T) {
	input := []byte(`{"name": "@myorg/payment-service", "version": "2.0.0"}`)

	got, err := parser.ParsePackageJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "@myorg/payment-service" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.EntityName != "payment-service" {
		t.Errorf("EntityName = %q, want %q", got.EntityName, "payment-service")
	}
}

func TestParsePackageJSON_NoDependencies(t *testing.T) {
	input := []byte(`{"name": "lean-service", "version": "0.1.0"}`)

	got, err := parser.ParsePackageJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.DirectDeps) != 0 {
		t.Errorf("expected no deps, got %v", got.DirectDeps)
	}
}

func TestParsePackageJSON_MissingName(t *testing.T) {
	input := []byte(`{"version": "1.0.0", "dependencies": {}}`)

	_, err := parser.ParsePackageJSON(input)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestParsePackageJSON_InvalidJSON(t *testing.T) {
	_, err := parser.ParsePackageJSON([]byte(`{not valid json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestPackageEntityName_Scoped(t *testing.T) {
	cases := []struct{ in, want string }{
		{"@myorg/my-service", "my-service"},
		{"my-service", "my-service"},
		{"@scope/pkg", "pkg"},
		{"plain", "plain"},
	}
	for _, c := range cases {
		got := parser.PackageEntityName(c.in)
		if got != c.want {
			t.Errorf("PackageEntityName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
