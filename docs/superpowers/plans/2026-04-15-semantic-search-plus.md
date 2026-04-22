# Semantic Search Plus Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add opt-in vector-based semantic search to DocScout-MCP via a new `semantic_search` MCP tool backed by OpenAI or Ollama embeddings stored in the existing SQLite DB.

**Architecture:** A pluggable `EmbeddingProvider` interface (OpenAI / Ollama) drives a `VectorStore` (two new GORM tables with content-hash staleness tracking) and a `SemanticSearcher`. The feature is disabled when neither `DOCSCOUT_EMBED_OPENAI_KEY` nor `DOCSCOUT_EMBED_OLLAMA_URL` is set — the server starts normally with no penalty.

**Tech Stack:** Go 1.26, `gorm.io/gorm`, `crypto/sha256`, `encoding/binary`, `math`, `net/http`, `github.com/modelcontextprotocol/go-sdk/mcp`

---

## File Map

| Action | Path                                            | Responsibility                                                                       |
| ------ | ----------------------------------------------- | ------------------------------------------------------------------------------------ |
| Create | `embeddings/provider.go`                        | `EmbeddingProvider` interface + `Config` + `NewProvider` factory                     |
| Create | `embeddings/similarity.go`                      | `CosineSimilarity`, `EntityText`, `sha256hex`                                        |
| Create | `embeddings/store.go`                           | `VectorStore`: DB models, migrate, encode/decode, upsert, load, delete               |
| Create | `embeddings/openai.go`                          | OpenAI REST client                                                                   |
| Create | `embeddings/ollama.go`                          | Ollama REST client                                                                   |
| Create | `embeddings/indexer.go`                         | `Indexer`: IndexDocs, IndexEntities, ScheduleEntities (debounce)                     |
| Create | `embeddings/searcher.go`                        | `SemanticSearcher`: SearchDocs, SearchEntities, facade methods                       |
| Modify | `memory/content.go`                             | Add `DocRecord` + `ListDocs(repo string)` to `ContentCache`                          |
| Create | `tools/semantic_search.go`                      | MCP tool definition + handler                                                        |
| Modify | `tools/ports.go`                                | Add `SemanticSearch` interface                                                       |
| Modify | `tools/tools.go`                                | Add `semantic SemanticSearch` param to `Register`; wire tool                         |
| Modify | `main.go`                                       | Init provider, VectorStore, Indexer, Searcher; hook post-scan; update Register calls |
| Create | `tests/semantic_search/semantic_search_test.go` | End-to-end MCP integration test                                                      |

---

### Task 1: `EmbeddingProvider` interface + cosine similarity utilities

**Files:**

- Create: `embeddings/provider.go`
- Create: `embeddings/similarity.go`
- Create: `embeddings/similarity_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// embeddings/similarity_test.go
package embeddings_test

import (
	"math"
	"testing"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	v := []float32{1.0, 0.0, 0.0}
	got := embeddings.CosineSimilarity(v, v)
	if math.Abs(got-1.0) > 1e-6 {
		t.Errorf("identical vectors: want 1.0, got %f", got)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	got := embeddings.CosineSimilarity(a, b)
	if math.Abs(got) > 1e-6 {
		t.Errorf("orthogonal vectors: want 0.0, got %f", got)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	z := []float32{0.0, 0.0}
	a := []float32{1.0, 0.0}
	got := embeddings.CosineSimilarity(z, a)
	if got != 0 {
		t.Errorf("zero vector: want 0, got %f", got)
	}
}

func TestEntityText_SortedObservations(t *testing.T) {
	e1 := memory.Entity{Name: "svc", EntityType: "service", Observations: []string{"b", "a"}}
	e2 := memory.Entity{Name: "svc", EntityType: "service", Observations: []string{"a", "b"}}
	if embeddings.EntityText(e1) != embeddings.EntityText(e2) {
		t.Error("EntityText must sort observations for deterministic hashing")
	}
}

func TestEntityText_Format(t *testing.T) {
	e := memory.Entity{Name: "payment-svc", EntityType: "service", Observations: []string{"lang:go", "owner:platform"}}
	got := embeddings.EntityText(e)
	want := "payment-svc [service]: lang:go, owner:platform"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestEntityText_NoObservations(t *testing.T) {
	e := memory.Entity{Name: "foo", EntityType: "team"}
	got := embeddings.EntityText(e)
	want := "foo [team]"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
cd /mnt/e/DEV/mcpdocs/.worktrees/feat-semantic-search
go test ./embeddings/...
```

Expected: `cannot find package "github.com/doc-scout/mcp-server/embeddings"`

- [ ] **Step 3: Create `embeddings/provider.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"context"
	"log/slog"
	"os"
)

// EmbeddingProvider generates vector embeddings for a batch of texts.
// Implementations must be safe for concurrent use.
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// ModelKey returns "<provider>:<model>" (e.g. "openai:text-embedding-3-small").
	// A change in ModelKey triggers re-indexing of all stored vectors.
	ModelKey() string
}

// Config holds embedding provider configuration.
type Config struct {
	OpenAIKey   string
	OpenAIModel string
	OllamaURL   string
	OllamaModel string
}

// ConfigFromEnv reads provider configuration from environment variables.
func ConfigFromEnv() Config {
	c := Config{
		OpenAIKey:   os.Getenv("DOCSCOUT_EMBED_OPENAI_KEY"),
		OpenAIModel: os.Getenv("DOCSCOUT_EMBED_OPENAI_MODEL"),
		OllamaURL:   os.Getenv("DOCSCOUT_EMBED_OLLAMA_URL"),
		OllamaModel: os.Getenv("DOCSCOUT_EMBED_OLLAMA_MODEL"),
	}
	if c.OpenAIModel == "" {
		c.OpenAIModel = "text-embedding-3-small"
	}
	if c.OllamaModel == "" {
		c.OllamaModel = "nomic-embed-text"
	}
	return c
}

// NewProvider returns the appropriate EmbeddingProvider based on cfg.
// Returns nil when no provider is configured (Plus feature disabled).
// When both OpenAI key and Ollama URL are set, OpenAI takes precedence.
func NewProvider(cfg Config) EmbeddingProvider {
	if cfg.OpenAIKey != "" && cfg.OllamaURL != "" {
		slog.Warn("[embeddings] Both DOCSCOUT_EMBED_OPENAI_KEY and DOCSCOUT_EMBED_OLLAMA_URL are set; using OpenAI")
	}
	if cfg.OpenAIKey != "" {
		return NewOpenAIProvider(cfg.OpenAIKey, cfg.OpenAIModel)
	}
	if cfg.OllamaURL != "" {
		return NewOllamaProvider(cfg.OllamaURL, cfg.OllamaModel)
	}
	return nil
}
```

- [ ] **Step 4: Create `embeddings/similarity.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/doc-scout/mcp-server/memory"
)

// CosineSimilarity returns the cosine similarity in [-1, 1] between two equal-length
// float32 vectors. Returns 0 when either vector has zero magnitude.
func CosineSimilarity(a, b []float32) float64 {
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
```

- [ ] **Step 5: Run tests to confirm they pass**

```bash
go test ./embeddings/... -run TestCosine -v
go test ./embeddings/... -run TestEntityText -v
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add embeddings/provider.go embeddings/similarity.go embeddings/similarity_test.go
git commit -m "feat(embeddings): add EmbeddingProvider interface and cosine similarity utils"
```

---

### Task 2: VectorStore — DB schema, encode/decode, upsert, load

**Files:**

