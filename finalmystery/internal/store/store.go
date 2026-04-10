package store

import (
	"context"
	"errors"
	"io"

	"finalmystery/internal/model"
)

// ErrNotFound is returned when a requested resource does not exist or has been deleted.
var ErrNotFound = errors.New("not found")

// AlbumStore handles album persistence.
type AlbumStore interface {
	// Upsert creates or updates an album. Returns created=true if a new row was inserted.
	Upsert(ctx context.Context, a model.Album) (created bool, err error)

	// Get returns the album by ID. Returns ErrNotFound if not found.
	Get(ctx context.Context, albumID string) (model.Album, error)

	// List returns every album ever created — no limit, no pagination skipping.
	List(ctx context.Context) ([]model.Album, error)
}

// PhotoStore handles photo metadata persistence.
type PhotoStore interface {
	// Create inserts a new photo row and atomically increments the per-album seq counter.
	// Returns the assigned seq number. albumID must already exist in the albums table.
	Create(ctx context.Context, photoID, albumID string) (seq int, err error)

	// Get returns the photo. Returns ErrNotFound if status=deleted or row missing.
	Get(ctx context.Context, albumID, photoID string) (model.Photo, error)

	// SetCompleted updates status→completed and sets url + s3_key.
	// Uses a conditional UPDATE (WHERE status='processing') to handle the delete race.
	// Returns updated=false (no error) if the photo was deleted before the worker finished;
	// caller must then delete the newly-uploaded S3 object.
	SetCompleted(ctx context.Context, photoID, url, s3Key string) (updated bool, err error)

	// SetFailed updates status→failed.
	SetFailed(ctx context.Context, photoID string) error

	// Delete marks status→deleted and returns the s3Key for file cleanup.
	// Returns ErrNotFound if the photo does not exist or is already deleted.
	Delete(ctx context.Context, albumID, photoID string) (s3Key string, err error)
}

// FileStore handles binary photo file storage (S3).
type FileStore interface {
	// Upload streams the image to storage and returns a permanent public URL and the S3 object key.
	// size must be the exact byte length — required for S3 Content-Length header.
	// The caller is responsible for closing body; Upload does NOT close it.
	Upload(ctx context.Context, photoID string, body io.Reader, size int64, contentType string) (url string, s3Key string, err error)

	// Delete removes the object from storage. Must be idempotent — no error if already gone.
	Delete(ctx context.Context, s3Key string) error
}
