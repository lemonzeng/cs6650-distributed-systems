━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Part II — Sync vs Async + Worker Scaling
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Phase 1 (sync, 5 users, normal ops):
  → 38 requests, 0 failures (100%), avg 3,066ms ✅

  Phase 2 (sync, 20 users, flash sale):
  → 95 requests, avg 10,746ms (median 12s) ⚠️
  → Max throughput: 5 slots ÷ 3s = 1.67 orders/sec
  → Flash sale demand: 57/sec → ~55.4 orders/sec cannot be served

  Phase 3 (async, 20 users, flash sale):
  → 1,550 requests, 0 failures (100%), avg 53ms ✅
  → 16x more orders than sync, 202x faster response time

  Phase 4 (1 worker, queue buildup):
  → Peak queue depth: ~5,225 messages
  → Drain time with 1 worker (0.33/sec): ~261 minutes (never reached 0)
  → Queue growth rate: ~56.7 msg/sec

  Phase 5 (worker scaling, same 60s flash sale each):

  Workers | Throughput  | Peak Queue | Drain Time | CPU    | Memory
  --------|-------------|------------|------------|--------|-------
  1       | 0.33/sec    | ~5,225     | ~261 min   | ~0%    | ~1.5%
  5       | 1.67/sec    | ~10,204    | ~102 min   | ~0%    | ~1.6%
  20      | 6.67/sec    | ~12,728    | ~31 min    | ~0%    | ~1.7%
  100     | 33.3/sec    | ~7,523     | ~2 min ✅  | ~12.7% | ~1.7%

  Minimum workers to match 57 orders/sec: ~171 goroutines (57 × 3s = 171)
  At 100 workers: queue drained from 7,523 → 161 in ~2 minutes.
  CPU only 12.7% at 100 workers — bottleneck is I/O, not compute.

  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Part III — SNS → Lambda (Serverless)
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Runtime: provided.al2 (Go binary), 512MB memory, direct SNS trigger
  Sent 10 test orders via /orders/async, observed CloudWatch logs:

    REPORT ...  Duration: 3002ms  Init Duration: 77.63ms  ← COLD START (1st call)
    REPORT ...  Duration: 3001ms                          ← WARM
    REPORT ...  Duration: 3001ms                          ← WARM
    REPORT ...  Duration: 3002ms                          ← WARM

  Cold start init:     77.63ms
  Warm execution:      ~3,001ms
  Cold start overhead: ~2.6%  (negligible on 3s processing)
  Memory used:         20MB / 512MB (3.9% of allocation)
  Cold starts occur:   first invocation only, or after ~5min idle

  Cost comparison:
    ECS workers (2 tasks, always on): ~$17/month
    Lambda (10K orders/month):         $0  (free tier)
    Lambda (267K orders/month):        $0  (free tier limit)
    Lambda break-even vs ECS:          ~1.7M orders/month

  Recommendation:
  Switch to Lambda. 2.6% cold start overhead is negligible for 3s payment processing. Zero operational burden (no queue monitoring, no worker scaling, no ECS health checks). Cost is $0 within free tier (~267K orders/month). Trade-off: SNS only retries Lambda twice on failure vs SQS's configurable retry/DLQ. Acceptable for a startup — reintroduce SQS only when order durability becomes critical at scale.
