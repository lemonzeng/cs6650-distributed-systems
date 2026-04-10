package handler

import (
	"encoding/json"
	"net/http"

	"finalmystery/internal/store"
	"finalmystery/internal/worker"
)

// maxConcurrentUploads is the semaphore capacity — the maximum number of photo uploads
// that may be simultaneously in-flight (from handler ParseMultipartForm through S3 upload).
// With streaming, each in-flight upload holds at most 32 MB in RAM (the multipart
// in-memory threshold), so 40 slots ≈ 1.3 GB peak — safe on a 4 GB t3.medium.
const maxConcurrentUploads = 40

// Handler holds the shared dependencies injected by main.
type Handler struct {
	albums    store.AlbumStore
	photos    store.PhotoStore
	files     store.FileStore
	jobs      worker.JobChan
	uploadSem chan struct{} // counting semaphore, size = maxConcurrentUploads
}

// New constructs a Handler with all dependencies.
func New(albums store.AlbumStore, photos store.PhotoStore, files store.FileStore, jobs worker.JobChan) *Handler {
	return &Handler{
		albums:    albums,
		photos:    photos,
		files:     files,
		jobs:      jobs,
		uploadSem: make(chan struct{}, maxConcurrentUploads),
	}
}

// writeJSON sets Content-Type, writes the status code, and encodes v as JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON {"error": msg} response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
