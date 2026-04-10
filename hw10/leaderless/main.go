package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Data model
// ---------------------------------------------------------------------------

type Entry struct {
	Value   string `json:"value"`
	Version int64  `json:"version"` // Unix nanoseconds — no central counter needed
}

var (
	mu    sync.RWMutex
	store = make(map[string]Entry)
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

var (
	nodeID   string
	peerURLs []string
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

type setRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type updateRequest struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func readBody(r *http.Request, dst any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

// ---------------------------------------------------------------------------
// Store helpers
// ---------------------------------------------------------------------------

// localWrite writes the entry only if version is newer than what we have.
func localWrite(key, value string, version int64) {
	mu.Lock()
	defer mu.Unlock()
	cur, ok := store[key]
	if !ok || version > cur.Version {
		store[key] = Entry{Value: value, Version: version}
	}
}

func localRead(key string) (Entry, bool) {
	mu.RLock()
	defer mu.RUnlock()
	e, ok := store[key]
	return e, ok
}

// ---------------------------------------------------------------------------
// Peer HTTP calls
// ---------------------------------------------------------------------------

func postUpdate(url, key, value string, version int64) error {
	body, _ := json.Marshal(updateRequest{Key: key, Value: value, Version: version})
	resp, err := http.Post(url+"/internal/update", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("peer returned %d", resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// POST /set — write coordinator (W=N=5)
func handleSet(w http.ResponseWriter, r *http.Request) {
	var req setRequest
	if err := readBody(r, &req); err != nil || req.Key == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	version := time.Now().UnixNano()
	localWrite(req.Key, req.Value, version)

	// Propagate to all peers sequentially (W=N)
	for _, purl := range peerURLs {
		postUpdate(purl, req.Key, req.Value, version) //nolint:errcheck
		time.Sleep(200 * time.Millisecond)
	}

	writeJSON(w, http.StatusCreated, map[string]int64{"version": version})
}

// GET /get?key=K — R=1, return local value immediately
func handleGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}
	e, ok := localRead(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// GET /local_read?key=K — alias for /get (no coordination)
func handleLocalRead(w http.ResponseWriter, r *http.Request) {
	handleGet(w, r)
}

// POST /internal/update — called by write coordinator
func handleInternalUpdate(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := readBody(r, &req); err != nil || req.Key == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	time.Sleep(100 * time.Millisecond)
	localWrite(req.Key, req.Value, req.Version)
	w.WriteHeader(http.StatusOK)
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	nodeID = envOr("NODE_ID", "node?")
	port := envOr("PORT", "8000")

	if raw := os.Getenv("PEER_URLS"); raw != "" {
		for _, u := range strings.Split(raw, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				peerURLs = append(peerURLs, u)
			}
		}
	}

	log.Printf("[%s] starting leaderless node on :%s peers=%v", nodeID, port, peerURLs)

	mux := http.NewServeMux()
	mux.HandleFunc("/set", handleSet)
	mux.HandleFunc("/get", handleGet)
	mux.HandleFunc("/local_read", handleLocalRead)
	mux.HandleFunc("/internal/update", handleInternalUpdate)

	log.Fatal(http.ListenAndServe(":"+port, mux))
}
