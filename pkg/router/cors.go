package router

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"toron/pkg/httpparser"
)

// CORSConfig defines Cross-Origin Resource Sharing settings.
type CORSConfig struct {
	Enabled          bool     `yaml:"enabled" json:"enabled"`
	AllowOrigins     []string `yaml:"allow_origins" json:"allow_origins"`
	AllowMethods     []string `yaml:"allow_methods" json:"allow_methods"`
	AllowHeaders     []string `yaml:"allow_headers" json:"allow_headers"`
	ExposeHeaders    []string `yaml:"expose_headers" json:"expose_headers"`
	AllowCredentials bool     `yaml:"allow_credentials" json:"allow_credentials"`
	MaxAge           int      `yaml:"max_age" json:"max_age"`
}

// isOriginAllowed checks if an incoming origin matches the configured allowed origins.
func isOriginAllowed(origin string, allowOrigins []string) bool {
	if len(allowOrigins) == 0 {
		return false
	}
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}

	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	originHost := strings.ToLower(u.Host)
	if h, _, err := net.SplitHostPort(originHost); err == nil {
		originHost = h
	}

	for _, pattern := range allowOrigins {
		pattern = strings.TrimSpace(pattern)
		if pattern == "*" {
			return true
		}
		if strings.EqualFold(origin, pattern) {
			return true
		}
		if strings.Contains(pattern, "*") {
			patURL, err := url.Parse(pattern)
			if err == nil && patURL.Scheme != "" && patURL.Host != "" {
				if !strings.EqualFold(u.Scheme, patURL.Scheme) {
					continue
				}
				patHost := strings.ToLower(patURL.Host)
				if h, _, err := net.SplitHostPort(patHost); err == nil {
					patHost = h
				}
				if strings.HasPrefix(patHost, "*.") {
					domainSuffix := patHost[1:] // e.g. ".example.com"
					if strings.HasSuffix(originHost, domainSuffix) || originHost == patHost[2:] {
						return true
					}
				}
			}
		}
	}
	return false
}

func appendVaryHeader(res *httpparser.Response, val string) {
	vary := res.Header.Get("Vary")
	if vary == "" {
		res.Header.Set("Vary", val)
	} else if !strings.Contains(vary, val) {
		res.Header.Set("Vary", vary+", "+val)
	}
}

// NewCORSMiddleware constructs a high-performance CORS middleware handling preflights and header injection.
func NewCORSMiddleware(cfg CORSConfig) MiddlewareFunc {
	methodsStr := "GET, POST, PUT, DELETE, OPTIONS, HEAD, PATCH"
	if len(cfg.AllowMethods) > 0 {
		methodsStr = strings.Join(cfg.AllowMethods, ", ")
	}
	headersStr := "Origin, Content-Type, Accept, Authorization, X-Requested-With"
	if len(cfg.AllowHeaders) > 0 {
		headersStr = strings.Join(cfg.AllowHeaders, ", ")
	}
	exposeStr := ""
	if len(cfg.ExposeHeaders) > 0 {
		exposeStr = strings.Join(cfg.ExposeHeaders, ", ")
	}
	maxAgeStr := ""
	if cfg.MaxAge > 0 {
		maxAgeStr = strconv.Itoa(cfg.MaxAge)
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			origin := req.Header.Get("Origin")
			if origin == "" {
				next(req, res)
				return
			}

			allowed := isOriginAllowed(origin, cfg.AllowOrigins)

			// Handle Preflight OPTIONS request
			if req.Method == "OPTIONS" && req.Header.Get("Access-Control-Request-Method") != "" {
				if !allowed {
					res.SetStatus(http.StatusForbidden)
					res.Header.Set("Content-Type", "application/json")
					res.Body.Reset()
					_, _ = res.WriteString(`{"error":"403 Forbidden","message":"CORS origin not allowed"}`)
					return
				}

				if cfg.AllowCredentials {
					res.Header.Set("Access-Control-Allow-Origin", origin)
					res.Header.Set("Access-Control-Allow-Credentials", "true")
				} else if len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*" {
					res.Header.Set("Access-Control-Allow-Origin", "*")
				} else {
					res.Header.Set("Access-Control-Allow-Origin", origin)
				}

				res.Header.Set("Access-Control-Allow-Methods", methodsStr)
				res.Header.Set("Access-Control-Allow-Headers", headersStr)
				if exposeStr != "" {
					res.Header.Set("Access-Control-Expose-Headers", exposeStr)
				}
				if maxAgeStr != "" {
					res.Header.Set("Access-Control-Max-Age", maxAgeStr)
				}
				appendVaryHeader(res, "Origin")

				res.SetStatus(http.StatusNoContent)
				return
			}

			// Actual Request (GET, POST, etc.)
			if allowed {
				if cfg.AllowCredentials {
					res.Header.Set("Access-Control-Allow-Origin", origin)
					res.Header.Set("Access-Control-Allow-Credentials", "true")
				} else if len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*" {
					res.Header.Set("Access-Control-Allow-Origin", "*")
				} else {
					res.Header.Set("Access-Control-Allow-Origin", origin)
				}

				if exposeStr != "" {
					res.Header.Set("Access-Control-Expose-Headers", exposeStr)
				}
				appendVaryHeader(res, "Origin")
			}

			next(req, res)
		}
	}
}
