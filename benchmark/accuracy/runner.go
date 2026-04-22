// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package accuracy

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

// TestCase is one entry in ground_truth.json.
type TestCase struct {
	ID                string        `json:"id"`
	Parser            string        `json:"parser"`
	InputFile         string        `json:"input_file"`
	ExpectedName      string        `json:"expected_entity_name"`
	ExpectedType      string        `json:"expected_entity_type"`
	ExpectedObsSubset []string      `json:"expected_obs_subset"`
	ExpectedRels      []ExpectedRel `json:"expected_rels"`
	ExpectedAux       []ExpectedAux `json:"expected_aux"`
}

// ExpectedRel is a relation assertion in a test case.
type ExpectedRel struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// ExpectedAux is an auxiliary entity assertion in a test case.
type ExpectedAux struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// GroundTruth is the root structure of ground_truth.json.
type GroundTruth struct {
	Version string     `json:"version"`
	Cases   []TestCase `json:"cases"`
}

// ParserStats holds accuracy metrics for one parser type.
type ParserStats struct {
	Type       string
	TP, FP, FN int
	Precision  float64
	Recall     float64
	F1         float64
}

// Results holds accuracy results across all parser types.
type Results struct {
	ByParser map[string]*ParserStats
	Overall  ParserStats
}

// parserRegistry maps FileType → FileParser for all parsers covered in the benchmark.
var parserRegistry = map[string]parser.FileParser{
	"gomod":        parser.GoModParser(),
	"packagejson":  parser.PackageJSONParser(),
	"pomxml":       parser.PomParser(),
	"catalog-info": parser.CatalogParser(),
	"codeowners":   parser.CodeownersParser(),
	"asyncapi":     parser.AsyncAPIParser(),
	"openapi":      parser.OpenAPIParser(),
	"proto":        parser.ProtoParser(),
	"spring-kafka": parser.SpringKafkaParser(),
	"k8s":          parser.K8sServiceParser(),
}

// Run executes the accuracy benchmark against all cases in ground_truth.json.
// fsys must be an fs.FS rooted at the benchmark/testdata directory.
func Run(fsys fs.FS) (*Results, error) {
	data, err := fs.ReadFile(fsys, "ground_truth.json")
	if err != nil {
		return nil, fmt.Errorf("accuracy: read ground_truth.json: %w", err)
	}
	var gt GroundTruth
	if err := json.Unmarshal(data, &gt); err != nil {
		return nil, fmt.Errorf("accuracy: parse ground_truth.json: %w", err)
	}

	res := &Results{ByParser: make(map[string]*ParserStats)}

	for _, tc := range gt.Cases {
		p, ok := parserRegistry[tc.Parser]
		if !ok {
			return nil, fmt.Errorf("accuracy: unknown parser %q in case %q", tc.Parser, tc.ID)
		}

		raw, err := fs.ReadFile(fsys, tc.InputFile)
		if err != nil {
			return nil, fmt.Errorf("accuracy: read %q: %w", tc.InputFile, err)
		}

		pf, err := p.Parse(raw)
		if err != nil {
			stats := getOrCreate(res, tc.Parser)
			stats.FN += len(tc.ExpectedRels) + len(tc.ExpectedAux) + boolInt(tc.ExpectedName != "")
			continue
		}

		score(res, tc, pf)
	}

	computeMetrics(res)
	return res, nil
}

func getOrCreate(res *Results, parserType string) *ParserStats {
	if _, ok := res.ByParser[parserType]; !ok {
		res.ByParser[parserType] = &ParserStats{Type: parserType}
	}
	return res.ByParser[parserType]
}

func score(res *Results, tc TestCase, pf parser.ParsedFile) {
	stats := getOrCreate(res, tc.Parser)

	// Entity name + type
	if tc.ExpectedName != "" {
		if pf.EntityName == tc.ExpectedName && pf.EntityType == tc.ExpectedType {
			stats.TP++
		} else {
			stats.FN++
			if pf.EntityName != "" {
				stats.FP++
			}
		}
	}

	// Observations (subset match: every expected obs must appear in actual)
	for _, obs := range tc.ExpectedObsSubset {
		if slices.Contains(pf.Observations, obs) {
			stats.TP++
		} else {
			stats.FN++
		}
	}

	// Relations
	for _, er := range tc.ExpectedRels {
		if containsRel(pf.Relations, er) {
			stats.TP++
		} else {
			stats.FN++
		}
	}
	for _, rel := range pf.Relations {
		if !containsExpectedRel(tc.ExpectedRels, rel) {
			stats.FP++
		}
	}

	// AuxEntities
	for _, ea := range tc.ExpectedAux {
		if containsAux(pf.AuxEntities, ea) {
			stats.TP++
		} else {
			stats.FN++
		}
	}
	for _, aux := range pf.AuxEntities {
		if !containsExpectedAux(tc.ExpectedAux, aux) {
			stats.FP++
		}
	}
}

func computeMetrics(res *Results) {
	var totalTP, totalFP, totalFN int
	for _, s := range res.ByParser {
		if s.TP+s.FP > 0 {
			s.Precision = float64(s.TP) / float64(s.TP+s.FP)
		}
		if s.TP+s.FN > 0 {
			s.Recall = float64(s.TP) / float64(s.TP+s.FN)
		}
		if s.Precision+s.Recall > 0 {
			s.F1 = 2 * s.Precision * s.Recall / (s.Precision + s.Recall)
		}
		totalTP += s.TP
		totalFP += s.FP
		totalFN += s.FN
	}
	o := &res.Overall
	o.Type = "overall"
	o.TP, o.FP, o.FN = totalTP, totalFP, totalFN
	if totalTP+totalFP > 0 {
		o.Precision = float64(totalTP) / float64(totalTP+totalFP)
	}
	if totalTP+totalFN > 0 {
		o.Recall = float64(totalTP) / float64(totalTP+totalFN)
	}
	if o.Precision+o.Recall > 0 {
		o.F1 = 2 * o.Precision * o.Recall / (o.Precision + o.Recall)
	}
}

func containsRel(rels []parser.ParsedRelation, er ExpectedRel) bool {
	for _, r := range rels {
		if r.From == er.From && r.To == er.To && r.RelationType == er.Type {
			return true
		}
	}
	return false
}

func containsExpectedRel(expected []ExpectedRel, rel parser.ParsedRelation) bool {
	for _, er := range expected {
		if er.From == rel.From && er.To == rel.To && er.Type == rel.RelationType {
			return true
		}
	}
	return false
}

func containsAux(auxEntities []parser.AuxEntity, ea ExpectedAux) bool {
	for _, a := range auxEntities {
		if a.Name == ea.Name && a.EntityType == ea.Type {
			return true
		}
	}
	return false
}

func containsExpectedAux(expected []ExpectedAux, aux parser.AuxEntity) bool {
	for _, ea := range expected {
		if ea.Name == aux.Name && ea.Type == aux.EntityType {
			return true
		}
	}
	return false
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
