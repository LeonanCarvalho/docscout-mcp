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
