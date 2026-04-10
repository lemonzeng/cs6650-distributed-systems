package db

import (
	"context"
	"database/sql"
	"errors"

	"finalmystery/internal/model"
	"finalmystery/internal/store"
)

type AlbumStore struct {
	db *sql.DB
}

func NewAlbumStore(db *sql.DB) *AlbumStore {
	return &AlbumStore{db: db}
}

// Upsert creates or updates an album.
// MySQL RowsAffected: 1 = inserted, 2 = updated, 0 = no-op (same values).
func (s *AlbumStore) Upsert(ctx context.Context, a model.Album) (created bool, err error) {
	const q = `
		INSERT INTO albums (album_id, title, description, owner)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			title       = VALUES(title),
			description = VALUES(description),
			owner       = VALUES(owner)
	`
	result, err := s.db.ExecContext(ctx, q, a.AlbumID, a.Title, a.Description, a.Owner)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func (s *AlbumStore) Get(ctx context.Context, albumID string) (model.Album, error) {
	const q = `SELECT album_id, title, description, owner FROM albums WHERE album_id = ?`
	var a model.Album
	err := s.db.QueryRowContext(ctx, q, albumID).Scan(
		&a.AlbumID, &a.Title, &a.Description, &a.Owner,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Album{}, store.ErrNotFound
	}
	return a, err
}

// List returns all albums. Returns an empty slice (not nil) if none exist.
func (s *AlbumStore) List(ctx context.Context) ([]model.Album, error) {
	const q = `SELECT album_id, title, description, owner FROM albums`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	albums := []model.Album{}
	for rows.Next() {
		var a model.Album
		if err := rows.Scan(&a.AlbumID, &a.Title, &a.Description, &a.Owner); err != nil {
			return nil, err
		}
		albums = append(albums, a)
	}
	return albums, rows.Err()
}
