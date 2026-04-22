# Roadmap

This page tracks completed features and the path forward for DocScout-MCP.

For the full technical roadmap with implementation details, see [`ROADMAP.md`](https://github.com/doc-scout/mcp-server/blob/main/ROADMAP.md) in the repository.

---

## Completed

| #   | Feature                                       | Summary                                                                                                                                                                                                                                                                                                                                                      |
| --- | --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1   | **Incremental Ingestion (Webhooks)**          | GitHub Webhooks trigger instant single-repo scans on push/create/delete events, bypassing the polling cycle.                                                                                                                                                                                                                                                 |
| 3   | **Rate Limiting & Resilience**                | Every GitHub API call is wrapped with up to 3 retries: primary rate limit wait, abuse rate limit `Retry-After`, and exponential backoff for transient 5xx errors.                                                                                                                                                                                            |
| 6   | **Auto-Discovery (AST & Dependencies)**       | Automatic service entity and dependency graph inference from `go.mod`, `package.json`, `pom.xml`, and `CODEOWNERS` — no Backstage catalog-info required.                                                                                                                                                                                                     |
| 7   | **Deployment & Operations**                   | Multi-arch Docker image, docker-compose profiles, Kubernetes manifests, Helm chart v2, and Terraform module.                                                                                                                                                                                                                                                 |
| 8   | **Observability & Metrics**                   | Per-tool call counters and per-document access tracking via `get_usage_stats` tool and `/metrics` Prometheus endpoint.                                                                                                                                                                                                                                       |
| 9   | **Knowledge Graph Protection**                | Mass-delete guard (> 10 requires `confirm: true`), observation quality filter, and full mutation audit log to slog.                                                                                                                                                                                                                                          |
| 10  | **Architecture Discovery & Content Search**   | Auto-populates the knowledge graph from `catalog-info.yaml` (Backstage format). Opt-in full-text content search via `search_content`.                                                                                                                                                                                                                        |
| 11  | **Infra Asset Scanning**                      | Recursive scanning of `deploy/`, `infra/`, `.github/workflows/` for Helm, Terraform, K8s, and workflow files.                                                                                                                                                                                                                                                |
| 12  | **Security Input Hardening**                  | Entity name validation, SQL LIKE wildcard escaping, whitespace-only query rejection, constant-time bearer token comparison, HTTP server timeouts.                                                                                                                                                                                                            |
| 13  | **Custom Parser Extension**                   | `FileParser` interface and `ParserRegistry` allow custom manifest parsers to be plugged in without forking. Built-in parsers for go.mod, package.json, pom.xml, catalog-info.yaml, CODEOWNERS, AsyncAPI, Spring Kafka, OpenAPI, and Protobuf.                                                                                                                |
| 14  | **Graph Traversal & Impact Analysis**         | `traverse_graph` tool: server-side BFS with configurable direction, edge-type filter, and depth. Answers impact and ownership questions without loading the full graph.                                                                                                                                                                                      |
| 15  | **Integration Topology Discovery**            | Five parsers (AsyncAPI, Spring Kafka, OpenAPI, Proto, K8s env vars) auto-populate producer/consumer and API edges. `get_integration_map` tool returns the complete integration topology of a service in one call with `graph_coverage` confidence field.                                                                                                     |
| 16  | **Documentation Site (GitHub Pages)**         | MkDocs Material site auto-deployed to GitHub Pages on every push to `main`.                                                                                                                                                                                                                                                                                  |
| 17  | **FTS5 Full-Text Content Search**             | Content search upgraded from SQL `LIKE` to SQLite FTS5 — BM25 relevance ranking, Porter stemmer, multi-word AND queries. Zero new dependencies. `search_mode` field in `get_scan_status` reports active engine.                                                                                                                                              |
| 18  | **Graph Query Tools**                         | `list_entities` (filter by type), `list_relations` (filter by type and/or source), `update_entity` (atomic rename + reclassify with full relation/observation cascade), `find_path` (undirected BFS shortest path between any two nodes), `trigger_scan` (on-demand scan queuing with deduplication), and `entity_breakdown` breakdown in `get_scan_status`. |
| 19  | **File-Type Filters**                         | `list_repos` and `search_docs` accept an optional `file_type` parameter to narrow results to a specific document category (e.g. `openapi`, `asyncapi`, `proto`, `helm`).                                                                                                                                                                                     |
| 2   | **Semantic Search & Vector Embeddings (RAG)** | `semantic_search` MCP tool with vector embeddings via OpenAI or Ollama. Cosine similarity ranking, hash-based staleness detection, debounced background re-indexing. Zero-downtime: server starts normally if no embedding provider configured.                                                                                                              |

---

## Future Work

### Graph Knowledge Access Control (RBAC)

**Goal:** Role-Based Access Control so sensitive architectural or security documents are only accessible to authorized users or service accounts.

---

### Multi-Cloud and Platform Adapters

**Goal:** A generic Provider interface to support GitLab, Bitbucket, Confluence, Notion, and enterprise wikis alongside GitHub.
