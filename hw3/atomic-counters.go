package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {

	var ops atomic.Uint64 //atomic counter(safe)
	var opsRegular uint64 //regular counter(unsafe)
	var wg sync.WaitGroup

	for range 50 {
		wg.Go(func() {
			for range 1000 {
				ops.Add(1)
				opsRegular++
			}
		})
	}

	wg.Wait()

	fmt.Println("ops:", ops.Load())
	fmt.Println("opsRegular:", opsRegular)
}
