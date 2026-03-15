"""
Locust load tests for HW7 — Synchronous vs Asynchronous Order Processing

Usage:
  locust -f locustfile.py --host http://<ALB_DNS>

Then open http://localhost:8089 and run the scenarios below.

----------------------------------------------------------------------
Phase 1 — Normal operations (sync endpoint):
  User class : SyncOrderUser
  Users      : 5
  Spawn rate : 1 user/sec
  Duration   : 30 seconds
  Expected   : 100% success, ~3s response time per request

Phase 1 — Flash sale (sync endpoint):
  User class : SyncOrderUser
  Users      : 20
  Spawn rate : 10 users/sec
  Duration   : 60 seconds
  Expected   : Errors / timeouts as semaphore queue fills up

Phase 3 — Flash sale (async endpoint):
  User class : AsyncOrderUser
  Users      : 20
  Spawn rate : 10 users/sec
  Duration   : 60 seconds
  Expected   : 100% 202 Accepted, <100ms response time
----------------------------------------------------------------------
"""

import random
import json
from locust import HttpUser, task, between


def random_order_payload() -> dict:
    return {
        "customer_id": random.randint(1, 10000),
        "items": [
            {
                "product_id": f"PROD-{random.randint(1, 500)}",
                "quantity": random.randint(1, 5),
                "price": round(random.uniform(9.99, 199.99), 2),
            }
        ],
    }


class SyncOrderUser(HttpUser):
    """
    Simulates a customer placing orders via the synchronous endpoint.

    Normal ops  : 5 users, 1/sec spawn, 30s
    Flash sale  : 20 users, 10/sec spawn, 60s
    """

    wait_time = between(0.1, 0.5)  # 100–500ms between requests per user

    @task
    def place_sync_order(self):
        with self.client.post(
            "/orders/sync",
            json=random_order_payload(),
            catch_response=True,
            timeout=30,  # generous timeout so Locust records the full delay
        ) as resp:
            if resp.status_code == 200:
                resp.success()
            else:
                resp.failure(f"Expected 200, got {resp.status_code}: {resp.text[:100]}")


class AsyncOrderUser(HttpUser):
    """
    Simulates a customer placing orders via the asynchronous endpoint.

    Flash sale  : 20 users, 10/sec spawn, 60s
    Expected    : 100% 202 Accepted, sub-100ms latency
    """

    wait_time = between(0.1, 0.5)

    @task
    def place_async_order(self):
        with self.client.post(
            "/orders/async",
            json=random_order_payload(),
            catch_response=True,
        ) as resp:
            if resp.status_code == 202:
                resp.success()
            else:
                resp.failure(f"Expected 202, got {resp.status_code}: {resp.text[:100]}")
