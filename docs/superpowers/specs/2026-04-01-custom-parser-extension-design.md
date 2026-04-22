# Custom Parser Extension — Design Spec

**Date:** 2026-04-01
**Status:** Approved
**Goal:** Allow users to register custom manifest parsers (e.g. `Pipfile`, `.tool-versions`, `chart.lock`) without modifying DocScout-MCP core code, while simultaneously eliminating the five hardcoded parser phases and five duplicate upsert methods from the indexer.

---

## Problem

Every new manifest parser today requires edits in four or more locations:

1. `scanner/scanner.go` — add filename to `DefaultTargetFiles`, add case to `classifyFile()`
2. `indexer/indexer.go` — add a new phase loop in `Run()` (~19 lines) and a new `upsertX()` method (~50 lines)

No extension point exists. Every parser is hardcoded into the indexer core. Users who need to index a proprietary format must fork the repository. The five existing parsers also duplicate the same upsert logic five times.

---

## Solution: Approach C — Global Registry Injected into Scanner and Indexer

A `FileParser` interface and a `ParserRegistry` are introduced in `scanner/parser/`. A global `Default` registry is the target for `init()`-based auto-registration. The registry instance is injected into both `Scanner` and `AutoIndexer` constructors, so tests can supply a fresh `NewRegistry()` without touching global state.

The five built-in parsers are fully migrated to implement `FileParser`. The five hardcoded phase loops and five duplicate upsert methods in `indexer/indexer.go` are replaced by a single generic loop and one `upsertParsedFile()` method.

---

## Interface Contract

**File:** `scanner/parser/extension.go`

```go
// ParsedRelation is a directed edge produced by a parser.
type ParsedRelation struct {
    From         string
    To           string
    RelationType string // e.g. "depends_on", "owns", "provides_api"
}

// MergeMode controls how upsertParsedFile() handles existing graph entities.
type MergeMode int

const (
    // MergeModeUpsert (default) — create entity if absent, update observations if present.
    MergeModeUpsert MergeMode = iota
    // MergeModeCatalog — three-tier strategy: create-new, update-auto (_source:catalog-info),
    // or add-missing-only (manually created entities). Used exclusively by catalog.Parser.
    MergeModeCatalog
)

// ParsedFile is the normalized, graph-ready output every parser must produce.
// EntityName must be non-empty. EntityType defaults to "service" if blank.
// Observations and Relations may be nil or empty.
// MergeMode defaults to MergeModeUpsert if zero.
type ParsedFile struct {
    EntityName   string
    EntityType   string
    Observations []string
    Relations    []ParsedRelation
    MergeMode    MergeMode
}

// FileParser is the extension point for manifest parsers.
// All methods must be safe for concurrent use (implementations are typically stateless).
type FileParser interface {
    // FileType returns the classifier key used by classifyFile() and the indexer.
    // Must be unique across the registry. Examples: "pipfile", "toolversions".
    FileType() string

    // Filenames returns the exact root-level filenames this parser handles.
    // Examples: ["Pipfile"], ["go.mod"], [".tool-versions"].
    Filenames() []string

    // Parse converts raw file bytes into a normalized graph-ready result.
    // An error fails the entire scan (Run() returns immediately).
    Parse(data []byte) (ParsedFile, error)
}
```

### Design Decisions

- **`EntityType` defaults to `"service"`** if blank — keeps custom parsers minimal.
- **`Parse` error fails the scan** — prevents partial graph population from a buggy parser. This matches the existing behaviour of phase errors in `Run()`.
- **`Filenames()` returns multiple names** — supports both `CODEOWNERS` and `.github/CODEOWNERS` from a single parser.
- **Backward-compatible helpers** — `ParseGoMod([]byte) (ParsedGoMod, error)` etc. remain as package-level functions delegating to the new implementations, so any external callers are unaffected.

---

## Registry

**File:** `scanner/parser/registry.go`

