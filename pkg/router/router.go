package router

import (
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/metrics"
	"toron/pkg/proxy"
	"toron/pkg/waf"
)

// RouteType specifies whether a route handler serves static site assets or proxies requests upstream.
type RouteType string

const (
	RouteTypeUpstream RouteType = "upstream"
	RouteTypeStatic   RouteType = "static"
)

// HandlerFunc describes an HTTP request handler function in Toron.
type HandlerFunc func(req *httpparser.Request, res *httpparser.Response)

// MiddlewareFunc describes middleware wrapping a HandlerFunc.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

type routeEntry struct {
	host    string
	headers map[string]string
	handler HandlerFunc
}

type prefixRoute struct {
	host    string
	prefix  string
	headers map[string]string
	handler HandlerFunc
}

// Router handles URL routing, method dispatching, domain matching, header-based routing, static file serving, reverse proxying, and middleware execution.
type Router struct {
	mu               sync.RWMutex
	routes           map[string]map[string][]routeEntry // path -> method -> []routeEntry
	prefixRoutes     []prefixRoute
	middlewares      []MiddlewareFunc
	NotFound         HandlerFunc
	MethodNotAllowed HandlerFunc
}

// New creates a new Router instance with default 404/405 handlers.
func New() *Router {
	r := &Router{
		routes: make(map[string]map[string][]routeEntry),
	}

	r.NotFound = func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusNotFound)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"error":"404 Not Found"}`)
	}

	r.MethodNotAllowed = func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusMethodNotAllowed)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"error":"405 Method Not Allowed"}`)
	}

	r.GET("/metrics", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = res.WriteString(metrics.DefaultRegistry.ExportPrometheus())
	})

	return r
}

// Use attaches one or more global middlewares to the router execution chain.
func (r *Router) Use(mw ...MiddlewareFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middlewares = append(r.middlewares, mw...)
}

// Reset clears all registered exact and prefix routes while preserving global middlewares and 404/405 handlers.
func (r *Router) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = make(map[string]map[string][]routeEntry)
	r.prefixRoutes = nil
}

// Handle registers a handler for a specific HTTP method and exact path.
func (r *Router) Handle(method, path string, handler HandlerFunc) {
	r.HandleHostHeader(method, "", path, nil, handler)
}

// HandleHeader registers a handler conditional on matching specific HTTP header key/value pairs.
func (r *Router) HandleHeader(method, path string, headers map[string]string, handler HandlerFunc) {
	r.HandleHostHeader(method, "", path, headers, handler)
}

// HandleHostHeader registers a handler conditional on domain Host and specific HTTP headers.
func (r *Router) HandleHostHeader(method, host, path string, headers map[string]string, handler HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.routes[path]; !exists {
		r.routes[path] = make(map[string][]routeEntry)
	}
	r.routes[path][method] = append(r.routes[path][method], routeEntry{
		host:    strings.ToLower(strings.TrimSpace(host)),
		headers: headers,
		handler: handler,
	})
}

// GET convenience helper.
func (r *Router) GET(path string, handler HandlerFunc) {
	r.Handle("GET", path, handler)
}

// GETHost registers a GET route matching a domain host.
func (r *Router) GETHost(host, path string, handler HandlerFunc) {
	r.HandleHostHeader("GET", host, path, nil, handler)
}

// GETHeader registers a GET route conditional on matching an HTTP header.
func (r *Router) GETHeader(path, headerKey, headerVal string, handler HandlerFunc) {
	r.HandleHeader("GET", path, map[string]string{headerKey: headerVal}, handler)
}

// POST convenience helper.
func (r *Router) POST(path string, handler HandlerFunc) {
	r.Handle("POST", path, handler)
}

// POSTHost registers a POST route matching a domain host.
func (r *Router) POSTHost(host, path string, handler HandlerFunc) {
	r.HandleHostHeader("POST", host, path, nil, handler)
}

