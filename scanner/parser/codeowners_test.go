// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

func TestParseCodeowners_Basic(t *testing.T) {

	input := []byte(`# Global owner

* @myorg/platform-team



# Backend services

/services/ @myorg/backend-team @alice



# Frontend

/frontend/ bob@example.com

`)

	got := parser.ParseCodeowners(input)

	wantOwners := map[string]string{

		"platform-team": "team",

		"backend-team": "team",

		"alice": "person",

		"bob": "person",
	}

	if len(got.UniqueOwners) != len(wantOwners) {

		t.Fatalf("UniqueOwners count = %d, want %d: %+v", len(got.UniqueOwners), len(wantOwners), got.UniqueOwners)

	}

	for _, o := range got.UniqueOwners {

		wantType, ok := wantOwners[o.EntityName]

		if !ok {

			t.Errorf("unexpected owner entity name %q", o.EntityName)

			continue

		}

		if o.EntityType != wantType {

			t.Errorf("owner %q: EntityType = %q, want %q", o.EntityName, o.EntityType, wantType)

		}

	}

}

func TestParseCodeowners_Deduplication(t *testing.T) {

	input := []byte(`/docs/ @myorg/docs-team

/api/  @myorg/docs-team @alice

*.md   @alice

`)

	got := parser.ParseCodeowners(input)

	// @myorg/docs-team and @alice appear multiple times — should deduplicate to 2.

	if len(got.UniqueOwners) != 2 {

		t.Errorf("UniqueOwners count = %d, want 2: %+v", len(got.UniqueOwners), got.UniqueOwners)

	}

}

func TestParseCodeowners_EmptyFile(t *testing.T) {

	got := parser.ParseCodeowners([]byte(""))

	if len(got.UniqueOwners) != 0 {

		t.Errorf("expected no owners for empty file, got %+v", got.UniqueOwners)

	}

}

func TestParseCodeowners_CommentsOnly(t *testing.T) {

	input := []byte(`# This is a comment

# Another comment

`)

	got := parser.ParseCodeowners(input)

	if len(got.UniqueOwners) != 0 {

		t.Errorf("expected no owners for comments-only file, got %+v", got.UniqueOwners)

	}

}

func TestParseCodeowners_PatternWithNoOwners(t *testing.T) {

	input := []byte(`# Rule with no owners — should be skipped

/orphan/

/with-owner/ @solo

`)

	got := parser.ParseCodeowners(input)

	if len(got.UniqueOwners) != 1 {

		t.Fatalf("UniqueOwners count = %d, want 1: %+v", len(got.UniqueOwners), got.UniqueOwners)

	}

	if got.UniqueOwners[0].EntityName != "solo" {

		t.Errorf("EntityName = %q, want %q", got.UniqueOwners[0].EntityName, "solo")

	}

}

func TestParseCodeowners_OwnerTypes(t *testing.T) {

	cases := []struct {
		token string

		wantName string

		wantType string
	}{

		{"@myorg/infra-team", "infra-team", "team"},

		{"@alice", "alice", "person"},

		{"alice@company.com", "alice", "person"}, // plain email (no leading @)

		{"UNKNOWN_TOKEN", "unknown_token", "person"}, // fallback

	}

	for _, tc := range cases {

		input := []byte("* " + tc.token + "\n")

		got := parser.ParseCodeowners(input)

		if len(got.UniqueOwners) != 1 {

			t.Errorf("token %q: expected 1 owner, got %d", tc.token, len(got.UniqueOwners))

			continue

		}

		o := got.UniqueOwners[0]

		if o.EntityName != tc.wantName {

			t.Errorf("token %q: EntityName = %q, want %q", tc.token, o.EntityName, tc.wantName)

		}

		if o.EntityType != tc.wantType {

			t.Errorf("token %q: EntityType = %q, want %q", tc.token, o.EntityType, tc.wantType)

		}

		if o.Raw != tc.token {

			t.Errorf("token %q: Raw = %q, want original token", tc.token, o.Raw)

		}

	}

}
