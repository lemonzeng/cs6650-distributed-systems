package main

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
)

// --- 1. Models ---
type Product struct {
	ProductID    int    `json:"product_id"`
	SKU          string `json:"sku" binding:"required"`
	Manufacturer string `json:"manufacturer" binding:"required"`
	CategoryID   int    `json:"category_id"`
	Weight       int    `json:"weight"`
	SomeOtherID  int    `json:"some_other_id"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// --- 2. Storage ---
type ProductStore struct {
	sync.RWMutex
	data map[int]Product
}

var store = ProductStore{
	data: make(map[int]Product),
}

func main() {

	r := gin.Default()

	// --- 3. Handlers ---

	// GET /products/{productId}
	r.GET("/products/:productId", func(c *gin.Context) {
		idStr := c.Param("productId")
		id, err := strconv.Atoi(idStr)

		// handle 400: invalid product ID
		if err != nil || id < 1 {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "INVALID_ID",
				Message: "The provided input data is invalid",
				Details: "Product ID must be a positive integer",
			})
			return
		}

		store.RLock()
		product, exists := store.data[id]
		store.RUnlock()

		// handle 404: product not found
		if !exists {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "NOT_FOUND",
				Message: "Product not found",
				Details: "The requested product ID does not exist in memory",
			})
			return
		}

		// handle 200: product found
		c.JSON(http.StatusOK, product)
	})

	// POST /products/{productId}/details
	r.POST("/products/:productId/details", func(c *gin.Context) {
		idStr := c.Param("productId")
		id, err := strconv.Atoi(idStr)

		// handle 400: invalid product ID
		if err != nil || id < 1 {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "INVALID_INPUT",
				Message: "The provided input data is invalid",
				Details: "Product ID must be a positive integer",
			})
			return
		}

		// handle 400: invalid input data (missing required fields)
		var incomingProduct Product
		if err := c.ShouldBindJSON(&incomingProduct); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "INVALID_INPUT",
				Message: "Invalid data provided",
				Details: err.Error(),
			})
			return
		}

		// handle 404: product not found
		// store.RLock()
		// _, exists := store.data[id]
		// store.RUnlock()
		// if !exists {
		// 	c.JSON(http.StatusNotFound, ErrorResponse{
		// 		Error:   "NOT_FOUND",
		// 		Message: "Product not found",
		// 		Details: "The requested product ID does not exist in memory",
		// 	})
		// 	return
		// }

		incomingProduct.ProductID = id

		store.Lock()
		store.data[id] = incomingProduct
		store.Unlock()

		// handle 204: update successful
		c.Status(http.StatusNoContent)
	})

	r.Run(":8080")
}
