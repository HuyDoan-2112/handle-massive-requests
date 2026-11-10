#!/usr/bin/env python3
"""Compare completed benchmark runs with matching workloads and environments."""

import argparse
import json
from pathlib import Path

MATCH_FIELDS = (
    "inventory_rows_per_run", "workload_percent", "search_page_size",
    "request_timeout_seconds", "warmup_requests_per_run", "configured_concurrency",
    "methodology", "go_version", "ghz_version", "postgres_version",
    "postgres_max_connections", "cpu_model", "cpu_count",
)


def cases(summary):
    if "finished_utc" not in summary or summary.get("execution_error"):
        raise ValueError("Cannot compare an incomplete run")
    result = {}
    levels = summary["configured_concurrency"]
    for volume in summary["volumes"]:
        budget = volume["request_budget"]
        stages = volume["stages"]
        if len(stages) != len(levels) or {stage["concurrency"] for stage in stages} != set(levels):
            raise ValueError(f"Incomplete concurrency matrix for {budget} requests")
        for stage in stages:
            if stage["requests"] * len(levels) != budget:
                raise ValueError(f"Incorrect stage request count for {budget} requests")
            key = (budget, stage["concurrency"])
            if key in result:
                raise ValueError(f"Duplicate case: {key}")
            result[key] = stage
    if not result:
        raise ValueError("Run has no measured cases")
    return result


def compare(before, after):
    mismatches = [field for field in MATCH_FIELDS if before.get(field) != after.get(field)]
    missing = [field for field in MATCH_FIELDS if field not in before or field not in after]
    if mismatches or missing:
        raise ValueError("Incompatible setup: " + ", ".join(sorted(set(mismatches + missing))))
    first, second = cases(before), cases(after)
    common = sorted(first.keys() & second.keys())
    if not common:
        raise ValueError("Runs share no request-volume/concurrency cases")
    return [(key, first[key], second[key]) for key in common], len(first.keys() ^ second.keys())


def percent(old, new):
    return f"{(new / old - 1) * 100:+.1f}%" if old else "n/a"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("baseline", type=Path)
    parser.add_argument("candidate", type=Path)
    args = parser.parse_args()
    try:
        rows, omitted = compare(json.loads(args.baseline.read_text()), json.loads(args.candidate.read_text()))
    except (ValueError, KeyError) as error:
        parser.error(str(error))
    print("| Requests | Workers | Baseline req/s | Candidate req/s | Change | Baseline p99 ms | Candidate p99 ms | Change | Errors before → after |")
    print("| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
    for (budget, workers), before, after in rows:
        rps0, rps1 = before["requests_per_second_wall"], after["requests_per_second_wall"]
        p990, p991 = before["p99_ms"], after["p99_ms"]
        print(f"| {budget:,} | {workers:,} | {rps0:,.0f} | {rps1:,.0f} | {percent(rps0, rps1)} | {p990:.2f} | {p991:.2f} | {percent(p990, p991)} | {before['errors']} → {after['errors']} |")
    print(f"\nCompared {len(rows)} matching cases; {omitted} unshared cases omitted.")
    print("Higher req/s and lower p99 are favorable. Single-run differences may be noise.")


if __name__ == "__main__":
    main()
