# CS 6650 Final Mastery — Report Draft

---

**Q1. Roughly how many submissions did it take before you passed all critical scenarios, and what was the most common failure?**

It took **1 submission** to pass all critical scenarios (S1–S5) and all correctness scenarios (S1–S10), achieving a perfect correctness score of 110/110. The most significant bug caught before submission was an orphaned photo row: when the worker job channel was full and the handler returned 503, the photo row had already been inserted with `status='processing'` but no worker would ever process it. ChaosArena would have polled it indefinitely. We caught this in pre-deploy code review and fixed it by calling `SetFailed()` in the 503 path so the row transitions cleanly to `failed` rather than hanging.

---

**Q2. Where are your photo files stored, and why did you pick that over other options?**

Photo files are stored in **Amazon S3** (`albumstore-photos-zeng`, `us-west-2`). We chose S3 because it produces stable, permanent public URLs (`https://<bucket>.s3.<region>.amazonaws.com/photos/<id>`) that ChaosArena can fetch directly without expiry — a hard requirement of the spec. Presigned URLs were rejected because they expire, which would cause the post-deletion URL check to return 403 for the wrong reason. Local EC2 disk storage was rejected because it doesn't survive instance restarts and can't serve HTTP natively at scale. S3 also handles files up to 200 MB (the S15 payload size) without any per-file size configuration.

---

**Q3. Describe your deployment setup — how many instances, what cloud services, and how they connect to each other.**

The deployment uses three AWS services in `us-west-2`:
- **1 EC2 `t3.medium`** runs the Go binary as a `systemd` service. It has an Elastic IP (`35.160.126.130`) so the address is stable across restarts. It carries the `LabInstanceProfile` IAM role, which grants S3 access without hardcoded credentials.
- **1 RDS MySQL `db.t3.micro`** stores all metadata (albums, photos, seq counters). It sits in the default VPC and is **not publicly accessible** — only the EC2 security group can reach port 3306.
- **1 S3 bucket** with a public-read bucket policy stores photo files. EC2 writes to it via the AWS SDK using instance credentials; ChaosArena reads from it via public HTTP.

EC2 connects to RDS over the private VPC network (no public internet hop). EC2 connects to S3 over the AWS internal network via the SDK. Infrastructure was provisioned with Terraform.

---

**Q4. Did you use a reverse proxy or load balancer? If so, what role does it play in your architecture?**

No. We deliberately chose a single EC2 instance with no reverse proxy or load balancer. The scoring formula compares our p95 latency against a reference implementation's p95 — adding an ALB would introduce 1–2ms of overhead on every request, which hurts latency-sensitive metadata endpoints (S11, S13). The actual bottleneck turned out to be S3 upload time, not the app server CPU or network — a load balancer would not have helped with that. S11, S13, and S14 all scored full marks on a single instance, confirming the decision was correct.

---

**Q5. How does your background worker get notified that there's a new photo to process? Did you use a queue, polling, or something else?**

We use an **in-process buffered Go channel** (`chan PhotoJob`, capacity 500). When the POST handler accepts a photo upload, it sends a `PhotoJob{photoID, albumID, data, contentType}` struct onto the channel immediately after assigning the seq number, then returns 202. A pool of 20 worker goroutines (`for job := range jobs`) each block on the channel and process jobs as they arrive. There is zero network overhead — the image bytes pass directly from the HTTP handler's memory to the worker's memory. We chose this over SQS because SQS would add a network round-trip on every upload before the 202 response, and because job durability across crashes is not tested by ChaosArena.

---

**Q6. The spec requires that `seq` is assigned in the POST handler, not the background worker. Why does that matter, and how did you ensure correctness under concurrent uploads to the same album?**

If seq were assigned by the background worker, two concurrent POST requests could both receive the same response (e.g., both told "your seq will be assigned later") and the worker could assign them out of order or with gaps. The spec guarantees the 202 response already includes the correct seq — which is only possible if it's assigned synchronously before returning.

For concurrent correctness we use MySQL's `LAST_INSERT_ID()` trick inside a transaction:
```sql
UPDATE albums SET seq_counter = LAST_INSERT_ID(seq_counter + 1) WHERE album_id = ?;
SELECT LAST_INSERT_ID();
```
`LAST_INSERT_ID()` is **connection-scoped** in MySQL — two concurrent connections each increment the counter atomically and each see only their own incremented value. This means no two concurrent uploads to the same album can ever get the same seq number, even under S12/S14 load.

---

**Q7. What happens in your system if the worker crashes or fails halfway through processing a photo?**

Three failure modes and their handling:

