// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"

	"github.com/doc-scout/mcp-server/memory"
)

// CosineSimilarity returns the cosine similarity in [-1, 1] between two equal-length

// float32 vectors. Returns 0 when either vector has zero magnitude or the lengths differ.

func CosineSimilarity(a, b []float32) float64 {

	if len(a) != len(b) {

		slog.Error("[embeddings] CosineSimilarity: dimension mismatch — returning 0", "lenA", len(a), "lenB", len(b))

		return 0

	}

	var dot, magA, magB float64

	for i := range a {

		ai, bi := float64(a[i]), float64(b[i])

		dot += ai * bi

		magA += ai * ai

		magB += bi * bi

	}

	if magA == 0 || magB == 0 {

		return 0

	}

	return dot / (math.Sqrt(magA) * math.Sqrt(magB))

}

// sha256hex returns the hex-encoded SHA-256 digest of s.

func sha256hex(s string) string {

	h := sha256.Sum256([]byte(s))

	return fmt.Sprintf("%x", h)

}

// EntityText builds the canonical text representation of an entity for embedding.

// Format: "<name> [<entityType>]: <sorted_observations_comma_separated>"

// Observations are sorted so the hash is deterministic regardless of insertion order.

func EntityText(e memory.Entity) string {

	sorted := make([]string, len(e.Observations))

	copy(sorted, e.Observations)

	sort.Strings(sorted)

	obs := strings.Join(sorted, ", ")

	if obs == "" {

		return fmt.Sprintf("%s [%s]", e.Name, e.EntityType)

	}

	return fmt.Sprintf("%s [%s]: %s", e.Name, e.EntityType, obs)

}
