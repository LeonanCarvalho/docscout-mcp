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
