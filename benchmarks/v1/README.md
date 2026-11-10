# v1 load benchmark

One Go API process and local PostgreSQL. [Recorded results](results/baseline-2026-11-09/summary.json): **1,110,000 measured requests, zero errors**.

## Diagrams

![v1 architecture](diagrams/architecture.png)

[Request flow](diagrams/request-flow.png) · [Benchmark topology](diagrams/topology.png)

Editable sources are the corresponding `.excalidraw` files. For v2, copy these diagrams and keep the same canvas, palette, and component style. Preserve the v1 drawings as the visual baseline.

## Run

Requires Linux, Python 3.9+, Go, `psql`, ghz v0.121.0, and local PostgreSQL.

```bash
TEST_DATABASE_URL='postgres://app:app@localhost:5433/train?sslmode=disable' \
  make benchmark-v1
```

For a quick run:

```bash
TEST_DATABASE_URL='postgres://app:app@localhost:5433/train?sslmode=disable' \
  python3 benchmarks/v1/run.py --requests 10000
```

Set `GHZ_BIN` to an executable if ghz is not on PATH. Install with `go install github.com/bojand/ghz/cmd/ghz@v0.121.0` or use an [official release](https://github.com/bojand/ghz/releases/tag/v0.121.0).

## Workload

- Shared defaults: [`protocol-v1.json`](../protocol-v1.json).
- Request budgets: 10,000, 100,000, and 1,000,000.
- Each budget is split across 10, 50, 100, 250, and 1,000 workers.
- Mix: 70% searches, 20% ticket views, 10% distinct-ticket reservations.
- Every budget starts with 100,000 synthetic tickets, including quick runs.
- Each budget uses a fresh schema and API process; existing application tickets are not used.

The runner saves results under a new timestamped directory in `results/`. The local database role needs schema-creation permission. [Compare architecture versions](../README.md) using the same runner and protocol.

## Outputs and checks

`summary.json` contains throughput, p50/p95/p99, errors, API resources, and environment information. Per-request samples, source snapshots, and logs stay in the git-ignored `raw/` directory.

```bash
python3 -m unittest discover -s benchmarks/v1 -p 'test_*.py' -v
```

Measurements are from a same-host, closed-loop workload. The method-specific worker pools drain independently. RPS includes client startup and report output; latency percentiles include failed requests. These results do not establish maximum sustainable production capacity.
