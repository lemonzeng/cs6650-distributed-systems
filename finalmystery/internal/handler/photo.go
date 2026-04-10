package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finalmystery/internal/model"
	"finalmystery/internal/store"
	"finalmystery/internal/worker"
)

// photoAcceptedResponse is the exact 202 shape for POST /photos.
// Must not include album_id or url — spec is strict.
type photoAcceptedResponse struct {
	PhotoID string            `json:"photo_id"`
	Seq     int               `json:"seq"`
	Status  model.PhotoStatus `json:"status"`
}

// safeClose closes a multipart.File without panicking on nil-Closer.
// Go's in-memory multipart files (sectionReadCloser) embed a nil io.Closer;
// calling Close() panics. This helper swallows that panic.
func safeClose(f interface{ Close() error }) {
	defer func() { recover() }()
	f.Close()
}

// UploadPhoto handles POST /albums/{album_id}/photos.
// Returns 202 immediately; processing happens asynchronously in the worker pool.
// The multipart file is streamed directly to the worker — no full read into RAM.
func (h *Handler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "album_id")

	// Hard cap: reject bodies over 210 MB before reading anything.
	r.Body = http.MaxBytesReader(w, r.Body, 210<<20)

	// Receive the file first — all concurrent requests can download in parallel.
	// 32 MB in-memory threshold — larger files spill to a temp file automatically.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing photo field")
		return
	}
	// Do NOT close file here — ownership transfers to the worker on success.

	// Acquire a semaphore slot AFTER receiving the file. This limits how many
	// uploads are simultaneously in-flight (S3 upload stage) and prevents the
	// job channel from being overwhelmed between test scenarios.
	// The slot is released by the worker after the S3 upload finishes.
	select {
	case h.uploadSem <- struct{}{}:
	case <-r.Context().Done():
		safeClose(file)
		writeError(w, http.StatusServiceUnavailable, "server busy")
		return
	}
	// From here on: we own a semaphore slot. Release it on every error path.

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	photoID := uuid.New().String()

	// Create DB row and get the atomically-assigned seq number.
	seq, err := h.photos.Create(r.Context(), photoID, albumID)
	if err != nil {
		<-h.uploadSem
		safeClose(file)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Non-blocking send — semaphore ownership transfers to the worker on success.
	// The semaphore (size 40) is always < the channel buffer (size 100), so this
	// send never blocks when the semaphore is the rate limiter.
	job := worker.PhotoJob{
		PhotoID:     photoID,
		AlbumID:     albumID,
		Body:        file,
		Size:        header.Size,
		ContentType: contentType,
		Done:        func() { <-h.uploadSem },
	}
	select {
	case h.jobs <- job:
		// Worker owns file and semaphore slot now.
	default:
		safeClose(file)
		<-h.uploadSem
		_ = h.photos.SetFailed(r.Context(), photoID)
		writeError(w, http.StatusServiceUnavailable, "server busy")
		return
	}

	writeJSON(w, http.StatusAccepted, photoAcceptedResponse{
		PhotoID: photoID,
		Seq:     seq,
		Status:  model.StatusProcessing,
	})
}

// GetPhoto handles GET /albums/{album_id}/photos/{photo_id}.
// Returns the photo at any lifecycle stage. url field only present when completed.
func (h *Handler) GetPhoto(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "album_id")
	photoID := chi.URLParam(r, "photo_id")

	photo, err := h.photos.Get(r.Context(), albumID, photoID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, photo)
}

// DeletePhoto handles DELETE /albums/{album_id}/photos/{photo_id}.
// Marks the photo deleted in DB and removes the file from S3 (best-effort).
func (h *Handler) DeletePhoto(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "album_id")
	photoID := chi.URLParam(r, "photo_id")

	s3Key, err := h.photos.Delete(r.Context(), albumID, photoID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// S3 cleanup is best-effort — never fail the HTTP response on S3 errors.
	if s3Key != "" {
		_ = h.files.Delete(r.Context(), s3Key)
	}

	writeJSON(w, http.StatusOK, map[string]any{})
}
