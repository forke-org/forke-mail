package ratelimit

import (
	"context"
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Rule struct {
	PerSecond float64
	Burst     int
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type Store struct {
	mu      sync.Mutex
	entries map[string]*entry
}

func NewStore() *Store {
	return &Store{entries: make(map[string]*entry)}
}

func (s *Store) limiter(key string, rule Rule) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok || e.limiter.Limit() != rate.Limit(rule.PerSecond) || e.limiter.Burst() != rule.Burst {
		e = &entry{limiter: rate.NewLimiter(rate.Limit(rule.PerSecond), rule.Burst)}
		s.entries[key] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

func (s *Store) Allow(key string, rule Rule) (ok bool, remaining int, retryAfter time.Duration) {
	lim := s.limiter(key, rule)
	now := time.Now()
	if lim.AllowN(now, 1) {
		return true, int(math.Floor(lim.TokensAt(now))), 0
	}
	res := lim.ReserveN(now, 1)
	retryAfter = res.DelayFrom(now)
	res.CancelAt(now)
	return false, 0, retryAfter
}

func (s *Store) Blocked(key string, rule Rule) bool {
	return s.limiter(key, rule).Tokens() < 1
}

func (s *Store) Janitor(ctx context.Context, idle time.Duration) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.evictIdle(idle)
		}
	}
}

func (s *Store) evictIdle(idle time.Duration) {
	cutoff := time.Now().Add(-idle)
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range s.entries {
		if e.lastSeen.Before(cutoff) {
			delete(s.entries, k)
		}
	}
}
