package router

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"toron/pkg/httpparser"
)

// CacheConfig defines configuration for the in-memory response cache middleware.
type CacheConfig struct {
	Enabled        bool          `yaml:"enabled" json:"enabled"`
	DefaultTTL     time.Duration `yaml:"default_ttl" json:"default_ttl"`
	MaxEntries     int           `yaml:"max_entries" json:"max_entries"`
	MaxPayloadSize int           `yaml:"max_payload_size" json:"max_payload_size"`
}

// DefaultCacheConfig returns sensible production defaults for response caching.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled:        true,
		DefaultTTL:     60 * time.Second,
		MaxEntries:     1000,
		MaxPayloadSize: 1024 * 1024, // 1 MB
	}
}

// CachedResponse represents a frozen snapshot of an HTTP response stored in memory.
type CachedResponse struct {
	StatusCode int
	Header     httpparser.Header
	Body       []byte
	CachedAt   time.Time
	ExpiresAt  time.Time
	Public     bool
}

// IsExpired checks if the cached response has exceeded its time-to-live.
func (c *CachedResponse) IsExpired(now time.Time) bool {
	return now.After(c.ExpiresAt)
}

// ResponseCache is a thread-safe in-memory response cache store.
type ResponseCache struct {
	mu      sync.RWMutex
	entries map[string]*CachedResponse
	config  CacheConfig
}

// NewResponseCache initializes a ResponseCache with the given configuration.
func NewResponseCache(cfg CacheConfig) *ResponseCache {
	if cfg.DefaultTTL <= 0 {
		cfg.DefaultTTL = 60 * time.Second
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 1000
	}
	if cfg.MaxPayloadSize <= 0 {
		cfg.MaxPayloadSize = 1024 * 1024
	}

	return &ResponseCache{
		entries: make(map[string]*CachedResponse),
		config:  cfg,
	}
}

// Get retrieves a non-expired cached response by key.
func (rc *ResponseCache) Get(key string, now time.Time) (*CachedResponse, bool) {
	rc.mu.RLock()
	entry, exists := rc.entries[key]
	rc.mu.RUnlock()

	if !exists {
		return nil, false
	}

	if entry.IsExpired(now) {
		rc.mu.Lock()
		delete(rc.entries, key)
		rc.mu.Unlock()
		return nil, false
	}

	return entry, true
}

// Set stores a cached response snapshot, evicting expired entries if capacity is reached.
func (rc *ResponseCache) Set(key string, entry *CachedResponse) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if len(rc.entries) >= rc.config.MaxEntries {
		now := time.Now()
		for k, v := range rc.entries {
			if v.IsExpired(now) {
				delete(rc.entries, k)
			}
		}
		if len(rc.entries) >= rc.config.MaxEntries {
			for k := range rc.entries {
				delete(rc.entries, k)
				break
			}
		}
	}

	rc.entries[key] = entry
}

// Clear flushes all cached entries from memory.
func (rc *ResponseCache) Clear() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.entries = make(map[string]*CachedResponse)
}

// CacheControlDirectives represents parsed RFC 7234 Cache-Control header tokens.
type CacheControlDirectives struct {
	NoStore bool
	NoCache bool
	Private bool
	Public  bool
	MaxAge  *time.Duration
}

// ParseCacheControl extracts RFC 7234 directives from a Cache-Control header string.
func ParseCacheControl(header string) CacheControlDirectives {
	var d CacheControlDirectives
	if strings.TrimSpace(header) == "" {
		return d
	}

	parts := strings.Split(header, ",")
	for _, part := range parts {
		item := strings.TrimSpace(strings.ToLower(part))
		switch {
		case item == "no-store":
			d.NoStore = true
		case item == "no-cache":
			d.NoCache = true
		case item == "private":
			d.Private = true
		case item == "public":
			d.Public = true
		case strings.HasPrefix(item, "max-age="):
			secStr := strings.TrimPrefix(item, "max-age=")
			if sec, err := strconv.Atoi(strings.TrimSpace(secStr)); err == nil && sec >= 0 {
				dur := time.Duration(sec) * time.Second
				d.MaxAge = &dur
			}
		}
	}
	return d
}

