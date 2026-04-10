package main

import (
	"github.com/go-chi/chi/v5"

	"finalmystery/internal/handler"
)

func newRouter(h *handler.Handler) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/health", h.Health)

	r.Put("/albums/{album_id}", h.PutAlbum)
	r.Get("/albums/{album_id}", h.GetAlbum)
	r.Get("/albums", h.ListAlbums)

	r.Post("/albums/{album_id}/photos", h.UploadPhoto)
	r.Get("/albums/{album_id}/photos/{photo_id}", h.GetPhoto)
	r.Delete("/albums/{album_id}/photos/{photo_id}", h.DeletePhoto)

	return r
}
