package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type Item struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	OrderID    string    `json:"order_id"`
	CustomerID int       `json:"customer_id"`
	Status     string    `json:"status"`
	Items      []Item    `json:"items"`
	CreatedAt  time.Time `json:"created_at"`
}

// SNSEnvelope is the outer wrapper SNS puts around messages delivered to SQS.
type SNSEnvelope struct {
	Type    string `json:"Type"`
	Message string `json:"Message"` // the actual JSON-encoded order
}

var (
	sqsClient *sqs.Client
	queueURL  string
)

// processPayment simulates the 3-second payment verification delay.
func processPayment(order Order) {
	log.Printf("[WORKER] Processing order %s for customer %d", order.OrderID, order.CustomerID)
	time.Sleep(3 * time.Second)
	log.Printf("[WORKER] Completed order %s", order.OrderID)
}

// handleMessage parses an SQS message (SNS-wrapped), processes the order,
// then deletes the message from the queue on success.
func handleMessage(msg types.Message) {
	// SNS delivers to SQS inside a JSON envelope — unwrap it first.
	var envelope SNSEnvelope
	if err := json.Unmarshal([]byte(*msg.Body), &envelope); err != nil {
		log.Printf("[WORKER] Failed to parse SNS envelope for msg %s: %v", *msg.MessageId, err)
		return
	}

	var order Order
	if err := json.Unmarshal([]byte(envelope.Message), &order); err != nil {
		log.Printf("[WORKER] Failed to parse order JSON for msg %s: %v", *msg.MessageId, err)
		return
	}

	processPayment(order)

	// Delete from queue after successful processing to prevent redelivery.
	_, err := sqsClient.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		log.Printf("[WORKER] Failed to delete msg %s: %v", *msg.MessageId, err)
	}
}

// poll continuously long-polls SQS. For each received message it acquires a
// worker slot from the semaphore channel, then spawns a goroutine. When all
// slots are full the acquire blocks, naturally throttling message dispatch
// without spinning. This is how we control concurrency for Phase 5 scaling.
func poll(workerSem chan struct{}) {
	for {
		result, err := sqsClient.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10, // fetch up to 10 per call
			WaitTimeSeconds:     20, // long polling — avoids busy-wait when queue is empty
		})
		if err != nil {
			log.Printf("[POLL] ReceiveMessage error: %v — retrying in 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, msg := range result.Messages {
			msg := msg
			workerSem <- struct{}{} // acquire a worker slot (blocks if all are busy)
			go func() {
				defer func() { <-workerSem }() // release slot when goroutine exits
				handleMessage(msg)
			}()
		}
	}
}

func main() {
	queueURL = os.Getenv("SQS_QUEUE_URL")
	if queueURL == "" {
		log.Fatal("SQS_QUEUE_URL environment variable is required")
	}

	workerCount := 1
	if wc := os.Getenv("WORKER_COUNT"); wc != "" {
		if n, err := strconv.Atoi(wc); err == nil && n > 0 {
			workerCount = n
		}
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	sqsClient = sqs.NewFromConfig(cfg)

	log.Printf("Order Processor starting — worker goroutines: %d", workerCount)
	log.Printf("Theoretical max throughput: %.2f orders/sec", float64(workerCount)/3.0)

	// The semaphore channel has capacity == workerCount.
	// Phase 3 default: 1 worker  → 0.33 orders/sec
	// Phase 5 testing: 5 workers → 1.67/sec | 20 → 6.67/sec | 100 → 33.3/sec
	workerSem := make(chan struct{}, workerCount)
	poll(workerSem)
}
