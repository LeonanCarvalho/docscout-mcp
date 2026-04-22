// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package accuracy_test

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/doc-scout/mcp-server/benchmark/accuracy"
)

func TestRunnerAllCasesPass(t *testing.T) {
	fsys := os.DirFS("../testdata")
	results, err := accuracy.Run(fsys)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	for parserType, stats := range results.ByParser {
		if stats.F1 < 0.95 {
			t.Errorf("parser %q: F1=%.2f < 0.95 (TP=%d FP=%d FN=%d)",
				parserType, stats.F1, stats.TP, stats.FP, stats.FN)
		}
	}
}

func TestRunnerUnknownParser(t *testing.T) {
	gtJSON := `{"version":"1.0","cases":[{"id":"x","parser":"nonexistent","input_file":"f","expected_entity_name":"","expected_entity_type":"","expected_obs_subset":[],"expected_rels":[],"expected_aux":[]}]}`
	memFS := fstest.MapFS{
		"ground_truth.json": {Data: []byte(gtJSON)},
		"f":                 {Data: []byte("dummy")},
	}
	_, err := accuracy.Run(memFS)
	if err == nil {
		t.Fatal("expected error for unknown parser, got nil")
	}
}