// POSTHeader registers a POST route conditional on matching an HTTP header.
func (r *Router) POSTHeader(path, headerKey, headerVal string, handler HandlerFunc) {
	r.HandleHeader("POST", path, map[string]string{headerKey: headerVal}, handler)
}

// RoutePrefix registers a prefix route that acts either as a static file server or an upstream reverse proxy, matching optional host and headers.
func (r *Router) RoutePrefix(targetType RouteType, host, prefix string, headers map[string]string, dirPath string, opts proxy.ProxyOptions) error {
	cleanPrefix := "/" + strings.Trim(prefix, "/")
	if cleanPrefix == "/" {
		cleanPrefix = ""
	}

	normType := RouteType(strings.ToLower(strings.TrimSpace(string(targetType))))
	if normType == "proxy" {
		normType = RouteTypeUpstream
	}

	var handler HandlerFunc

	if normType == RouteTypeStatic {
		if dirPath == "" {
			return fmt.Errorf("router: static route for prefix %q requires non-empty dirPath", prefix)
		}
		absDir, err := filepath.Abs(dirPath)
		if err != nil {
			absDir = dirPath
		}
		handler = r.createStaticHandler(cleanPrefix, absDir)
	} else if normType == RouteTypeUpstream {
		px, err := proxy.NewProxyWithOptions(opts)
		if err != nil {
			return err
		}
		handler = func(req *httpparser.Request, res *httpparser.Response) {
			px.ServeHTTPWithPrefix(req, res, cleanPrefix)
		}
	} else {
		return fmt.Errorf("router: invalid route type %q (must be 'static' or 'upstream')", targetType)
	}

	if strings.TrimSpace(opts.RateLimit) != "" {
		rlMw, err := NewRateLimitMiddleware(opts.RateLimit)
		if err != nil {
			return fmt.Errorf("router: invalid rate_limit %q for prefix %q: %w", opts.RateLimit, prefix, err)
		}
		handler = rlMw(handler)
	}

	if opts.Auth != nil {
		if authCfg, ok := opts.Auth.(AuthConfig); ok && authCfg.Type != "" {
			handler = NewAuthMiddleware(authCfg)(handler)
		}
	}

	if opts.WAF != nil {
		if wafCfg, ok := opts.WAF.(waf.WAFConfig); ok && (wafCfg.Enabled || len(wafCfg.AllowedIPs) > 0 || len(wafCfg.DeniedIPs) > 0 || len(wafCfg.DisabledRules) > 0 || wafCfg.Mode != "") {
			if routeWafEngine, err := waf.NewEngine(wafCfg); err == nil {
				nextHandler := handler
				wafMw := waf.NewWAFMiddleware(routeWafEngine)
				handler = func(req *httpparser.Request, res *httpparser.Response) {
					wafMw(waf.HandlerFunc(nextHandler))(req, res)
				}
			}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefixRoutes = append(r.prefixRoutes, prefixRoute{
		host:    strings.ToLower(strings.TrimSpace(host)),
		prefix:  cleanPrefix,
		headers: headers,
		handler: handler,
	})
	return nil
}

// Proxy registers a URL prefix to reverse proxy incoming requests to an upstream target URL string.
func (r *Router) Proxy(prefix, targetURLStr string) error {
	return r.ProxyHeaders(prefix, nil, targetURLStr)
}

// ProxyHeader registers a prefix reverse proxy route conditional on matching an HTTP header.
func (r *Router) ProxyHeader(prefix, headerKey, headerVal, targetURLStr string) error {
	return r.ProxyHeaders(prefix, map[string]string{headerKey: headerVal}, targetURLStr)
}

// ProxyHeaders registers a prefix reverse proxy route conditional on matching multiple HTTP headers.
func (r *Router) ProxyHeaders(prefix string, headers map[string]string, targetURLStr string) error {
	return r.ProxyBalancerHeaders(prefix, headers, []string{targetURLStr}, proxy.AlgorithmRoundRobin)
}

// ProxyBalancer registers a prefix reverse proxy route load balancing across multiple upstream target URL strings.
func (r *Router) ProxyBalancer(prefix string, targets []string, algo proxy.Algorithm) error {
	return r.ProxyBalancerHeaders(prefix, nil, targets, algo)
}

// ProxyBalancerHeaders registers a prefix reverse proxy route load balancing across multiple upstream targets with header matching.
func (r *Router) ProxyBalancerHeaders(prefix string, headers map[string]string, targets []string, algo proxy.Algorithm) error {
	return r.ProxyWithOptions("", prefix, headers, proxy.ProxyOptions{
		Targets:   targets,
		Algorithm: algo,
		Timeout:   10 * time.Second,
	})
}

// ProxyWithOptions registers a prefix reverse proxy route using domain host and full ProxyOptions (health check, circuit breaker).
func (r *Router) ProxyWithOptions(host, prefix string, headers map[string]string, opts proxy.ProxyOptions) error {
	return r.RoutePrefix(RouteTypeUpstream, host, prefix, headers, "", opts)
}

// Static registers a URL prefix to serve static files from a local directory path.
func (r *Router) Static(prefix, dirPath string) {
	_ = r.RoutePrefix(RouteTypeStatic, "", prefix, nil, dirPath, proxy.ProxyOptions{})
}

// StaticWithOptions registers a URL prefix to serve static files with host and header matching options.
func (r *Router) StaticWithOptions(host, prefix string, headers map[string]string, dirPath string) error {
	return r.RoutePrefix(RouteTypeStatic, host, prefix, headers, dirPath, proxy.ProxyOptions{})
}

func (r *Router) createStaticHandler(cleanPrefix, absDir string) HandlerFunc {
	return func(req *httpparser.Request, res *httpparser.Response) {
		if req.Method != "GET" && req.Method != "HEAD" {
			r.MethodNotAllowed(req, res)
			return
		}

		relPath := req.Path
		if cleanPrefix != "" {
			if req.Path == cleanPrefix {
				res.SetStatus(http.StatusFound)
				res.Header.Set("Location", cleanPrefix+"/")
				return
			}
			relPath = strings.TrimPrefix(req.Path, cleanPrefix)
		}
		if relPath == "" || relPath == "/" {
			relPath = "/index.html"
		}

		// Security: Prevent path traversal
		cleanRel := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(relPath, "/")))
		targetPath := filepath.Join(absDir, cleanRel)

		relFromDir, err := filepath.Rel(absDir, targetPath)
		if err != nil || strings.HasPrefix(relFromDir, "..") || strings.HasPrefix(relFromDir, ".") && len(relFromDir) > 1 && relFromDir[1] == '.' {
			res.SetStatus(http.StatusForbidden)
			res.Header.Set("Content-Type", "application/json")
			_, _ = res.WriteString(`{"error":"403 Forbidden: Path Traversal Disallowed"}`)
			return
		}

		fileInfo, err := os.Stat(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				r.NotFound(req, res)
				return
			}
			res.SetStatus(http.StatusInternalServerError)
			return
		}

		if fileInfo.IsDir() {
			targetPath = filepath.Join(targetPath, "index.html")
			fileInfo, err = os.Stat(targetPath)
			if err != nil || fileInfo.IsDir() {
				r.NotFound(req, res)
				return
			}
		}

		data, err := os.ReadFile(targetPath)
		if err != nil {
			r.NotFound(req, res)
			return
		}

		// Detect Content-Type
		ext := filepath.Ext(targetPath)
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			switch ext {
			case ".html", ".htm":
				mimeType = "text/html; charset=utf-8"
			case ".css":
				mimeType = "text/css; charset=utf-8"
			case ".js":
				mimeType = "application/javascript; charset=utf-8"
			case ".json":
				mimeType = "application/json; charset=utf-8"
			case ".png":
				mimeType = "image/png"
			case ".jpg", ".jpeg":
				mimeType = "image/jpeg"
			case ".svg":
				mimeType = "image/svg+xml"
			default:
				mimeType = "application/octet-stream"
			}
		}

		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", mimeType)
		if req.Method != "HEAD" {
			_, _ = res.Write(data)
		}
	}
}

