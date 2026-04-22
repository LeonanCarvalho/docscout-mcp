# Benchmarks

This page reports DocScout-MCP's accuracy and token efficiency, measured against a [synthetic corpus](https://github.com/doc-scout/mcp-server/tree/main/benchmark/testdata) with committed ground truth.

## Reproducing Results

```bash
# Theoretical (no API key needed)
make benchmark

# Live (requires ANTHROPIC_API_KEY)
make benchmark-live
```

For the full methodology, see the [design spec](https://github.com/doc-scout/mcp-server/blob/main/docs/superpowers/specs/2026-04-16-community-credibility-benchmarks-design.md).

---

> Reproduce: `make benchmark` (theoretical) or `make benchmark-live` (requires `ANTHROPIC_API_KEY`)

---

## Accuracy (Synthetic Corpus)

Tests whether each parser correctly extracts entities, relations, and observations from known fixtures.

| Parser         | Precision | Recall | F1       | TP  | FP  | FN  |
| -------------- | --------- | ------ | -------- | --- | --- | --- |
| `asyncapi`     | 1.00      | 1.00   | **1.00** | 4   | 0   | 0   |
| `catalog-info` | 1.00      | 1.00   | **1.00** | 4   | 0   | 0   |
| `codeowners`   | 1.00      | 1.00   | **1.00** | 4   | 0   | 0   |
| `gomod`        | 1.00      | 1.00   | **1.00** | 12  | 0   | 0   |
| `k8s`          | 1.00      | 1.00   | **1.00** | 2   | 0   | 0   |
| `openapi`      | 1.00      | 1.00   | **1.00** | 5   | 0   | 0   |
| `packagejson`  | 1.00      | 1.00   | **1.00** | 4   | 0   | 0   |
| `pomxml`       | 1.00      | 1.00   | **1.00** | 5   | 0   | 0   |
| `proto`        | 1.00      | 1.00   | **1.00** | 4   | 0   | 0   |
| `spring-kafka` | 1.00      | 1.00   | **1.00** | 3   | 0   | 0   |
| **overall**    | 1.00      | 1.00   | **1.00** | 47  | 0   | 0   |

## Token Efficiency (Theoretical Model)

Estimated tokens consumed per question: DocScout vs naive file-by-file reading.
Naive baseline assumes an AI reads each relevant file individually from GitHub.

| #   | Question                                           | DocScout | Naive     | Savings   |
| --- | -------------------------------------------------- | -------- | --------- | --------- |
| 1   | Which services depend on billing-service?          | 320      | 27705     | **98.8%** |
| 2   | Who owns the checkout service?                     | 180      | 14776     | **98.8%** |
| 3   | What would break if database goes down?            | 450      | 36940     | **98.8%** |
| 4   | List all services that expose a gRPC endpoint      | 280      | 46175     | **99.4%** |
| 5   | Which repos have no CODEOWNERS?                    | 210      | 36940     | **99.4%** |
| 6   | What Go services depend on billing-service dire... | 300      | 22164     | **98.6%** |
| 7   | Which teams own more than one service?             | 240      | 18470     | **98.7%** |
| 8   | What events does payment-worker publish?           | 190      | 9235      | **97.9%** |
| 9   | Find the shortest dependency path from checkout... | 380      | 33246     | **98.9%** |
| 10  | Which services have no OpenAPI spec?               | 220      | 36940     | **99.4%** |
| 11  | What is the Go version of billing-service?         | 150      | 18470     | **99.2%** |
| 12  | List all services that depend on kafka-client      | 290      | 27705     | **99.0%** |
| —   | **Average**                                        | **267**  | **27397** | **99.0%** |
