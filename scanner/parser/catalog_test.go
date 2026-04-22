// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"strings"
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

func TestParseCatalog_Component(t *testing.T) {

	yaml := []byte(`

apiVersion: backstage.io/v1alpha1

kind: Component

metadata:

  name: payment-service

  description: Handles payment transactions

  tags:

    - payment

    - critical

spec:

  type: service

  lifecycle: production

  owner: team-payments

  system: payment-platform

  dependsOn:

    - component:database

  providesApis:

    - payment-api

  consumesApis:

    - notification-api

`)

	got, err := parser.ParseCatalog(yaml)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if got.EntityName != "payment-service" {

		t.Errorf("EntityName: want payment-service, got %s", got.EntityName)

	}

	if got.EntityType != "service" {

		t.Errorf("EntityType: want service, got %s", got.EntityType)

	}

	wantObs := map[string]bool{

		"lifecycle:production": true,

		"description:Handles payment transactions": true,

		"tag:payment": true,

		"tag:critical": true,
	}

	for _, obs := range got.Observations {

		delete(wantObs, obs)

	}

	if len(wantObs) > 0 {

		t.Errorf("missing observations: %v", wantObs)

	}

	wantRels := map[string]bool{

		"payment-service->team-payments:owned_by": true,

		"payment-service->payment-platform:part_of": true,

		"payment-service->component:database:depends_on": true,

		"payment-service->payment-api:provides_api": true,

		"payment-service->notification-api:consumes_api": true,
	}

	for _, r := range got.Relations {

		key := r.From + "->" + r.To + ":" + r.RelationType

		delete(wantRels, key)

	}

	if len(wantRels) > 0 {

		t.Errorf("missing relations: %v", wantRels)

	}

}

func TestParseCatalog_KindMapping(t *testing.T) {

	cases := []struct {
		kind string

		specType string

		wantType string
	}{

		{"API", "", "api"},

		{"System", "", "system"},

		{"Resource", "", "resource"},

		{"Group", "", "team"},

		{"Component", "library", "library"},

		{"Component", "", "component"},

		{"Unknown", "", "component"},
	}

	for _, tc := range cases {

		yaml := []byte("apiVersion: backstage.io/v1alpha1\nkind: " + tc.kind + "\nmetadata:\n  name: test-entity\nspec:\n  type: " + tc.specType + "\n")

		got, err := parser.ParseCatalog(yaml)

		if err != nil {

			t.Fatalf("kind=%s: unexpected error: %v", tc.kind, err)

		}

		if got.EntityType != tc.wantType {

			t.Errorf("kind=%s specType=%s: want entityType=%s, got=%s", tc.kind, tc.specType, tc.wantType, got.EntityType)

		}

	}

}

func TestParseCatalog_MissingOptionalFields(t *testing.T) {

	yaml := []byte(`

apiVersion: backstage.io/v1alpha1

kind: Component

metadata:

  name: minimal-service

spec:

  type: service

`)

	got, err := parser.ParseCatalog(yaml)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if got.EntityName != "minimal-service" {

		t.Errorf("want minimal-service, got %s", got.EntityName)

	}

	if len(got.Observations) != 0 {

		t.Errorf("expected no observations, got %v", got.Observations)

	}

	if len(got.Relations) != 0 {

		t.Errorf("expected no relations, got %v", got.Relations)

	}

}

func TestParseCatalog_MissingName(t *testing.T) {

	yaml := []byte("apiVersion: backstage.io/v1alpha1\nkind: Component\nmetadata:\n  description: no name\n")

	_, err := parser.ParseCatalog(yaml)

	if err == nil {

		t.Fatal("expected error for missing metadata.name")

	}

}

func TestParseCatalog_MalformedYAML(t *testing.T) {

	_, err := parser.ParseCatalog([]byte("this: is: not: valid: yaml: :::"))

	if err == nil {

		t.Fatal("expected error for malformed YAML")

	}

}

func TestParseCatalog_InvalidEntityNames(t *testing.T) {

	cases := []struct {
		name string

		input string

		wantErr bool
	}{

		{"null byte", "payment\x00service", true},

		{"newline", "payment\nservice", true},

		{"too long", "a" + strings.Repeat("b", 253), true},

		{"empty after trim", "  ", true},

		{"valid with dash", "payment-service", false},

		{"valid with dot", "payment.v2", false},

		{"valid with namespace", "default/payment-service", false},

		{"valid with underscore", "payment_service", false},

		{"valid with number", "svc-1", false},
	}

	for _, tc := range cases {

		yaml := []byte("apiVersion: backstage.io/v1alpha1\nkind: Component\nmetadata:\n  name: " + tc.input + "\nspec:\n  type: service\n")

		_, err := parser.ParseCatalog(yaml)

		if tc.wantErr && err == nil {

			t.Errorf("name=%q: expected error but got none", tc.name)

		}

		if !tc.wantErr && err != nil {

			t.Errorf("name=%q: unexpected error: %v", tc.name, err)

		}

	}

}