- Create: `embeddings/store.go`
- Create: `embeddings/store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// embeddings/store_test.go
package embeddings_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
)

var storeCounter atomic.Int64

func newTestStore(t *testing.T) *embeddings.VectorStore {
	t.Helper()
	dsn := fmt.Sprintf("file:store_test_%d?mode=memory&cache=shared", storeCounter.Add(1))
	db, err := memory.OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	vs, err := embeddings.NewVectorStore(db)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	return vs
}

func TestVectorStore_UpsertAndLoadDoc(t *testing.T) {
	vs := newTestStore(t)
	vec := []float32{0.1, 0.2, 0.3, 0.4}
	if err := vs.UpsertDoc("org/svc#README.md", "hash1", "openai:text-embedding-3-small", vec); err != nil {
		t.Fatalf("UpsertDoc: %v", err)
	}
	rows, err := vs.LoadDocEmbeddings("openai:text-embedding-3-small")
	if err != nil {
		t.Fatalf("LoadDocEmbeddings: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].DocID != "org/svc#README.md" {
		t.Errorf("wrong docID: %s", rows[0].DocID)
	}
	if rows[0].ContentHash != "hash1" {
		t.Errorf("wrong hash: %s", rows[0].ContentHash)
	}
	for i, v := range rows[0].Vector {
		if abs32(v-vec[i]) > 1e-6 {
			t.Errorf("vector[%d]: want %f, got %f", i, vec[i], v)
		}
	}
}

func TestVectorStore_UpsertDocUpdates(t *testing.T) {
	vs := newTestStore(t)
	vs.UpsertDoc("org/svc#README.md", "hash1", "openai:text-embedding-3-small", []float32{0.1, 0.2})
	vs.UpsertDoc("org/svc#README.md", "hash2", "openai:text-embedding-3-small", []float32{0.9, 0.8})
	rows, _ := vs.LoadDocEmbeddings("openai:text-embedding-3-small")
	if len(rows) != 1 {
		t.Fatalf("upsert should not duplicate; got %d rows", len(rows))
	}
	if rows[0].ContentHash != "hash2" {
		t.Errorf("expected updated hash2, got %s", rows[0].ContentHash)
	}
}

func TestVectorStore_UpsertAndLoadEntity(t *testing.T) {
	vs := newTestStore(t)
	vec := []float32{0.5, 0.5}
	if err := vs.UpsertEntity("payment-svc", "obshash1", "ollama:nomic-embed-text", vec); err != nil {
		t.Fatalf("UpsertEntity: %v", err)
	}
	rows, err := vs.LoadEntityEmbeddings("ollama:nomic-embed-text")
	if err != nil {
		t.Fatalf("LoadEntityEmbeddings: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].EntityName != "payment-svc" {
		t.Errorf("wrong name: %s", rows[0].EntityName)
	}
}

func TestVectorStore_DeleteDocByID(t *testing.T) {
	vs := newTestStore(t)
	vs.UpsertDoc("org/svc#a.md", "h1", "openai:text-embedding-3-small", []float32{0.1})
	vs.UpsertDoc("org/svc#b.md", "h2", "openai:text-embedding-3-small", []float32{0.2})
	if err := vs.DeleteDocByID("org/svc#a.md"); err != nil {
		t.Fatalf("DeleteDocByID: %v", err)
	}
	rows, _ := vs.LoadDocEmbeddings("openai:text-embedding-3-small")
	if len(rows) != 1 || rows[0].DocID != "org/svc#b.md" {
		t.Errorf("expected only b.md to remain, got %v", rows)
	}
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./embeddings/... -run TestVectorStore
```

Expected: FAIL — `NewVectorStore` undefined

- [ ] **Step 3: Create `embeddings/store.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"encoding/binary"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type dbDocEmbedding struct {
	DocID       string    `gorm:"primaryKey"`
	ContentHash string    `gorm:"not null"`
	Vector      []byte    `gorm:"not null"` // []float32 little-endian IEEE 754 — stale when ContentHash != sha256hex(current content)
	ModelKey    string    `gorm:"not null;index"`
	UpdatedAt   time.Time `gorm:"not null"`
}

type dbEntityEmbedding struct {
	EntityName string    `gorm:"primaryKey"`
	ObsHash    string    `gorm:"not null"`
	Vector     []byte    `gorm:"not null"` // []float32 little-endian IEEE 754 — stale when ObsHash != sha256hex(EntityText(current entity))
	ModelKey   string    `gorm:"not null;index"`
	UpdatedAt  time.Time `gorm:"not null"`
}

// VectorStore persists and retrieves float32 embedding vectors in the existing DB.
type VectorStore struct {
	db *gorm.DB
}

// NewVectorStore creates the two embedding tables (auto-migrate) and returns a VectorStore.
func NewVectorStore(db *gorm.DB) (*VectorStore, error) {
	if err := db.AutoMigrate(&dbDocEmbedding{}, &dbEntityEmbedding{}); err != nil {
		return nil, err
	}
	return &VectorStore{db: db}, nil
}

// encodeVector serialises []float32 as little-endian IEEE 754 bytes (4 bytes per element).
func encodeVector(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// decodeVector deserialises little-endian IEEE 754 bytes back to []float32.
func decodeVector(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// DocEmbedding is a decoded document embedding row.
type DocEmbedding struct {
	DocID       string
	ContentHash string
	Vector      []float32
}

// EntityEmbedding is a decoded entity embedding row.
type EntityEmbedding struct {
	EntityName string
	ObsHash    string
	Vector     []float32
}

// UpsertDoc stores or replaces a document embedding.
func (vs *VectorStore) UpsertDoc(docID, contentHash, modelKey string, vector []float32) error {
	row := dbDocEmbedding{
		DocID:       docID,
		ContentHash: contentHash,
		Vector:      encodeVector(vector),
		ModelKey:    modelKey,
		UpdatedAt:   time.Now().UTC(),
	}
	return vs.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

// UpsertEntity stores or replaces an entity embedding.
func (vs *VectorStore) UpsertEntity(entityName, obsHash, modelKey string, vector []float32) error {
	row := dbEntityEmbedding{
		EntityName: entityName,
		ObsHash:    obsHash,
		Vector:     encodeVector(vector),
		ModelKey:   modelKey,
		UpdatedAt:  time.Now().UTC(),
	}
	return vs.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

// LoadDocEmbeddings returns all doc embeddings for the given modelKey.
func (vs *VectorStore) LoadDocEmbeddings(modelKey string) ([]DocEmbedding, error) {
	var rows []dbDocEmbedding
	if err := vs.db.Where("model_key = ?", modelKey).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DocEmbedding, len(rows))
	for i, r := range rows {
		out[i] = DocEmbedding{DocID: r.DocID, ContentHash: r.ContentHash, Vector: decodeVector(r.Vector)}
	}
	return out, nil
}

// LoadEntityEmbeddings returns all entity embeddings for the given modelKey.
func (vs *VectorStore) LoadEntityEmbeddings(modelKey string) ([]EntityEmbedding, error) {
	var rows []dbEntityEmbedding
	if err := vs.db.Where("model_key = ?", modelKey).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]EntityEmbedding, len(rows))
	for i, r := range rows {
		out[i] = EntityEmbedding{EntityName: r.EntityName, ObsHash: r.ObsHash, Vector: decodeVector(r.Vector)}
	}
	return out, nil
}

// DeleteDocByID removes a single doc embedding row.
func (vs *VectorStore) DeleteDocByID(docID string) error {
	return vs.db.Where("doc_id = ?", docID).Delete(&dbDocEmbedding{}).Error
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./embeddings/... -run TestVectorStore -v
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add embeddings/store.go embeddings/store_test.go
git commit -m "feat(embeddings): add VectorStore with float32 BLOB encoding and content-hash tracking"
```

