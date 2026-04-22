// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package token_test

import (
	"testing"

	"github.com/doc-scout/mcp-server/benchmark/token"
)

func TestEstimateNaiveTokensAllQuestions(t *testing.T) {
	for i := range 12 {
		n := token.EstimateNaiveTokens(i)
		if n <= 0 {
			t.Errorf("question %d: got %d tokens, want > 0", i, n)
		}
	}
}

func TestEstimateNaiveTokensOutOfRange(t *testing.T) {
	if got := token.EstimateNaiveTokens(-1); got != 0 {
		t.Errorf("index -1: got %d, want 0", got)
	}
	if got := token.EstimateNaiveTokens(99); got != 0 {
		t.Errorf("index 99: got %d, want 0", got)
	}
}

func TestEstimateDocScoutTokensAllQuestions(t *testing.T) {
	for i := range 12 {
		n := token.EstimateDocScoutTokens(i)
		if n <= 0 {
			t.Errorf("question %d: got %d tokens, want > 0", i, n)
		}
		naive := token.EstimateNaiveTokens(i)
		if n >= naive {
			t.Errorf("question %d: DocScout=%d >= Naive=%d — savings claim is wrong", i, n, naive)
		}
	}
}
