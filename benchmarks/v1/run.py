#!/usr/bin/env python3
"""Measure the real Go API against isolated local PostgreSQL test data."""

import argparse
import concurrent.futures
import datetime as dt
import hashlib
import io
import json
import math
import os
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import tarfile
import tempfile
import threading
import time
import urllib.parse
import uuid

REPO = Path(__file__).resolve().parents[2]
PROTOCOL_PATH = REPO / "benchmarks/protocol-v1.json"
PROTOCOL = json.loads(PROTOCOL_PATH.read_text())


def summarize(details):
    """Nearest-rank percentiles over all RPCs, including failed requests."""
    if not details:
        raise ValueError("No request samples")
    latencies = sorted(row["latency"] / 1e6 for row in details)
    return {
        "requests": len(details),
        "errors": sum(row["status"] != "OK" for row in details),
        **{f"p{p}_ms": round(latencies[math.ceil(len(latencies) * p / 100) - 1], 3)
           for p in (50, 95, 99)},
        "max_ms": round(latencies[-1], 3),
    }


def source_files():
    paths = [path for folder in ("cmd", "internal", "gen")
             for path in (REPO / folder).rglob("*.go") if not path.name.endswith("_test.go")]
    paths += [REPO / "go.mod", REPO / "go.sum", REPO / "migrations/001_create_tickets.sql"]
    return {str(path.relative_to(REPO)): path.read_bytes() for path in sorted(paths)}


def fingerprint(files):
    return {name: hashlib.sha256(content).hexdigest() for name, content in files.items()}


def resources(pid):
    fields = Path(f"/proc/{pid}/stat").read_text().split()
    status = dict(line.split(":", 1) for line in Path(f"/proc/{pid}/status").read_text().splitlines() if ":" in line)
    return {"cpu_ticks": int(fields[13]) + int(fields[14]),
            "rss_kib": int(status["VmRSS"].split()[0]), "threads": int(status["Threads"])}


def sql(dsn, query):
    result = subprocess.run(["psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", dsn, "-c", query],
                            capture_output=True, text=True, timeout=30)
    if result.returncode:
        raise RuntimeError(result.stderr.strip())
    return result.stdout.strip()


def call(ghz, endpoint, output, method, data, count, workers, label):
    payload, report_path = output / f"{label}-requests.json", output / f"{label}.json"
    payload.write_text(json.dumps(data))
    command = [ghz, "--insecure", "--call", "ticket.v1.TicketService/" + method,
               "--data-file", str(payload), "--total", str(count), "--concurrency", str(workers),
               "--connections", str(min(8, workers)), "--timeout", "5s", "--connect-timeout", "3s",
               "--count-errors", "--disable-template-data", "--format", "json",
               "--output", str(report_path), endpoint]
    # ghz v0.121.0's -x ignored -n in the pilot; bound the subprocess instead.
    result = subprocess.run(command, capture_output=True, text=True, timeout=max(60, count / 500 + 30))
    if result.returncode:
        raise RuntimeError(f"ghz {method}: {result.stderr.strip()}")
    report = json.loads(report_path.read_text())
    if report["count"] != count or len(report["details"]) != count:
        raise RuntimeError(f"{method}: expected {count} samples, got {report['count']}")
    return report


