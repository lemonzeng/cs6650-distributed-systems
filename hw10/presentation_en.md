# HW10 Presentation Script — English

---

## PART 1: Overview — Purpose & Code Structure

### Opening

> "This assignment is a hands-on implementation of the CAP theorem. We built two flavors of an in-memory distributed Key-Value store — a Leader-Follower database and a Leaderless database — and load-tested them under four different read/write ratios to observe exactly how consistency and latency trade off against each other."

### What we built

**Two services, both expose the same two-endpoint API:**
- `POST /set` — write a key-value pair, returns `{"version": N}` and 201-Created
- `GET /get?key=K` — read a value, returns 200 or 404

**Leader-Follower (`leader-follower/`):**
- 1 Leader + 4 Followers, N = 5 nodes total
- Three configurations: W=5 R=1 / W=1 R=5 / W=3 R=3
- Deployed via `docker-compose.yml` (and overlay files for each W/R config)

**Leaderless (`leaderless/`):**
- 5 equal peers, any node can be write coordinator
- Fixed at W=N=5, R=1
- Deployed via `docker-compose.leaderless.yml`

**Load Tester (`load-tester/`):**
- Python script that generates mixed read/write traffic with a controlled key pool
- Records latency for every request, detects stale reads by comparing versions
- Produces CSV results and 4-panel PNG plots

**Tests (`tests/`):**
- Unit tests (pytest) that prove consistency behavior under each configuration

### Why artificial delays?

The assignment requires:
- Leader sleeps **200 ms** after pushing to each follower
- Follower sleeps **100 ms** on receiving an update
- Follower sleeps **50 ms** on an internal read request

This widens the consistency window from microseconds to seconds, making it easy to observe and measure stale reads during load testing.

---

## PART 2: Key Code Walkthrough

### 2.1 The Write Path — `leaderSet` (leader-follower)

**File:** `leader-follower/main.go`

```
Line 145 — func leaderSet(...)
Line 152 — version := atomic.AddInt64(&versionCounter, 1)
Line 153 — localWrite(req.Key, req.Value, version)
Line 155 — acks := 1  // self counts as ack #1
```

The leader immediately writes to its own store and counts itself as ack #1. Then:

```
Line 158 — if wQuorum == 1 {
Line 160 —     remaining = followerURLs   // W=1: ALL followers go async
            } else {
Line 163 —     for _, furl := range followerURLs {
Line 164 —         if acks >= wQuorum { remaining = append(...); continue }
Line 168 —         err := postUpdate(furl, ...)   // synchronous follower push
Line 169 —         time.Sleep(200 * time.Millisecond)  // 200ms leader sleep
Line 171 —         if err == nil { acks++ }
            }
```

**Key insight:** W=1 short-circuits the loop entirely — no follower is contacted synchronously. W=3 contacts followers one at a time until it has 3 total acks (itself + 2), then stops. W=5 contacts all 4 followers synchronously.

```
Line 177 — for _, furl := range remaining {
Line 178 —     go func(u string) { postUpdate(u, ...) }(furl)   // async goroutines
```

Any follower not needed for the write quorum gets updated in a background goroutine. This is the source of the consistency window in W=1 and the residual staleness in W=3.

**Timing consequence:**
- W=5: 4 hops × (200 + 100) ms = **1,200 ms** minimum
- W=3: 2 hops × 300 ms = **600 ms** minimum
- W=1: 0 hops = **~2 ms** (local + HTTP only)

---

### 2.2 The Read Path — `leaderGet`

**File:** `leader-follower/main.go`

```
Line 186 — func leaderGet(...)
Line 193 — local, ok := localRead(key)

Line 195 — if rQuorum == 1 {
Line 200 —     writeJSON(w, http.StatusOK, local)   // R=1: return local instantly
Line 201 —     return
```

R=1 is a pure local lookup — no network, no coordination. This is why W=5 R=1 reads are ~2 ms.

For R > 1:

```
Line 209 — needed := rQuorum - 1   // contact R-1 followers in parallel
Line 214 — for i := 0; i < needed; i++ {
Line 215 —     go func(u string) {
Line 216 —         e, err := getInternalRead(u, key)   // hits /internal/read (50ms sleep)
Line 220 —         ch <- result{entry: e, ok: true}
```

```
Line 225 — best := local
Line 229 — if res.entry.Version > best.Version {
Line 230 —     best = res.entry    // pick highest version across all responses
```

