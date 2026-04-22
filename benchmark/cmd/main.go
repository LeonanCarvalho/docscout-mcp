// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

// Package benchmarkcmd implements the --benchmark subcommand for docscout-mcp.
// Imported by the root main.go via early os.Args intercept.
package benchmarkcmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"

	"github.com/doc-scout/mcp-server/benchmark/accuracy"
	"github.com/doc-scout/mcp-server/benchmark/report"
	"github.com/doc-scout/mcp-server/benchmark/testdata"
	"github.com/doc-scout/mcp-server/benchmark/token"
)

// testdataFS returns the best available fs.FS for the benchmark testdata.
// Go's embed directive cannot include directories containing go.mod files
// (they are treated as separate modules). So we prefer os.DirFS when the
// testdata directory is accessible on disk (typical for developer runs),
// falling back to the embedded FS for the subset of files it does contain.
func testdataFS() fs.FS {
	const dir = "benchmark/testdata"
	if _, err := os.Stat(dir); err == nil {
		return os.DirFS(dir)
	}
	return testdata.FS
}

// Run is the entry point called from main.go when --benchmark is the first arg.
// args is os.Args[2:] (everything after "--benchmark").
// Returns an exit code.
func Run(args []string) int {
	fset := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	mode := fset.String("mode", "theoretical", "theoretical|live")
	output := fset.String("output", "benchmark/RESULTS.md", "output file path (- for stdout)")
	maxQ := fset.Int("max-questions", 12, "cap number of questions (live mode cost guard)")
	dryRun := fset.Bool("dry-run", false, "print plan and estimated cost, do not run")
	version := fset.String("version", "dev", "docscout version stamp for the report")
	_ = fset.Parse(args)

	if *dryRun {
		printDryRun(*mode, *maxQ)
		return 0
	}

	ctx := context.Background()

	fsys := testdataFS()

	questionsData, err := fs.ReadFile(fsys, "questions.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "benchmark: %v\n", err)
		return 1
	}
	var questions []string
	if err := json.Unmarshal(questionsData, &questions); err != nil {
		fmt.Fprintf(os.Stderr, "benchmark: parse questions.json: %v\n", err)
		return 1
	}
	if *maxQ > 0 && *maxQ < len(questions) {
		questions = questions[:*maxQ]
	}

	fmt.Fprintln(os.Stderr, "Running accuracy benchmark...")
	accResults, err := accuracy.Run(fsys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "benchmark: accuracy: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Accuracy: overall F1=%.2f\n", accResults.Overall.F1)

	estimates := token.AllEstimates()
	if *maxQ < len(estimates) {
		estimates = estimates[:*maxQ]
	}

	inp := report.Input{
		Version:   *version,
		Accuracy:  accResults,
		TokenEsts: estimates,
		Questions: questions,
	}

	if *mode == "live" {
		fmt.Fprintln(os.Stderr, "Running live token benchmark (Claude API)...")
		runner, err := token.NewLiveRunner(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "benchmark: live mode: %v\n", err)
			return 1
		}
		liveResults, err := runLiveQuestions(ctx, runner, questions, fsys)
		if err != nil {
			fmt.Fprintf(os.Stderr, "benchmark: live questions: %v\n", err)
			return 1
		}
		inp.LiveRes = liveResults
	}

	md := report.Generate(inp)

	if *output == "-" {
		fmt.Print(md)
	} else {
		if err := os.WriteFile(*output, []byte(md), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "benchmark: write %s: %v\n", *output, err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "Report written to %s\n", *output)
	}
	return 0
}

func printDryRun(mode string, maxQ int) {
	fmt.Fprintf(os.Stderr, "Benchmark dry-run\n")
	fmt.Fprintf(os.Stderr, "  mode:          %s\n", mode)
	fmt.Fprintf(os.Stderr, "  max-questions: %d\n", maxQ)
	if mode == "live" {
		estimatedCalls := maxQ * 2
		fmt.Fprintf(os.Stderr, "  estimated API calls: %d (claude-haiku-4-5, max_tokens=100)\n", estimatedCalls)
		fmt.Fprintf(os.Stderr, "  estimated cost:      ~$%.4f USD\n", float64(estimatedCalls)*0.00025)
	}
	fmt.Fprintf(os.Stderr, "  ANTHROPIC_API_KEY: %s\n", keyStatus())
}

func keyStatus() string {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "set"
	}
	return "NOT SET"
}

// runLiveQuestions is a stub pending full in-process graph wiring (see TODO).
// Returns nil so --mode live works end-to-end without live results in the report.
func runLiveQuestions(_ context.Context, _ *token.LiveRunner, _ []string, _ fs.FS) ([]token.LiveResult, error) {
	return nil, nil
}
