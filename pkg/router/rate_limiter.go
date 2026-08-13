package router

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"toron/pkg/httpparser"
)

// ParseRateLimit parses a rate limit string (e.g. "10/sec", "100/min", "1000/hour") into refill rate per second and max burst capacity.
func ParseRateLimit(s string) (ratePerSec float64, burst int, err error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, 0, nil
	}

	parts := strings.Split(s, "/")
	countStr := strings.TrimSpace(parts[0])
	var count int
	if _, parseErr := fmt.Sscanf(countStr, "%d", &count); parseErr != nil || count <= 0 {
		return 0, 0, fmt.Errorf("router: invalid rate limit count %q in %q", countStr, s)
	}

	unit := "sec"
	if len(parts) > 1 {
		unit = strings.TrimSpace(parts[1])
	}

	switch unit {
	case "s", "sec", "second", "seconds":
		return float64(count), count, nil
	case "m", "min", "minute", "minutes":
		return float64(count) / 60.0, count, nil
	case "h", "hr", "hour", "hours":
		return float64(count) / 3600.0, count, nil
	default:
		return 0, 0, fmt.Errorf("router: unknown rate limit unit %q in %q (expected sec, min, or hour)", unit, s)
	}
}

// TokenBucket implements a thread-safe token bucket rate limiter.
type TokenBucket struct {
	mu         sync.Mutex
	rate       float64 // Tokens per second
	capacity   float64 // Max token capacity (burst)
	tokens     float64
	lastRefill time.Time
}

// NewTokenBucket creates a new TokenBucket instance with specified fill rate and burst capacity.
func NewTokenBucket(ratePerSec float64, burst int) *TokenBucket {
	return &TokenBucket{
		rate:       ratePerSec,
		capacity:   float64(burst),
		tokens:     float64(burst),
		lastRefill: time.Now(),
	}
}

// Allow attempts to consume 1 token from the bucket. Returns true if allowed, or false with retry delay if exhausted.
func (tb *TokenBucket) Allow() (bool, time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	// Refill tokens based on elapsed duration
	tb.tokens = math.Min(tb.capacity, tb.tokens+elapsed*tb.rate)

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true, 0
	}

	// Calculate retry delay required until 1 full token accumulates
	needed := 1.0 - tb.tokens
	retrySeconds := math.Ceil(needed / tb.rate)
	if retrySeconds < 1 {
		retrySeconds = 1
	}
	return false, time.Duration(retrySeconds) * time.Second
}

// RateLimiter manages client token buckets for a route.
type RateLimiter struct {
	mu         sync.RWMutex
	ratePerSec float64
	burst      int
	buckets    map[string]*TokenBucket
}

// NewRateLimiter creates a RateLimiter instance.
func NewRateLimiter(ratePerSec float64, burst int) *RateLimiter {
	return &RateLimiter{
		ratePerSec: ratePerSec,
		burst:      burst,
		buckets:    make(map[string]*TokenBucket),
	}
}

// GetBucket returns or initializes the TokenBucket for a specific client key.
func (rl *RateLimiter) GetBucket(clientKey string) *TokenBucket {
	rl.mu.RLock()
	tb, exists := rl.buckets[clientKey]
	rl.mu.RUnlock()

	if exists {
		return tb
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if tb, exists := rl.buckets[clientKey]; exists {
		return tb
	}

	tb = NewTokenBucket(rl.ratePerSec, rl.burst)
	rl.buckets[clientKey] = tb
	return tb
}

// ExtractClientKey extracts X-API-Key header, Authorization header, or remote IP address from request.
func ExtractClientKey(req *httpparser.Request) string {
	if apiKey := req.Header.Get("X-API-Key"); apiKey != "" {
		return "key:" + strings.TrimSpace(apiKey)
	}
	if auth := req.Header.Get("Authorization"); auth != "" {
		return "auth:" + strings.TrimSpace(auth)
	}
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return "ip:" + strings.TrimSpace(parts[0])
	}
	if req.RawConn != nil {
		if remoteAddr := req.RawConn.RemoteAddr(); remoteAddr != nil {
			host, _, err := net.SplitHostPort(remoteAddr.String())
			if err == nil {
				return "ip:" + host
			}
			return "ip:" + remoteAddr.String()
		}
	}
	return "ip:anonymous"
}

// NewRateLimitMiddleware creates a MiddlewareFunc that enforces route-level token bucket rate limits.
func NewRateLimitMiddleware(rateLimitStr string) (MiddlewareFunc, error) {
	ratePerSec, burst, err := ParseRateLimit(rateLimitStr)
	if err != nil {
		return nil, err
	}

	if ratePerSec <= 0 || burst <= 0 {
		return func(next HandlerFunc) HandlerFunc {
			return next
		}, nil
	}

	limiter := NewRateLimiter(ratePerSec, burst)

	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			clientKey := ExtractClientKey(req)
			bucket := limiter.GetBucket(clientKey)

			allowed, retryAfter := bucket.Allow()
			if !allowed {
				retrySecs := int(math.Ceil(retryAfter.Seconds()))
				if retrySecs < 1 {
					retrySecs = 1
				}
				res.SetStatus(http.StatusTooManyRequests)
				res.Header.Set("Content-Type", "application/json")
				res.Header.Set("Retry-After", fmt.Sprintf("%d", retrySecs))
				_, _ = res.WriteString(fmt.Sprintf(`{"error":"429 Too Many Requests","retry_after":%d}`, retrySecs))
				return
			}

			next(req, res)
		}
	}, nil
}
