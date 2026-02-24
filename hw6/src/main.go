package main

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// --- 1. Product Struct ---
type Product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

// --- 2. Search Response Struct ---
type SearchResponse struct {
	Products   []Product `json:"products"`
	TotalFound int       `json:"total_found"`
	SearchTime string    `json:"search_time"`
}

// --- 3. Global Storage ---
// sync.Map stores products with int(ID) as key and Product as value
// A separate ordered keys slice is maintained to guarantee "exactly 100 checked" during search
var (
	productStore sync.Map
	productKeys  []int   // read-only after startup, no lock needed
	cpuSink      float64 // package-level sink prevents compiler from optimizing away the spin loop
)

// --- 4. Data Generation (runs at startup) ---
// Fixed brands and categories arrays with modulo rotation ensure consistent search behavior during testing
func generateProducts() {
	brands := []string{"Alpha", "Beta", "Gamma", "Delta", "Echo",
		"Foxtrot", "Golf", "Hotel", "India", "Juliet"}
	categories := []string{"Electronics", "Books", "Home", "Sports",
		"Clothing", "Toys", "Garden", "Automotive"}

	keys := make([]int, 0, 100000)
	// Generate 100,000 products with predictable patterns for testing
	for i := 1; i <= 100000; i++ {
		brand := brands[i%len(brands)]
		category := categories[i%len(categories)]

		p := Product{
			ID:          i,
			Name:        fmt.Sprintf("Product %s %d", brand, i), // "Product Alpha 1"
			Category:    category,
			Description: fmt.Sprintf("A high quality %s product from %s", category, brand),
			Brand:       brand,
		}
		productStore.Store(i, p)
		keys = append(keys, i)
	}

	productKeys = keys
	fmt.Printf("Generated %d products\n", len(productKeys))
}

// --- 5. Search Logic ---
// - productKeys is an ordered slice; iterating the first 100 guarantees "exactly 100 checked"
// - checked counter increments at the top of each iteration regardless of whether it matches
// - Case-insensitive matching via strings.ToLower
// - Returns at most 20 results
func searchProducts(query string) ([]Product, int) {
	queryLower := strings.ToLower(query)
	results := make([]Product, 0, 20)
	totalFound := 0
	checked := 0

	for _, key := range productKeys {
		// Critical: increment counter first, then check whether to break
		checked++
		if checked > 100 {
			break
		}

		// Load product from sync.Map
		val, ok := productStore.Load(key)
		if !ok {
			continue
		}
		p := val.(Product)

		// Simulate fixed-time computation per product (e.g., AI model inference).
		// math.Sqrt in a tight loop burns real CPU cycles without compiler optimization.
		// Tune spinIterations to hit target CPU% on your Fargate instance:
		//   too low  → CPU never saturates even at 20 users
		//   too high → CPU saturates even at 5 users
		spinResult := 0.0
		for i := 0; i < 10000; i++ {
			spinResult += math.Sqrt(float64(i + p.ID))
		}
		cpuSink = spinResult // write to global: prevents dead-code elimination

		// Search name and category (case-insensitive)
		if strings.Contains(strings.ToLower(p.Name), queryLower) ||
			strings.Contains(strings.ToLower(p.Category), queryLower) {
			totalFound++
			if len(results) < 20 {
				results = append(results, p)
			}
		}
	}

	return results, totalFound
}

func main() {
	// Data generation
	fmt.Println("⏳ Generating 100,000 products...")
	generateProducts()

	r := gin.Default()

	// --- 6. /health endpoint (used by ALB health checks in Part III) ---
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":   "healthy",
			"products": len(productKeys),
		})
	})

	// --- 7. /products/search endpoint ---
	// Return 400 when query param 'q' is missing to reject invalid requests
	r.GET("/products/search", func(c *gin.Context) {
		query := c.Query("q")
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "query parameter 'q' is required",
			})
			return
		}

		start := time.Now()
		products, totalFound := searchProducts(query)
		elapsed := time.Since(start)

		c.JSON(http.StatusOK, SearchResponse{
			Products:   products,
			TotalFound: totalFound,
			SearchTime: elapsed.String(),
		})
	})

	fmt.Println("🚀 Server starting on :8080")
	r.Run(":8080")
}
