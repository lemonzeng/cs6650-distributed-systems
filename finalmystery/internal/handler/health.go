package handler

import "net/http"

// Health handles GET /health.
// Must return exactly {"status": "ok"} — ChaosArena checks the exact string.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
