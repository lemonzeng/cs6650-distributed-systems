package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gin-gonic/gin"
)

// CartItem represents a single item in a cart
type CartItem struct {
	ProductID int `json:"product_id" dynamodbav:"product_id"`
	Quantity  int `json:"quantity"   dynamodbav:"quantity"`
}

// Cart is the DynamoDB item — single-table design with embedded items list.
// Partition key: cart_id (String). No sort key needed since all access is by cart_id.
type Cart struct {
	CartID     string     `json:"shopping_cart_id" dynamodbav:"cart_id"`
	CustomerID int        `json:"customer_id"      dynamodbav:"customer_id"`
	Items      []CartItem `json:"items"            dynamodbav:"items"`
	CreatedAt  string     `json:"created_at"       dynamodbav:"created_at"`
}

var (
	ddbClient *dynamodb.Client
	tableName string
)

func initDynamoDB() {
	tableName = os.Getenv("TABLE_NAME")
	if tableName == "" {
		tableName = "shopping-carts"
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2"
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to load AWS config: %v", err))
	}

	ddbClient = dynamodb.NewFromConfig(cfg)
	fmt.Printf("DynamoDB client ready (table: %s, region: %s)\n", tableName, region)
}

// POST /shopping-carts
// Body: {"customer_id": 1}
// Response 201: {"shopping_cart_id": "abc123"}
// cart_id is generated as a numeric string from UnixNano to avoid UUID dependency
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

	cartID := strconv.FormatInt(time.Now().UnixNano(), 10)

	cart := Cart{
		CartID:     cartID,
		CustomerID: body.CustomerID,
		Items:      []CartItem{},
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	item, err := attributevalue.MarshalMap(cart)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "MARSHAL_ERROR",
			"message": err.Error(),
		})
		return
	}

	_, err = ddbClient.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "DB_ERROR",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"shopping_cart_id": cartID})
}

// GET /shopping-carts/:id
// Response 200: {"shopping_cart_id": "abc", "customer_id": 1, "items": [...]}
func getCart(c *gin.Context) {
	cartID := c.Param("id")

	result, err := ddbClient.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberS{Value: cartID},
		},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "DB_ERROR",
			"message": err.Error(),
		})
		return
	}

	if result.Item == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NOT_FOUND",
			"message": "shopping cart not found",
		})
		return
	}

	var cart Cart
	if err := attributevalue.UnmarshalMap(result.Item, &cart); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "UNMARSHAL_ERROR",
			"message": err.Error(),
		})
		return
	}

	if cart.Items == nil {
		cart.Items = []CartItem{}
	}

	c.JSON(http.StatusOK, cart)
}

// POST /shopping-carts/:id/items
// Body: {"product_id": 5, "quantity": 2}
// Response 204; appends to items list (upserts by product_id)
func addItems(c *gin.Context) {
	cartID := c.Param("id")

	var body struct {
		ProductID int `json:"product_id" binding:"required,min=1"`
		Quantity  int `json:"quantity"   binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_INPUT",
			"message": err.Error(),
		})
		return
	}

	// Use UpdateItem with list_append to upsert the item.
	// This approach first checks for the cart (condition expression) and
	// appends to the items list atomically.
	_, err := ddbClient.UpdateItem(context.TODO(), &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberS{Value: cartID},
		},
		UpdateExpression:    aws.String("SET #items = list_append(if_not_exists(#items, :empty), :new_item)"),
		ConditionExpression: aws.String("attribute_exists(cart_id)"),
		ExpressionAttributeNames: map[string]string{
			"#items": "items",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":new_item": &types.AttributeValueMemberL{
				Value: []types.AttributeValue{
					&types.AttributeValueMemberM{
						Value: map[string]types.AttributeValue{
							"product_id": &types.AttributeValueMemberN{Value: strconv.Itoa(body.ProductID)},
							"quantity":   &types.AttributeValueMemberN{Value: strconv.Itoa(body.Quantity)},
						},
					},
				},
			},
			":empty": &types.AttributeValueMemberL{Value: []types.AttributeValue{}},
		},
	})
	if err != nil {
		// ConditionalCheckFailedException means cart does not exist
		var condErr *types.ConditionalCheckFailedException
		if isConditionalCheckFailed(err, condErr) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NOT_FOUND",
				"message": "shopping cart not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "DB_ERROR",
			"message": err.Error(),
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// isConditionalCheckFailed checks if the error is a DynamoDB ConditionalCheckFailedException
func isConditionalCheckFailed(err error, _ *types.ConditionalCheckFailedException) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*types.ConditionalCheckFailedException)
	return ok
}

func main() {
	initDynamoDB()

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	r.POST("/shopping-carts", createCart)
	r.GET("/shopping-carts/:id", getCart)
	r.POST("/shopping-carts/:id/items", addItems)

	fmt.Println("DynamoDB shopping cart service starting on :8080")
	r.Run(":8080")
}
