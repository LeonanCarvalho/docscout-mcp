# Ecosystem Analysis: Graphify Insights — Roadmap Additions

**Date:** 2026-04-22  
**Author:** Leonan Carvalho  
**Status:** Approved

---

## Context

On April 5, 2026 — ten days after DocScout-MCP launched (March 26) — a project called [Graphify](https://github.com/safishamsi/graphify) appeared in the same ecosystem. By April 22 it had accumulated 32.8k stars, 3.6k forks, and 61 releases. This spec captures what drove that growth and the resulting roadmap additions for DocScout-MCP.

---

## Diagnosis: Why Graphify Got Traction in 17 Days

Seven factors combined:

### Distribution factors (highest impact)
1. **Claude Code Skill distribution** — `pip install graphifyy && graphify install`. Zero friction for existing Claude Code users. DocScout requires GitHub token, org config, server URL, MCP client wiring — 10x more steps.
2. **"Any folder" use case** — works on any local directory with no account required. Audience = every developer. DocScout's audience = developers with GitHub orgs.
3. **Shareable visual output** — interactive HTML graph is a screenshot-and-post artifact. Drives organic viral loops. DocScout has no visual output today.

### Credibility factors
4. **Specific benchmark claim** — "71.5x fewer tokens per query vs. reading raw files". One sentence, memorable number. DocScout has benchmark infrastructure (item 20) but no published number.
5. **61 releases in 17 days** — signals active project to any visitor looking at the releases badge.
6. **Edge tagging (EXTRACTED/INFERRED/AMBIGUOUS)** — transparency about what was extracted vs. inferred vs. guessed. Builds user trust in AI-generated data.

### Community factor
7. **28 language translations + Community Hub** — global reach from day one. Low-barrier contributions.

### What Graphify does NOT have
- No persistence — every run rebuilds the graph from scratch
- Does not scale to multiple repositories across an org
- No security / RBAC
- No webhooks / event-driven updates
- No Prometheus metrics
- No CI/CD integration
- No persistent embeddings
- Not a server — not "always on" for LLM consumption

---

## Positioning

Graphify and DocScout solve different problems that look similar on the surface:

| Dimension | Graphify | DocScout-MCP |
|---|---|---|
| Deployment | Local CLI / skill | Persistent MCP server |
| Scope | Single folder/repo | Entire GitHub org |
| Persistence | None (rebuilds each run) | SQLite / PostgreSQL |
| Updates | Manual / watch mode | Webhooks + polling |
| Security | None | Path validation, auth middleware |
| Audience | Individual developers | Engineering orgs / teams |
| Distribution | pip + Claude Code skills | Binary + Docker + Helm |

**Core narrative:**

> *"Graphify for one repo. DocScout for your entire organization."*

> *"DocScout is the always-on knowledge layer for engineering orgs — not a one-shot CLI, but a persistent MCP server that indexes your entire GitHub org, tracks changes via webhooks, and serves accurate architectural context to every AI agent your team uses."*

---

## Strategy: Converge + Dominate

Add the highest-ROI visibility features that Graphify validated, while simultaneously deepening the production-grade, org-wide capabilities that Graphify cannot replicate.

---

## New Roadmap Items

### Frente 1 — Viral Wins (Cycle 1, immediate)

#### 25. Graph Visualization Export
**Goal:** Export the knowledge graph as an interactive HTML artifact (vis.js or D3) and/or Obsidian-compatible JSON vault.  
**Why:** The single highest-impact gap. The DocScout graph already exists in memory — this is purely a rendering layer. Shareable artefacts drive organic discovery.  
**Scope:**
- New MCP tool `export_graph` — accepts optional `entity`, `depth`, `format` (html | json | obsidian)
- `memory/export.go` — query graph and serialize to chosen format
- `tools/export_graph.go` — handler
- Static HTML template with vis.js bundled (no CDN dependency)
- Output: file written to configurable path or returned as base64

#### 26. URL Ingestion Tool
**Goal:** `ingest_url` MCP tool that fetches a URL (web page, paper, changelog, blog post), extracts entities and observations, and adds them to the graph.  
**Why:** Extends DocScout's scope beyond GitHub repositories to any web content.  
**Scope:**
- `tools/ingest_url.go` — fetches URL, extracts title/headings/links, creates entity + observations
- Content stored in `ContentCache` if `SCAN_CONTENT=true`
- Rate-limited; domain allowlist via `ALLOWED_INGEST_DOMAINS` env var (empty = allow all)
- Returns created entity name and observation count

#### 27. Benchmark Narrative — "The Number"
**Goal:** Publish a single, memorable, defensible benchmark number in the README badge and `benchmark/RESULTS.md`.  
**Why:** Graphify's "71.5x fewer tokens" is why it gets cited. DocScout has benchmark infrastructure (item 20) but no published result. The number must exist before any visibility push.  
**Scope:**
- Run `make benchmark-live` against canonical corpus
- Compute: tokens consumed by DocScout tools vs. tokens that would have been consumed reading raw files
- Publish as `benchmark/RESULTS.md` with methodology section
- Add badge to README: `![Token Reduction](https://img.shields.io/badge/token_reduction-Xx-green)`
- Pin the corpus commit so the number is reproducible

#### 28. Onboarding in 60 Seconds
**Goal:** A single command that installs, configures, and starts DocScout with a demo repo in under 60 seconds.  
**Why:** Removes the largest adoption barrier. Graphify's two-command install is why individuals try it. DocScout needs an equivalent.  
**Scope:**
- `bin/docscout-init` shell script (curl-installable): downloads binary, creates `.env.local` with SQLite + demo GitHub repo, starts server, prints Claude Desktop config snippet
- Or: `npx docscout-mcp@latest init` Node wrapper over the binary
- Target: zero manual config for the "try it" path
- Full production config remains unchanged

---

### Frente 2 — Enterprise Moat (Cycle 2-3)

#### 29. Relation Confidence Scores
**Goal:** Add a `confidence` field to graph relations: `"authoritative" | "inferred" | "ambiguous"`.  
**Why:** Analogous to Graphify's EXTRACTED/INFERRED/AMBIGUOUS edge tags. Increases LLM trust in graph data. DocScout already has `_source:*` observations — confidence scores make it per-edge and queryable.  
**Scope:**
- Add `confidence` column to `relations` table (migration)
- Parser-set confidence: catalog/asyncapi/proto/openapi → `authoritative`; spring_kafka/k8s env vars → `inferred`; manual `create_relations` → `authoritative` by default, caller can override
- `traverse_graph` and `get_integration_map` return confidence in results

#### 30. GitHub Actions Action (elevated from item 22)
**Goal:** `docscout-action` — run a DocScout scan in CI and post graph diff as PR comment.  
**Why:** GitHub Actions Marketplace is a distribution channel unavailable to local CLI tools. A Marketplace listing opens a completely different audience: platform teams, DevOps, CTOs evaluating tooling.  
**Scope:** (unchanged from item 22, elevated in cycle priority)

#### 31. Contributing Examples Corpus
**Goal:** Committed synthetic corpus with ground truth, honest accuracy evaluation, and worked examples.  
**Why:** Graphify explicitly prioritizes "real corpus demonstrations with honest evaluation". This is a community contribution magnet — easy to contribute, improves trust.  
**Scope:**
- `tests/corpus/` — 3-5 representative repo structures (Go microservices, Node monorepo, Java Spring)
- Ground truth: expected entities, relations, observations per corpus
- `make benchmark` runs accuracy F1 against corpus (no API key needed)
- `CONTRIBUTING.md` section: "Add a corpus example"

#### 32. Documentation i18n
**Goal:** README and key docs in PT-BR, ES, ZH as starting point.  
**Why:** Graphify's 28 translations drove global reach and low-barrier contributions.  
**Scope:**
- `docs/i18n/pt-BR/`, `docs/i18n/es/`, `docs/i18n/zh/`
- README badge linking to translations
- Contributing guide section: "Translate to your language"

---

### Existing Items — Reaffirmed Priority

| Item | Title | Cycle | Notes |
|---|---|---|---|
| 4 | Graph Knowledge Access Control (RBAC) | 3 | Org-wide security; no equivalent in local CLI tools |
| 5 | Multi-Cloud Platform Adapters | 3 | GitLab/Bitbucket opens non-GitHub orgs |
| 20 | Benchmark Suite | 1 | Prerequisite for item 27 |
| 21 | `--benchmark` CLI Mode | 2 | User-facing benchmark runner |
| 23 | LLM Eval Harness | 3 | Answer-quality beyond token counting |
| 24 | OpenTelemetry Traces | 3 | Production observability |

---

## Implementation Cycles

### Cycle 1 — Traction (immediate)
**Items:** 27 (The Number), 25 (Graph Visualization), 28 (Onboarding 60s), 20 (Benchmark Suite prerequisite)  
**Goal:** Have something shareable and a number that appears in posts.

### Cycle 2 — Expansion
**Items:** 26 (URL Ingestion), 30 (GitHub Action), 29 (Confidence Scores), 31 (Examples corpus), 21 (Benchmark CLI)  
**Goal:** Open distribution channels beyond "people who already use Claude Code".

### Cycle 3 — Moat
**Items:** 4 (RBAC), 5 (GitLab/Bitbucket), 24 (OTel), 23 (LLM Eval Harness), 32 (i18n docs)  
**Goal:** Make DocScout the default choice for orgs that need persistent, secure, multi-repo knowledge infrastructure.

---

## What Was Not Changed

No existing roadmap items were removed. Items 4, 5, 20, 21, 22, 23, 24 were reaffirmed and sequenced. The 8 new items (25–32) complement rather than replace the existing plan.
