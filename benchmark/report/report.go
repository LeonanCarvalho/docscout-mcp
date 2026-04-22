// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package report

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/doc-scout/mcp-server/benchmark/accuracy"
	"github.com/doc-scout/mcp-server/benchmark/token"
)

// Input holds everything needed to generate a benchmark report.
type Input struct {
	Version   string
	Accuracy  *accuracy.Results
	TokenEsts []token.QuestionEstimate // theoretical estimates
	LiveRes   []token.LiveResult       // optional, live mode results
	Questions []string                 // canonical question strings
}

// Generate produces a markdown benchmark report from the given inputs.
// The output is suitable for committing as benchmark/RESULTS.md.
func Generate(in Input) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# DocScout-MCP Benchmark Results\n\n")
	fmt.Fprintf(&b, "<!-- generated_at: %s -->\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "<!-- docscout_version: %s -->\n\n", in.Version)
	fmt.Fprintf(&b, "> Reproduce: `make benchmark` (theoretical) or `make benchmark-live` (requires `ANTHROPIC_API_KEY`)\n\n")
	fmt.Fprintf(&b, "---\n\n")

	// Accuracy section
	fmt.Fprintf(&b, "## Accuracy (Synthetic Corpus)\n\n")
	fmt.Fprintf(&b, "Tests whether each parser correctly extracts entities, relations, and observations from known fixtures.\n\n")
	fmt.Fprintf(&b, "| Parser | Precision | Recall | F1 | TP | FP | FN |\n")
	fmt.Fprintf(&b, "|--------|-----------|--------|----|----|----|----|\n")
	for _, s := range sortedParsers(in.Accuracy.ByParser) {
		fmt.Fprintf(&b, "| `%s` | %.2f | %.2f | **%.2f** | %d | %d | %d |\n",
			s.Type, s.Precision, s.Recall, s.F1, s.TP, s.FP, s.FN)
	}
	fmt.Fprintf(&b, "| **overall** | %.2f | %.2f | **%.2f** | %d | %d | %d |\n\n",
		in.Accuracy.Overall.Precision, in.Accuracy.Overall.Recall, in.Accuracy.Overall.F1,
		in.Accuracy.Overall.TP, in.Accuracy.Overall.FP, in.Accuracy.Overall.FN)

	// Token efficiency section
	fmt.Fprintf(&b, "## Token Efficiency (Theoretical Model)\n\n")
	fmt.Fprintf(&b, "Estimated tokens consumed per question: DocScout vs naive file-by-file reading.\n")
	fmt.Fprintf(&b, "Naive baseline assumes an AI reads each relevant file individually from GitHub.\n\n")
	fmt.Fprintf(&b, "| # | Question | DocScout | Naive | Savings |\n")
	fmt.Fprintf(&b, "|---|----------|----------|-------|---------|\n")

	var totalDS, totalNaive int
	for i, est := range in.TokenEsts {
		q := questionLabel(in.Questions, i)
		fmt.Fprintf(&b, "| %d | %s | %d | %d | **%.1f%%** |\n",
			i+1, q, est.DocScoutToks, est.NaiveToks, est.SavingsPct)
		totalDS += est.DocScoutToks
		totalNaive += est.NaiveToks
	}
	n := len(in.TokenEsts)
	avg := savingsPct(totalDS/n, totalNaive/n)
	fmt.Fprintf(&b, "| — | **Average** | **%d** | **%d** | **%.1f%%** |\n\n",
		totalDS/n, totalNaive/n, avg)

	// Live results section (optional)
	if len(in.LiveRes) > 0 {
		fmt.Fprintf(&b, "## Token Efficiency (Live — Claude API)\n\n")
		fmt.Fprintf(&b, "Actual input tokens recorded from Claude API calls (claude-haiku-4-5, max_tokens=100).\n\n")
		fmt.Fprintf(&b, "| # | Question | DocScout | Naive | Savings |\n")
		fmt.Fprintf(&b, "|---|----------|----------|-------|---------|\n")
		var liveDS, liveNaive int
		for _, lr := range in.LiveRes {
			q := questionLabel(in.Questions, lr.Index)
			fmt.Fprintf(&b, "| %d | %s | %d | %d | **%.1f%%** |\n",
				lr.Index+1, q, lr.DocScoutToks, lr.NaiveToks, lr.SavingsPct)
			liveDS += lr.DocScoutToks
			liveNaive += lr.NaiveToks
		}
		nl := len(in.LiveRes)
		liveAvg := savingsPct(liveDS/nl, liveNaive/nl)
		fmt.Fprintf(&b, "| — | **Average** | **%d** | **%d** | **%.1f%%** |\n\n",
			liveDS/nl, liveNaive/nl, liveAvg)
	}

	return b.String()
}

// savingsPct returns the percentage of tokens saved by using DocScout vs naive.
func savingsPct(ds, naive int) float64 {
	if naive == 0 {
		return 0
	}
	return float64(naive-ds) / float64(naive) * 100
}

func questionLabel(questions []string, i int) string {
	if i >= 0 && i < len(questions) {
		q := questions[i]
		if len(q) > 50 {
			return q[:47] + "..."
		}
		return q
	}
	return fmt.Sprintf("question %d", i)
}

func sortedParsers(m map[string]*accuracy.ParserStats) []*accuracy.ParserStats {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]*accuracy.ParserStats, len(keys))
	for i, k := range keys {
		out[i] = m[k]
	}
	return out
}
