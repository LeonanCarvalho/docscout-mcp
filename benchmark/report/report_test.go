// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package report_test

import (
	"strings"
	"testing"

	"github.com/doc-scout/mcp-server/benchmark/accuracy"
	"github.com/doc-scout/mcp-server/benchmark/report"
	"github.com/doc-scout/mcp-server/benchmark/token"
)

func TestReportContainsRequiredSections(t *testing.T) {
	acc := &accuracy.Results{
		ByParser: map[string]*accuracy.ParserStats{
			"gomod": {Type: "gomod", TP: 5, FP: 0, FN: 0, Precision: 1.0, Recall: 1.0, F1: 1.0},
		},
		Overall: accuracy.ParserStats{Type: "overall", F1: 1.0},
	}
	tok := []token.QuestionEstimate{
		{Index: 0, DocScoutToks: 320, NaiveToks: 27705, SavingsPct: 98.8},
	}

	md := report.Generate(report.Input{
		Version:   "1.0.0",
		Accuracy:  acc,
		TokenEsts: tok,
	})

	required := []string{
		"## Accuracy",
		"## Token Efficiency",
		"gomod",
		"1.00",  // F1 score
		"98.8%", // savings
		"generated_at",
		"docscout_version",
	}
	for _, s := range required {
		if !strings.Contains(md, s) {
			t.Errorf("report missing expected string %q", s)
		}
	}
}
