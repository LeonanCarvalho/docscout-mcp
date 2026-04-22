// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"sync"
	"testing"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

// stubParser is a minimal FileParser for testing.
type stubParser struct {
	fileType  string
	filenames []string
}

func (s *stubParser) FileType() string    { return s.fileType }
func (s *stubParser) Filenames() []string { return s.filenames }
func (s *stubParser) Parse(_ []byte) (parser.ParsedFile, error) {
	return parser.ParsedFile{EntityName: "stub"}, nil
}

func TestRegistry_GetReturnsRegistered(t *testing.T) {
	reg := parser.NewRegistry()
	p := &stubParser{fileType: "mytype", filenames: []string{"myfile"}}
	reg.Register(p)

	got, ok := reg.Get("mytype")
	if !ok {
		t.Fatal("expected Get to return true")
	}
	if got.FileType() != "mytype" {
		t.Errorf("got FileType %q, want %q", got.FileType(), "mytype")
	}
}

func TestRegistry_GetUnknownReturnsFalse(t *testing.T) {
	reg := parser.NewRegistry()
	_, ok := reg.Get("nope")
	if ok {
		t.Fatal("expected Get to return false for unknown type")
	}
}

func TestRegistry_TargetFilenames(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubParser{fileType: "a", filenames: []string{"file-a", "alt-a"}})
	reg.Register(&stubParser{fileType: "b", filenames: []string{"file-b"}})

	names := reg.TargetFilenames()
	want := map[string]bool{"file-a": true, "alt-a": true, "file-b": true}
	if len(names) != len(want) {
		t.Fatalf("TargetFilenames len=%d, want %d: %v", len(names), len(want), names)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected filename %q", n)
		}
	}
}

func TestRegistry_DuplicateFileTypePanics(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubParser{fileType: "dup", filenames: []string{"f1"}})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate FileType")
		}
	}()
	reg.Register(&stubParser{fileType: "dup", filenames: []string{"f2"}})
}

func TestRegistry_DuplicateFilenamePanics(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubParser{fileType: "a", filenames: []string{"shared"}})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate filename")
		}
	}()
	reg.Register(&stubParser{fileType: "b", filenames: []string{"shared"}})
}

func TestRegistry_AllReturnsAll(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubParser{fileType: "x", filenames: []string{"fx"}})
	reg.Register(&stubParser{fileType: "y", filenames: []string{"fy"}})

	all := reg.All()
	if len(all) != 2 {
		t.Errorf("All() len=%d, want 2", len(all))
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := parser.NewRegistry()
	var wg sync.WaitGroup
	// Pre-register a few parsers so Get has something to find.
	for i := range 5 {
		ft := "type" + string(rune('a'+i))
		fn := "file" + string(rune('a'+i))
		reg.Register(&stubParser{fileType: ft, filenames: []string{fn}})
	}
	// Concurrent reads.
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.Get("typea")
			reg.All()
			reg.TargetFilenames()
		}()
	}
	wg.Wait()
}
