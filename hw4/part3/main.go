package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Configuration - Use environment variables in ECS
var (
	bucketName = os.Getenv("S3_BUCKET_NAME")
	region     = os.Getenv("AWS_REGION")
	role       = os.Getenv("APP_ROLE") // splitter, mapper, or reducer
)

func main() {
	http.HandleFunc("/", handleRequest)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Starting %s service on port %s...\n", role, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	ctx := context.TODO()
	cfg, _ := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	s3Client := s3.NewFromConfig(cfg)

	switch role {
	case "splitter":
		runSplitter(w, r, s3Client)
	case "mapper":
		runMapper(w, r, s3Client)
	case "reducer":
		runReducer(w, r, s3Client)
	default:
		fmt.Fprintf(w, "Unknown role: %s. Set APP_ROLE to splitter, mapper, or reducer.", role)
	}
}

// --- Splitter Logic ---
func runSplitter(w http.ResponseWriter, r *http.Request, client *s3.Client) {
	// Data Ingestion (The Fetch)
	url := r.URL.Query().Get("url")
	resp, _ := http.Get(url)
	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	// Split into 3 chunks roughly
	chunkSize := len(text) / 3
	indices := []int{0, chunkSize, chunkSize * 2, len(text)}

	var urls []string
	for i := 0; i < 3; i++ {
		content := text[indices[i]:indices[i+1]]
		key := fmt.Sprintf("chunks/chunk-%d.txt", i)
		uploadToS3(client, key, content)
		urls = append(urls, fmt.Sprintf("s3://%s/%s", bucketName, key))
	}

	// uses Go's json.Encoder to stream the resulting S3
	// paths back to the client as a JSON array. This ensures
	// that the output is structured and can be easily parsed by
	// the next stage of the pipeline (the Mappers).
	json.NewEncoder(w).Encode(urls)
}

// --- Mapper Logic ---
func runMapper(w http.ResponseWriter, r *http.Request, client *s3.Client) {
	s3Url := r.URL.Query().Get("url") // format: s3://bucket/key
	key := strings.Replace(s3Url, fmt.Sprintf("s3://%s/", bucketName), "", 1)

	// Download from S3
	obj, _ := client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	})

	counts := make(map[string]int)
	scanner := bufio.NewScanner(obj.Body)
	scanner.Split(bufio.ScanWords)

	for scanner.Scan() {
		// Normalize the word: convert to lowercase and trim punctuation
		word := strings.ToLower(scanner.Text())
		// Remove any non-letter characters from the start and end of the word
		word = strings.TrimFunc(word, func(r rune) bool {
			return !unicode.IsLetter(r)
		})
		if word != "" {
			counts[word]++
		}
	}

	// Upload the result back to S3 with a unique key based on the input chunk's key.
	// This ensures that each Mapper's output is stored separately and can be easily
	// retrieved by the Reducer.
	resultKey := fmt.Sprintf("mapped/result-%s.json", strings.ReplaceAll(key, "/", "-"))
	jsonData, _ := json.Marshal(counts)
	uploadToS3(client, resultKey, string(jsonData))

	fmt.Fprintf(w, "s3://%s/%s", bucketName, resultKey)
}

// --- Reducer Logic ---
func runReducer(w http.ResponseWriter, r *http.Request, client *s3.Client) {
	rawUrls := r.URL.Query()["urls"]
	var finalUrls []string
	for _, val := range rawUrls {
		finalUrls = append(finalUrls, strings.Split(val, ",")...)
	}

	log.Printf("Reducer starting. Input URLs count: %d", len(finalUrls))

	// Include a safety check to ensure that we have valid URLs to process
	// before attempting to download from S3.
	if len(finalUrls) == 0 {
		http.Error(w, "No URLs provided in query parameters", 400)
		return
	}

	finalCounts := make(map[string]int)
	totalFilesProcessed := 0

	for _, u := range finalUrls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}

		key := strings.Replace(u, fmt.Sprintf("s3://%s/", bucketName), "", 1)
		log.Printf("Reducer attempting to download: %s", key)

		obj, err := client.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(key),
		})
		if err != nil {
			log.Printf("FAILED to download %s: %v", key, err)
			continue
		}

		var m map[string]int
		err = json.NewDecoder(obj.Body).Decode(&m)
		obj.Body.Close()

		if err != nil {
			log.Printf("JSON DECODE ERROR for %s: %v", key, err)
			continue
		}

		log.Printf("SUCCESS: Added %d unique words from %s", len(m), key)
		for word, count := range m {
			finalCounts[word] += count
		}
		totalFilesProcessed++
	}

	// Before proceeding to upload the final results, we check if the
	// finalCounts map is empty.
	if len(finalCounts) == 0 {
		log.Printf("ABORT: No words were processed. Skipping S3 upload to prevent overwriting data.")
		http.Error(w, "Reduction resulted in empty map. Check logs for S3 download failures.", 500)
		return
	}

	log.Printf("Reduction complete. Processed %d files. Total unique words: %d", totalFilesProcessed, len(finalCounts))

	finalKey := "results/final_count.json"
	finalJson, _ := json.Marshal(finalCounts)
	uploadToS3(client, finalKey, string(finalJson))

	fmt.Fprintf(w, "s3://%s/%s", bucketName, finalKey)
}

func uploadToS3(client *s3.Client, key string, body string) {
	client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
		Body:   strings.NewReader(body),
	})
}
