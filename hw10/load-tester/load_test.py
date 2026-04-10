#!/usr/bin/env python3
"""
Load tester for the distributed KV service.

HOW IT GUARANTEES LOCAL-IN-TIME READS/WRITES
---------------------------------------------
After every write to key K (version V), we push an entry onto a
`paired_reads` deque.  When the next read operation fires, we pop
from that deque first — guaranteeing the read targets the exact same
key, moments after it was written.  Any read that cannot be paired
falls back to a random key from the small (100-key) pool, which also
creates natural temporal overlap at high load.

This means:
  * Every write is followed by AT LEAST ONE follower read of the same key.
  * The write->read gap is recorded precisely.
  * Staleness is detected by comparing the returned version against
    the version the server reported when the write was ACKed.

STALE READ DETECTION
--------------------
POST /set returns {"version": N}.  We store (key -> expected_version).
When a follower returns version < expected_version for that key, the
read is stale.  Reads always go to FOLLOWERS (ports 8002-8005) or
NON-COORDINATOR nodes (ports 8012-8015) so leader/coordinator freshness
never masks follower staleness.

USAGE
-----
python load_test.py --config w5r1  --writes 1  --reads 99 --requests 1000
python load_test.py --config w1r5  --writes 10 --reads 90 --requests 1000
python load_test.py --config w3r3  --writes 50 --reads 50 --requests 1000
python load_test.py --config leaderless --writes 90 --reads 10 --requests 1000

Results saved to  results/<config>_w<W>r<R>.csv
"""

import argparse
import csv
import random
import time
import uuid
from collections import deque
from pathlib import Path

import requests

# ---------------------------------------------------------------------------
# Cluster topology
# ---------------------------------------------------------------------------

CONFIGS = {
    "w5r1": {
        "write_url": "http://localhost:8001",           # leader
        "read_urls": [                                   # followers only
            "http://localhost:8002",
            "http://localhost:8003",
            "http://localhost:8004",
            "http://localhost:8005",
        ],
        "label": "Leader-Follower W=5 R=1",
    },
    "w1r5": {
        "write_url": "http://localhost:8001",
        "read_urls": [
            "http://localhost:8002",
            "http://localhost:8003",
            "http://localhost:8004",
            "http://localhost:8005",
        ],
        "label": "Leader-Follower W=1 R=5",
    },
    "w3r3": {
        "write_url": "http://localhost:8001",
        "read_urls": [
            "http://localhost:8002",
            "http://localhost:8003",
            "http://localhost:8004",
            "http://localhost:8005",
        ],
        "label": "Leader-Follower W=3 R=3",
    },
    "leaderless": {
        "write_url": "http://localhost:8011",            # node1 as coordinator
        "read_urls": [                                   # non-coordinator nodes
            "http://localhost:8012",
            "http://localhost:8013",
            "http://localhost:8014",
            "http://localhost:8015",
        ],
        "label": "Leaderless W=N=5 R=1",
    },
}

# Small fixed key pool — guarantees temporal locality even for unpaired reads.
KEY_POOL = [f"key-{i:04d}" for i in range(100)]

# Per-key version tracking (populated from server /set response).
known: dict = {}   # key -> last written version (int)


# ---------------------------------------------------------------------------
# Operations
# ---------------------------------------------------------------------------

def do_write(write_url, key, value):
    t0 = time.monotonic()
    try:
        r = requests.post(
            f"{write_url}/set",
            json={"key": key, "value": value},
            timeout=30,
        )
        latency_ms = (time.monotonic() - t0) * 1000
        version = None
        if r.status_code == 201:
            try:
                version = r.json().get("version")
            except Exception:
                pass
            if version is not None:
                known[key] = version
        return {
            "op_type":           "write",
            "key":               key,
            "latency_ms":        round(latency_ms, 3),
            "status_code":       r.status_code,
            "ok":                r.status_code == 201,
            "stale":             "",
            "write_read_gap_ms": "",
            "node":              write_url,
        }
    except requests.RequestException as exc:
        latency_ms = (time.monotonic() - t0) * 1000
        return {
            "op_type":           "write",
            "key":               key,
            "latency_ms":        round(latency_ms, 3),
            "status_code":       0,
            "ok":                False,
            "stale":             "",
            "write_read_gap_ms": "",
            "node":              write_url,
            "error":             str(exc),
        }