// ServeHTTP dispatches the request to registered handlers through the middleware chain.
func (r *Router) ServeHTTP(req *httpparser.Request, res *httpparser.Response) {
	r.mu.RLock()
	var targetHandler HandlerFunc

	reqHost := extractHost(req)

	methodsMap, pathExists := r.routes[req.Path]
	if pathExists {
		entries, methodExists := methodsMap[req.Method]
		if methodExists {
			var fallbackEntry *routeEntry
			for i := range entries {
				if headersAndHostMatch(reqHost, req, entries[i].host, entries[i].headers) {
					targetHandler = entries[i].handler
					break
				}
				if entries[i].host == "" && len(entries[i].headers) == 0 {
					fallbackEntry = &entries[i]
				}
			}
			if targetHandler == nil && fallbackEntry != nil {
				targetHandler = fallbackEntry.handler
			}
			if targetHandler == nil && len(entries) > 0 {
				targetHandler = entries[0].handler
			}
		} else {
			targetHandler = r.MethodNotAllowed
		}
	}

	if targetHandler == nil {
		// Check prefix routes (static file or upstream reverse proxy routes)
		var fallbackPrefix *prefixRoute
		for i := range r.prefixRoutes {
			pr := &r.prefixRoutes[i]
			if pr.prefix == "" || strings.HasPrefix(req.Path, pr.prefix+"/") || req.Path == pr.prefix {
				if headersAndHostMatch(reqHost, req, pr.host, pr.headers) {
					targetHandler = pr.handler
					break
				}
				if pr.host == "" && len(pr.headers) == 0 && fallbackPrefix == nil {
					fallbackPrefix = pr
				}
			}
		}
		if targetHandler == nil && fallbackPrefix != nil {
			targetHandler = fallbackPrefix.handler
		}
		if targetHandler == nil {
			targetHandler = r.NotFound
		}
	}

	middlewares := append([]MiddlewareFunc(nil), r.middlewares...)
	r.mu.RUnlock()

	// Chain middlewares in reverse order
	finalChain := targetHandler
	for i := len(middlewares) - 1; i >= 0; i-- {
		finalChain = middlewares[i](finalChain)
	}

	start := time.Now()
	finalChain(req, res)
	metrics.DefaultRegistry.RecordRequest(req.Method, fmt.Sprintf("%d", res.StatusCode), req.Path, time.Since(start).Seconds())
}

