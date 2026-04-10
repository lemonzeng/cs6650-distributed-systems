"""
Unit tests for the leaderless KV service.

Start the cluster first:
    docker compose -f docker-compose.leaderless.yml up --build -d

All five nodes are equal; any node can accept a write.
W=N=5 (write coordinator syncs all peers sequentially).
R=1 (each node reads its own local store).
"""

import threading
import time
import uuid

import pytest
import requests

NODES = [
    "http://localhost:8011",
    "http://localhost:8012",
    "http://localhost:8013",
    "http://localhost:8014",
    "http://localhost:8015",
]

TIMEOUT = 10  # leaderless W=N writes are slower (4 peers × ~300 ms each)


def unique_key(prefix="k") -> str:
    return f"{prefix}-{uuid.uuid4().hex[:8]}"


def node_set(base_url: str, key: str, value: str) -> requests.Response:
    return requests.post(f"{base_url}/set", json={"key": key, "value": value}, timeout=TIMEOUT)


def node_get(base_url: str, key: str) -> requests.Response:
    return requests.get(f"{base_url}/get", params={"key": key}, timeout=TIMEOUT)


def node_local_read(base_url: str, key: str) -> requests.Response:
    return requests.get(f"{base_url}/local_read", params={"key": key}, timeout=TIMEOUT)


class TestLeaderless:

    def test_write_to_coordinator_read_back_consistent(self):
        """
        Write to node1 and immediately read back from node1.
        Because W=N, the coordinator has already written locally before
        propagating — the read always sees the fresh value.
        """
        key = unique_key("ll-coord")
        value = "coord-value"

        resp = node_set(NODES[0], key, value)
        assert resp.status_code == 201, f"set failed: {resp.text}"

        resp = node_get(NODES[0], key)
        assert resp.status_code == 200, f"get failed: {resp.text}"
        assert resp.json()["value"] == value

    def test_after_write_all_nodes_consistent(self):
        """
        Once the coordinator returns 201, it has synced all peers (W=N).
        Every node must now hold the value.
        """
        key = unique_key("ll-all")
        value = "all-nodes-value"

        resp = node_set(NODES[0], key, value)
        assert resp.status_code == 201

        for node in NODES:
            resp = node_get(node, key)
            assert resp.status_code == 200, f"{node}: {resp.status_code}"
            assert resp.json()["value"] == value, (
                f"{node}: expected {value!r}, got {resp.json()['value']!r}"
            )

    def test_inconsistency_window_during_propagation(self):
        """
        Fire a write to node1 in a background thread.
        Immediately poll other nodes — they should be stale because the
        sequential propagation takes 200 ms per peer (4 × 200 ms = ~800 ms).

        After the coordinator ACKs, all nodes must be consistent.
        """
        key = unique_key("ll-window")
        value = "window-value"
        stale_reads = []
        write_done = threading.Event()

        def do_write():
            node_set(NODES[0], key, value)
            write_done.set()

        writer = threading.Thread(target=do_write)
        writer.start()

        # Poll other nodes while write is in flight
        deadline = time.time() + 3.0  # poll for up to 3 seconds
        while not write_done.is_set() and time.time() < deadline:
            for node in NODES[1:]:
                try:
                    r = node_local_read(node, key)
                    if r.status_code == 404:
                        stale_reads.append((node, "404"))
                    elif r.json().get("value") != value:
                        stale_reads.append((node, r.json().get("value")))
                except Exception:
                    pass
            time.sleep(0.02)

        writer.join()

        assert len(stale_reads) > 0, (
            "Expected to observe stale reads on non-coordinator nodes during "
            "write propagation, but all reads were already consistent. "
            "The inconsistency window exists because peers are updated "
            "sequentially with 200 ms delays."
        )

        # After write completes, all nodes must be consistent
        for node in NODES:
            resp = node_get(node, key)
            assert resp.status_code == 200, f"post-write: {node} returned {resp.status_code}"
            assert resp.json()["value"] == value, (
                f"post-write: {node} has stale value {resp.json()['value']!r}"
            )

    def test_any_node_can_be_coordinator(self):
        """Every node should accept writes and propagate to all peers."""
        for i, node in enumerate(NODES):
            key = unique_key(f"ll-coord{i}")
            value = f"from-node{i+1}"
            resp = node_set(node, key, value)
            assert resp.status_code == 201, f"{node} rejected write: {resp.text}"

            # Read back from a different node
            other = NODES[(i + 1) % len(NODES)]
            resp = node_get(other, key)
            assert resp.status_code == 200
            assert resp.json()["value"] == value

    def test_last_write_wins_on_concurrent_writes(self):
        """
        Write the same key via two different coordinators rapidly.
        Because versions are nanosecond timestamps and updates only apply when
        newer, the last writer wins — no lost-write anomaly.
        """
        key = unique_key("ll-lww")
        results = {}

        def write_node(idx, val):
            r = node_set(NODES[idx], key, val)
            results[idx] = (val, time.time(), r.status_code)

        t1 = threading.Thread(target=write_node, args=(0, "from-node1"))
        t2 = threading.Thread(target=write_node, args=(2, "from-node3"))
        t1.start()
        t2.start()
        t1.join()
        t2.join()

        # Both writes succeeded
        for idx, (val, ts, code) in results.items():
            assert code == 201, f"node{idx+1} write failed"

        # After both finish, every node should have *one* of the two values
        seen_values = set()
        for node in NODES:
            r = node_get(node, key)
            assert r.status_code == 200
            seen_values.add(r.json()["value"])

        # All nodes must agree (eventual consistency)
        assert len(seen_values) == 1, (
            f"Nodes disagree after concurrent writes: {seen_values}"
        )

    def test_missing_key_returns_404(self):
        for node in NODES:
            resp = node_get(node, unique_key("ll-missing"))
            assert resp.status_code == 404, f"{node}: expected 404"
