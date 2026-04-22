// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package embeddings_test

import (
	"math"
	"testing"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
)

func TestCosineSimilarity_Identical(t *testing.T) {

	v := []float32{1.0, 0.0, 0.0}

	got := embeddings.CosineSimilarity(v, v)

	if math.Abs(got-1.0) > 1e-6 {

		t.Errorf("identical vectors: want 1.0, got %f", got)

	}

}

func TestCosineSimilarity_Orthogonal(t *testing.T) {

	a := []float32{1.0, 0.0}

	b := []float32{0.0, 1.0}

	got := embeddings.CosineSimilarity(a, b)

	if math.Abs(got) > 1e-6 {

		t.Errorf("orthogonal vectors: want 0.0, got %f", got)

	}

}

func TestCosineSimilarity_ZeroVector(t *testing.T) {

	z := []float32{0.0, 0.0}

	a := []float32{1.0, 0.0}

	got := embeddings.CosineSimilarity(z, a)

	if got != 0 {

		t.Errorf("zero vector: want 0, got %f", got)

	}

}

func TestEntityText_SortedObservations(t *testing.T) {

	e1 := memory.Entity{Name: "svc", EntityType: "service", Observations: []string{"b", "a"}}

	e2 := memory.Entity{Name: "svc", EntityType: "service", Observations: []string{"a", "b"}}

	if embeddings.EntityText(e1) != embeddings.EntityText(e2) {

		t.Error("EntityText must sort observations for deterministic hashing")

	}

}

func TestEntityText_Format(t *testing.T) {

	e := memory.Entity{Name: "payment-svc", EntityType: "service", Observations: []string{"lang:go", "owner:platform"}}

	got := embeddings.EntityText(e)

	want := "payment-svc [service]: lang:go, owner:platform"

	if got != want {

		t.Errorf("want %q, got %q", want, got)

	}

}

func TestEntityText_NoObservations(t *testing.T) {

	e := memory.Entity{Name: "foo", EntityType: "team"}

	got := embeddings.EntityText(e)

	want := "foo [team]"

	if got != want {

		t.Errorf("want %q, got %q", want, got)

	}

}
