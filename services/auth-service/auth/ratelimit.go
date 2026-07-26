package auth

import (
	"sync"
	"time"
)

type RateLimitRule struct {
	MaxRequests int
	Window      time.Duration
}

var DefaultRules = map[string]RateLimitRule{
	RoleAdmin:    {MaxRequests: 1000, Window: 1 * time.Minute},
	RoleOperator: {MaxRequests: 500, Window: 1 * time.Minute},
	RoleReadOnly: {MaxRequests: 100, Window: 1 * time.Minute},
}

type ClientBucket struct {
	Tokens     int
	MaxTokens  int
	LastRefill time.Time
	RefillRate time.Duration
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*ClientBucket // key -> bucket
	rules   map[string]RateLimitRule
}

var globalRateLimiter = NewRateLimiter(DefaultRules)

func NewRateLimiter(rules map[string]RateLimitRule) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*ClientBucket),
		rules:   rules,
	}
}

func GetGlobalRateLimiter() *RateLimiter {
	return globalRateLimiter
}

// Allow checks whether a request for client key with role is permitted within rate limit bounds.
func (r *RateLimiter) Allow(clientKey string, role string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	rule, exists := r.rules[role]
	if !exists {
		rule = r.rules[RoleReadOnly]
	}

	now := time.Now()
	b, exists := r.buckets[clientKey]
	if !exists {
		b = &ClientBucket{
			Tokens:     rule.MaxRequests - 1,
			MaxTokens:  rule.MaxRequests,
			LastRefill: now,
			RefillRate: rule.Window,
		}
		r.buckets[clientKey] = b
		return true
	}

	// Refill tokens if window has elapsed
	elapsed := now.Sub(b.LastRefill)
	if elapsed >= b.RefillRate {
		b.Tokens = b.MaxTokens
		b.LastRefill = now
	}

	if b.Tokens > 0 {
		b.Tokens--
		return true
	}

	return false
}
