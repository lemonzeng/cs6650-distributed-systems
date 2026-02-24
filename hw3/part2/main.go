package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// --- 1. Atomicity Experiment (Updated with atomic.Uint64) ---
func runAtomicExperiment() {
	fmt.Println("\n--- Running Atomicity Experiment ---")
	var ops atomic.Uint64 // Atomic counter (safe)
	var opsRegular uint64 // Regular counter (unsafe)
	// var wg sync.WaitGroup // WaitGroup to synchronize goroutines

	// Using the latest 'for range count' syntax (Go 1.22+)
	for range 50 {
		// wg.Add(1)
		go func() {
			// defer wg.Done()
			for range 1000 {
				// uses hardware-level synchronization to ensure correctness
				ops.Add(1)
				// the regular integer fails due to a race condition where
				// concurrent read-modify-write operations overlap
				opsRegular++
			}
		}()
	}

	// Wait for all goroutines to finish
	// wg.Wait()

	fmt.Println("Expected Value: 50000")
	fmt.Println("Atomic ops:    ", ops.Load())
	fmt.Println("Regular ops:   ", opsRegular)
	if opsRegular != 50000 {
		fmt.Println("Result: Race condition detected in regular counter!")
	}
}

// --- 2. Collections & Locks Experiments ---
func runMapExperiment(mode string) {
	fmt.Printf("\n--- Running Map Experiment: %s ---\n", mode)
	var m_plain map[int]int
	var m_sync sync.Map

	type MutexMap struct {
		sync.Mutex
		data map[int]int
	}
	type RWMutexMap struct {
		sync.RWMutex
		data map[int]int
	}

	muMap := MutexMap{data: make(map[int]int)}
	rwMap := RWMutexMap{data: make(map[int]int)}

	if mode == "plain" {
		m_plain = make(map[int]int)
	}

	var wg sync.WaitGroup
	start := time.Now()

	for g := 0; g < 50; g++ {
		wg.Add(1)
		// Use gID to avoid closure capture issues in goroutines
		// In Go, if I use the loop variable g directly inside a
		// goroutine, all goroutines might end up seeing the same final value of g due to closure capture.
		go func(gID int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				// Generate a unique key for each goroutine and iteration to minimize contention
				key := gID*1000 + i
				val := i
				switch mode {
				//When running the experiment in plain mode, the program consistently crashes with
				// a fatal error: concurrent map writes.This is because Go's built-in map is not
				// thread-safe. Even though each worker was writing to a unique key
				// (no logical collision), the internal structure of the map (like
				// hash buckets and pointers) cannot handle multiple simultaneous updates.
				// This 'fail-fast' behavior of Go forced me to implement synchronization using
				// a Mutex to protect the shared state.
				case "plain":
					m_plain[key] = val

				case "mutex":
					muMap.Lock()
					muMap.data[key] = val
					muMap.Unlock()
				case "rwmutex":
					rwMap.Lock()
					rwMap.data[key] = val
					rwMap.Unlock()
				case "syncmap":
					m_sync.Store(key, val)
				}
			}
		}(g)
	}

	wg.Wait()
	duration := time.Since(start)

	length := 0
	switch mode {
	case "plain":
		length = len(m_plain)
	case "mutex":
		length = len(muMap.data)
	case "rwmutex":
		length = len(rwMap.data)
	case "syncmap":
		m_sync.Range(func(_, _ interface{}) bool {
			length++
			return true
		})
	}
	fmt.Printf("Mode: %s | Length: %d | Elapsed Time: %v\n", mode, length, duration)
}

// --- 3. File Access Experiment ---
func runFileExperiment() {
	fmt.Println("\n--- Running File Access Experiment ---")
	lines := 100000
	data := []byte("test-line-data\n")

	// Unbuffered: High system call overhead
	f1, _ := os.Create("unbuffered.txt")
	start := time.Now()
	for i := 0; i < lines; i++ {
		f1.Write(data)
	}
	f1.Close()
	fmt.Printf("Unbuffered Write: %v\n", time.Since(start))

	// Buffered: Aggregated writes via memory buffer
	f2, _ := os.Create("buffered.txt")
	w := bufio.NewWriter(f2)
	start = time.Now()
	for i := 0; i < lines; i++ {
		w.Write(data)
	}
	w.Flush()
	f2.Close()
	fmt.Printf("Buffered Write:   %v\n", time.Since(start))
}

// --- 4. Context Switching Experiment ---
func runContextSwitch(procs int) {
	runtime.GOMAXPROCS(procs)
	iterations := 1000000
	ch1 := make(chan struct{})
	ch2 := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	start := time.Now()
	go func() {
		defer wg.Done()
		// struct{} is a zero-size type, so sending it doesn't 
		// involve copying data, making it ideal for signaling.

		//The arrow operator <- handles the synchronization. 
		// When a goroutine tries to send on a channel, 
		// it will block until another goroutine is ready to 
		// receive from that channel, and vice versa. 
		// This ensures that the two goroutines alternate their execution, 
		// effectively measuring the cost of context switching between them.
		for i := 0; i < iterations; i++ {
			ch1 <- struct{}{}
			<-ch2
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			<-ch1
			ch2 <- struct{}{}
		}
	}()
	wg.Wait()
	duration := time.Since(start)
	avg := time.Duration(int64(duration) / int64(iterations*2))
	fmt.Printf("GOMAXPROCS(%d): Total Duration: %v | Avg Switch Cost: %v\n", procs, duration, avg)
}

func main() {
	// --- Crash & Race Tests ---
	// runAtomicExperiment()
	// runMapExperiment("plain")

	// ---  Performance Tests ---
	// runMapExperiment("mutex")
	// runMapExperiment("rwmutex")
	// runMapExperiment("syncmap")

	// runFileExperiment()

	// fmt.Println("\n--- Context Switching ---")
	runContextSwitch(1)                // User-level scheduling
	runContextSwitch(runtime.NumCPU()) // Multi-threaded OS scheduling
}
