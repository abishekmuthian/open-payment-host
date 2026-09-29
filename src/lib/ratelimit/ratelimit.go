// Package ratelimit counts failures per key in fixed windows.
//
// Counters live in memory and are per process: a restart clears them.
package ratelimit

import (
	"sync"
	"time"
)

// DefaultMaxKeys bounds the number of tracked keys.
const DefaultMaxKeys = 10000

type entry struct {
	count int
	reset time.Time
}

// Limiter blocks a key after Max failures until its window, which starts at
// the first failure, expires.
type Limiter struct {
	Max     int
	Window  time.Duration
	MaxKeys int

	now     func() time.Time
	mu      sync.Mutex
	entries map[string]entry
}

// New returns a limiter allowing max failures per window.
func New(max int, window time.Duration) *Limiter {
	return &Limiter{
		Max:     max,
		Window:  window,
		MaxKeys: DefaultMaxKeys,
		now:     time.Now,
		entries: make(map[string]entry),
	}
}

// SetClock replaces the time source (for tests).
func (l *Limiter) SetClock(now func() time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = now
}

// Blocked reports whether key has reached Max failures in its current window,
// and if so how long until it may retry.
func (l *Limiter) Blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	e, ok := l.entries[key]
	if !ok || !now.Before(e.reset) {
		return false, 0
	}
	if e.count >= l.Max {
		return true, e.reset.Sub(now)
	}
	return false, 0
}

// Fail records a failure for key.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	e, ok := l.entries[key]
	if !ok || !now.Before(e.reset) {
		if !ok && l.MaxKeys > 0 && len(l.entries) >= l.MaxKeys {
			l.evict(now)
		}
		e = entry{reset: now.Add(l.Window)}
	}
	e.count++
	l.entries[key] = e
}

// Reset forgets failures for key.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// evict removes expired entries and, if the map is still full, the entry
// whose window ends first. Callers hold l.mu.
func (l *Limiter) evict(now time.Time) {
	for k, e := range l.entries {
		if !now.Before(e.reset) {
			delete(l.entries, k)
		}
	}
	if len(l.entries) < l.MaxKeys {
		return
	}
	var oldest string
	var oldestReset time.Time
	for k, e := range l.entries {
		if oldest == "" || e.reset.Before(oldestReset) {
			oldest, oldestReset = k, e.reset
		}
	}
	delete(l.entries, oldest)
}

// Len returns the number of tracked keys, including expired ones not yet pruned.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
