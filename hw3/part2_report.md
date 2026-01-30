## Atomic Operations Experiment Results

### Results from Multiple Runs:

Run the code several times:

```bash
go run atomic-counters.go
go run atomic-counters.go
go run atomic-counters.go
```

**Example outputs:**

- Run 1: ops: 50000, opsRegular: 18959
- Run 2: ops: 50000, opsRegular: 21140
- Run 3: ops: 50000, opsRegular: 19919
- Run 4: ops: 50000, opsRegular: 19496

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

**Output:** Shows warnings like:

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
