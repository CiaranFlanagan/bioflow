// Package state persists per-step results between runs.
//
// This is what makes a run resumable: on startup the executor loads the store,
// and any step whose recorded cache key matches its current key is skipped
// rather than re-executed.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Status is the terminal state of a step from a previous run.
type Status string

const (
	StatusDone   Status = "done"
	StatusFailed Status = "failed"
)

// Record is what we remember about one completed step.
type Record struct {
	CacheKey   string    `json:"cache_key"`
	Status     Status    `json:"status"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`
}

// Store is a crash-safe record of completed steps, backed by a JSON file.
// It is safe for concurrent use — the executor writes to it from every worker.
type Store struct {
	path string

	mu      sync.Mutex
	records map[string]Record
}

// Open loads an existing store, or creates an empty one if the file is absent.
func Open(path string) (*Store, error) {
	s := &Store{path: path, records: make(map[string]Record)}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open state: %w", err)
	}
	if err := json.Unmarshal(data, &s.records); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", path, err)
	}
	return s, nil
}

// Completed reports whether a step already succeeded with this exact cache key.
// A key mismatch means the step's inputs changed, so the old result is stale.
func (s *Store) Completed(stepID, cacheKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.records[stepID]
	return ok && r.Status == StatusDone && r.CacheKey == cacheKey
}

// Set records a step result and flushes it to disk immediately, so that a
// crash mid-run still leaves everything completed so far recoverable.
func (s *Store) Set(stepID string, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records[stepID] = r
	return s.flushLocked()
}

// CompletedCount returns how many steps are recorded as done.
func (s *Store) CompletedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for _, r := range s.records {
		if r.Status == StatusDone {
			n++
		}
	}
	return n
}

// flushLocked writes via a temp file and rename, so a crash during the write
// cannot leave a truncated state file behind. Caller must hold s.mu.
func (s *Store) flushLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("state dir: %w", err)
	}

	data, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("commit state: %w", err)
	}
	return nil
}
