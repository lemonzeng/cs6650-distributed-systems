package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"finalmystery/internal/model"
	"finalmystery/internal/store"
)

// PutAlbum handles PUT /albums/{album_id}.
// Creates or updates an album. Returns 201 on create, 200 on update.
// The URL param is authoritative — body's album_id is overwritten.
func (h *Handler) PutAlbum(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "album_id")

	var a model.Album
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// URL param wins — prevents body/URL mismatch bugs.
	a.AlbumID = albumID

	created, err := h.albums.Upsert(r.Context(), a)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, a)
}

// GetAlbum handles GET /albums/{album_id}.
func (h *Handler) GetAlbum(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "album_id")

	a, err := h.albums.Get(r.Context(), albumID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, a)
}

// ListAlbums handles GET /albums.
// Always returns a JSON array — never null — even when empty.
func (h *Handler) ListAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := h.albums.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if albums == nil {
		albums = []model.Album{}
	}
	writeJSON(w, http.StatusOK, albums)
}
