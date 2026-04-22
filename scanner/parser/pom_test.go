// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

func TestParsePom_Basic(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>com.myorg</groupId>
  <artifactId>my-service</artifactId>
  <version>1.2.3</version>
  <dependencies>
    <dependency>
      <groupId>org.springframework.boot</groupId>
      <artifactId>spring-boot-starter-web</artifactId>
    </dependency>
    <dependency>
      <groupId>com.fasterxml.jackson.core</groupId>
      <artifactId>jackson-databind</artifactId>
      <scope>compile</scope>
    </dependency>
    <dependency>
      <groupId>org.springframework.boot</groupId>
      <artifactId>spring-boot-starter-test</artifactId>
      <scope>test</scope>
    </dependency>
    <dependency>
      <groupId>javax.servlet</groupId>
      <artifactId>javax.servlet-api</artifactId>
      <scope>provided</scope>
    </dependency>
  </dependencies>
</project>`)

	got, err := parser.ParsePom(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.GroupID != "com.myorg" {
		t.Errorf("GroupID = %q, want %q", got.GroupID, "com.myorg")
	}
	if got.ArtifactID != "my-service" {
		t.Errorf("ArtifactID = %q, want %q", got.ArtifactID, "my-service")
	}
	if got.EntityName != "my-service" {
		t.Errorf("EntityName = %q, want %q", got.EntityName, "my-service")
	}
	if got.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", got.Version, "1.2.3")
	}

	// test and provided scopes must be excluded; compile (default and explicit) included.
	wantDeps := map[string]bool{
		"spring-boot-starter-web": true,
		"jackson-databind":        true,
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

func TestParsePom_OnlyTestDeps(t *testing.T) {
	input := []byte(`<project>
  <groupId>com.example</groupId>
  <artifactId>lean-service</artifactId>
  <dependencies>
    <dependency>
      <artifactId>junit</artifactId>
      <scope>test</scope>
    </dependency>
  </dependencies>
</project>`)

	got, err := parser.ParsePom(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.DirectDeps) != 0 {
		t.Errorf("expected no direct deps, got %v", got.DirectDeps)
	}
}

func TestParsePom_MissingArtifactID(t *testing.T) {
	input := []byte(`<project>
  <groupId>com.example</groupId>
  <version>1.0.0</version>
</project>`)

	_, err := parser.ParsePom(input)
	if err == nil {
		t.Error("expected error for missing artifactId")
	}
}

func TestParsePom_NoDependencies(t *testing.T) {
	input := []byte(`<project>
  <groupId>com.example</groupId>
  <artifactId>simple-lib</artifactId>
  <version>0.1.0</version>
</project>`)

	got, err := parser.ParsePom(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.EntityName != "simple-lib" {
		t.Errorf("EntityName = %q, want %q", got.EntityName, "simple-lib")
	}
	if len(got.DirectDeps) != 0 {
		t.Errorf("expected no deps, got %v", got.DirectDeps)
	}
}

func TestParsePom_RuntimeScope(t *testing.T) {
	input := []byte(`<project>
  <artifactId>my-app</artifactId>
  <dependencies>
    <dependency>
      <artifactId>mysql-connector-java</artifactId>
      <scope>runtime</scope>
    </dependency>
    <dependency>
      <artifactId>lombok</artifactId>
      <scope>provided</scope>
    </dependency>
  </dependencies>
</project>`)

	got, err := parser.ParsePom(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// runtime is included, provided is excluded
	if len(got.DirectDeps) != 1 || got.DirectDeps[0] != "mysql-connector-java" {
		t.Errorf("DirectDeps = %v, want [mysql-connector-java]", got.DirectDeps)
	}
}