func extractHost(req *httpparser.Request) string {
	h := req.Header.Get("Host")
	if h == "" {
		return ""
	}
	if idx := strings.Index(h, ":"); idx != -1 {
		h = h[:idx]
	}
	return strings.ToLower(strings.TrimSpace(h))
}

func headersAndHostMatch(reqHost string, req *httpparser.Request, routeHost string, expectedHeaders map[string]string) bool {
	if routeHost != "" {
		if reqHost != strings.ToLower(routeHost) {
			return false
		}
	}
	for k, expectedVal := range expectedHeaders {
		actualVal := req.Header.Get(k)
		if actualVal != expectedVal {
			return false
		}
	}
	return true
}

// LoggerMiddleware logs incoming requests and processing duration.
func LoggerMiddleware() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			start := time.Now()
			next(req, res)
			log.Printf("[TORON] %s %s -> %d (%v)", req.Method, req.Path, res.StatusCode, time.Since(start))
		}
	}
}

// RecoveryMiddleware captures panics and converts them to HTTP 500 Internal Server Error.
func RecoveryMiddleware() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[RECOVERY] Panic recovered: %v", r)
					res.SetStatus(http.StatusInternalServerError)
					res.Header.Set("Content-Type", "application/json")
					res.Body.Reset()
					_, _ = res.WriteString(fmt.Sprintf(`{"error":"500 Internal Server Error"}`))
				}
			}()
			next(req, res)
		}
	}
}