**Quorum guarantee:** For W=3 R=3, W+R = 6 > N = 5, so the write set and read set always share at least one node. That shared node has the latest value, so `best` will always be the current version.

**Important:** The load tester reads from follower `/get` ports directly — it bypasses `leaderGet` entirely. This is by design: to measure "raw" follower staleness, not the quorum-corrected view.

---

### 2.3 The Follower Handlers — three distinct endpoints

**File:** `leader-follower/main.go`

```
Line 246 — func followerInternalUpdate(...)   // called by leader during write propagation
Line 252 —     time.Sleep(100 * time.Millisecond)   // 100ms follower update delay
Line 253 —     localWrite(req.Key, req.Value, req.Version)
```

```
Line 257 — func followerInternalRead(...)   // called by leader's quorum read (R > 1)
Line 263 —     time.Sleep(50 * time.Millisecond)   // 50ms follower read delay
Line 264 —     e, ok := localRead(key)
```

```
Line 273 — func followerGet(...)   // direct client read — NO sleep, NO coordination
Line 280 —     e, ok := localRead(key)
Line 284 —     writeJSON(w, http.StatusOK, e)
```

Three endpoints, three different behaviors:
- `/internal/update` — write propagation, sleeps 100 ms (simulates storage write)
- `/internal/read` — quorum read by leader, sleeps 50 ms (simulates storage read)
- `/get` — direct client read, instant (no delays) — **this is what the load tester uses**

---

### 2.4 Leaderless Write Path — `handleSet`

**File:** `leaderless/main.go`

```
Line 118 — func handleSet(...)
Line 125 —     version := time.Now().UnixNano()   // timestamp as version — no central counter needed
Line 126 —     localWrite(req.Key, req.Value, version)

Line 129 —     for _, purl := range peerURLs {
Line 130 —         postUpdate(purl, req.Key, req.Value, version)
Line 131 —         time.Sleep(200 * time.Millisecond)   // same 200ms delay as leader-follower
```

The critical architectural difference: the leaderless service uses `time.Now().UnixNano()` as the version, not a centralized atomic counter. This means any node can generate a version number independently — there's no single leader needed to serialize writes.

**Last-write-wins in `localWrite`:**

```
Line 80  — func localWrite(key, value string, version int64) {
Line 84  —     if !ok || version > cur.Version {   // only update if newer
Line 85  —         store[key] = Entry{Value: value, Version: version}
```

If two concurrent writes race and arrive out of order, the node always keeps the one with the larger nanosecond timestamp. This prevents a slower network from overwriting a more recent write.

---

### 2.5 Load Tester — Paired Reads & Staleness Detection

**File:** `load-tester/load_test.py`

**Key pool:**
```
Line 96 — KEY_POOL = [f"key-{i:04d}" for i in range(100)]
```
100 keys for 1,000 requests means each key is touched ~10 times, guaranteeing temporal overlap.

**Write and version tracking:**
```
Line 109 — r = requests.post(f"{write_url}/set", json={"key": key, "value": value}, ...)
Line 116 — if r.status_code == 201:
Line 122 —     known[key] = version   # remember the version the server assigned
```

**Paired read queue — the local-in-time mechanism:**
```
Line 232 — if rec["ok"] and key in known:
Line 233 —     paired_reads.append({
Line 235 —         "expected_version": known[key],
Line 236 —         "write_time":       time.time(),   # timestamp when write ACK returned
```
```
Line 242 — if paired_reads:
Line 244 —     item = paired_reads.popleft()   # consume the exact same key
Line 245 —     rec = do_read(read_url, item["key"], item["expected_version"], item["write_time"])
```

Every write pushes one entry onto `paired_reads`. The very next read pops it, guaranteeing the read targets the exact same key moments after the write.

**Write→Read gap:**
```
Line 150 — gap_ms = round((time.time() - write_time) * 1000, 3)
```
This is the interval between when the write ACK returned and when the paired read starts. For W=1 it's ~2 ms; for W=5 it's ~1,230 ms.

**Staleness detection:**
```
Line 159 — if r.status_code == 200 and expected_version is not None:
Line 161 —     returned_version = r.json().get("version", -1)
Line 162 —     if returned_version < expected_version:
Line 163 —         stale = True
Line 166 — elif r.status_code == 404 and expected_version is not None:
Line 168 —     stale = True   # key was written, but follower returns 404 → stale
```

---

## PART 3: Walking Through the Report

> Open `report.md` now. Walk through it section by section.

---

### → Section 1: Load Test Design

> "Before looking at any numbers, let me quickly explain how we generated the data."