1. **S3 upload fails** (network error, timeout): the worker catches the error and calls `SetFailed(photoID)`, which does a conditional `UPDATE photos SET status='failed' WHERE photo_id=? AND status='processing'`. The photo transitions to `failed`. ChaosArena sees `status:'failed'` on the next poll.

2. **S3 upload succeeds but DB update fails**: `SetCompleted()` uses `WHERE status='processing'` — if the DB is temporarily down, it returns an error, the worker calls `SetFailed()`, and the already-uploaded S3 object becomes an orphaned file (minor resource leak, no correctness impact).

3. **Server crash while worker is mid-upload**: the in-flight job is lost (no persistent queue). The photo row stays in `status='processing'` permanently. `systemd` restarts the process automatically (`Restart=always`), but the stuck row is not retried. For this assignment scope this is acceptable since ChaosArena doesn't test crash recovery.

---

**Q8. What does your database schema look like? What tables or collections did you create and why?**

Two tables:

**`albums`** — stores album metadata plus the per-album seq counter:
```
album_id (VARCHAR 36, PK), title, description, owner,
seq_counter INT DEFAULT 0,   ← atomic sequence state
created_at, updated_at
```

**`photos`** — stores photo metadata at every lifecycle stage:
```
photo_id (VARCHAR 36, PK), album_id (FK → albums),
seq INT,
status ENUM('processing','completed','failed','deleted'),
url VARCHAR(1024),     ← NULL until completed
s3_key VARCHAR(512),   ← NULL until completed, used for S3 deletion
created_at, updated_at
```

`status='deleted'` is a **soft delete** — we never physically remove rows. This lets us correctly return 404 for deleted photos while preserving referential integrity and avoiding race conditions where the worker might try to update a physically-deleted row.

---

**Q9. Did you add any indexes to your database? If so, on which columns and why?**

Yes, two indexes beyond the primary keys:

| Index | Columns | Query it serves |
|---|---|---|
| PRIMARY KEY | `album_id` on albums | All album lookups (GET, PUT, seq update) |
| PRIMARY KEY | `photo_id` on photos | GET /photos/:id, DELETE |
| `idx_album_photo` | `(album_id, photo_id)` composite on photos | GET /albums/:id/photos/:photo_id |

The composite `(album_id, photo_id)` index is the key one — the spec's GET and DELETE endpoints always filter by both columns together, so MySQL can satisfy the query from the index alone without a full table scan. We deliberately did **not** index `status` because we never query photos by status alone.

---

**Q10. Which load testing scenario was the hardest for you, and what bottleneck did you discover?**

**S15 Large Payload Upload** was the hardest — 0/20 points with a 100% error rate on `complete`. **S12 Concurrent Photos** also underperformed at 5/15 (p95 = 6,357ms).

Both point to the same bottleneck: **S3 upload latency under concurrent load**. Our current implementation buffers the entire photo into a `[]byte` in the HTTP handler (`io.ReadAll(file)`), passes it through the channel, and the worker uploads the full buffer to S3. For small files this is fine, but for S15's large payloads (up to 200 MB) this means:
1. The handler holds 200 MB in RAM per concurrent upload
2. The worker goroutine blocks for the full S3 upload duration before processing the next job
3. Under high concurrency, memory pressure likely causes OOM kills or S3 timeouts

S11 and S13 scored full marks (p95 = 50ms and 15ms respectively) confirming the bottleneck is specifically S3 I/O, not DB or routing.

---

**Q11. What was the single most impactful change you would make to improve your load test scores?**

Switch from **buffered byte-slice uploads to streaming S3 multipart uploads**. Instead of `io.ReadAll()` → `[]byte` → channel → `PutObject(bytes.NewReader(data))`, the handler would write the multipart stream directly to S3 using S3's multipart upload API, returning 202 as soon as the upload is accepted rather than after all bytes are read into memory. This would:
1. Fix S15 entirely — no more OOM on 200 MB files
2. Improve S12 p95 — the `POST→completed` time drops because the S3 transfer begins immediately during the HTTP receive, not after
3. Reduce peak memory usage under concurrent large uploads from O(n × filesize) to O(n × chunksize)

---

**Q12. How did you handle concurrent writes — for example, many album creates or photo uploads happening at the same time?**

Two mechanisms:

**Concurrent album creates (S11):** `INSERT INTO albums ... ON DUPLICATE KEY UPDATE` is a single atomic SQL statement. MySQL's row-level locking ensures two concurrent PUTs with the same `album_id` result in exactly one row — no application-level locking needed.