---

### Task 3: OpenAI embedding provider

**Files:**

- Create: `embeddings/openai.go`
- Create: `embeddings/openai_test.go`

- [ ] **Step 1: Write the failing test**

```go
// embeddings/openai_test.go
package embeddings_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/doc-scout/mcp-server/embeddings"
)

func TestOpenAIProvider_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{0.1, 0.2, 0.3}},
				{"embedding": []float32{0.4, 0.5, 0.6}},
			},
		})
	}))
	defer srv.Close()

	p := embeddings.NewOpenAIProviderWithURL("test-key", "text-embedding-3-small", srv.URL)
	vecs, err := p.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("want 2 vectors, got %d", len(vecs))
	}
	if abs32(vecs[0][0]-0.1) > 1e-6 {
		t.Errorf("vecs[0][0]: want 0.1, got %f", vecs[0][0])
	}
}

func TestOpenAIProvider_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := embeddings.NewOpenAIProviderWithURL("key", "text-embedding-3-small", srv.URL)
	_, err := p.Embed(context.Background(), []string{"text"})
	if err == nil {
		t.Fatal("expected ErrRateLimit, got nil")
	}
}

func TestOpenAIProvider_ModelKey(t *testing.T) {
	p := embeddings.NewOpenAIProvider("key", "text-embedding-3-small")
	if p.ModelKey() != "openai:text-embedding-3-small" {
		t.Errorf("wrong model key: %s", p.ModelKey())
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./embeddings/... -run TestOpenAI
```

Expected: FAIL — `NewOpenAIProviderWithURL` undefined

- [ ] **Step 3: Create `embeddings/openai.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrRateLimit is returned when the OpenAI API responds with HTTP 429.
var ErrRateLimit = errors.New("openai: rate limit exceeded (HTTP 429)")

type openAIProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewOpenAIProvider creates a provider that calls the production OpenAI embeddings API.
func NewOpenAIProvider(apiKey, model string) EmbeddingProvider {
	return NewOpenAIProviderWithURL(apiKey, model, "https://api.openai.com")
}

// NewOpenAIProviderWithURL creates a provider with a configurable base URL (used in tests).
func NewOpenAIProviderWithURL(apiKey, model, baseURL string) EmbeddingProvider {
	return &openAIProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *openAIProvider) ModelKey() string { return "openai:" + p.model }

type openAIEmbedRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (p *openAIProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	var all [][]float32
	for i := 0; i < len(texts); i += 2048 {
		end := i + 2048
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := p.embedBatch(ctx, texts[i:end])
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
	}
	return all, nil
}

func (p *openAIProvider) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(openAIEmbedRequest{Input: texts, Model: p.model})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimit
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embed: HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result openAIEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai embed: decode: %w", err)
	}
	out := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		out[i] = d.Embedding
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./embeddings/... -run TestOpenAI -v
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add embeddings/openai.go embeddings/openai_test.go
git commit -m "feat(embeddings): add OpenAI embedding provider with httptest-verified HTTP client"
```

---

### Task 4: Ollama embedding provider

**Files:**

- Create: `embeddings/ollama.go`
- Create: `embeddings/ollama_test.go`

- [ ] **Step 1: Write the failing test**

```go
// embeddings/ollama_test.go
package embeddings_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/doc-scout/mcp-server/embeddings"
)

func TestOllamaProvider_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{{0.7, 0.8, 0.9}},
		})
	}))
	defer srv.Close()

	p := embeddings.NewOllamaProvider(srv.URL, "nomic-embed-text")
	vecs, err := p.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 1 {
		t.Fatalf("want 1 vector, got %d", len(vecs))
	}
	if abs32(vecs[0][0]-0.7) > 1e-6 {
		t.Errorf("vecs[0][0]: want 0.7, got %f", vecs[0][0])
	}
}

func TestOllamaProvider_Unreachable(t *testing.T) {
	p := embeddings.NewOllamaProvider("http://127.0.0.1:19999", "nomic-embed-text")
	_, err := p.Embed(context.Background(), []string{"text"})
	if err == nil {
		t.Fatal("expected error for unreachable Ollama, got nil")
	}
}

func TestOllamaProvider_ModelKey(t *testing.T) {
	p := embeddings.NewOllamaProvider("http://localhost:11434", "nomic-embed-text")
	if p.ModelKey() != "ollama:nomic-embed-text" {
		t.Errorf("wrong model key: %s", p.ModelKey())
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./embeddings/... -run TestOllama
```

Expected: FAIL — `NewOllamaProvider` undefined

- [ ] **Step 3: Create `embeddings/ollama.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ollamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaProvider creates a provider that calls a local Ollama server.
func NewOllamaProvider(baseURL, model string) EmbeddingProvider {
	return &ollamaProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *ollamaProvider) ModelKey() string { return "ollama:" + p.model }

type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed embeds each text sequentially — Ollama does not support batching.
func (p *ollamaProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := p.embedOne(ctx, text)
		if err != nil {
			return nil, err
		}
		out[i] = vec
	}
	return out, nil
}

func (p *ollamaProvider) embedOne(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(ollamaEmbedRequest{Model: p.model, Input: text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed: HTTP %d: %s", resp.StatusCode, string(b))
	}
	var result ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embed: decode: %w", err)
	}
	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama embed: empty response for model %s", p.model)
	}
	return result.Embeddings[0], nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./embeddings/... -run TestOllama -v
```

Expected: all PASS

- [ ] **Step 5: Confirm full embeddings package still builds**

```bash
go build ./embeddings/...
```

Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add embeddings/ollama.go embeddings/ollama_test.go
git commit -m "feat(embeddings): add Ollama embedding provider"
```

---

### Task 5: `memory.DocRecord` + `ContentCache.ListDocs`

**Files:**

- Modify: `memory/content.go`
- Modify: `memory/content_test.go`

- [ ] **Step 1: Write the failing test**

Add this test to `memory/content_test.go` (append below the last existing test):

```go
func TestContentCache_ListDocs(t *testing.T) {
	db := newTestDB(t)
	cc := memory.NewContentCache(db, true, 1024*1024)

	if err := cc.Upsert("org/svc-a", "README.md", "sha1", "content a", "readme"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := cc.Upsert("org/svc-b", "README.md", "sha2", "content b", "readme"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Filter by repo
	docs, err := cc.ListDocs("org/svc-a")
	if err != nil {
		t.Fatalf("ListDocs: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("want 1 doc, got %d", len(docs))
	}
	if docs[0].DocID != "org/svc-a#README.md" {
		t.Errorf("wrong DocID: %s", docs[0].DocID)
	}
	if docs[0].Content != "content a" {
		t.Errorf("wrong Content: %s", docs[0].Content)
	}

	// All repos
	all, err := cc.ListDocs("")
	if err != nil {
		t.Fatalf("ListDocs all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 docs, got %d", len(all))
	}
}
```

Check how `newTestDB` is defined in content_test.go first:

```bash
head -40 memory/content_test.go
```

Use the same helper pattern. If it's named differently, match it.

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./memory/... -run TestContentCache_ListDocs
```

Expected: FAIL — `cc.ListDocs` undefined

- [ ] **Step 3: Add `DocRecord` and `ListDocs` to `memory/content.go`**

Insert after the `ContentMatch` struct definition (after line `type ContentMatch struct { ... }`):

```go
// DocRecord is a lightweight document record for semantic indexing.
type DocRecord struct {
	RepoName string
	Path     string
	DocID    string // "<RepoName>#<Path>"
	Content  string
}
```

Append at the end of `memory/content.go`, before the closing brace of the file:

```go
// ListDocs returns all document records.
// If repoName is non-empty, only documents from that repository are returned.
// Passing "" returns all documents across all repos.
func (cc *ContentCache) ListDocs(repoName string) ([]DocRecord, error) {
	db := cc.db
	if repoName != "" {
		db = db.Where("repo_name = ?", repoName)
	}
	var rows []dbDocContent
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DocRecord, len(rows))
	for i, r := range rows {
		out[i] = DocRecord{
			RepoName: r.RepoName,
			Path:     r.Path,
			DocID:    r.RepoName + "#" + r.Path,
			Content:  r.Content,
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run test**

```bash
go test ./memory/... -run TestContentCache_ListDocs -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add memory/content.go memory/content_test.go
git commit -m "feat(memory): add DocRecord and ContentCache.ListDocs for semantic indexer"
```

---

### Task 6: Indexer — IndexDocs, IndexEntities, ScheduleEntities

**Files:**

- Create: `embeddings/indexer.go`
- Create: `embeddings/indexer_test.go`

- [ ] **Step 1: Write the failing test**

```go
// embeddings/indexer_test.go
package embeddings_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
)

var idxCounter atomic.Int64

type fixedProvider struct {
	dim int
}

func (f *fixedProvider) ModelKey() string { return "mock:v1" }
func (f *fixedProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, f.dim)
		out[i][0] = 0.9
	}
	return out, nil
}

