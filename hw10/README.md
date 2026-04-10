# Distributed Key-Value Store — hw10

A pair of distributed KV services exploring the CAP theorem and consistency windows.

---

## Project Structure

```
hw10/
├── leader-follower/        Go server (leader or follower via ROLE env var)
├── leaderless/             Go server (all nodes equal)
├── tests/                  Python unit tests
├── load-tester/            Python load tester + plot generator
├── docker-compose.yml              W=5 R=1 (default)
├── docker-compose.w1r5.yml         W=1 R=5 override
├── docker-compose.w3r3.yml         W=3 R=3 override
└── docker-compose.leaderless.yml   Leaderless 5 nodes
```

---

## Quick Start

### Build images

```bash
# Leader-follower image
cd leader-follower && docker build -t lf-server .
# Leaderless image
cd leaderless && docker build -t ll-server .
```

---

## Leader-Follower Service

### Architecture

1 leader + 4 followers.  The leader owns all writes and coordinates reads.
Followers hold replicas updated by the leader.

```
Client → Leader (POST /set, GET /get)
Leader → Follower1..4 (POST /internal/update, GET /internal/read)
Client → Follower (GET /get or GET /local_read — direct local reads)
```

### Configurable N/W/R

| Variable | Description |
|----------|-------------|
| `W`      | Write quorum: number of nodes that must ACK before the leader responds |
| `R`      | Read quorum: number of nodes the leader queries; returns highest-version entry |

### CAP Trade-offs

**W=5 R=1** — strong write guarantee; all followers have the data before ACK.
Reads are cheap (leader local only), but the write path is slow (4×200 ms delays).

**W=1 R=5** — writes are instant (leader local only, followers updated async).
Reads are slower (collect from all 5) but always see the latest version.  High
staleness window for direct follower reads.

**W=3 R=3** — balanced quorum.  W+R > N ensures the read and write sets always
overlap by at least one node, guaranteeing strong consistency with lower latency
than W=5.

### Running

```bash
# W=5 R=1 (default)
docker compose up --build

# W=1 R=5
docker compose -f docker-compose.yml -f docker-compose.w1r5.yml up --build

# W=3 R=3
docker compose -f docker-compose.yml -f docker-compose.w3r3.yml up --build
```

Ports: leader=8001, followers=8002–8005.

### API

```
POST http://localhost:8001/set
     Content-Type: application/json
     {"key": "mykey", "value": "myval"}
     → 201 Created

GET  http://localhost:8001/get?key=mykey
     → 200 {"value":"myval","version":1}

GET  http://localhost:8002/local_read?key=mykey
     → 200 {"value":"myval","version":1}   (no coordination, may be stale)
```

---

## Leaderless Service

### Architecture

5 equal nodes.  Any node can be the write coordinator.  **W=N=5** (coordinator
syncs every peer before responding).  **R=1** (each node reads its own store).

Versions are Unix nanosecond timestamps — no central counter needed.
`/internal/update` uses last-write-wins (update only if incoming version > stored).

### Consistency Window

Because propagation is sequential (200 ms sleep per peer), a write to node1
takes ~800 ms to reach node5.  During that window, reads from non-coordinator
nodes return stale values.

### Running

```bash
docker compose -f docker-compose.leaderless.yml up --build
```

Ports: node1=8011, node2=8012, node3=8013, node4=8014, node5=8015.

### API

Same as leader-follower.  Point `/set` and `/get` at any node:

```bash
curl -X POST http://localhost:8011/set -d '{"key":"x","value":"hello"}' -H 'Content-Type: application/json'
curl http://localhost:8013/get?key=x
```

---

## Unit Tests

```bash
cd tests
pip install -r requirements.txt

# Leader-follower tests (cluster must be running)
pytest test_leader_follower.py -v

# Leaderless tests
pytest test_leaderless.py -v
```

Tests are grouped by W/R configuration.  The test file docstrings explain which
`docker compose` command is required for each test class.

---

## Load Tester

### Run

```bash
cd load-tester
pip install -r requirements.txt

# 1000 requests, 70% reads, config w5r1
python load_test.py --config w5r1 --reads 70 --writes 30 --requests 1000

# All four configs
for cfg in w5r1 w1r5 w3r3 leaderless; do
  python load_test.py --config $cfg --reads 70 --writes 30 --requests 1000
done
```

Results saved as `results/<config>_r<reads>w<writes>.csv`.

### Plot

```bash
# Single file
python plot_results.py results/w5r1_r70w30.csv

# Entire results directory (+ summary CDF overlay)
python plot_results.py results/
```

Each CSV generates a 4-panel figure:
- Latency histogram (reads vs writes, log-Y)
- Latency CDF (shows the long tail)
- Stale vs fresh read latency histogram
- Write→Read time gap for stale reads

If multiple CSVs are found, a summary CDF (`summary_cdf.png`) is also generated.

---

## Verifying Consistency Windows

The inconsistency window is most visible with W=1:

```bash
# Terminal 1: start W=1 R=5 cluster
docker compose -f docker-compose.yml -f docker-compose.w1r5.yml up --build

# Terminal 2: write, then immediately local_read a follower
curl -s -X POST http://localhost:8001/set \
  -d '{"key":"demo","value":"fresh"}' -H 'Content-Type: application/json' &
curl -s http://localhost:8002/local_read?key=demo   # likely 404 (stale)
```

With W=5 the leader blocks until all followers ACK, so the same experiment
will show consistent reads.
