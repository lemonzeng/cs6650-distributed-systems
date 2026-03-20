#!/usr/bin/env python3
"""
Performance test for HW8 shopping cart services.

Usage:
  python3 run_test.py --url http://<alb-dns> --output mysql_test_results.json
  python3 run_test.py --url http://<alb-dns> --output dynamodb_test_results.json
"""

import argparse
import json
import random
import sys
import time
from datetime import datetime, timezone

import requests


def run_test(base_url: str, output_file: str) -> None:
    base_url = base_url.rstrip("/")
    results = []

    # ── Phase 1: CREATE CART (50 times) ────────────────────────────────────
    cart_ids = []
    print("Phase 1: Creating 50 carts...")
    for i in range(50):
        customer_id = random.randint(1, 10000)
        start = time.perf_counter()
        try:
            resp = requests.post(
                f"{base_url}/shopping-carts",
                json={"customer_id": customer_id},
                timeout=10,
            )
            elapsed_ms = (time.perf_counter() - start) * 1000
            success = resp.status_code == 201
            if success:
                cart_ids.append(resp.json().get("shopping_cart_id"))
        except requests.RequestException as e:
            elapsed_ms = (time.perf_counter() - start) * 1000
            success = False
            resp = type("R", (), {"status_code": 0})()
            print(f"  [create #{i+1}] error: {e}")

        results.append({
            "operation": "create_cart",
            "response_time": round(elapsed_ms, 2),
            "success": success,
            "status_code": resp.status_code,
            "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        })

    if not cart_ids:
        print("ERROR: No carts were created successfully. Aborting.")
        sys.exit(1)

    print(f"  Created {len(cart_ids)} carts successfully.")

    # ── Phase 2: ADD ITEMS (50 times) ──────────────────────────────────────
    print("Phase 2: Adding items to 50 carts...")
    for i in range(50):
        cart_id = cart_ids[i % len(cart_ids)]
        product_id = random.randint(1, 100000)
        quantity = random.randint(1, 10)
        start = time.perf_counter()
        try:
            resp = requests.post(
                f"{base_url}/shopping-carts/{cart_id}/items",
                json={"product_id": product_id, "quantity": quantity},
                timeout=10,
            )
            elapsed_ms = (time.perf_counter() - start) * 1000
            success = resp.status_code == 204
        except requests.RequestException as e:
            elapsed_ms = (time.perf_counter() - start) * 1000
            success = False
            resp = type("R", (), {"status_code": 0})()
            print(f"  [add_items #{i+1}] error: {e}")

        results.append({
            "operation": "add_items",
            "response_time": round(elapsed_ms, 2),
            "success": success,
            "status_code": resp.status_code,
            "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        })

    # ── Phase 3: GET CART (50 times) ───────────────────────────────────────
    print("Phase 3: Fetching 50 carts...")
    for i in range(50):
        cart_id = cart_ids[i % len(cart_ids)]
        start = time.perf_counter()
        try:
            resp = requests.get(
                f"{base_url}/shopping-carts/{cart_id}",
                timeout=10,
            )
            elapsed_ms = (time.perf_counter() - start) * 1000
            success = resp.status_code == 200
        except requests.RequestException as e:
            elapsed_ms = (time.perf_counter() - start) * 1000
            success = False
            resp = type("R", (), {"status_code": 0})()
            print(f"  [get_cart #{i+1}] error: {e}")

        results.append({
            "operation": "get_cart",
            "response_time": round(elapsed_ms, 2),
            "success": success,
            "status_code": resp.status_code,
            "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        })

    # ── Save results ────────────────────────────────────────────────────────
    with open(output_file, "w") as f:
        json.dump(results, f, indent=2)

    print(f"\nResults saved to {output_file}")
    _print_summary(results)


def _print_summary(results: list) -> None:
    for op in ("create_cart", "add_items", "get_cart"):
        ops = [r for r in results if r["operation"] == op]
        times = sorted(r["response_time"] for r in ops)
        successes = sum(1 for r in ops if r["success"])
        n = len(times)
        avg = sum(times) / n if n else 0
        p50 = times[int(n * 0.50)] if n else 0
        p95 = times[int(n * 0.95)] if n else 0
        p99 = times[int(n * 0.99)] if n else 0
        print(
            f"  {op:12s} | n={n} success={successes} "
            f"avg={avg:.1f}ms p50={p50:.1f}ms p95={p95:.1f}ms p99={p99:.1f}ms"
        )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="HW8 shopping cart performance test")
    parser.add_argument("--url", required=True, help="Base URL of the service (e.g. http://alb-dns)")
    parser.add_argument("--output", required=True, help="Output JSON file name")
    args = parser.parse_args()
    run_test(args.url, args.output)
