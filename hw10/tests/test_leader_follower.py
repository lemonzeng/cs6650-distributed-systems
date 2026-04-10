"""
Unit tests for the leader-follower KV service.

Assumes the cluster is already running via docker compose.

Default (W=5, R=1):
    docker compose up --build -d

W=1, R=5:
    docker compose -f docker-compose.yml -f docker-compose.w1r5.yml up --build -d

W=3, R=3:
    docker compose -f docker-compose.yml -f docker-compose.w3r3.yml up --build -d
"""

import threading
import time
import uuid

import pytest
import requests

LEADER = "http://localhost:8001"
FOLLOWERS = [
    "http://localhost:8002",
    "http://localhost:8003",
    "http://localhost:8004",
    "http://localhost:8005",
]

TIMEOUT = 5  # seconds per request


def unique_key(prefix="k") -> str:
    return f"{prefix}-{uuid.uuid4().hex[:8]}"


def leader_set(key: str, value: str) -> requests.Response:
    return requests.post(f"{LEADER}/set", json={"key": key, "value": value}, timeout=TIMEOUT)


def leader_get(key: str) -> requests.Response:
    return requests.get(f"{LEADER}/get", params={"key": key}, timeout=TIMEOUT)


def node_local_read(base_url: str, key: str) -> requests.Response:
    return requests.get(f"{base_url}/local_read", params={"key": key}, timeout=TIMEOUT)


def node_get(base_url: str, key: str) -> requests.Response:
    return requests.get(f"{base_url}/get", params={"key": key}, timeout=TIMEOUT)


# ---------------------------------------------------------------------------
# W=5 R=1  (default docker-compose.yml)
# ---------------------------------------------------------------------------

class TestW5R1:
    """
    W=5 means the leader waits for ACKs from all 4 followers before returning.
    Every read (R=1) from any node must see the latest value.
    """

    def test_write_then_leader_read_consistent(self):
        key = unique_key("w5r1-leader")
        value = "hello-leader"

        resp = leader_set(key, value)
        assert resp.status_code == 201, f"set failed: {resp.text}"

        resp = leader_get(key)
        assert resp.status_code == 200, f"get failed: {resp.text}"
        data = resp.json()
        assert data["value"] == value, f"expected {value!r}, got {data['value']!r}"

    def test_write_then_follower_read_consistent(self):
        """
        With W=5, the leader has already propagated to all followers before
        responding 201.  A direct follower GET must return the written value.
        """
        key = unique_key("w5r1-follower")
        value = "hello-follower"

        resp = leader_set(key, value)
        assert resp.status_code == 201

        for furl in FOLLOWERS:
            resp = node_get(furl, key)
            assert resp.status_code == 200, f"{furl} returned {resp.status_code}"
            data = resp.json()
            assert data["value"] == value, (
                f"{furl}: expected {value!r}, got {data['value']!r}"
            )

    def test_local_read_inconsistency_window(self):
        """
        Fire /set and immediately /local_read followers in a separate thread.
        Because the leader propagates sequentially (200 ms sleep between each
        follower), early followers may not yet have the new value — this
        demonstrates the inconsistency window.

        We run enough iterations that at least one stale read is observed.
        """
        stale_found = []

        for i in range(15):
            key = unique_key(f"w5r1-stale-{i}")
            value = f"val-{i}"
            results = {"stale": False}

            def do_reads(k=key, v=value, buf=results):
                for furl in FOLLOWERS:
                    try:
                        r = node_local_read(furl, k)
                        if r.status_code == 404:
                            buf["stale"] = True
                            return
                        if r.json().get("value") != v:
                            buf["stale"] = True
                            return
                    except Exception:
                        pass

            t = threading.Thread(target=do_reads)
            t.start()
            leader_set(key, value)  # triggers propagation
            t.join()

            if results["stale"]:
                stale_found.append(i)

        assert len(stale_found) > 0, (
            "Expected at least one stale local_read during W=5 propagation, "
            "but all reads were consistent.  The inconsistency window exists "
            "because updates are sequential with 200 ms delays."
        )

    def test_overwrite_and_read_latest(self):
        key = unique_key("w5r1-overwrite")
        for i in range(3):
            resp = leader_set(key, f"value-{i}")
            assert resp.status_code == 201

        resp = leader_get(key)
        assert resp.status_code == 200
        assert resp.json()["value"] == "value-2"

    def test_read_missing_key_returns_404(self):
        resp = leader_get(unique_key("w5r1-missing"))
        assert resp.status_code == 404


