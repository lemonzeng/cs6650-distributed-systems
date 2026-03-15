package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

type Item struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	OrderID    string    `json:"order_id"`
	CustomerID int       `json:"customer_id"`
	Status     string    `json:"status"` // pending, processing, completed
	Items      []Item    `json:"items"`
	CreatedAt  time.Time `json:"created_at"`
}

// paymentSlots is a buffered channel used as a semaphore to simulate the
// payment processor bottleneck. A plain time.Sleep does not block a goroutine's
// OS thread in Go (M:N scheduling), so we gate access through this channel to
// enforce real throughput limits: only N payments can proceed concurrently.
// With N=5 and 3s per payment, max throughput ≈ 5/3 ≈ 1.7 orders/sec.
const maxConcurrentPayments = 5

var paymentSlots = make(chan struct{}, maxConcurrentPayments)

var (
	snsClient   *sns.Client
	snsTopicARN string
)

func generateOrderID() string {
	return fmt.Sprintf("order-%d", time.Now().UnixNano())
}

// processPayment acquires a slot from the semaphore, sleeps to simulate the
// 3-second payment verification delay, then releases the slot.
func processPayment() {
	paymentSlots <- struct{}{}        // acquire — blocks when all slots are taken
	defer func() { <-paymentSlots }() // release when done
	time.Sleep(3 * time.Second)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

// syncHandler blocks until payment processing completes (up to 3s + queue wait).
// Under high load the semaphore queue fills up and customers experience long
// or timed-out responses — this is the failure mode the assignment studies.
func syncHandler(w http.ResponseWriter, r *http.Request) {
	var order Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	order.OrderID = generateOrderID()
	order.CreatedAt = time.Now()
	order.Status = "processing"

	log.Printf("[SYNC] Processing order %s for customer %d", order.OrderID, order.CustomerID)
	processPayment()

	order.Status = "completed"
	log.Printf("[SYNC] Completed order %s", order.OrderID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(order)
}

// asyncHandler publishes the order to SNS and immediately returns 202 Accepted.
// The actual payment processing happens in the order-processor service.
func asyncHandler(w http.ResponseWriter, r *http.Request) {
	var order Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	order.OrderID = generateOrderID()
	order.CreatedAt = time.Now()
	order.Status = "pending"

	msgBytes, err := json.Marshal(order)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	_, err = snsClient.Publish(context.Background(), &sns.PublishInput{
		TopicArn: aws.String(snsTopicARN),
		Message:  aws.String(string(msgBytes)),
	})
	if err != nil {
		log.Printf("[ASYNC] Failed to publish order %s to SNS: %v", order.OrderID, err)
		http.Error(w, "failed to queue order", http.StatusInternalServerError)
		return
	}

	log.Printf("[ASYNC] Queued order %s for customer %d", order.OrderID, order.CustomerID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(order)
}

func main() {
	snsTopicARN = os.Getenv("SNS_TOPIC_ARN")
	if snsTopicARN == "" {
		log.Fatal("SNS_TOPIC_ARN environment variable is required")
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	snsClient = sns.NewFromConfig(cfg)

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/orders/sync", syncHandler)
	http.HandleFunc("/orders/async", asyncHandler)

	log.Printf("Order Receiver starting on :8080 (max concurrent payments: %d)", maxConcurrentPayments)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