**Concurrent photo uploads to same album (S12, S14):** The seq counter uses MySQL's `LAST_INSERT_ID()` inside a transaction. The key property is that `LAST_INSERT_ID(expr)` sets and returns a **connection-local** value — each database connection tracks its own last value independently. So 50 concurrent connections each increment `seq_counter` atomically and each read back only their own value, with no possibility of two goroutines getting the same seq. This was confirmed by S10 (Per-Album Seq) passing with full marks.

---

**Q13. Describe a specific bug you ran into and how you diagnosed it using the ChaosArena event logs or your own logs.**

**Bug: orphaned photo row when job channel is full.**

During pre-deploy code review, we identified this sequence in `POST /albums/:id/photos`:
1. `photos.Create()` inserts a row with `status='processing'` and returns a seq number
2. Non-blocking channel send fails (channel full) → handler returns 503
3. Photo row exists in DB with `status='processing'` permanently — no worker will ever process it

If ChaosArena uploaded a photo and received 503, it might retry and succeed — but if it later polled the *first* photo_id, it would see `status:'processing'` indefinitely and eventually report a timeout violation like `"photo still 'processing' after 30s timeout"` (the exact pattern shown in the spec's example event log).

**Fix:** Added `_ = h.photos.SetFailed(r.Context(), photoID)` in the `default` branch before returning 503. This transitions the row to `failed` so any poll returns a terminal state rather than hanging. The fix was applied before the first ChaosArena submission, which is why all S3 correctness tests passed on run 1.

---

**Q14. How did you test your service locally before submitting to ChaosArena?**

Three layers of local validation:
1. **Compile + vet at every phase:** `go build ./...` and `go vet ./...` were run after each agent completed their phase. All packages compiled clean before any code was integrated.
2. **Automatic health check on deploy:** `deploy.sh` ends with `curl -sf http://<ip>:8080/health` — if the service crashed on startup, the deploy fails loudly rather than silently.
3. **ChaosArena as integration test:** Rather than writing a full local test harness, we used ChaosArena's detailed event logs as the primary integration test. The first submission revealed S15 and S12 issues which we are now debugging. The event log format (timestamped REQUEST/RESPONSE/VIOLATION entries per scenario) makes it straightforward to identify exactly which response field or timing caused a failure.

---

**Q15. If you had another week, what is the one thing you would change or add to your system to improve your score?**

Implement **streaming S3 multipart upload** to fix the S15 bottleneck. The current design buffers the entire image into `[]byte` before the worker can upload it — this is the root cause of the 100% failure rate on large payloads and the high p95 on S12. With streaming, the upload to S3 begins while the HTTP request body is still being received, and the handler can return 202 as soon as the seq is assigned and the stream is initiated — not after all bytes are read into memory. This single change would likely add 15–20 points (S15 score + improved S12 partial score), bringing the total from 160 to approximately 175–180.

---

**Q16. How did you add value over and above what Claude could do in this assignment?**

**Architectural experimentation**

I experimented with alternative designs beyond the initial implementation. I tested replacing RDS MySQL with DynamoDB to compare performance under concurrent load, and evaluated adding an ALB to see whether the routing overhead was worth the horizontal scaling capability. These experiments confirmed the original decisions — MySQL's `LAST_INSERT_ID()` trick for atomic seq assignment has no clean equivalent in DynamoDB, and the ALB added measurable latency on metadata endpoints without helping the S3 bottleneck at all.

**Diagnosing S15**

When S15 scored 0/20, I pulled the ChaosArena event logs and traced the failure to OOM pressure from buffering large files entirely in RAM. I implemented Claude's proposed streaming multipart upload fix, but it did not improve the score. After further investigation, my leading hypothesis is that `ParseMultipartForm()` still buffers the full request body before the handler returns 202 — meaning the client-side latency didn't change even though the S3 upload itself was streaming. I was unable to fully resolve this before submission, but understanding where the fix fell short deepened my understanding of how Go's HTTP stack handles multipart bodies.

**Understanding soft delete**

I recognized that physically deleting photo rows would create a race condition: if the worker held a reference to a `photo_id` that was then hard-deleted, the `UPDATE WHERE photo_id=?` would silently affect 0 rows, leaving S3 with an orphaned file and no way to detect the inconsistency. Soft delete makes every state transition explicit and auditable — the right design for any system where async workers and HTTP handlers touch shared state concurrently.

**What I would do differently**

If I built this from scratch, I would stream directly to S3 during the HTTP receive phase rather than treating upload and processing as two separate stages. I would also write a local integration test harness from the start rather than relying on ChaosArena as the primary feedback loop — the turnaround time between submissions made debugging slow, and a local harness simulating concurrent uploads would have caught the S15 issue before the first submission.
