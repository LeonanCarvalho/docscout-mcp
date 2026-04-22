// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package health_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/doc-scout/mcp-server/health"
)

type stubProvider struct {
	s health.Status
}

func (p *stubProvider) HealthStatus() health.Status { return p.s }

func TestHandler_Ready(t *testing.T) {

	p := &stubProvider{s: health.Status{

		Status: "ok",

		StartedAt: time.Now(),

		RepoCount: 3,

		Entities: 42,
	}}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	w := httptest.NewRecorder()

	health.Handler(p)(w, req)

	if w.Code != http.StatusOK {

		t.Fatalf("expected 200, got %d", w.Code)

	}

	var result health.Status

	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {

		t.Fatalf("decode: %v", err)

	}

	if result.Status != "ok" {

		t.Errorf("expected status ok, got %s", result.Status)

	}

	if result.RepoCount != 3 {

		t.Errorf("expected repos 3, got %d", result.RepoCount)

	}

	if result.Entities != 42 {

		t.Errorf("expected entities 42, got %d", result.Entities)

	}

}

func TestHandler_Starting(t *testing.T) {

	p := &stubProvider{s: health.Status{

		Status: "starting",

		StartedAt: time.Now(),
	}}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	w := httptest.NewRecorder()

	health.Handler(p)(w, req)

	if w.Code != http.StatusServiceUnavailable {

		t.Fatalf("expected 503, got %d", w.Code)

	}

	var result health.Status

	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {

		t.Fatalf("decode: %v", err)

	}

	if result.Status != "starting" {

		t.Errorf("expected status starting, got %s", result.Status)

	}

}

func TestHandler_ContentType(t *testing.T) {

	p := &stubProvider{s: health.Status{Status: "ok"}}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	w := httptest.NewRecorder()

	health.Handler(p)(w, req)

	ct := w.Header().Get("Content-Type")

	if ct != "application/json" {

		t.Errorf("expected Content-Type application/json, got %s", ct)

	}

}
