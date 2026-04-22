// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

func TestParseGoMod_Basic(t *testing.T) {

	input := []byte(`module github.com/myorg/my-service







go 1.22







require (



	github.com/foo/bar v1.2.3



	github.com/baz/qux v0.1.0 // indirect



)







require github.com/direct/single v1.0.0



`)

	got, err := parser.ParseGoMod(input)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if got.ModulePath != "github.com/myorg/my-service" {

		t.Errorf("ModulePath = %q, want %q", got.ModulePath, "github.com/myorg/my-service")

	}

	if got.EntityName != "my-service" {

		t.Errorf("EntityName = %q, want %q", got.EntityName, "my-service")

	}

	if got.GoVersion != "1.22" {

		t.Errorf("GoVersion = %q, want %q", got.GoVersion, "1.22")

	}

	// Only direct deps: bar and single — qux is indirect and must be excluded.

	wantDeps := map[string]bool{

		"github.com/foo/bar": true,

		"github.com/direct/single": true,
	}

	if len(got.DirectDeps) != len(wantDeps) {

		t.Errorf("DirectDeps count = %d, want %d: %v", len(got.DirectDeps), len(wantDeps), got.DirectDeps)

	}

	for _, dep := range got.DirectDeps {

		if !wantDeps[dep] {

			t.Errorf("unexpected dep %q", dep)

		}

	}

}

func TestParseGoMod_OnlyIndirectDeps(t *testing.T) {

	input := []byte(`module github.com/myorg/lean-service







go 1.21







require (



	github.com/transitive/dep v2.0.0 // indirect



)



`)

	got, err := parser.ParseGoMod(input)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if len(got.DirectDeps) != 0 {

		t.Errorf("expected no direct deps, got %v", got.DirectDeps)

	}

}

func TestParseGoMod_MissingModule(t *testing.T) {

	input := []byte(`go 1.22







require github.com/foo/bar v1.0.0



`)

	_, err := parser.ParseGoMod(input)

	if err == nil {

		t.Error("expected error for missing module declaration")

	}

}

func TestParseGoMod_SimpleModule(t *testing.T) {

	input := []byte(`module myapp







go 1.22



`)

	got, err := parser.ParseGoMod(input)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if got.EntityName != "myapp" {

		t.Errorf("EntityName = %q, want %q", got.EntityName, "myapp")

	}

}