def run_volume(args, binary, ghz, work, volume, inventory_rows, save):
    schema = "load_" + uuid.uuid4().hex
    server = None
    schema_created = False
    raw = args.output / "raw"
    label_prefix = f"n{volume['request_budget']}"
    try:
        sql(args.dsn, f'CREATE SCHEMA "{schema}"')
        schema_created = True
        sql(args.dsn, f'SET search_path TO "{schema}";\n' + (REPO / "migrations/001_create_tickets.sql").read_text())
        sql(args.dsn, f'''INSERT INTO "{schema}".tickets
            (name, departure_location, destination, price_cents, ticket_type, time_start, time_end)
            SELECT 'load-' || n,
            (ARRAY['Hanoi','Saigon','Danang','Hue'])[1 + n % 4],
            (ARRAY['Dalat','Haiphong','Vinh'])[1 + (n / 4) % 3],
            125000, 'Economy', now() + interval '1 day' + n * interval '1 minute',
            now() + interval '2 days' + n * interval '1 minute'
            FROM generate_series(0, {inventory_rows - 1}) AS n''')
        sql(args.dsn, f'ANALYZE "{schema}".tickets')
        ids = json.loads(sql(args.dsn, f'SELECT json_agg(id::text ORDER BY id) FROM "{schema}".tickets'))
        if len(ids) != inventory_rows or len(set(ids)) != inventory_rows:
            raise RuntimeError("Inventory fixture is incomplete")

        parts = urllib.parse.urlsplit(args.dsn)
        query = dict(urllib.parse.parse_qsl(parts.query))
        query.update(search_path=schema, application_name=schema)
        server_dsn = urllib.parse.urlunsplit(parts._replace(query=urllib.parse.urlencode(query)))
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        server_dir = work / label_prefix
        server_dir.mkdir()
        env_file = server_dir / ".env"
        env_file.write_text(f"POSTGRES_URL={server_dsn}\nGRPC_PORT={port}\n")
        env_file.chmod(0o600)
        with (raw / f"{label_prefix}-server.log").open("w") as log:
            server = subprocess.Popen([str(binary)], cwd=server_dir, stdout=log, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 10
        while True:
            if server.poll() is not None:
                raise RuntimeError("API stopped during startup; inspect its server.log")
            try:
                with socket.create_connection(("127.0.0.1", port), timeout=0.2):
                    break
            except OSError:
                if time.monotonic() > deadline:
                    raise RuntimeError("API did not start")
                time.sleep(0.05)

        endpoint = f"127.0.0.1:{port}"
        searches = [{"departure_location": a, "destination": b, "page_size": 50}
                    for a in ("Hanoi", "Saigon", "Danang", "Hue") for b in ("Dalat", "Haiphong", "Vinh")]
        for method, data in [("ListTickets", searches), ("GetTicket", [{"id": ids[0]}])]:
            report = call(ghz, endpoint, raw, method, data, 100, 8, f"{label_prefix}-warmup-{method}")
            if report["statusCodeDistribution"] != {"OK": 100}:
                raise RuntimeError("Warmup failed")

        budget = volume["request_budget"] // len(args.concurrency)
        for phase, concurrency in enumerate(args.concurrency):
            print(f"Running {volume['request_budget']:,} total: {budget:,} requests at C={concurrency}", flush=True)
            samples, stop = [resources(server.pid)], threading.Event()
            def sample():
                while not stop.wait(0.02):
                    try:
                        samples.append(resources(server.pid))
                    except OSError:
                        break
            observer = threading.Thread(target=sample, daemon=True)
            observer.start()
            started = time.perf_counter()
            try:
                get_count, reserve_count = budget * 2 // 10, budget // 10
                jobs = [
                    ("ListTickets", searches, budget * 7 // 10, concurrency * 7 // 10),
                    ("GetTicket", [{"id": ids[(phase * get_count + i) % len(ids)]} for i in range(get_count)], get_count, concurrency * 2 // 10),
                    ("UpdateTicketStatus", [{"id": value, "status": "TICKET_STATUS_RESERVED"}
                                            for value in ids[phase * reserve_count:(phase + 1) * reserve_count]], reserve_count, concurrency // 10),
                ]
                with concurrent.futures.ThreadPoolExecutor(max_workers=3) as executor:
                    futures = [(method, executor.submit(call, ghz, endpoint, raw, method, data, count, workers,
                                                        f"{label_prefix}-c{concurrency}-{method}"))
                               for method, data, count, workers in jobs]
                    reports = {method: future.result() for method, future in futures}
            finally:
                elapsed = time.perf_counter() - started
                stop.set()
                observer.join()
            after = resources(server.pid)
            details = [entry for report in reports.values() for entry in report["details"]]
            row = {
                "concurrency": concurrency, **summarize(details), "wall_seconds": round(elapsed, 4),
                "requests_per_second_wall": round(len(details) / elapsed, 2),
                "server_cpu_one_core_percent": round((after["cpu_ticks"] - samples[0]["cpu_ticks"]) / os.sysconf("SC_CLK_TCK") / elapsed * 100, 2),
                "server_peak_rss_mib": round(max(s["rss_kib"] for s in samples) / 1024, 2),
                "server_peak_os_threads": max(s["threads"] for s in samples),
                "server_pool_connections": int(sql(args.dsn, f"SELECT count(*) FROM pg_stat_activity WHERE application_name = '{schema}'")),
                "methods": {method: {**summarize(report["details"]), "ghz_rps": round(report["rps"], 2),
                                     "status_codes": report["statusCodeDistribution"], "duration_seconds": report["total"] / 1e9,
                                     "raw_report": f"raw/{label_prefix}-c{concurrency}-{method}.json"}
                            for method, report in reports.items()},
            }
            if row["requests"] != budget:
                raise RuntimeError("Stage request count differs from its budget")
            volume["stages"].append(row)
            save()
            print(f"  {row['requests_per_second_wall']:,.0f} req/s | p50 {row['p50_ms']:.2f} ms | p95 {row['p95_ms']:.2f} ms | p99 {row['p99_ms']:.2f} ms | errors {row['errors']}", flush=True)

        volume["database_state"] = json.loads(sql(args.dsn, f'''SELECT json_object_agg(status, n) FROM
            (SELECT status, count(*) AS n FROM "{schema}".tickets GROUP BY status) s'''))
        acknowledged = sum(stage["methods"]["UpdateTicketStatus"]["status_codes"].get("OK", 0) for stage in volume["stages"])
        volume["acknowledged_reservations"] = acknowledged
        volume["database_state_matches_acknowledgements"] = volume["database_state"].get("reserved", 0) == acknowledged
        if not any(stage["errors"] for stage in volume["stages"]) and not volume["database_state_matches_acknowledgements"]:
            raise RuntimeError("Successful reservation responses disagree with PostgreSQL state")
    finally:
        if server is not None:
            if server.poll() is None:
                server.send_signal(signal.SIGINT)
                try:
                    server.wait(timeout=15)
                    volume["api_shutdown"] = "graceful" if server.returncode == 0 else f"exit {server.returncode}"
                except subprocess.TimeoutExpired:
                    server.kill()
                    server.wait()
                    volume["api_shutdown"] = "killed after shutdown timeout"
            else:
                volume["api_shutdown"] = f"exited before cleanup: {server.returncode}"
        if schema_created:
            sql(args.dsn, f'DROP SCHEMA "{schema}" CASCADE')
            volume["temporary_schema_removed"] = True
        save()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--requests", nargs="+", type=int, default=PROTOCOL["request_budgets"], help="Total measured RPCs for each run")
    parser.add_argument("--concurrency", nargs="+", type=int, default=PROTOCOL["concurrency"], help="Worker levels; positive multiples of 10")
    parser.add_argument("--label", default="v1", help="Architecture version being tested, such as v1 or v2")
    parser.add_argument("--output", type=Path, default=Path(__file__).parent / "results" / dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ"))
    args = parser.parse_args()
    args.dsn = os.environ.get("TEST_DATABASE_URL", "")
    if urllib.parse.urlsplit(args.dsn).hostname not in ("localhost", "127.0.0.1", "::1"):
        parser.error("Set TEST_DATABASE_URL to a local PostgreSQL URL")
    if any(c <= 0 or c % 10 for c in args.concurrency):
        parser.error("Concurrency values must be positive multiples of 10")
    if any(n <= 0 or n % (10 * len(args.concurrency)) or n // len(args.concurrency) < max(args.concurrency) for n in args.requests):
        parser.error("Each request budget must divide into stages of at least the largest concurrency, with a 70/20/10 mix")
    if max(args.requests) // 10 > PROTOCOL["inventory_rows"]:
        parser.error("This protocol has insufficient distinct tickets for that request budget")
    ghz = shutil.which(os.environ.get("GHZ_BIN", "ghz"))
    if not ghz:
        parser.error("Install ghz v0.121.0, or set GHZ_BIN to its executable")
    if not shutil.which("go") or not shutil.which("psql"):
        parser.error("go and psql must be available on PATH")
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=False)
    (args.output / "raw").mkdir()
    shutil.copyfile(Path(__file__), args.output / "raw/runner.py")
    summary = {
        "architecture": args.label,
        "protocol_id": PROTOCOL["id"],
        "protocol_sha256": hashlib.sha256(PROTOCOL_PATH.read_bytes()).hexdigest(),
        "created_utc": dt.datetime.now(dt.timezone.utc).isoformat(),
        "worktree_revision_before_build": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=REPO, text=True).strip(),
        "go_version": subprocess.check_output(["go", "version"], text=True).strip(),
        "ghz_version": subprocess.check_output([ghz, "--version"], stderr=subprocess.STDOUT, text=True).strip(),
        "runner_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "runner_source": "raw/runner.py", "source_archive": "raw/source.tar.gz",
        "cpu_count": os.cpu_count(),
        "cpu_model": next(line.split(":", 1)[1].strip() for line in Path("/proc/cpuinfo").read_text().splitlines() if line.startswith("model name")),
        "inventory_rows_per_run": PROTOCOL["inventory_rows"],
        "workload_percent": {"ListTickets": 70, "GetTicket": 20, "UpdateTicketStatus_RESERVED": 10},
        "search_page_size": 50, "request_timeout_seconds": 5, "warmup_requests_per_run": 200,
        "configured_concurrency": args.concurrency, "volumes": [],
        "methodology": {
            "percentiles": "Nearest rank over individual RPC latencies, including failures; never average method percentiles",
            "rps": "Completed measured calls divided by stage wall time, including load-client startup and JSON output",
            "load_shape": "Three closed-loop method-specific worker pools, split 70/20/10; instantaneous mix changes as pools drain",
            "connections_per_method": "min(8, method worker count)",
            "dataset": "Same initial inventory size for all volumes; fresh schema and API process per volume; reservations accumulate within each volume",
        },
        "limitations": ["Generator, API, and PostgreSQL share a host with other workstation processes.",
                        "No think time; configured workers are not simultaneous human users.",
                        "No TLS, remote network, hot-ticket contention, or complete checkout workflow.",
                        "Pool waits, query durations, database CPU/IO, and Go goroutines are not instrumented.",
                        "CPU is percent of one core; OS thread counts are not goroutine counts.",
                        "Raw per-request reports remain local under the git-ignored raw directory."],
    }
    def save():
        (args.output / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    save()
    try:
        with tempfile.TemporaryDirectory(prefix="cdc-benchmark-v1-") as folder:
            work, files = Path(folder), source_files()
            binary = work / "api"
            subprocess.run(["go", "build", "-mod=readonly", "-o", str(binary), "./cmd/api"], cwd=REPO, check=True, timeout=120)
            if fingerprint(files) != fingerprint(source_files()):
                raise RuntimeError("Application source changed during the build; rerun for a consistent snapshot")
            summary["source_sha256"] = fingerprint(files)
            summary["api_binary_sha256"] = hashlib.sha256(binary.read_bytes()).hexdigest()
            summary["build_info"] = subprocess.check_output(["go", "version", "-m", str(binary)], text=True)
            with tarfile.open(args.output / "raw/source.tar.gz", "w:gz") as archive:
                for name, content in files.items():
                    info = tarfile.TarInfo(name)
                    info.size = len(content)
                    archive.addfile(info, io.BytesIO(content))
            summary["postgres_version"] = sql(args.dsn, "SHOW server_version")
            summary["postgres_max_connections"] = int(sql(args.dsn, "SHOW max_connections"))
            for count in args.requests:
                volume = {"request_budget": count, "stages": []}
                summary["volumes"].append(volume)
                run_volume(args, binary, ghz, work, volume, summary["inventory_rows_per_run"], save)
            summary["source_changed_after_build"] = fingerprint(files) != fingerprint(source_files())
            summary["measured_requests"] = sum(stage["requests"] for volume in summary["volumes"] for stage in volume["stages"])
            summary["errors"] = sum(stage["errors"] for volume in summary["volumes"] for stage in volume["stages"])
            summary["passed"] = summary["errors"] == 0 and all(v["database_state_matches_acknowledgements"] for v in summary["volumes"])
    except BaseException as error:
        summary["execution_error"] = str(error) or type(error).__name__
        raise
    finally:
        summary["finished_utc"] = dt.datetime.now(dt.timezone.utc).isoformat()
        save()
        print(f"Results: {args.output / 'summary.json'}", flush=True)
    return 0 if summary["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
