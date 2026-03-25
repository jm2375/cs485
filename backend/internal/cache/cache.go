// Package cache provides a simple in-memory key-value store with TTL.
package cache

import (
	"fmt"
	"sync"
	"time"
)

type entry struct {
	value     string
	expiresAt time.Time
}

// Store is a thread-safe in-memory cache with optional TTL per key.
type Store struct {
	mu   sync.Mutex
	data map[string]entry
}

// New returns a Store with a background cleanup goroutine.
func New() *Store {
	s := &Store{data: make(map[string]entry)}
	go s.cleanup()
	return s
}

// Set stores value under key with the given TTL (0 = no expiry).
func (s *Store) Set(key, value string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := entry{value: value}
	if ttl > 0 {
		e.expiresAt = time.Now().Add(ttl)
	}
	s.data[key] = e
}

// Get retrieves a value. Returns ("", false) if missing or expired.
func (s *Store) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[key]
	if !ok {
		return "", false
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		delete(s.data, key)
		return "", false
	}
	return e.value, true
}

// Del removes a key.
func (s *Store) Del(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// Incr atomically increments an integer counter, setting a TTL on first create.
func (s *Store) Incr(key string, ttl time.Duration) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.data[key]
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		e = entry{}
	}
	var count int64
	fmt.Sscanf(e.value, "%d", &count)
	count++
	expiry := e.expiresAt
	if expiry.IsZero() && ttl > 0 {
		expiry = time.Now().Add(ttl)
	}
	s.data[key] = entry{value: fmt.Sprintf("%d", count), expiresAt: expiry}
	return count
}

func (s *Store) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for k, e := range s.data {
			if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
				delete(s.data, k)
			}
		}
		s.mu.Unlock()
	}
}