```go
// Default is the global registry. Built-in parsers register here from main.go;
// custom parsers register here from their own init().
var Default = NewRegistry()

// Register adds p to the global Default registry.
// Panics on duplicate FileType — fail-fast, not silent override.
func Register(p FileParser) { Default.Register(p) }

type ParserRegistry struct {
    mu      sync.RWMutex
    parsers map[string]FileParser // keyed by FileType()
}

func NewRegistry() *ParserRegistry
// Register panics on duplicate FileType() OR if any filename in Filenames()
// is already claimed by another registered parser.
func (r *ParserRegistry) Register(p FileParser)
func (r *ParserRegistry) Get(fileType string) (FileParser, bool)
func (r *ParserRegistry) All() []FileParser
func (r *ParserRegistry) TargetFilenames() []string      // union of all Filenames()
```

**Duplicate detection panics** at process startup (called from `init()` or `main.go`), making conflicts impossible to miss.

### Custom Parser — User-Side Pattern

```go
// mypkg/pipfile/parser.go
package pipfile

import "github.com/doc-scout/mcp-server/scanner/parser"

func init() { parser.Register(&Parser{}) }

type Parser struct{}

func (p *Parser) FileType()  string   { return "pipfile" }
func (p *Parser) Filenames() []string { return []string{"Pipfile"} }
func (p *Parser) Parse(data []byte) (parser.ParsedFile, error) { ... }
```

**User's `main.go` addition:**

```go
import _ "mypkg/pipfile" // triggers init(), zero other changes required
```

### Built-in Registration in `main.go`

```go
parser.Register(&gomod.Parser{})
parser.Register(&packagejson.Parser{})
parser.Register(&pom.Parser{})
parser.Register(&codeowners.Parser{})
parser.Register(&catalog.Parser{})

reg := parser.Default // or parser.NewRegistry() for test injection
sc  := scanner.New(cfg, reg)
idx := indexer.New(store, getter, cache, reg)
```

---

## Scanner Changes

**File:** `scanner/scanner.go`

`Scanner` constructor gains a `*parser.ParserRegistry` parameter.

**`DefaultTargetFiles`** is no longer a hardcoded `var`. At construction time, it is computed as:

```
registry.TargetFilenames()
∪ {"README.md", "mkdocs.yml", "openapi.yaml", "swagger.json",
   "Dockerfile", "docker-compose.yml", "docker-compose.yaml",
   "Makefile", ".mise.toml", "mise.toml", "SKILLS.md", "AGENTS.md"}
```

The second set contains file types that are indexed as documentation or infra assets but have no parser producing graph entities.

**`classifyFile()`** checks the registry first:

```go
func classifyFile(name string, reg *ParserRegistry) string {
    normalized := strings.ToLower(filepath.Base(name))
    for _, p := range reg.All() {
        for _, fn := range p.Filenames() {
            if normalized == strings.ToLower(fn) {
                return p.FileType()
            }
        }
    }
    // existing hardcoded switch for infra/docs types
    switch normalized { ... }
}
```

This is backward-compatible: adding a parser to the registry never breaks existing classifications because hardcoded types (`helm`, `k8s`, `workflow`, `terraform`, `docs`) are not covered by any built-in `FileParser`.

---

## Indexer Changes

**File:** `indexer/indexer.go`

`AutoIndexer` gains a `registry *parser.ParserRegistry` field.

### Single Generic Loop (replaces phases 2a–2e)

```go
func (ai *AutoIndexer) runParsers(ctx context.Context, repos []scanner.Repository) error {
    for _, repo := range repos {
        for _, file := range repo.Files {
            p, ok := ai.registry.Get(file.Type)
            if !ok {
                continue // infra/docs file — no graph entity
            }
            data, err := ai.getter.GetFileContent(ctx, repo.Owner, repo.Name, file.Path)
            if err != nil {
                return fmt.Errorf("fetch %s/%s: %w", repo.FullName, file.Path, err)
            }
            parsed, err := p.Parse(data)
            if err != nil {
                return fmt.Errorf("parser %q failed on %s/%s: %w",
                    file.Type, repo.FullName, file.Path, err)
            }
            if err := ai.upsertParsedFile(ctx, repo, parsed); err != nil {
                return err
            }
        }
    }
    return nil
}
```

### Single `upsertParsedFile()` (replaces upsertGoMod, upsertPackageJSON, upsertPom, upsertCodeowners, upsertCatalog)