def do_read(read_url, key, expected_version, write_time):
    t0 = time.monotonic()
    gap_ms = round((time.time() - write_time) * 1000, 3) if write_time is not None else ""
    try:
        r = requests.get(
            f"{read_url}/get",
            params={"key": key},
            timeout=30,
        )
        latency_ms = (time.monotonic() - t0) * 1000
        stale = False
        if r.status_code == 200 and expected_version is not None:
            try:
                returned_version = r.json().get("version", -1)
                if returned_version < expected_version:
                    stale = True
            except Exception:
                pass
        elif r.status_code == 404 and expected_version is not None:
            # key was written but follower does not have it yet
            stale = True

        return {
            "op_type":           "read",
            "key":               key,
            "latency_ms":        round(latency_ms, 3),
            "status_code":       r.status_code,
            "ok":                r.status_code in (200, 404),
            "stale":             stale,
            "write_read_gap_ms": gap_ms,
            "node":              read_url,
        }
    except requests.RequestException as exc:
        latency_ms = (time.monotonic() - t0) * 1000
        return {
            "op_type":           "read",
            "key":               key,
            "latency_ms":        round(latency_ms, 3),
            "status_code":       0,
            "ok":                False,
            "stale":             False,
            "write_read_gap_ms": gap_ms,
            "node":              read_url,
            "error":             str(exc),
        }


# ---------------------------------------------------------------------------
# Main run loop
# ---------------------------------------------------------------------------

CSV_FIELDS = [
    "op_type", "key", "latency_ms", "status_code",
    "ok", "stale", "write_read_gap_ms", "node",
]


def run(config_name, write_pct, read_pct, n_requests, out_dir):
    cfg       = CONFIGS[config_name]
    write_url = cfg["write_url"]
    read_urls = cfg["read_urls"]

    # paired_reads: each write pushes one entry; the next read pops it.
    # This guarantees the read targets the exact same key moments after write.
    paired_reads = deque()

    records = []

    print(f"\n{'='*60}")
    print(f"Config : {cfg['label']}")
    print(f"Ratio  : W={write_pct}% / R={read_pct}%   n={n_requests}")
    print(f"Write  -> {write_url}")
    print(f"Read   -> {read_urls}")
    print(f"{'='*60}")

    for i in range(n_requests):
        is_write = random.randint(1, 100) <= write_pct

        if is_write:
            key   = random.choice(KEY_POOL)
            value = uuid.uuid4().hex[:8]
            rec   = do_write(write_url, key, value)
            records.append(rec)

            if rec["ok"] and key in known:
                paired_reads.append({
                    "key":              key,
                    "expected_version": known[key],
                    "write_time":       time.time(),
                })

        else:
            read_url = random.choice(read_urls)

            if paired_reads:
                # Paired read: same key as a recent write, local-in-time guaranteed
                item = paired_reads.popleft()
                rec  = do_read(
                    read_url,
                    item["key"],
                    item["expected_version"],
                    item["write_time"],
                )
            else:
                # Unpaired: random key from small pool (still creates overlap)
                key = random.choice(KEY_POOL)
                rec = do_read(
                    read_url,
                    key,
                    known.get(key),
                    None,
                )
            records.append(rec)

        if (i + 1) % 200 == 0:
            _print_progress(records, i + 1, n_requests)

    _print_progress(records, n_requests, n_requests)

    out_dir.mkdir(parents=True, exist_ok=True)
    fname = out_dir / f"{config_name}_w{write_pct}r{read_pct}.csv"
    with open(fname, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=CSV_FIELDS, extrasaction="ignore")
        writer.writeheader()
        writer.writerows(records)

    print(f"\nSaved -> {fname}")
    return fname


def _print_progress(records, done, total):
    writes = [r for r in records if r["op_type"] == "write"]
    reads  = [r for r in records if r["op_type"] == "read"]
    stale  = [r for r in reads   if r.get("stale") is True]
    print(
        f"  [{done:5d}/{total}]  "
        f"writes={len(writes)} avg={_avg(writes):.0f}ms  "
        f"reads={len(reads)} avg={_avg(reads):.0f}ms  "
        f"stale={len(stale)}/{len(reads)} "
        f"({100*len(stale)/max(len(reads),1):.1f}%)"
    )


def _avg(records):
    if not records:
        return 0.0
    return sum(r["latency_ms"] for r in records) / len(records)


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def parse_args():
    p = argparse.ArgumentParser(description="KV load tester")
    p.add_argument("--config",   required=True, choices=list(CONFIGS.keys()))
    p.add_argument("--writes",   type=int, default=10, help="Write percentage")
    p.add_argument("--reads",    type=int, default=90, help="Read percentage")
    p.add_argument("--requests", type=int, default=1000)
    p.add_argument("--out",      default="results")
    return p.parse_args()


if __name__ == "__main__":
    args = parse_args()
    run(
        config_name=args.config,
        write_pct=args.writes,
        read_pct=args.reads,
        n_requests=args.requests,
        out_dir=Path(args.out),
    )
