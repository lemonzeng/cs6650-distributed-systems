package main

import (
	"fmt"
	"sync"
	"time"
)

// 练习 1: 基础读写对比
// 场景：3个读者和1个写者。
func exercise1() {
	var mu sync.RWMutex
	var wg sync.WaitGroup

	fmt.Println("--- Exercise 1 Start ---")
	start := time.Now()

	// 启动 3 个读者
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			mu.RLock() // 申请读锁
			fmt.Printf("读者 %d: 正在看书...\n", id)
			time.Sleep(1 * time.Second) // 模拟阅读耗时
			mu.RUnlock()
		}(i)
	}

	// 启动 1 个写者
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond) // 确保读者先拿到锁
		mu.Lock()                          // 申请写锁
		fmt.Println("写者: 正在写书 (静默)...")
		time.Sleep(1 * time.Second) // 模拟写入耗时
		mu.Unlock()
	}()

	wg.Wait()
	fmt.Printf("总耗时: %v\n", time.Since(start).Round(time.Second))
}

// 练习 2: 锁的排队
// 场景：写者先到，读者后到。
func exercise2() {
	var mu sync.RWMutex
	var wg sync.WaitGroup

	fmt.Println("\n--- Exercise 2 Start ---")

	wg.Add(1)
	go func() {
		defer wg.Done()
		mu.Lock()
		fmt.Println("写者: 拿到锁了，我要写 2 秒...")
		time.Sleep(2 * time.Second)
		mu.Unlock()
		fmt.Println("写者: 写完放锁。")
	}()

	// 稍等一下让写者先锁上
	time.Sleep(100 * time.Millisecond)

	for i := 1; i <= 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			fmt.Printf("读者 %d: 尝试进入图书馆...\n", id)
			mu.RLock()
			fmt.Printf("读者 %d: 终于进来了！\n", id)
			mu.RUnlock()
		}(i)
	}

	wg.Wait()
}

// 练习 3: 逻辑排错 (这个最难，请仔细看)
func exercise3() {
	var mu sync.RWMutex
	fmt.Println("\n--- Exercise 3 Start ---")

	// 猜猜这几行代码运行后会发生什么？
	mu.RLock()
	fmt.Println("拿到第一层读锁")
	mu.RLock()
	fmt.Println("拿到第二层读锁")

	mu.Lock() // 这里会发生什么？
	fmt.Println("拿到写锁")

	mu.Unlock()
	mu.RUnlock()
	mu.RUnlock()
}

func main() {
	// 你可以一个一个取消注释来运行
	exercise1()
	// exercise2()
	// exercise3()
}
