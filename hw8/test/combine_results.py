#!/usr/bin/env python3
"""
Combines mysql_test_results.json and dynamodb_test_results.json into
combined_results.json and prints the STEP III comparison tables.

Usage:
  python combine_results.py
"""

import json
import sys
from pathlib import Path


def load(path: str) -> list:
    p = Path(path)
    if not p.exists():
        print(f"ERROR: {path} not found. Run run_test.py first.")
        sys.exit(1)
    with open(p) as f:
        data = json.load(f)
    if len(data) != 150:
        print(f"WARNING: {path} has {len(data)} records (expected 150)")
    return data


def stats(records: list) -> dict:
    times = sorted(r["response_time"] for r in records)
    n = len(times)
    successes = sum(1 for r in records if r["success"])
    return {
        "count": n,
        "success_rate": round(successes / n * 100, 1) if n else 0,
        "avg": round(sum(times) / n, 2) if n else 0,
        "p50": round(times[int(n * 0.50)], 2) if n else 0,
        "p95": round(times[int(n * 0.95)], 2) if n else 0,
        "p99": round(times[int(n * 0.99)], 2) if n else 0,
    }


def main() -> None:
    mysql_data = load("mysql_test_results.json")
    dynamodb_data = load("dynamodb_test_results.json")

    # Tag each record with its source database
    for r in mysql_data:
        r["database"] = "mysql"
    for r in dynamodb_data:
        r["database"] = "dynamodb"

    combined = mysql_data + dynamodb_data

    with open("combined_results.json", "w") as f:
        json.dump(combined, f, indent=2)
    print("combined_results.json written.\n")

    # ── Overall comparison table ────────────────────────────────────────────
    ms = stats(mysql_data)
    ds = stats(dynamodb_data)

    print("=== Overall Performance Comparison ===")
    print(f"{'Metric':<30} {'MySQL':>10} {'DynamoDB':>10} {'Winner':>10} {'Margin':>10}")
    print("-" * 72)

    def row(label, mk, dk, lower_is_better=True):
        if lower_is_better:
            winner = "MySQL" if mk < dk else "DynamoDB"
            margin = abs(dk - mk)
        else:
            winner = "MySQL" if mk > dk else "DynamoDB"
            margin = abs(mk - dk)
        print(f"{label:<30} {mk:>10} {dk:>10} {winner:>10} {margin:>10.2f}")

    row("Avg Response Time (ms)", ms["avg"], ds["avg"])
    row("P50 Response Time (ms)", ms["p50"], ds["p50"])
    row("P95 Response Time (ms)", ms["p95"], ds["p95"])
    row("P99 Response Time (ms)", ms["p99"], ds["p99"])
    row("Success Rate (%)", ms["success_rate"], ds["success_rate"], lower_is_better=False)
    print(f"{'Total Operations':<30} {'150':>10} {'150':>10}")

    # ── Per-operation breakdown ─────────────────────────────────────────────
    print("\n=== Operation-Specific Breakdown ===")
    print(f"{'Operation':<15} {'MySQL Avg (ms)':>15} {'DynamoDB Avg (ms)':>18} {'Faster By':>12}")
    print("-" * 62)

    for op in ("create_cart", "add_items", "get_cart"):
        m_ops = [r for r in mysql_data if r["operation"] == op]
        d_ops = [r for r in dynamodb_data if r["operation"] == op]
        m_avg = stats(m_ops)["avg"]
        d_avg = stats(d_ops)["avg"]
        faster = "MySQL" if m_avg < d_avg else "DynamoDB"
        margin = abs(m_avg - d_avg)
        print(f"{op:<15} {m_avg:>15.2f} {d_avg:>18.2f} {faster + f' +{margin:.1f}ms':>12}")


if __name__ == "__main__":
    main()
