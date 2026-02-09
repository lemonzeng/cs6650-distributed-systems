## 1. Atomic Operations Experiment Results

### Results from Multiple Runs:

Run the code several times:

```bash
go run atomic-counters.go
go run atomic-counters.go
go run atomic-counters.go
```

**Outputs:**
![alt text](image.png)
- Run 1: ops: 50000, opsRegular: 18357
- Run 2: ops: 50000, opsRegular: 17350
- Run 3: ops: 50000, opsRegular: 14507
- Run 4: ops: 50000, opsRegular: 17751
![alt text](image-2.png)

### What's Happening:

**Atomic counter (`ops`):** Consistently shows **50,000** (50 goroutines × 1000 increments)

- Uses `atomic.Uint64` with hardware-level synchronization
- All increments are preserved safely

**Regular counter (`opsRegular`):** Shows **random values** (much less than 50,000)

- Multiple goroutines increment concurrently without synchronization
- Many increments are **lost** due to race conditions
- Result varies each run because timing is unpredictable

### With -race Flag:

Run:

```bash
go run -race atomic-counters.go
```

**Output:** 

Shows warnings:
![alt text](image-1.png)

```
WARNING: DATA RACE
Write at 0x... by goroutine 6:
  main.main()
      atomic-counters.go:18
Previous write at 0x... by goroutine 7:
  main.main()
      atomic-counters.go:18
```

**What it does:**

- Detects concurrent access to `opsRegular` without synchronization
- Proves the race condition exists
- Demonstrates why `atomic.Uint64` is necessary

### Conclusion:

Atomic operations guarantee safe concurrent access; regular variables don't.


![alt text](image-3.png)

2. Shared Collections (Maps)

Observation:

Plain Map: Crashed with fatal error: concurrent map writes.

Reason: Go maps are not designed for concurrent safety to maximize single-threaded performance.

Locking Tradeoffs:
| Method | Result | Performance Analysis |
| :--- | :--- | :--- |
| Mutex | Stable | Good for general purpose. Provides strict serialization. |
| RWMutex | Stable | In this write-heavy test, it's similar to Mutex. Better for read-heavy loads. |
| Sync.Map | Stable | Optimized for specific scenarios (stable keys/many readers). Slightly slower for high-frequency writes. |

3. File Access (Persistence)

Results:

Unbuffered: [Insert Time]

Buffered: [Insert Time]

Conclusion: Buffered IO is significantly faster because it minimizes the number of System Calls. Writing to the kernel is expensive; bufio aggregates many small writes into one large batch.

4. Context Switching

Results:

Single Thread (GOMAXPROCS 1): [Insert Time]

Multi Thread: [Insert Time]

Analysis:
Goroutines are extremely lightweight. Switching between them on a single OS thread is faster as it avoids OS-level scheduling overhead. This efficiency is why Go can handle millions of goroutines compared to thousands of traditional OS threads.