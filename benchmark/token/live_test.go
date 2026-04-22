// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package token_test

import (
	"context"
	"errors"
	"testing"

	"github.com/doc-scout/mcp-server/benchmark/token"
)

func TestLiveRunnerRejectsEmptyAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	_, err := token.NewLiveRunner(context.Background())
	if err == nil {
		t.Fatal("expected error for empty ANTHROPIC_API_KEY, got nil")
	}
	if !errors.Is(err, token.ErrNoAPIKey) {
		t.Errorf("want ErrNoAPIKey, got: %v", err)
	}
}

func TestSavingsPct(t *testing.T) {
	tests := []struct {
		ds, naive int
		want      float64
	}{
		{300, 3000, 90.0},
		{0, 0, 0.0},
		{500, 1000, 50.0},
	}
	for _, tt := range tests {
		got := token.SavingsPct(tt.ds, tt.naive)
		if got != tt.want {
			t.Errorf("SavingsPct(%d,%d) = %.1f, want %.1f", tt.ds, tt.naive, got, tt.want)
		}
	}
}
