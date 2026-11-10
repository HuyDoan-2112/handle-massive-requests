# Benchmarks

[v1 benchmark and diagrams](v1/README.md) · [Shared protocol](protocol-v1.json)

Use the same protocol and runner when testing the next architecture. Keep v1 results and diagrams as the baseline; write new results to a new version folder.

```bash
# Run after switching the application to the next architecture.
TEST_DATABASE_URL='postgres://app:app@localhost:5433/train?sslmode=disable' \
  python3 benchmarks/v1/run.py --label v2 --output benchmarks/v2/results/run-01

# Compare matching request-volume/concurrency cases.
python3 benchmarks/compare.py \
  benchmarks/v1/results/baseline-2026-11-09/summary.json \
  benchmarks/v2/results/run-01/summary.json
```

The comparison checks dataset size, request mix, pagination, timeouts, warm-up, concurrency, measurement definitions, tool versions, and recorded hardware/database settings. It rejects incompatible setups and incomplete runs. Complete runs with different volume subsets compare only their shared cases.

Request IDs and fixture timestamps are regenerated per run. A matching setup does not eliminate variation from background load; repeat important comparisons.

```bash
python3 -m unittest discover -s benchmarks -p 'test_compare.py' -v
```