Point to the two paragraphs:
- **Paired-reads queue**: every write pushes the key onto a deque; the next read pops it. This guarantees reads and writes cluster on the same key at nearly the same time — that's what makes stale reads observable.
- **Staleness detection**: the server returns a version number on every write. We store it in `known[key]`. If a follower returns a lower version (or 404), we count it as stale. Reads always go to follower ports, never to the leader, so the leader's quorum logic can't hide any inconsistency.

---

### → Section 2: Delay Model

> "Before looking at the results, I want to set up your expectations with a simple calculation."

Point to the **second table** (Write latency floor):

> "Every write latency number you'll see comes directly from this formula. W=5 contacts 4 followers synchronously — 4 × 300 ms = 1,200 ms minimum. W=3 contacts 2 — 600 ms. W=1 contacts none — about 2 ms. These are hard floors, not averages. That's why every write histogram in the graphs is a sharp spike with almost no spread."

---

### → Section 3: Summary Table

> "This table is the core result. Everything else in the report explains why these numbers look the way they do."

Scroll through the table and highlight three things:

**1. Write latency column — scan it vertically.**
> "W=5 and Leaderless are always ~1,230 ms, W=3 always ~616 ms, W=1 always ~2 ms. The ratio has zero effect on write latency. Changing from 1% writes to 90% writes doesn't change what each write costs — it only changes how often you pay."

**2. Read latency column — notice the slight increase.**
> "W=5 reads go from 2.0 ms at 1% writes to 7.1 ms at 90% writes. This isn't a consistency problem. It's lock contention — more concurrent writers competing for the in-memory store."

**3. Stale % column — point to the three highlighted cells.**
> "These are the interesting ones. Look at W=1 R=5. Staleness is 1.8% at 1% writes, jumps to 88.7% at 50/50, then *drops* to 6.6% at 90% writes. That non-monotonic pattern is the key insight: it's not about *how many* writes there are, it's about whether a read arrives inside the ~1,200 ms propagation window. At 50/50, writes arrive every ~2 ms, propagation can never keep up, so almost every paired read hits a stale follower. At 90% writes, reads are rare enough that they mostly land after propagation has already finished."

---

### → Section 4: Results by Read/Write Ratio

> "Now let's look at the graphs. Each graph overlays the latency CDF of all four configs at once — that's why these summary views are more useful than showing each config separately."

**W=1% R=99% graph:**
> "With 99% reads, write latency is paid on only 10 out of 1,000 requests. W=5 and Leaderless win here — 0% stale, 2 ms reads. The write cost is nearly invisible at this ratio."

**W=10% R=90% graph:**
> "Now writes are 10% — 100 requests each costing 1,200 ms for W=5. W=3 R=3 cuts that to 600 ms and keeps staleness at 5.1%. W=1 R=5 shows 14.5% stale — that's borderline. W=3 R=3 is the pragmatic choice here."

**W=50% R=50% graph:**
> "This is where it gets clear. W=5 and Leaderless have a massive step in the CDF at ~1,200 ms — half the requests cost over a second. W=1 R=5 is fast but 88.7% stale — almost useless for read-your-own-writes. W=3 R=3 is the only config with both reasonable write cost and near-zero staleness. It wins cleanly."

**W=90% R=10% graph:**
> "At 90% writes, write latency *is* your throughput. W=5 would spend 900 × 1,200 ms = 18 minutes just waiting. W=1 spends 900 × 2 ms = 1.8 seconds. The 6.6% stale reads are acceptable because reads are rare and the propagation window has usually closed by the time they happen. W=1 R=5 wins here by 600×."

---

### → Section 5: Application Mapping

> "The table summarizes where you'd actually deploy each configuration."

Point to each row:
- **W=5 R=1** → banking, medical — writes are rare, correctness is everything
- **W=1 R=5** → IoT, social feeds — you need to ingest millions of writes; brief staleness is fine, but never use this where users immediately read back what they wrote
- **W=3 R=3** → shopping cart, sessions — general purpose, W+R > N gives you quorum overlap so the leader path always returns fresh data
- **Leaderless** → distributed config, geo-distributed systems — no single-leader bottleneck, any node can coordinate, but all N nodes must be reachable for every write

> "The big takeaway: there's no free lunch. The artificial sleeps in our code exaggerate the numbers, but the shape of the trade-off is exactly what you'd see in DynamoDB, Cassandra, or Riak. The first question any engineer should ask when picking a database is: what's your read/write ratio, and how much staleness can you tolerate?"