func newIdxEnv(t *testing.T) (*embeddings.VectorStore, *memory.ContentCache, *memory.MemoryService) {
	t.Helper()
	dsn := fmt.Sprintf("file:idx_test_%d?mode=memory&cache=shared", idxCounter.Add(1))
	db, err := memory.OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	vs, err := embeddings.NewVectorStore(db)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	cc := memory.NewContentCache(db, true, 1024*1024)
	ms := memory.NewMemoryService(db)
	return vs, cc, ms
}

func TestIndexer_IndexDocs_Upserts(t *testing.T) {
	vs, cc, ms := newIdxEnv(t)
	provider := &fixedProvider{dim: 4}

	cc.Upsert("org/svc", "README.md", "sha1", "hello world payment", "readme")

	idx := embeddings.NewIndexer(provider, vs, cc, ms)
	idx.IndexDocs(context.Background(), "org/svc")

	rows, err := vs.LoadDocEmbeddings("mock:v1")
	if err != nil {
		t.Fatalf("LoadDocEmbeddings: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(rows))
	}
	if rows[0].DocID != "org/svc#README.md" {
		t.Errorf("wrong docID: %s", rows[0].DocID)
	}
}

func TestIndexer_IndexDocs_SkipsUpToDate(t *testing.T) {
	vs, cc, ms := newIdxEnv(t)
	provider := &fixedProvider{dim: 4}

	cc.Upsert("org/svc", "README.md", "sha1", "content", "readme")
	idx := embeddings.NewIndexer(provider, vs, cc, ms)
	idx.IndexDocs(context.Background(), "org/svc")

	// Second run — content unchanged — should not re-embed (embed count stays 1)
	idx.IndexDocs(context.Background(), "org/svc")

	rows, _ := vs.LoadDocEmbeddings("mock:v1")
	if len(rows) != 1 {
		t.Fatalf("expected still 1 embedding after second run, got %d", len(rows))
	}
}

func TestIndexer_IndexEntities_Upserts(t *testing.T) {
	vs, cc, ms := newIdxEnv(t)
	provider := &fixedProvider{dim: 4}

	ms.CreateEntities([]memory.Entity{{Name: "payment-svc", EntityType: "service", Observations: []string{"lang:go"}}})

	idx := embeddings.NewIndexer(provider, vs, cc, ms)
	idx.IndexEntities(context.Background(), []string{"payment-svc"})

	rows, err := vs.LoadEntityEmbeddings("mock:v1")
	if err != nil {
		t.Fatalf("LoadEntityEmbeddings: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 entity embedding, got %d", len(rows))
	}
	if rows[0].EntityName != "payment-svc" {
		t.Errorf("wrong entity: %s", rows[0].EntityName)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./embeddings/... -run TestIndexer
```

Expected: FAIL — `NewIndexer` undefined

- [ ] **Step 3: Create `embeddings/indexer.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/doc-scout/mcp-server/memory"
)

const indexBatchSize = 50

// DocStore lists cached documents for a repo.
type DocStore interface {
	ListDocs(repoName string) ([]memory.DocRecord, error)
}

// EntityOpener fetches entities with their observations by name.
type EntityOpener interface {
	OpenNodes(names []string) (memory.KnowledgeGraph, error)
}

// Indexer embeds and stores document and entity vectors after scans/mutations.
type Indexer struct {
	provider EmbeddingProvider
	store    *VectorStore
	docs     DocStore
	entities EntityOpener

	mu sync.Mutex // serialises concurrent IndexDocs / IndexEntities calls

	debounce struct {
		mu    sync.Mutex
		timer *time.Timer
		names map[string]bool
	}
}

// NewIndexer creates an Indexer. docs and entities may be nil (indexing of that type is skipped).
func NewIndexer(provider EmbeddingProvider, store *VectorStore, docs DocStore, entities EntityOpener) *Indexer {
	return &Indexer{provider: provider, store: store, docs: docs, entities: entities}
}

// IndexDocs re-indexes all documents for repoFullName. Skips docs whose stored
// content_hash already matches. Safe to call from a background goroutine.
func (idx *Indexer) IndexDocs(ctx context.Context, repoFullName string) {
	if idx.docs == nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()

	docs, err := idx.docs.ListDocs(repoFullName)
	if err != nil {
		slog.Error("[embeddings] IndexDocs: list docs", "repo", repoFullName, "error", err)
		return
	}

	existing, err := idx.store.LoadDocEmbeddings(idx.provider.ModelKey())
	if err != nil {
		slog.Error("[embeddings] IndexDocs: load existing", "error", err)
		return
	}
	storedHash := make(map[string]string, len(existing))
	for _, e := range existing {
		storedHash[e.DocID] = e.ContentHash
	}

	var toEmbed []memory.DocRecord
	for _, d := range docs {
		if storedHash[d.DocID] == sha256hex(d.Content) {
			continue
		}
		toEmbed = append(toEmbed, d)
	}

	for i := 0; i < len(toEmbed); i += indexBatchSize {
		end := min(i+indexBatchSize, len(toEmbed))
		batch := toEmbed[i:end]
		texts := make([]string, len(batch))
		for j, d := range batch {
			texts[j] = d.Content
		}
		vectors, err := idx.provider.Embed(ctx, texts)
		if err != nil {
			slog.Error("[embeddings] IndexDocs: embed batch", "repo", repoFullName, "error", err)
			continue
		}
		for j, d := range batch {
			if err := idx.store.UpsertDoc(d.DocID, sha256hex(d.Content), idx.provider.ModelKey(), vectors[j]); err != nil {
				slog.Error("[embeddings] IndexDocs: upsert", "docID", d.DocID, "error", err)
			}
		}
	}

	// Remove stale rows for docs that no longer exist in this repo.
	currentIDs := make(map[string]bool, len(docs))
	for _, d := range docs {
		currentIDs[d.DocID] = true
	}
	for _, e := range existing {
		if !currentIDs[e.DocID] {
			if err := idx.store.DeleteDocByID(e.DocID); err != nil {
				slog.Error("[embeddings] IndexDocs: delete stale", "docID", e.DocID, "error", err)
			}
		}
	}
}

// IndexEntities re-indexes the named entities. Skips those whose stored obs_hash matches.
func (idx *Indexer) IndexEntities(ctx context.Context, names []string) {
	if idx.entities == nil || len(names) == 0 {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()

	kg, err := idx.entities.OpenNodes(names)
	if err != nil {
		slog.Error("[embeddings] IndexEntities: open nodes", "error", err)
		return
	}

	existing, err := idx.store.LoadEntityEmbeddings(idx.provider.ModelKey())
	if err != nil {
		slog.Error("[embeddings] IndexEntities: load existing", "error", err)
		return
	}
	storedHash := make(map[string]string, len(existing))
	for _, e := range existing {
		storedHash[e.EntityName] = e.ObsHash
	}

	type item struct {
		name string
		text string
	}
	var toEmbed []item
	for _, e := range kg.Entities {
		text := EntityText(e)
		if storedHash[e.Name] == sha256hex(text) {
			continue
		}
		toEmbed = append(toEmbed, item{name: e.Name, text: text})
	}

	for i := 0; i < len(toEmbed); i += indexBatchSize {
		end := min(i+indexBatchSize, len(toEmbed))
		batch := toEmbed[i:end]
		texts := make([]string, len(batch))
		for j, it := range batch {
			texts[j] = it.text
		}
		vectors, err := idx.provider.Embed(ctx, texts)
		if err != nil {
			slog.Error("[embeddings] IndexEntities: embed batch", "error", err)
			continue
		}
		for j, it := range batch {
			if err := idx.store.UpsertEntity(it.name, sha256hex(it.text), idx.provider.ModelKey(), vectors[j]); err != nil {
				slog.Error("[embeddings] IndexEntities: upsert", "entity", it.name, "error", err)
			}
		}
	}
}

// ScheduleEntities queues entity names for re-indexing with a 2-second debounce.
// Burst calls are coalesced: all accumulated names are indexed in one pass.
// No-op when provider is nil.
func (idx *Indexer) ScheduleEntities(names []string) {
	if len(names) == 0 {
		return
	}
	idx.debounce.mu.Lock()
	defer idx.debounce.mu.Unlock()

	if idx.debounce.names == nil {
		idx.debounce.names = make(map[string]bool)
	}
	for _, n := range names {
		idx.debounce.names[n] = true
	}
	if idx.debounce.timer != nil {
		idx.debounce.timer.Reset(2 * time.Second)
		return
	}
	idx.debounce.timer = time.AfterFunc(2*time.Second, func() {
		idx.debounce.mu.Lock()
		names := make([]string, 0, len(idx.debounce.names))
		for n := range idx.debounce.names {
			names = append(names, n)
		}
		idx.debounce.names = nil
		idx.debounce.timer = nil
		idx.debounce.mu.Unlock()
		idx.IndexEntities(context.Background(), names)
	})
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./embeddings/... -run TestIndexer -v
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add embeddings/indexer.go embeddings/indexer_test.go
git commit -m "feat(embeddings): add Indexer with hash-based staleness detection and debounced entity scheduling"
```

---

### Task 7: SemanticSearcher — SearchDocs, SearchEntities

**Files:**

- Create: `embeddings/searcher.go`
- Create: `embeddings/searcher_test.go`

- [ ] **Step 1: Write the failing test**

```go
// embeddings/searcher_test.go
package embeddings_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
)

var searchCounter atomic.Int64

// keywordProvider returns a 3-dim vector based on keyword presence.
// dim0=payment, dim1=auth, dim2=notification
type keywordProvider struct{}

func (k *keywordProvider) ModelKey() string { return "mock:keyword" }
func (k *keywordProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		lower := strings.ToLower(t)
		v := []float32{0.1, 0.1, 0.1}
		if strings.Contains(lower, "payment") || strings.Contains(lower, "stripe") {
			v[0] = 0.9
		}
		if strings.Contains(lower, "auth") || strings.Contains(lower, "jwt") {
			v[1] = 0.9
		}
		if strings.Contains(lower, "notification") || strings.Contains(lower, "email") {
			v[2] = 0.9
		}
		out[i] = v
	}
	return out, nil
}

func newSearchEnv(t *testing.T) (*embeddings.VectorStore, *memory.ContentCache, *memory.MemoryService) {
	t.Helper()
	dsn := fmt.Sprintf("file:search_test_%d?mode=memory&cache=shared", searchCounter.Add(1))
	db, err := memory.OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	vs, err := embeddings.NewVectorStore(db)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	return vs, memory.NewContentCache(db, true, 1024*1024), memory.NewMemoryService(db)
}

func TestSemanticSearcher_SearchDocs_TopResult(t *testing.T) {
	vs, cc, ms := newSearchEnv(t)
	provider := &keywordProvider{}

	cc.Upsert("org/pay", "README.md", "sha1", "Payment service handles Stripe transactions.", "readme")
	cc.Upsert("org/auth", "README.md", "sha2", "Auth service manages JWT tokens.", "readme")

	idx := embeddings.NewIndexer(provider, vs, cc, ms)
	idx.IndexDocs(context.Background(), "org/pay")
	idx.IndexDocs(context.Background(), "org/auth")

	searcher := embeddings.NewSemanticSearcher(provider, vs, idx, cc, ms)
	results, stale, err := searcher.SearchDocs(context.Background(), "payment billing stripe", "", 5)
	if err != nil {
		t.Fatalf("SearchDocs: %v", err)
	}
	if stale != 0 {
		t.Errorf("expected 0 stale docs, got %d", stale)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Repo != "org/pay" {
		t.Errorf("expected payment doc first, got %s", results[0].Repo)
	}
}

func TestSemanticSearcher_SearchDocs_Disabled(t *testing.T) {
	vs, cc, ms := newSearchEnv(t)
	searcher := embeddings.NewSemanticSearcher(nil, vs, nil, cc, ms)
	_, _, err := searcher.SearchDocs(context.Background(), "query", "", 5)
	if err == nil {
		t.Fatal("expected error when provider is nil")
	}
}

func TestSemanticSearcher_SearchEntities_TopResult(t *testing.T) {
	vs, cc, ms := newSearchEnv(t)
	provider := &keywordProvider{}

	ms.CreateEntities([]memory.Entity{
		{Name: "payment-svc", EntityType: "service", Observations: []string{"handles stripe payments"}},
		{Name: "auth-svc", EntityType: "service", Observations: []string{"manages jwt tokens"}},
	})

	idx := embeddings.NewIndexer(provider, vs, cc, ms)
	idx.IndexEntities(context.Background(), []string{"payment-svc", "auth-svc"})

	searcher := embeddings.NewSemanticSearcher(provider, vs, idx, cc, ms)
	results, _, err := searcher.SearchEntities(context.Background(), "payment stripe", 5)
	if err != nil {
		t.Fatalf("SearchEntities: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Name != "payment-svc" {
		t.Errorf("expected payment-svc first, got %s", results[0].Name)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./embeddings/... -run TestSemanticSearcher
```

Expected: FAIL — `NewSemanticSearcher` undefined

- [ ] **Step 3: Create `embeddings/searcher.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/doc-scout/mcp-server/memory"
)

const disabledMsg = "semantic search not enabled: set DOCSCOUT_EMBED_OPENAI_KEY or DOCSCOUT_EMBED_OLLAMA_URL"

// DocResult is a single semantic search result for a document.
type DocResult struct {
	DocID   string  `json:"doc_id"`
	Repo    string  `json:"repo"`
	Path    string  `json:"path"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

// EntityResult is a single semantic search result for a knowledge graph entity.
type EntityResult struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entity_type"`
	Score        float64  `json:"score"`
	Observations []string `json:"observations"`
}

// GraphReader reads the full knowledge graph.
type GraphReader interface {
	ReadGraph() (memory.KnowledgeGraph, error)
}

// SemanticSearcher runs semantic similarity search over docs and entities.
// It also implements ScheduleIndexEntities and IndexDocs as a facade over Indexer.
type SemanticSearcher struct {
	provider EmbeddingProvider
	store    *VectorStore
	indexer  *Indexer
	docs     DocStore
	entities GraphReader
}

// NewSemanticSearcher creates a SemanticSearcher.
// provider may be nil — all Search* methods return a "not enabled" error.
func NewSemanticSearcher(provider EmbeddingProvider, store *VectorStore, indexer *Indexer, docs DocStore, entities GraphReader) *SemanticSearcher {
	return &SemanticSearcher{
		provider: provider,
		store:    store,
		indexer:  indexer,
		docs:     docs,
		entities: entities,
	}
}

// Enabled returns true when a provider is configured.
func (ss *SemanticSearcher) Enabled() bool { return ss.provider != nil }

// ScheduleIndexEntities delegates to the Indexer's debounced scheduler.
func (ss *SemanticSearcher) ScheduleIndexEntities(names []string) {
	if ss.indexer != nil {
		ss.indexer.ScheduleEntities(names)
	}
}

// IndexDocs delegates to the Indexer for post-scan wiring in main.go.
func (ss *SemanticSearcher) IndexDocs(ctx context.Context, repo string) {
	if ss.indexer != nil {
		ss.indexer.IndexDocs(ctx, repo)
	}
}

// SearchDocs returns the top-k semantically similar documents.
// repo may be empty to search all repos. Returns stale count alongside results.
func (ss *SemanticSearcher) SearchDocs(ctx context.Context, query, repo string, topK int) ([]DocResult, int, error) {
	if ss.provider == nil {
		return nil, 0, fmt.Errorf(disabledMsg)
	}

	vecs, err := ss.provider.Embed(ctx, []string{query})
	if err != nil {
		return nil, 0, fmt.Errorf("embed query: %w", err)
	}
	queryVec := vecs[0]

	stored, err := ss.store.LoadDocEmbeddings(ss.provider.ModelKey())
	if err != nil {
		return nil, 0, fmt.Errorf("load doc embeddings: %w", err)
	}

	current, err := ss.docs.ListDocs(repo)
	if err != nil {
		return nil, 0, fmt.Errorf("list docs: %w", err)
	}
	type docInfo struct {
		hash    string
		snippet string
	}
	currentMap := make(map[string]docInfo, len(current))
	for _, d := range current {
		snip := d.Content
		if len(snip) > 300 {
			snip = snip[:300]
		}
		currentMap[d.DocID] = docInfo{hash: sha256hex(d.Content), snippet: snip}
	}

	type candidate struct {
		e     DocEmbedding
		score float64
	}
	var candidates []candidate
	stale := 0
	for _, e := range stored {
		if repo != "" && !strings.HasPrefix(e.DocID, repo+"#") {
			continue
		}
		info, ok := currentMap[e.DocID]
		if !ok || info.hash != e.ContentHash {
			stale++
			continue
		}
		candidates = append(candidates, candidate{e: e, score: CosineSimilarity(queryVec, e.Vector)})
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}

	results := make([]DocResult, len(candidates))
	for i, c := range candidates {
		parts := strings.SplitN(c.e.DocID, "#", 2)
		repoName, path := c.e.DocID, ""
		if len(parts) == 2 {
			repoName, path = parts[0], parts[1]
		}
		results[i] = DocResult{
			DocID:   c.e.DocID,
			Repo:    repoName,
			Path:    path,
			Score:   c.score,
			Snippet: currentMap[c.e.DocID].snippet,
		}
	}
	return results, stale, nil
}

// SearchEntities returns the top-k semantically similar knowledge graph entities.
func (ss *SemanticSearcher) SearchEntities(ctx context.Context, query string, topK int) ([]EntityResult, int, error) {
	if ss.provider == nil {
		return nil, 0, fmt.Errorf(disabledMsg)
	}

	vecs, err := ss.provider.Embed(ctx, []string{query})
	if err != nil {
		return nil, 0, fmt.Errorf("embed query: %w", err)
	}
	queryVec := vecs[0]

	stored, err := ss.store.LoadEntityEmbeddings(ss.provider.ModelKey())
	if err != nil {
		return nil, 0, fmt.Errorf("load entity embeddings: %w", err)
	}

	kg, err := ss.entities.ReadGraph()
	if err != nil {
		return nil, 0, fmt.Errorf("read graph: %w", err)
	}

	type entityInfo struct {
		hash         string
		entityType   string
		observations []string
	}
	currentMap := make(map[string]entityInfo, len(kg.Entities))
	for _, e := range kg.Entities {
		currentMap[e.Name] = entityInfo{
			hash:         sha256hex(EntityText(e)),
			entityType:   e.EntityType,
			observations: e.Observations,
		}
	}

	type candidate struct {
		e     EntityEmbedding
		score float64
	}
	var candidates []candidate
	stale := 0
	for _, e := range stored {
		info, ok := currentMap[e.EntityName]
		if !ok || info.hash != e.ObsHash {
			stale++
			continue
		}
		candidates = append(candidates, candidate{e: e, score: CosineSimilarity(queryVec, e.Vector)})
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}

	results := make([]EntityResult, len(candidates))
	for i, c := range candidates {
		info := currentMap[c.e.EntityName]
		results[i] = EntityResult{
			Name:         c.e.EntityName,
			EntityType:   info.entityType,
			Score:        c.score,
			Observations: info.observations,
		}
	}
	return results, stale, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./embeddings/... -run TestSemanticSearcher -v
```

Expected: all PASS

- [ ] **Step 5: Run all embeddings tests**

```bash
go test ./embeddings/... -v
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add embeddings/searcher.go embeddings/searcher_test.go
git commit -m "feat(embeddings): add SemanticSearcher with staleness-aware doc and entity search"
```

---

### Task 8: MCP tool, ports.go, tools.go, main.go wiring

**Files:**

- Create: `tools/semantic_search.go`
- Modify: `tools/ports.go`
- Modify: `tools/tools.go`
- Modify: `main.go`

- [ ] **Step 1: Add `SemanticSearch` interface to `tools/ports.go`**

Append at the end of `tools/ports.go`:

```go
// SemanticSearch gates the semantic search Plus feature.
// Pass nil to Register to disable semantic search entirely.
type SemanticSearch interface {
	Enabled() bool
	SearchDocs(ctx context.Context, query, repo string, topK int) ([]embeddings.DocResult, int, error)
	SearchEntities(ctx context.Context, query string, topK int) ([]embeddings.EntityResult, int, error)
	// ScheduleIndexEntities queues entities for debounced re-indexing after graph mutations.
	ScheduleIndexEntities(names []string)
	// IndexDocs synchronously re-indexes docs for a repo. Call from a background goroutine.
	IndexDocs(ctx context.Context, repo string)
}
```

Add the import for `embeddings` at the top of `tools/ports.go`:

```go
import (
	"context"
	"time"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/scanner"
)
```

- [ ] **Step 2: Create `tools/semantic_search.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/embeddings"
)

type SemanticSearchArgs struct {
	Query  string `json:"query"            jsonschema:"Natural-language search query. Required."`
	Target string `json:"target,omitempty" jsonschema:"What to search: 'content' (indexed docs), 'entities' (knowledge graph), or 'both'. Defaults to 'both'."`
	TopK   int    `json:"top_k,omitempty"  jsonschema:"Maximum number of results per target (default 5, max 20)."`
	Repo   string `json:"repo,omitempty"   jsonschema:"Optional: scope content search to a single repository full name (e.g. 'org/payment-service')."`
}

type SemanticSearchResult struct {
	ContentResults []embeddings.DocResult    `json:"content_results,omitempty"`
	EntityResults  []embeddings.EntityResult `json:"entity_results,omitempty"`
	StaleDocs      int                       `json:"stale_docs"`
	StaleEntities  int                       `json:"stale_entities"`
	Provider       string                    `json:"provider"`
}

func semanticSearchHandler(semantic SemanticSearch) func(ctx context.Context, req *mcp.CallToolRequest, args SemanticSearchArgs) (*mcp.CallToolResult, SemanticSearchResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args SemanticSearchArgs) (*mcp.CallToolResult, SemanticSearchResult, error) {
		if strings.TrimSpace(args.Query) == "" {
			return nil, SemanticSearchResult{}, fmt.Errorf("parameter 'query' must not be empty")
		}
		if !semantic.Enabled() {
			return nil, SemanticSearchResult{}, fmt.Errorf("semantic search not enabled: set DOCSCOUT_EMBED_OPENAI_KEY or DOCSCOUT_EMBED_OLLAMA_URL")
		}

		target := strings.ToLower(strings.TrimSpace(args.Target))
		if target == "" {
			target = "both"
		}
		if target != "content" && target != "entities" && target != "both" {
			return nil, SemanticSearchResult{}, fmt.Errorf("invalid target %q: must be 'content', 'entities', or 'both'", target)
		}

		topK := args.TopK
		if topK <= 0 {
			topK = 5
		}
		if topK > 20 {
			topK = 20
		}

		var result SemanticSearchResult

		if target == "content" || target == "both" {
			docs, stale, err := semantic.SearchDocs(ctx, args.Query, args.Repo, topK)
			if err != nil {
				return nil, SemanticSearchResult{}, fmt.Errorf("search docs: %w", err)
			}
			result.ContentResults = docs
			result.StaleDocs = stale
		}

		if target == "entities" || target == "both" {
			entities, stale, err := semantic.SearchEntities(ctx, args.Query, topK)
			if err != nil {
				return nil, SemanticSearchResult{}, fmt.Errorf("search entities: %w", err)
			}
			result.EntityResults = entities
			result.StaleEntities = stale
		}

		return nil, result, nil
	}
}
```

- [ ] **Step 3: Update `tools/tools.go` — add `semantic` parameter to `Register`**

Change the `Register` function signature from:

```go
func Register(s *mcp.Server, sc DocumentScanner, graph GraphStore, search ContentSearcher, metrics *ToolMetrics, docMetrics *DocMetrics, readOnly bool) {
```

To:

```go
func Register(s *mcp.Server, sc DocumentScanner, graph GraphStore, search ContentSearcher, semantic SemanticSearch, metrics *ToolMetrics, docMetrics *DocMetrics, readOnly bool) {
```

At the end of `Register`, before the closing `}`, add:

```go
	// --- Semantic Search (Plus) ---

	if semantic != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "semantic_search",
			Description: "Runs a natural-language semantic search over indexed documentation content and/or knowledge graph entities using vector embeddings. Returns results ranked by cosine similarity. Requires the server to be started with DOCSCOUT_EMBED_OPENAI_KEY or DOCSCOUT_EMBED_OLLAMA_URL set. Use 'target' to choose 'content', 'entities', or 'both'. Check stale_docs/stale_entities to know how many items are pending re-indexing.",
		}, withMetrics("semantic_search", metrics, withRecovery("semantic_search", semanticSearchHandler(semantic))))
	}
```

Also update the comment on `Register`:

```go
// Register adds all DocScout MCP tools to the server.
// graph, search, and semantic may be nil — tools that require them are omitted.
// metrics and docMetrics must not be nil.
```

- [ ] **Step 4: Update mutation tool handlers to call ScheduleIndexEntities**

In `tools/create_entities.go`, find the handler function and add scheduling. The handler currently returns created entities. After the successful call to `graph.CreateEntities(...)`, add:

```go
// Schedule semantic re-indexing of newly created entities (no-op when semantic is nil).
if semantic != nil {
    names := make([]string, len(created))
    for i, e := range created {
        names[i] = e.Name
    }
    semantic.ScheduleIndexEntities(names)
}
```

Follow the same pattern for `tools/add_observations.go` (schedule affected entity names) and `tools/delete_entities.go` (schedule deleted entity names).

This means the handler factory functions for those three tools need a `semantic SemanticSearch` parameter. Update their signatures:

`createEntitiesHandler(graph GraphStore, semantic SemanticSearch)`
`addObservationsHandler(graph GraphStore, semantic SemanticSearch)`
`deleteEntitiesHandler(graph GraphStore, semantic SemanticSearch)`

Pass `semantic` from `Register` to these handlers.

- [ ] **Step 5: Update `main.go`**

Add to the env-reading block:

```go
embedCfg := embeddings.ConfigFromEnv()
embProvider := embeddings.NewProvider(embedCfg)
```

After the `contentCache` block, add:

```go
// --- Semantic Search Plus ---
var semanticSrv tools.SemanticSearch
if embProvider != nil {
    embStore, err := embeddings.NewVectorStore(db)
    if err != nil {
        slog.Error("Failed to create vector store", "error", err)
        os.Exit(1)
    }
    var docSrc embeddings.DocStore
    if contentCache != nil {
        docSrc = contentCache
    }
    embIndexer := embeddings.NewIndexer(embProvider, embStore, docSrc, memorySrv)
    semanticSrv = embeddings.NewSemanticSearcher(embProvider, embStore, embIndexer, docSrc, memorySrv)
    slog.Info("[embeddings] Semantic search enabled", "provider", embProvider.ModelKey())
} else {
    slog.Info("[embeddings] Semantic search disabled (no DOCSCOUT_EMBED_OPENAI_KEY or DOCSCOUT_EMBED_OLLAMA_URL)")
}
```

Update both `tools.Register(...)` calls to pass `semanticSrv`:

```go
tools.Register(mcpServer, sc, auditedGraph, searcher, semanticSrv, toolMetrics, docMetrics, graphReadOnly)
```

In the `sc.SetOnScanComplete` callback, after `ai.Run(...)`, add per-repo semantic indexing:

```go
if semanticSrv != nil {
    for _, repo := range repos {
        go semanticSrv.IndexDocs(context.Background(), repo.FullName)
    }
}
```

Add `"github.com/doc-scout/mcp-server/embeddings"` to the import block in `main.go`.

- [ ] **Step 6: Build to verify everything compiles**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 7: Run all existing tests**

```bash
go test ./...
```

Expected: all PASS (no regressions)

- [ ] **Step 8: Commit**

```bash
git add tools/ports.go tools/semantic_search.go tools/tools.go tools/create_entities.go tools/add_observations.go tools/delete_entities.go main.go
git commit -m "feat: add semantic_search MCP tool and wire SemanticSearcher into server"
```

---

### Task 9: End-to-end integration test

**Files:**

- Create: `tests/semantic_search/semantic_search_test.go`

- [ ] **Step 1: Create the test file**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package semantic_search_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/tests/testutils"
	"github.com/doc-scout/mcp-server/tools"
)

var testCounter atomic.Int64

// keywordProvider returns vectors based on keyword presence (3 dims: payment, auth, notification).
type keywordProvider struct{}

func (k *keywordProvider) ModelKey() string { return "mock:keyword" }
func (k *keywordProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		lower := strings.ToLower(t)
		v := []float32{0.1, 0.1, 0.1}
		if strings.Contains(lower, "payment") || strings.Contains(lower, "stripe") {
			v[0] = 0.9
		}
		if strings.Contains(lower, "auth") || strings.Contains(lower, "jwt") || strings.Contains(lower, "token") {
			v[1] = 0.9
		}
		if strings.Contains(lower, "notification") || strings.Contains(lower, "email") {
			v[2] = 0.9
		}
		return out, nil
	}
	return out, nil
}

func setupServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := t.Context()

	dsn := fmt.Sprintf("file:semantic_e2e_%d?mode=memory&cache=shared", testCounter.Add(1))
	db, err := memory.OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}

	memorySrv := memory.NewMemoryService(db)
	cc := memory.NewContentCache(db, true, 1024*1024)

	// Pre-populate docs
	cc.Upsert("org/payment-svc", "README.md", "sha1", "Payment service handles Stripe transactions and refunds.", "readme")
	cc.Upsert("org/auth-svc", "README.md", "sha2", "Auth service manages JWT tokens and OAuth2 flows.", "readme")
	cc.Upsert("org/notification-svc", "README.md", "sha3", "Notification service sends emails and SMS alerts.", "readme")

	// Pre-populate entities
	memorySrv.CreateEntities([]memory.Entity{
		{Name: "payment-svc", EntityType: "service", Observations: []string{"lang:go", "handles stripe payments"}},
		{Name: "auth-svc", EntityType: "service", Observations: []string{"lang:go", "manages jwt tokens"}},
	})

	// Build semantic service
	provider := &keywordProvider{}
	store, err := embeddings.NewVectorStore(db)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	indexer := embeddings.NewIndexer(provider, store, cc, memorySrv)
	searcher := embeddings.NewSemanticSearcher(provider, store, indexer, cc, memorySrv)

	// Index everything
	indexer.IndexDocs(ctx, "org/payment-svc")
	indexer.IndexDocs(ctx, "org/auth-svc")
	indexer.IndexDocs(ctx, "org/notification-svc")
	indexer.IndexEntities(ctx, []string{"payment-svc", "auth-svc"})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v1"}, nil)
	tools.Register(server, &testutils.MockScanner{}, memorySrv, cc, searcher, tools.NewToolMetrics(), tools.NewDocMetrics(), false)

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	return session
}

func callSemantic(t *testing.T, session *mcp.ClientSession, args map[string]any) map[string]any {
	t.Helper()
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "semantic_search",
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %v", res.Content)
	}
	var out map[string]any
	raw, _ := json.Marshal(res.Content[0])
	// The MCP SDK wraps text content — unmarshal the inner text
	var wrapper struct {
		Text string `json:"text"`
	}
	json.Unmarshal(raw, &wrapper)
	json.Unmarshal([]byte(wrapper.Text), &out)
	return out
}

func TestSemanticSearch_ContentTarget_ReturnsPaymentFirst(t *testing.T) {
	session := setupServer(t)
	result := callSemantic(t, session, map[string]any{
		"query":  "stripe payment billing",
		"target": "content",
		"top_k":  3,
	})

	contentResults, ok := result["content_results"].([]any)
	if !ok || len(contentResults) == 0 {
		t.Fatalf("expected content_results, got %v", result)
	}
	first := contentResults[0].(map[string]any)
	repo, _ := first["repo"].(string)
	if repo != "org/payment-svc" {
		t.Errorf("expected payment-svc first, got %s", repo)
	}
}

func TestSemanticSearch_EntitiesTarget_ReturnsAuthFirst(t *testing.T) {
	session := setupServer(t)
	result := callSemantic(t, session, map[string]any{
		"query":  "jwt authentication tokens",
		"target": "entities",
		"top_k":  5,
	})

	entityResults, ok := result["entity_results"].([]any)
	if !ok || len(entityResults) == 0 {
		t.Fatalf("expected entity_results, got %v", result)
	}
	first := entityResults[0].(map[string]any)
	name, _ := first["name"].(string)
	if name != "auth-svc" {
		t.Errorf("expected auth-svc first, got %s", name)
	}
}

func TestSemanticSearch_BothTarget_ReturnsBothSections(t *testing.T) {
	session := setupServer(t)
	result := callSemantic(t, session, map[string]any{
		"query":  "payment",
		"target": "both",
	})

	if _, ok := result["content_results"]; !ok {
		t.Error("expected content_results in 'both' target")
	}
	if _, ok := result["entity_results"]; !ok {
		t.Error("expected entity_results in 'both' target")
	}
}

func TestSemanticSearch_EmptyQuery_ReturnsError(t *testing.T) {
	session := setupServer(t)
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "semantic_search",
		Arguments: map[string]any{"query": ""},
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for empty query")
	}
}

func TestSemanticSearch_ToolIsListed(t *testing.T) {
	session := setupServer(t)
	resp, err := session.ListTools(t.Context(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range resp.Tools {
		if tool.Name == "semantic_search" {
			return
		}
	}
	t.Error("semantic_search not found in tool list")
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./tests/semantic_search/... -v
```

Expected: all PASS

- [ ] **Step 3: Run full test suite**

```bash
go test ./...
```

Expected: all PASS, no regressions

- [ ] **Step 4: Commit**

```bash
git add tests/semantic_search/semantic_search_test.go
git commit -m "test(semantic_search): add end-to-end MCP integration tests"
```

---

## Self-Review

**Spec coverage check:**

- ✅ EmbeddingProvider interface + OpenAI + Ollama backends → Tasks 1, 3, 4
- ✅ VectorStore with `content_hash`/`obs_hash` staleness columns → Task 2
- ✅ FLOAT32 BLOB little-endian encoding → Task 2 (`encodeVector`/`decodeVector`)
- ✅ Cosine similarity in Go → Task 1 (`CosineSimilarity`)
- ✅ `IndexDocs` post-scan with hash-based staleness → Task 6
- ✅ `IndexEntities` post-mutation with debounce → Task 6 (`ScheduleEntities`)
- ✅ `semantic_search` MCP tool with `target`, `top_k`, `repo` → Task 8
- ✅ Stale counts in response → Task 8 (`StaleDocs`, `StaleEntities`)
- ✅ Graceful degradation (nil provider → clear error) → Tasks 7, 8
- ✅ Env vars `DOCSCOUT_EMBED_OPENAI_KEY`, `DOCSCOUT_EMBED_OLLAMA_URL` → Task 8 (main.go)
- ✅ OpenAI takes precedence when both set, logs warning → Task 1 (`NewProvider`)
- ✅ `model_key` staleness on provider change → Tasks 2, 6, 7
- ✅ Integration test (end-to-end MCP call) → Task 9
- ✅ `ScheduleIndexEntities` called from create_entities, add_observations, delete_entities → Task 8

**Type consistency check:**

- `DocResult`, `EntityResult` defined in `embeddings/searcher.go`, used in `tools/ports.go` and `tools/semantic_search.go` — consistent
- `DocStore` interface defined once in `embeddings/indexer.go`, used by both Indexer and Searcher — consistent
- `sha256hex` and `EntityText` defined in `embeddings/similarity.go`, used in indexer.go and searcher.go — consistent
- `ModelKey()` format `"<provider>:<model>"` consistent across `openai.go`, `ollama.go`, and the `SemanticSearcher` staleness check
