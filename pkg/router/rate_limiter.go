package router

import (
	"container/list"
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"toron/pkg/httpparser"
)

const (
	// DefaultMaxBuckets specifies the maximum number of active rate-limiter buckets.
	DefaultMaxBuckets = 10000
	// DefaultIdleTTL specifies the duration of inactivity after which an idle bucket is evicted.
	DefaultIdleTTL = 10 * time.Minute
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

// RateLimiterOptions defines configuration parameters for rate limiter memory bounds and proxy trust.
type RateLimiterOptions struct {
	MaxBuckets     int
	IdleTTL        time.Duration
	TrustedProxies []string
}

type bucketNode struct {
	key          string
	bucket       *TokenBucket
	lastAccessed time.Time
}

// RateLimiter manages client token buckets with bounded LRU memory storage and TTL eviction.
type RateLimiter struct {
	mu             sync.Mutex
	ratePerSec     float64
	burst          int
	maxBuckets     int
	idleTTL        time.Duration
	buckets        map[string]*list.Element
	lruList        *list.List
	trustedProxies []*net.IPNet
	stopCh         chan struct{}
	stopped        bool
}

// NewRateLimiter creates a bounded RateLimiter instance with optional configuration.
func NewRateLimiter(ratePerSec float64, burst int, opts ...RateLimiterOptions) *RateLimiter {
	maxBuckets := DefaultMaxBuckets
	idleTTL := DefaultIdleTTL
	var trustedProxies []*net.IPNet

	if len(opts) > 0 {
		if opts[0].MaxBuckets > 0 {
			maxBuckets = opts[0].MaxBuckets
		}
		if opts[0].IdleTTL > 0 {
			idleTTL = opts[0].IdleTTL
		}
		for _, tp := range opts[0].TrustedProxies {
			tp = strings.TrimSpace(tp)
			if tp == "" {
				continue
			}
			if !strings.Contains(tp, "/") {
				if ip := net.ParseIP(tp); ip != nil {
					if ip.To4() != nil {
						tp += "/32"
					} else {
						tp += "/128"
					}
				}
			}
			_, ipNet, err := net.ParseCIDR(tp)
			if err == nil && ipNet != nil {
				trustedProxies = append(trustedProxies, ipNet)
			}
		}
	}

	rl := &RateLimiter{
		ratePerSec:     ratePerSec,
		burst:          burst,
		maxBuckets:     maxBuckets,
		idleTTL:        idleTTL,
		buckets:        make(map[string]*list.Element),
		lruList:        list.New(),
		trustedProxies: trustedProxies,
		stopCh:         make(chan struct{}),
	}

	go rl.cleanupLoop()

	return rl
}

// Stop terminates the background TTL cleanup goroutine.
func (rl *RateLimiter) Stop() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if !rl.stopped {
		rl.stopped = true
		close(rl.stopCh)
	}
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.CleanupStale()
		}
	}
}

// CleanupStale manually purges idle buckets that have exceeded IdleTTL.
func (rl *RateLimiter) CleanupStale() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for rl.lruList.Len() > 0 {
		oldest := rl.lruList.Back()
		if oldest == nil {
			break
		}
		node := oldest.Value.(*bucketNode)
		if now.Sub(node.lastAccessed) > rl.idleTTL {
			rl.lruList.Remove(oldest)
			delete(rl.buckets, node.key)
		} else {
			break
		}
	}
}

// Len returns the current number of active rate-limiting buckets in memory.
func (rl *RateLimiter) Len() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.lruList.Len()
}

// GetBucket returns or initializes the TokenBucket for a specific client key using LRU eviction.
func (rl *RateLimiter) GetBucket(clientKey string) *TokenBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Opportunistic eviction of stale entries from tail of LRU
	for rl.lruList.Len() > 0 {
		oldest := rl.lruList.Back()
		if oldest == nil {
			break
		}
		node := oldest.Value.(*bucketNode)
		if now.Sub(node.lastAccessed) > rl.idleTTL {
			rl.lruList.Remove(oldest)
			delete(rl.buckets, node.key)
		} else {
			break
		}
	}

	if elem, exists := rl.buckets[clientKey]; exists {
		rl.lruList.MoveToFront(elem)
		node := elem.Value.(*bucketNode)
		node.lastAccessed = now
		return node.bucket
	}

	// Capacity enforcement: evict oldest entry if capacity is reached
	if rl.lruList.Len() >= rl.maxBuckets {
		if oldest := rl.lruList.Back(); oldest != nil {
			node := oldest.Value.(*bucketNode)
			rl.lruList.Remove(oldest)
			delete(rl.buckets, node.key)
		}
	}

	node := &bucketNode{
		key:          clientKey,
		bucket:       NewTokenBucket(rl.ratePerSec, rl.burst),
		lastAccessed: now,
	}
	elem := rl.lruList.PushFront(node)
	rl.buckets[clientKey] = elem
	return node.bucket
}

// ExtractClientKey extracts the client identity key using the limiter's configured trusted proxies.
func (rl *RateLimiter) ExtractClientKey(req *httpparser.Request) string {
	return extractClientKeyInternal(req, rl.trustedProxies)
}

// ExtractClientKey extracts client identity key from request.
func ExtractClientKey(req *httpparser.Request) string {
	return extractClientKeyInternal(req, nil)
}

func extractClientKeyInternal(req *httpparser.Request, trustedProxies []*net.IPNet) string {
	var socketIP net.IP
	var socketHost string
	if req.RawConn != nil {
		if remoteAddr := req.RawConn.RemoteAddr(); remoteAddr != nil {
			raw := remoteAddr.String()
			if host, _, err := net.SplitHostPort(raw); err == nil {
				socketHost = host
				socketIP = net.ParseIP(host)
			} else {
				socketHost = raw
				socketIP = net.ParseIP(raw)
			}
		}
	}

	// Check if peer is in trusted proxies
	peerIsTrusted := false
	if socketIP != nil && len(trustedProxies) > 0 {
		for _, tp := range trustedProxies {
			if tp.Contains(socketIP) {
				peerIsTrusted = true
				break
			}
		}
	}

	// If connection is from a trusted proxy or if synthetic request (no socket), honor forwarded/client headers
	if peerIsTrusted || req.RawConn == nil {
		if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return "ip:" + strings.TrimSpace(parts[0])
		}
		if apiKey := req.Header.Get("X-API-Key"); apiKey != "" {
			return "key:" + strings.TrimSpace(apiKey)
		}
		if auth := req.Header.Get("Authorization"); auth != "" {
			return "auth:" + strings.TrimSpace(auth)
		}
	}

	// If physical socket is present and untrusted, anchor strictly to socket IP
	if socketHost != "" {
		return "ip:" + socketHost
	}

	// Fallbacks for headless/synthetic testing requests
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

	return "ip:anonymous"
}

// NewRateLimitMiddleware creates a MiddlewareFunc that enforces route-level token bucket rate limits.
func NewRateLimitMiddleware(rateLimitStr string, opts ...RateLimiterOptions) (MiddlewareFunc, error) {
	ratePerSec, burst, err := ParseRateLimit(rateLimitStr)
	if err != nil {
		return nil, err
	}

	if ratePerSec <= 0 || burst <= 0 {
		return func(next HandlerFunc) HandlerFunc {
			return next
		}, nil
	}

	limiter := NewRateLimiter(ratePerSec, burst, opts...)

	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			clientKey := limiter.ExtractClientKey(req)
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
