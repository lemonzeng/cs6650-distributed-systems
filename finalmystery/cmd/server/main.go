package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"

	"finalmystery/internal/db"
	"finalmystery/internal/handler"
	"finalmystery/internal/storage"
	"finalmystery/internal/worker"
)

func main() {
	// --- Config from environment ---
	port := getenv("PORT", "8080")
	dsn := mustenv("DB_DSN")
	bucket := mustenv("S3_BUCKET")
	region := mustenv("S3_REGION")
	workerCount, _ := strconv.Atoi(getenv("WORKER_COUNT", "20"))

	// --- Database ---
	sqlDB, err := db.New(dsn)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer sqlDB.Close()
	log.Println("DB connected")

	// --- Run migrations ---
	if err := runMigrations(sqlDB); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Println("Migrations applied")

	// --- S3 ---
	ctx := context.Background()
	s3store, err := storage.NewS3Store(ctx, bucket, region)
	if err != nil {
		log.Fatalf("s3 connect: %v", err)
	}
	log.Println("S3 connected")

	// --- Store implementations ---
	albumStore := db.NewAlbumStore(sqlDB)
	photoStore := db.NewPhotoStore(sqlDB)

	// --- Worker pool ---
	jobs := make(worker.JobChan, 100)
	w := worker.New(jobs, photoStore, s3store)
	w.Start(workerCount)
	log.Printf("Worker pool started (%d goroutines)", workerCount)

	// --- HTTP server ---
	h := handler.New(albumStore, photoStore, s3store, jobs)
	router := newRouter(h)

	addr := ":" + port
	log.Printf("Listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}
