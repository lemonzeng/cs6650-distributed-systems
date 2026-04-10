package worker

import (
	"context"
	"time"

	"finalmystery/internal/store"
)

// uploadTimeout caps how long a single S3 upload may take.
const uploadTimeout = 2 * time.Minute

type Worker struct {
	jobs   JobChan
	photos store.PhotoStore
	files  store.FileStore
}

func New(jobs JobChan, photos store.PhotoStore, files store.FileStore) *Worker {
	return &Worker{jobs: jobs, photos: photos, files: files}
}

func (w *Worker) Start(n int) {
	for i := 0; i < n; i++ {
		go w.loop()
	}
}

func (w *Worker) loop() {
	for job := range w.jobs {
		w.process(job)
	}
}

func (w *Worker) process(job PhotoJob) {
	// Release semaphore slot when done (registered first → runs last, after panic recovery).
	if job.Done != nil {
		defer job.Done()
	}

	// Close the multipart file handle after upload.
	// Go's in-memory multipart files (sectionReadCloser) have a nil io.Closer; calling
	// Close() on them panics. The inner defer+recover handles that gracefully.
	defer func() {
		if job.Body != nil {
			defer func() { recover() }() // catches nil-Closer panic
			job.Body.Close()
		}
	}()

	// Recover from any other panic so the goroutine stays alive.
	defer func() {
		if r := recover(); r != nil {
			w.photos.SetFailed(context.Background(), job.PhotoID)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), uploadTimeout)
	defer cancel()

	// Stream directly from the multipart file (disk or in-memory) to S3 — no full copy in RAM.
	url, s3Key, err := w.files.Upload(ctx, job.PhotoID, job.Body, job.Size, job.ContentType)
	if err != nil {
		w.photos.SetFailed(context.Background(), job.PhotoID)
		return
	}

	// Conditional update — WHERE status='processing' handles the delete race.
	updated, err := w.photos.SetCompleted(context.Background(), job.PhotoID, url, s3Key)
	if err != nil {
		w.photos.SetFailed(context.Background(), job.PhotoID)
		return
	}

	// Photo was deleted while uploading — clean up the orphaned S3 object.
	if !updated {
		w.files.Delete(context.Background(), s3Key)
	}
}
