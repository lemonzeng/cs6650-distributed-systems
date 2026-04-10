package db

import (
	"context"
	"database/sql"
	"errors"

	"finalmystery/internal/model"
	"finalmystery/internal/store"
)

type PhotoStore struct {
	db *sql.DB
}

func NewPhotoStore(db *sql.DB) *PhotoStore {
	return &PhotoStore{db: db}
}

// Create inserts a photo row and atomically assigns the next seq number.
// Uses the LAST_INSERT_ID trick inside a transaction to get a connection-scoped
// monotonically increasing sequence per album.
func (s *PhotoStore) Create(ctx context.Context, photoID, albumID string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Atomically increment seq_counter; LAST_INSERT_ID() captures the new value
	// for this connection only — safe even under concurrent requests.
	var result sql.Result
	result, err = tx.ExecContext(ctx,
		`UPDATE albums SET seq_counter = LAST_INSERT_ID(seq_counter + 1) WHERE album_id = ?`,
		albumID,
	)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected == 0 {
		return 0, store.ErrNotFound
	}

	var seq int
	if err = tx.QueryRowContext(ctx, `SELECT LAST_INSERT_ID()`).Scan(&seq); err != nil {
		return 0, err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO photos (photo_id, album_id, seq, status) VALUES (?, ?, ?, 'processing')`,
		photoID, albumID, seq,
	)
	if err != nil {
		return 0, err
	}

	return seq, tx.Commit()
}

// Get returns the photo. Returns ErrNotFound if the row doesn't exist or status=deleted.
func (s *PhotoStore) Get(ctx context.Context, albumID, photoID string) (model.Photo, error) {
	const q = `
		SELECT photo_id, album_id, seq, status, COALESCE(url, ''), COALESCE(s3_key, '')
		FROM photos
		WHERE photo_id = ? AND album_id = ?
	`
	var p model.Photo
	var status string
	err := s.db.QueryRowContext(ctx, q, photoID, albumID).Scan(
		&p.PhotoID, &p.AlbumID, &p.Seq, &status, &p.URL, &p.S3Key,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Photo{}, store.ErrNotFound
	}
	if err != nil {
		return model.Photo{}, err
	}
	if status == string(model.StatusDeleted) {
		return model.Photo{}, store.ErrNotFound
	}
	p.Status = model.PhotoStatus(status)
	return p, nil
}

// SetCompleted conditionally updates status→completed only if still 'processing'.
// Returns updated=false (no error) when the photo was deleted before the worker finished.
func (s *PhotoStore) SetCompleted(ctx context.Context, photoID, url, s3Key string) (bool, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE photos SET status = 'completed', url = ?, s3_key = ?
		 WHERE photo_id = ? AND status = 'processing'`,
		url, s3Key, photoID,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// SetFailed updates status→failed only if still 'processing'.
func (s *PhotoStore) SetFailed(ctx context.Context, photoID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE photos SET status = 'failed' WHERE photo_id = ? AND status = 'processing'`,
		photoID,
	)
	return err
}

// Delete marks status→deleted and returns the s3Key for file cleanup.
// Returns ErrNotFound if the photo doesn't exist or was already deleted.
// s3Key may be empty string if the photo was still processing when deleted.
func (s *PhotoStore) Delete(ctx context.Context, albumID, photoID string) (string, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE photos SET status = 'deleted'
		 WHERE photo_id = ? AND album_id = ? AND status != 'deleted'`,
		photoID, albumID,
	)
	if err != nil {
		return "", err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if rows == 0 {
		return "", store.ErrNotFound
	}

	// Fetch s3_key after marking deleted (may be NULL if still processing).
	var s3Key sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT s3_key FROM photos WHERE photo_id = ?`, photoID,
	).Scan(&s3Key)
	if err != nil {
		return "", err
	}
	return s3Key.String, nil
}
