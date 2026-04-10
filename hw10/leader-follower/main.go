package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Data model
// ---------------------------------------------------------------------------

type Entry struct {
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

var (
	mu             sync.RWMutex
	store          = make(map[string]Entry)
	versionCounter int64
)

// ---------------------------------------------------------------------------
// Configuration (read once at startup)
// ---------------------------------------------------------------------------

var (
	role         string // "leader" | "follower"
	nodeID       string
	followerURLs []string
	wQuorum      int
	rQuorum      int
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
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

func localWrite(key, value string, version int64) {
	mu.Lock()
	defer mu.Unlock()
	store[key] = Entry{Value: value, Version: version}
}

func localRead(key string) (Entry, bool) {
	mu.RLock()
	defer mu.RUnlock()
	e, ok := store[key]
	return e, ok
}

// ---------------------------------------------------------------------------
// Follower HTTP calls (called by leader)
// ---------------------------------------------------------------------------

func postUpdate(url, key, value string, version int64) error {
	body, _ := json.Marshal(updateRequest{Key: key, Value: value, Version: version})
	resp, err := http.Post(url+"/internal/update", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("follower returned %d", resp.StatusCode)
	}
	return nil
}

func getInternalRead(url, key string) (Entry, error) {
	resp, err := http.Get(url + "/internal/read?key=" + key)
	if err != nil {
		return Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Entry{}, fmt.Errorf("not found")
	}
	var e Entry
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// ---------------------------------------------------------------------------
// Leader handlers
// ---------------------------------------------------------------------------

func leaderSet(w http.ResponseWriter, r *http.Request) {
	var req setRequest
	if err := readBody(r, &req); err != nil || req.Key == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	version := atomic.AddInt64(&versionCounter, 1)
	localWrite(req.Key, req.Value, version)

	acks := 1 // self counts
	remaining := []string{}

	if wQuorum == 1 {
		// fire all updates async
		remaining = followerURLs
	} else {
		// propagate sequentially until quorum met
		for _, furl := range followerURLs {
			if acks >= wQuorum {
				remaining = append(remaining, furl)
				continue
			}
			err := postUpdate(furl, req.Key, req.Value, version)
			time.Sleep(200 * time.Millisecond)
			if err == nil {
				acks++
			}
		}
	}

	// async updates for any remaining followers
	for _, furl := range remaining {
		go func(u string) {
			postUpdate(u, req.Key, req.Value, version)
		}(furl)
	}

	writeJSON(w, http.StatusCreated, map[string]int64{"version": version})
}

func leaderGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}

	local, ok := localRead(key)

	// If R=1, return local value immediately without reading followers
	if rQuorum == 1 {
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, local)
		return
	}

	// Read from (R-1) followers in parallel
	type result struct {
		entry Entry
		ok    bool
	}
	needed := rQuorum - 1
	if needed > len(followerURLs) {
		needed = len(followerURLs)
	}
	ch := make(chan result, needed)
	for i := 0; i < needed; i++ {
		go func(u string) {
			e, err := getInternalRead(u, key)
			if err != nil {
				ch <- result{ok: false}
			} else {
				ch <- result{entry: e, ok: true}
			}
		}(followerURLs[i])
	}

	best := local
	bestOK := ok
	for i := 0; i < needed; i++ {
		res := <-ch
		if res.ok && (!bestOK || res.entry.Version > best.Version) {
			best = res.entry
			bestOK = true
		}
	}

	if !bestOK {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, best)
}

// ---------------------------------------------------------------------------
// Follower handlers
// ---------------------------------------------------------------------------

// /internal/update is called by leader to apply an update to
// this follower's local store
func followerInternalUpdate(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := readBody(r, &req); err != nil || req.Key == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	time.Sleep(100 * time.Millisecond)
	localWrite(req.Key, req.Value, req.Version)
	w.WriteHeader(http.StatusOK)
}

// /internal/read is called by leader to read this follower's local value for a key
func followerInternalRead(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}
	time.Sleep(50 * time.Millisecond)
	e, ok := localRead(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// Followers respond to client /get and /local_read directly from local store
// /get is the "normal" read path for followers,
// which includes an artificial delay to simulate slower response than leader
// that is what the load tester uses
func followerGet(w http.ResponseWriter, r *http.Request) {
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

// /local_read is available on ALL nodes — returns local value without any delay or coordination
func localReadHandler(w http.ResponseWriter, r *http.Request) {
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

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	role = envOr("ROLE", "follower")
	nodeID = envOr("NODE_ID", "unknown")
	port := envOr("PORT", "8000")
	wQuorum = envInt("W", 1)
	rQuorum = envInt("R", 1)

	if raw := os.Getenv("FOLLOWER_URLS"); raw != "" {
		for _, u := range strings.Split(raw, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				followerURLs = append(followerURLs, u)
			}
		}
	}

	mux := http.NewServeMux()

	// /local_read available on all nodes
	mux.HandleFunc("/local_read", localReadHandler)

	if role == "leader" {
		log.Printf("[%s] starting as LEADER on :%s W=%d R=%d followers=%v",
			nodeID, port, wQuorum, rQuorum, followerURLs)

		mux.HandleFunc("/set", leaderSet)
		mux.HandleFunc("/get", leaderGet)
	} else {
		log.Printf("[%s] starting as FOLLOWER on :%s", nodeID, port)

		// Followers expose /get as a direct local read for clients
		mux.HandleFunc("/get", followerGet)

		// Followers reject /set from clients
		mux.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "writes must go to leader", http.StatusBadRequest)
		})

		// Internal endpoints used by leader
		mux.HandleFunc("/internal/update", followerInternalUpdate)
		mux.HandleFunc("/internal/read", followerInternalRead)
	}

	log.Fatal(http.ListenAndServe(":"+port, mux))
}
