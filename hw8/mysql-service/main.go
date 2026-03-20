package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

// CartItem represents a single item in a cart
type CartItem struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

// CartResponse is returned by GET /shopping-carts/:id
type CartResponse struct {
	ShoppingCartID int        `json:"shopping_cart_id"`
	CustomerID     int        `json:"customer_id"`
	Items          []CartItem `json:"items"`
}

var db *sql.DB

func initDB() {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		panic("DB_DSN environment variable is required")
	}

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		panic(fmt.Sprintf("failed to open DB: %v", err))
	}

	// Connection pool tuning for 100 concurrent sessions
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)

	if err = db.Ping(); err != nil {
		panic(fmt.Sprintf("failed to connect to DB: %v", err))
	}

	createTables()
	fmt.Println("Database connected and tables ready")
}

// createTables auto-creates the schema on startup so no manual migration is needed
func createTables() {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS carts (
			id          INT AUTO_INCREMENT PRIMARY KEY,
			customer_id INT NOT NULL,
			created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_customer_id (customer_id)
		) ENGINE=InnoDB
	`)
	if err != nil {
		panic(fmt.Sprintf("failed to create carts table: %v", err))
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS cart_items (
			id         INT AUTO_INCREMENT PRIMARY KEY,
			cart_id    INT NOT NULL,
			product_id INT NOT NULL,
			quantity   INT NOT NULL,
			FOREIGN KEY (cart_id) REFERENCES carts(id) ON DELETE CASCADE,
			INDEX idx_cart_id (cart_id),
			UNIQUE KEY uq_cart_product (cart_id, product_id)
		) ENGINE=InnoDB
	`)
	if err != nil {
		panic(fmt.Sprintf("failed to create cart_items table: %v", err))
	}
}

// POST /shopping-carts
// Body: {"customer_id": 1}
// Response 201: {"shopping_cart_id": 42}
func createCart(c *gin.Context) {
	var body struct {
		CustomerID int `json:"customer_id" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_INPUT",
			"message": err.Error(),
		})
		return
	}

	result, err := db.Exec("INSERT INTO carts (customer_id) VALUES (?)", body.CustomerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "DB_ERROR",
			"message": err.Error(),
		})
		return
	}

	id, _ := result.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{"shopping_cart_id": id})
}

// GET /shopping-carts/:id
// Response 200: {"shopping_cart_id": 42, "customer_id": 1, "items": [...]}
func getCart(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_INPUT",
			"message": "cart id must be an integer",
		})
		return
	}

	var cart CartResponse
	err = db.QueryRow(
		"SELECT id, customer_id FROM carts WHERE id = ?", id,
	).Scan(&cart.ShoppingCartID, &cart.CustomerID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NOT_FOUND",
			"message": "shopping cart not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "DB_ERROR",
			"message": err.Error(),
		})
		return
	}

	rows, err := db.Query(
		"SELECT product_id, quantity FROM cart_items WHERE cart_id = ?", id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "DB_ERROR",
			"message": err.Error(),
		})
		return
	}
	defer rows.Close()

	cart.Items = []CartItem{}
	for rows.Next() {
		var item CartItem
		if err := rows.Scan(&item.ProductID, &item.Quantity); err != nil {
			continue
		}
		cart.Items = append(cart.Items, item)
	}

	c.JSON(http.StatusOK, cart)
}

// POST /shopping-carts/:id/items
// Body: {"product_id": 5, "quantity": 2}
// Response 204 on success; upserts quantity if product already in cart
func addItems(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_INPUT",
			"message": "cart id must be an integer",
		})
		return
	}

	var body struct {
		ProductID int `json:"product_id" binding:"required,min=1"`
		Quantity  int `json:"quantity" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_INPUT",
			"message": err.Error(),
		})
		return
	}

	// Verify cart exists
	var exists int
	err = db.QueryRow("SELECT id FROM carts WHERE id = ?", id).Scan(&exists)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NOT_FOUND",
			"message": "shopping cart not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "DB_ERROR",
			"message": err.Error(),
		})
		return
	}

	// Upsert: add quantity if item already exists, insert otherwise
	_, err = db.Exec(`
		INSERT INTO cart_items (cart_id, product_id, quantity)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE quantity = quantity + VALUES(quantity)
	`, id, body.ProductID, body.Quantity)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "DB_ERROR",
			"message": err.Error(),
		})
		return
	}

	c.Status(http.StatusNoContent)
}

func main() {
	initDB()

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	r.POST("/shopping-carts", createCart)
	r.GET("/shopping-carts/:id", getCart)
	r.POST("/shopping-carts/:id/items", addItems)

	fmt.Println("MySQL shopping cart service starting on :8080")
	r.Run(":8080")
}