// NewCacheMiddleware creates an HTTP response caching middleware honoring RFC 7234 rules.
func NewCacheMiddleware(cfg CacheConfig) MiddlewareFunc {
	cache := NewResponseCache(cfg)

	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			if !cfg.Enabled {
				next(req, res)
				return
			}

			// Only idempotent GET and HEAD requests are eligible for caching
			if req.Method != "GET" && req.Method != "HEAD" {
				next(req, res)
				return
			}

			// Check client cache bypass request headers
			clientCC := ParseCacheControl(req.Header.Get("Cache-Control"))
			clientPragma := strings.ToLower(req.Header.Get("Pragma"))
			clientBypass := clientCC.NoCache || clientCC.NoStore || (clientCC.MaxAge != nil && *clientCC.MaxAge == 0) || strings.Contains(clientPragma, "no-cache")

			hasAuth := req.Header.Get("Authorization") != ""

			uri := req.RequestURI
			if uri == "" {
				uri = req.Path
			}
			cacheKey := req.Method + ":" + extractHost(req) + ":" + uri
			if ae := req.Header.Get("Accept-Encoding"); ae != "" {
				cacheKey += ":ae=" + ae
			}
			now := time.Now()

			if !clientBypass {
				if cached, hit := cache.Get(cacheKey, now); hit {
					// RFC 7234 §3.2: Requests with Authorization header cannot be satisfied
					// from shared cache unless the cached response is explicitly public.
					if !hasAuth || cached.Public {
						res.SetStatus(cached.StatusCode)
						for k, vals := range cached.Header {
							for _, v := range vals {
								res.Header.Set(k, v)
							}
						}
						// Ensure Set-Cookie is never emitted from shared cache
						res.Header.Del("Set-Cookie")
						res.Header.Del("Set-Cookie2")

						res.Header.Set("X-Cache", "HIT")
						ageSec := int(now.Sub(cached.CachedAt).Seconds())
						if ageSec < 0 {
							ageSec = 0
						}
						res.Header.Set("Age", strconv.Itoa(ageSec))

						res.Body.Reset()
						_, _ = res.Body.Write(cached.Body)
						res.Header.Set("Content-Length", strconv.Itoa(res.Body.Len()))
						return
					}
				}
			}

			// Execute downstream handler
			next(req, res)

			res.Header.Set("X-Cache", "MISS")

			// WebSocket upgrades, raw upgraded socket tunnels, or live streaming responses must never be cached
			if res.StatusCode == http.StatusSwitchingProtocols || res.UpgradedConn != nil || res.StreamBody != nil {
				return
			}

			// Streaming MIME or unbuffered responses must never be cached (REQ-128)
			if strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "text/event-stream") || strings.EqualFold(res.Header.Get("X-Accel-Buffering"), "no") {
				return
			}

			// Only cache standard successful or permanent redirection/not-found status codes
			if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusMovedPermanently && res.StatusCode != http.StatusNotFound {
				return
			}

			// Payload size validation
			if res.Body.Len() > cfg.MaxPayloadSize || res.Body.Len() == 0 {
				return
			}

			// Evaluate response Cache-Control headers
			resCC := ParseCacheControl(res.Header.Get("Cache-Control"))
			if resCC.NoStore || resCC.Private {
				return
			}

			// RFC 7234 §3.2: Shared cache must not store response to request with Authorization
			// unless explicitly marked public.
			if hasAuth && !resCC.Public {
				return
			}

			// Evaluate Vary header: if Vary is * or contains dimensions other than Accept-Encoding, bypass caching
			if vary := res.Header.Get("Vary"); vary != "" {
				if strings.TrimSpace(vary) == "*" {
					return
				}
				for _, part := range strings.Split(vary, ",") {
					item := strings.TrimSpace(strings.ToLower(part))
					if item != "" && item != "accept-encoding" {
						return
					}
				}
			}

			// Determine expiration duration
			ttl := cfg.DefaultTTL
			if resCC.MaxAge != nil {
				ttl = *resCC.MaxAge
			}

			if ttl <= 0 {
				return
			}

			// Clone headers for cached snapshot, explicitly stripping Set-Cookie, Set-Cookie2, Age, X-Cache, Connection
			clonedHeader := make(httpparser.Header)
			for k, vals := range res.Header {
				if strings.EqualFold(k, "X-Cache") || strings.EqualFold(k, "Age") ||
					strings.EqualFold(k, "Set-Cookie") || strings.EqualFold(k, "Set-Cookie2") ||
					strings.EqualFold(k, "Connection") {
					continue
				}
				clonedVals := make([]string, len(vals))
				copy(clonedVals, vals)
				clonedHeader[k] = clonedVals
			}

			bodySnapshot := make([]byte, res.Body.Len())
			copy(bodySnapshot, res.Body.Bytes())

			cache.Set(cacheKey, &CachedResponse{
				StatusCode: res.StatusCode,
				Header:     clonedHeader,
				Body:       bodySnapshot,
				CachedAt:   now,
				ExpiresAt:  now.Add(ttl),
				Public:     resCC.Public,
			})
		}
	}
}