# ---------------------------------------------------------------------------
# W=1 R=5  (docker-compose.w1r5.yml override)
# ---------------------------------------------------------------------------

class TestW1R5:
    """
    W=1: leader ACKs after writing locally only; followers are updated async.
    R=5: leader reads from itself + all 4 followers, returns highest version.
    """

    def test_write_then_read_consistent(self):
        """
        Even though W=1, R=5 collects the value from all 5 nodes and returns
        the highest-version entry, so the client always sees the latest write.
        """
        key = unique_key("w1r5-consistent")
        value = "quorum-read-value"

        resp = leader_set(key, value)
        assert resp.status_code == 201

        resp = leader_get(key)
        assert resp.status_code == 200
        assert resp.json()["value"] == value

    def test_follower_local_read_likely_stale(self):
        """
        With W=1, follower updates are asynchronous.  A local_read immediately
        after the write ACK is very likely to see a stale (404 or old) value.
        """
        stale_found = []

        for i in range(15):
            key = unique_key(f"w1r5-stale-{i}")
            value = f"v{i}"

            leader_set(key, value)  # ACKs after local write only

            # Immediately local_read from each follower
            for furl in FOLLOWERS:
                try:
                    r = node_local_read(furl, key)
                    if r.status_code == 404 or r.json().get("value") != value:
                        stale_found.append((i, furl))
                        break
                except Exception:
                    pass

        assert len(stale_found) > 0, (
            "Expected stale local_reads after W=1 write, but all reads were fresh. "
            "This is unexpected — async propagation should create staleness."
        )

    def test_missing_key_returns_404(self):
        resp = leader_get(unique_key("w1r5-missing"))
        assert resp.status_code == 404


# ---------------------------------------------------------------------------
# W=3 R=3  (docker-compose.w3r3.yml override)
# ---------------------------------------------------------------------------

class TestW3R3:
    """
    W=3: leader waits for 2 follower ACKs (self + 2 = 3) before responding.
    R=3: leader reads from self + 2 followers, returns highest-version entry.
    """

    def test_quorum_read_consistent(self):
        """
        After a W=3 write, an R=3 read must return the latest value because
        the write and read quorums overlap (3+3 > 5).
        """
        key = unique_key("w3r3-quorum")
        value = "quorum-value"

        resp = leader_set(key, value)
        assert resp.status_code == 201

        resp = leader_get(key)
        assert resp.status_code == 200
        assert resp.json()["value"] == value

    def test_write_then_leader_read_version_increasing(self):
        key = unique_key("w3r3-version")
        versions = []
        for i in range(3):
            leader_set(key, f"v{i}")
            r = leader_get(key)
            assert r.status_code == 200
            versions.append(r.json()["version"])

        assert versions == sorted(versions), f"versions not monotonic: {versions}"

    def test_partial_followers_read_consistent(self):
        """
        At least R-1=2 followers get the write before ACK; reading from those
        followers directly must show the value.
        """
        key = unique_key("w3r3-partial")
        value = "partial-follower"

        resp = leader_set(key, value)
        assert resp.status_code == 201

        # At least 2 of the 4 followers must have it (W=3 wrote to 2)
        consistent = 0
        for furl in FOLLOWERS:
            r = node_get(furl, key)
            if r.status_code == 200 and r.json()["value"] == value:
                consistent += 1

        assert consistent >= 2, (
            f"Expected at least 2 followers consistent after W=3, got {consistent}"
        )

    def test_missing_key_returns_404(self):
        resp = leader_get(unique_key("w3r3-missing"))
        assert resp.status_code == 404