```go
func (ai *AutoIndexer) upsertParsedFile(ctx context.Context, repo scanner.Repository, f parser.ParsedFile) error {
    entityType := f.EntityType
    if entityType == "" {
        entityType = "service"
    }
    // 1. Create or update entity
    // 2. Add auto-observations: _source:<FileType>, _scan_repo:<repo.FullName>
    // 3. Add parser-provided observations (via sanitizeObservations)
    // 4. Create relations
}
```

**Catalog merge strategy** (create-new / update-auto / add-missing-only) moves into `catalog.Parser`, which returns a `ParsedFile` with `MergeMode: MergeModeCatalog`. `upsertParsedFile()` switches on `MergeMode` — keeping the three-tier logic out of the generic path while preserving catalog semantics via a declared contract, not an observation string check.

### `Run()` structure after refactor

```
Phase 1:  refreshContent   (unchanged)
Phase 2:  runParsers       (new — replaces phases 2a–2e)
Phase 3:  archiveStale     (unchanged)
```

---

## Error Handling

| Scenario                                  | Behaviour                                                                                                                                                            |
| ----------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `FileParser.Parse()` returns error        | `Run()` returns immediately with wrapped error; scan is aborted                                                                                                      |
| `FileParser.Parse()` panics               | Recovered by `withRecovery` in the MCP tool layer (does not apply to indexer); indexer does **not** recover panics from parsers — let them propagate to surface bugs |
| Duplicate `FileType()` in `Register()`    | Panic at startup — caught immediately during development                                                                                                             |
| `Filenames()` overlap between two parsers | Panics at registry construction (checked in `Register`)                                                                                                              |

---

## Files Changed

| File                              | Change                                                                                                         |
| --------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `scanner/parser/extension.go`     | **New** — `FileParser` interface, `ParsedFile`, `ParsedRelation`                                               |
| `scanner/parser/registry.go`      | **New** — `ParserRegistry`, global `Default`, `Register()` func                                                |
| `scanner/parser/gomod.go`         | Add `FileType()` + `Filenames()` methods; keep `ParseGoMod()` as helper                                        |
| `scanner/parser/packagejson.go`   | Same pattern                                                                                                   |
| `scanner/parser/pom.go`           | Same pattern                                                                                                   |
| `scanner/parser/codeowners.go`    | Same pattern                                                                                                   |
| `scanner/parser/catalog.go`       | Same pattern; merge strategy moves here                                                                        |
| `scanner/scanner.go`              | Constructor takes `*ParserRegistry`; `DefaultTargetFiles` + `classifyFile` read from registry                  |
| `indexer/indexer.go`              | Constructor takes `*ParserRegistry`; `runParsers()` + `upsertParsedFile()` replace 5 phases + 5 upsert methods |
| `main.go`                         | Register built-ins; pass `parser.Default` to scanner + indexer constructors                                    |
| `scanner/parser/registry_test.go` | **New** — unit tests for registry                                                                              |
| `indexer/indexer_test.go`         | Add mock-parser integration test                                                                               |
| `AGENTS.md`                       | Update §7 with `FileParser` contract and registration guide                                                    |

---

## Testing Strategy

**Unit — `scanner/parser/registry_test.go`:**

- `Register` panics on duplicate `FileType()`
- `Register` panics on `Filenames()` overlap between two parsers
- `TargetFilenames()` returns the union of all registered parsers
- `Get` returns correct parser; returns `false` for unknown types
- Concurrent `Register` + `Get` passes `-race`

**Unit — each built-in parser:**

- Existing `TestParse*` tests are unchanged
- New: `FileType()` and `Filenames()` return expected values

**Integration — `indexer/indexer_test.go`:**

- Register a `mockParser` via `parser.NewRegistry()` (isolated from `Default`)
- Run `AutoIndexer` with a fake repo containing a file of that type
- Assert: entity created, observations present, relations created
- Assert: `Run()` returns an error if `mockParser.Parse` returns one

---

## Non-Goals

- Dynamic plugin loading (`.so` files, Go plugins) — out of scope
- Parser versioning or priority ordering — all parsers are peers
- Per-parser error recovery — a failing parser always aborts the scan
