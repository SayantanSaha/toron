package router

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/logging"
	"toron/pkg/metrics"
	"toron/pkg/proxy"
	"toron/pkg/waf"
)

// RouterContextKeyType is the private type for router context keys.
type RouterContextKeyType string

const (
	MatchedRouteContextKey      RouterContextKeyType = "toron.matched_route"
	RouteTypeContextKey         RouterContextKeyType = "toron.route_type"
	DestinationContextKey       RouterContextKeyType = "toron.destination"
	RouteSlotAcquiredContextKey RouterContextKeyType = "toron.route_slot_acquired"
)

// GetMatchedRoute extracts the matched route prefix/identifier from ctx.
func GetMatchedRoute(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(MatchedRouteContextKey).(string); ok {
		return v
	}
	return ""
}

// SetMatchedRoute stores the matched route prefix in the request context.
func SetMatchedRoute(req *httpparser.Request, prefix string) {
	if req != nil {
		req.SetContext(context.WithValue(req.Context(), MatchedRouteContextKey, prefix))
	}
}

// GetRouteType extracts the matched route type ("upstream", "static", "exact") from ctx.
func GetRouteType(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(RouteTypeContextKey).(string); ok {
		return v
	}
	return ""
}

// SetRouteType stores the matched route type in the request context.
func SetRouteType(req *httpparser.Request, routeType string) {
	if req != nil {
		req.SetContext(context.WithValue(req.Context(), RouteTypeContextKey, routeType))
	}
}

// GetDestination extracts the destination label (e.g. "static", "in-process") from ctx.
func GetDestination(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(DestinationContextKey).(string); ok {
		return v
	}
	return ""
}

// SetDestination stores the destination label in the request context.
func SetDestination(req *httpparser.Request, dest string) {
	if req != nil {
		req.SetContext(context.WithValue(req.Context(), DestinationContextKey, dest))
	}
}

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
	source                string
	proxy                 *proxy.ReverseProxy
	routeType             string
	method                string
	host                  string
	prefix                string
	matcher               func(path string) bool
	headers               map[string]string
	redirectHTTP          *bool
	accessLog             string
	securityLog           string
	handler               HandlerFunc
	maxConcurrency        int
	activeRequests        int64
	maxBodyBytes          int64
	readTimeout           time.Duration
	writeTimeout          time.Duration
	responseHeaderTimeout time.Duration
	streamRequestBody     *bool
}

// Router handles URL routing, method dispatching, domain matching, header-based routing, static file serving, reverse proxying, and middleware execution.
type Router struct {
	mu               sync.RWMutex
	routes           map[string]map[string][]routeEntry // path -> method -> []routeEntry
	prefixRoutes     []prefixRoute
	middlewares      []MiddlewareFunc
	workerPoolSize   int
	NotFound         HandlerFunc
	MethodNotAllowed HandlerFunc
}

// New creates a new Router instance with default 404/405 handlers.
func New() *Router {
	r := &Router{
		routes:         make(map[string]map[string][]routeEntry),
		workerPoolSize: 128,
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
	evicted := r.prefixRoutes
	r.routes = make(map[string]map[string][]routeEntry)
	r.prefixRoutes = nil
	r.mu.Unlock()

	for _, pr := range evicted {
		if pr.proxy != nil {
			pr.proxy.Close()
		}
	}
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

// SetWorkerPoolSize configures the worker pool size used for automatic bulkhead guardrails.
func (r *Router) SetWorkerPoolSize(size int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if size > 0 {
		r.workerPoolSize = size
	}
}

// WorkerPoolSize returns the configured worker pool size.
func (r *Router) WorkerPoolSize() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.workerPoolSize <= 0 {
		return 128
	}
	return r.workerPoolSize
}

// PrefixRouteSpec defines a declarative configuration for registering or replacing a prefix route.
type PrefixRouteSpec struct {
	TargetType            RouteType
	RouteType             RouteType
	Host                  string
	Prefix                string
	Method                string
	Headers               map[string]string
	DirPath               string
	Opts                  proxy.ProxyOptions
	Handler               HandlerFunc
	MaxConcurrency        int
	MaxBodyBytes          int64
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	ResponseHeaderTimeout time.Duration
	StreamRequestBody     *bool
}

// CanonicalHeaderString formats headers map into a deterministic sorted query-like string (k1=v1&k2=v2).
func CanonicalHeaderString(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(headers[k])
	}
	return sb.String()
}

func comparePrefixRoutes(a, b prefixRoute) int {
	// Tier 1: Prefix Length (Longest prefix first)
	if len(a.prefix) != len(b.prefix) {
		if len(a.prefix) > len(b.prefix) {
			return -1
		}
		return 1
	}

	// Tier 2: Host Specificity (Explicit domain host before wildcard/empty)
	aHasHost := a.host != ""
	bHasHost := b.host != ""
	if aHasHost != bHasHost {
		if aHasHost {
			return -1
		}
		return 1
	}
	if aHasHost && bHasHost {
		aHasPort := hasExplicitPort(a.host)
		bHasPort := hasExplicitPort(b.host)
		if aHasPort != bHasPort {
			if aHasPort {
				return -1
			}
			return 1
		}
	}

	// Tier 3: Header Specificity (More headers before fewer headers)
	if len(a.headers) != len(b.headers) {
		if len(a.headers) > len(b.headers) {
			return -1
		}
		return 1
	}

	// Tier 4: Method Specificity (Explicit method before any-method)
	aHasMethod := a.method != ""
	bHasMethod := b.method != ""
	if aHasMethod != bHasMethod {
		if aHasMethod {
			return -1
		}
		return 1
	}

	// Tier 5: Deterministic Tie-Breaking (Alphabetical)
	if a.host != b.host {
		if a.host < b.host {
			return -1
		}
		return 1
	}
	if len(a.headers) > 0 || len(b.headers) > 0 {
		aCanon := CanonicalHeaderString(a.headers)
		bCanon := CanonicalHeaderString(b.headers)
		if aCanon != bCanon {
			if aCanon < bCanon {
				return -1
			}
			return 1
		}
	}
	if a.method != b.method {
		if a.method < b.method {
			return -1
		}
		return 1
	}
	if a.prefix != b.prefix {
		if a.prefix < b.prefix {
			return -1
		}
		return 1
	}
	if a.source != b.source {
		if a.source < b.source {
			return -1
		}
		return 1
	}
	return 0
}

func sortPrefixRoutes(routes []prefixRoute) {
	sort.SliceStable(routes, func(i, j int) bool {
		return comparePrefixRoutes(routes[i], routes[j]) < 0
	})
}

func comparePrefixRouteSpecs(a, b PrefixRouteSpec) int {
	aPrefix := "/" + strings.Trim(a.Prefix, "/")
	if aPrefix == "/" {
		aPrefix = ""
	}
	bPrefix := "/" + strings.Trim(b.Prefix, "/")
	if bPrefix == "/" {
		bPrefix = ""
	}
	if len(aPrefix) != len(bPrefix) {
		if len(aPrefix) > len(bPrefix) {
			return -1
		}
		return 1
	}

	aHost := strings.ToLower(strings.TrimSpace(a.Host))
	bHost := strings.ToLower(strings.TrimSpace(b.Host))
	aHasHost := aHost != ""
	bHasHost := bHost != ""
	if aHasHost != bHasHost {
		if aHasHost {
			return -1
		}
		return 1
	}
	if aHasHost && bHasHost {
		aHasPort := hasExplicitPort(aHost)
		bHasPort := hasExplicitPort(bHost)
		if aHasPort != bHasPort {
			if aHasPort {
				return -1
			}
			return 1
		}
	}

	if len(a.Headers) != len(b.Headers) {
		if len(a.Headers) > len(b.Headers) {
			return -1
		}
		return 1
	}

	aMethod := strings.ToUpper(strings.TrimSpace(a.Method))
	bMethod := strings.ToUpper(strings.TrimSpace(b.Method))
	aHasMethod := aMethod != ""
	bHasMethod := bMethod != ""
	if aHasMethod != bHasMethod {
		if aHasMethod {
			return -1
		}
		return 1
	}

	// Tier 5: Deterministic Tie-Breaking
	if aHost != bHost {
		if aHost < bHost {
			return -1
		}
		return 1
	}
	if len(a.Headers) > 0 || len(b.Headers) > 0 {
		aCanon := CanonicalHeaderString(a.Headers)
		bCanon := CanonicalHeaderString(b.Headers)
		if aCanon != bCanon {
			if aCanon < bCanon {
				return -1
			}
			return 1
		}
	}
	if aMethod != bMethod {
		if aMethod < bMethod {
			return -1
		}
		return 1
	}
	if aPrefix != bPrefix {
		if aPrefix < bPrefix {
			return -1
		}
		return 1
	}
	return 0
}

// SortPrefixRouteSpecs sorts a slice of PrefixRouteSpec in descending order of specificity per ADR-005.
func SortPrefixRouteSpecs(specs []PrefixRouteSpec) {
	sort.SliceStable(specs, func(i, j int) bool {
		return comparePrefixRouteSpecs(specs[i], specs[j]) < 0
	})
}

func (r *Router) compilePrefixRoute(source string, spec PrefixRouteSpec) (prefixRoute, error) {
	cleanSource := strings.TrimSpace(source)
	if cleanSource == "" {
		cleanSource = "config"
	}

	cleanPrefix := "/" + strings.Trim(spec.Prefix, "/")
	if cleanPrefix == "/" {
		cleanPrefix = ""
	}

	targetTypeStr := string(spec.TargetType)
	if targetTypeStr == "" {
		targetTypeStr = string(spec.RouteType)
	}
	normType := RouteType(strings.ToLower(strings.TrimSpace(targetTypeStr)))
	if normType == "proxy" {
		normType = RouteTypeUpstream
	}

	var (
		handler HandlerFunc
		px      *proxy.ReverseProxy
	)

	if spec.Handler != nil {
		handler = spec.Handler
		if normType == "" {
			normType = RouteType("handler")
		}
	} else if normType == RouteTypeStatic {
		if spec.DirPath == "" {
			return prefixRoute{}, fmt.Errorf("router: static route for prefix %q requires non-empty dirPath", spec.Prefix)
		}
		absDir, err := filepath.Abs(spec.DirPath)
		if err != nil {
			absDir = spec.DirPath
		}
		handler = r.createStaticHandler(cleanPrefix, absDir, spec.Opts)
	} else if normType == RouteTypeUpstream {
		var err error
		px, err = proxy.NewProxyWithOptions(spec.Opts)
		if err != nil {
			return prefixRoute{}, err
		}
		handler = func(req *httpparser.Request, res *httpparser.Response) {
			px.ServeHTTPWithPrefix(req, res, cleanPrefix)
		}
	} else {
		return prefixRoute{}, fmt.Errorf("router: invalid route type %q (must be 'static' or 'upstream')", spec.TargetType)
	}

	if strings.TrimSpace(spec.Opts.RateLimit) != "" {
		rlMw, err := NewRateLimitMiddleware(spec.Opts.RateLimit)
		if err != nil {
			if px != nil {
				px.Close()
			}
			return prefixRoute{}, fmt.Errorf("router: invalid rate_limit %q for prefix %q: %w", spec.Opts.RateLimit, spec.Prefix, err)
		}
		handler = rlMw(handler)
	}

	if spec.Opts.Auth != nil {
		if authCfg, ok := spec.Opts.Auth.(AuthConfig); ok && authCfg.Type != "" {
			handler = NewAuthMiddleware(authCfg)(handler)
		}
	}

	if spec.Opts.WAF != nil {
		if wafCfg, ok := spec.Opts.WAF.(waf.WAFConfig); ok && (wafCfg.Enabled || len(wafCfg.AllowedIPs) > 0 || len(wafCfg.DeniedIPs) > 0 || len(wafCfg.DisabledRules) > 0 || wafCfg.Mode != "") {
			if len(spec.Opts.TrustedProxies) > 0 && len(wafCfg.TrustedProxies) == 0 {
				wafCfg.TrustedProxies = spec.Opts.TrustedProxies
			}
			if strings.TrimSpace(spec.Opts.SecurityLog) != "" {
				secLog := strings.ToLower(strings.TrimSpace(spec.Opts.SecurityLog))
				if secLog == "off" || secLog == "none" {
					wafCfg.AuditLog.Enabled = false
				} else {
					wafCfg.AuditLog.Enabled = true
					wafCfg.AuditLog.Output = spec.Opts.SecurityLog
				}
			}
			if routeWafEngine, err := waf.NewEngine(wafCfg); err == nil {
				nextHandler := handler
				wafMw := waf.NewWAFMiddleware(routeWafEngine)
				handler = func(req *httpparser.Request, res *httpparser.Response) {
					wafMw(waf.HandlerFunc(nextHandler))(req, res)
				}
			}
		}
	}

	normMethod := strings.ToUpper(strings.TrimSpace(spec.Method))

	maxConcurrency := spec.MaxConcurrency
	if maxConcurrency == 0 {
		maxConcurrency = spec.Opts.MaxConcurrency
	}
	maxBodyBytes := spec.MaxBodyBytes
	if maxBodyBytes == 0 {
		maxBodyBytes = spec.Opts.MaxBodyBytes
	}
	readTimeout := spec.ReadTimeout
	if readTimeout == 0 {
		readTimeout = spec.Opts.ReadTimeout
	}
	writeTimeout := spec.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = spec.Opts.WriteTimeout
	}
	respTimeout := spec.ResponseHeaderTimeout
	if respTimeout == 0 {
		respTimeout = spec.Opts.ResponseHeaderTimeout
	}
	if respTimeout == 0 {
		respTimeout = spec.Opts.Transport.ResponseHeaderTimeout
	}
	streamBody := spec.StreamRequestBody
	if streamBody == nil {
		streamBody = spec.Opts.StreamRequestBody
	}

	// Automatic safety guardrail:
	// If maxConcurrency == 0 and (maxBodyBytes > 4MB or respTimeout > 10s)
	if maxConcurrency == 0 && (maxBodyBytes > 4*1024*1024 || respTimeout > 10*time.Second) {
		poolSize := r.WorkerPoolSize()
		safeLimit := poolSize / 4
		if safeLimit < 1 {
			safeLimit = 1
		}
		if safeLimit > 32 {
			safeLimit = 32
		}
		maxConcurrency = safeLimit
	}

	return prefixRoute{
		source:                cleanSource,
		proxy:                 px,
		routeType:             string(normType),
		method:                normMethod,
		host:                  strings.ToLower(strings.TrimSpace(spec.Host)),
		prefix:                cleanPrefix,
		headers:               spec.Headers,
		redirectHTTP:          spec.Opts.RedirectHTTP,
		accessLog:             strings.TrimSpace(spec.Opts.AccessLog),
		securityLog:           strings.TrimSpace(spec.Opts.SecurityLog),
		handler:               handler,
		maxConcurrency:        maxConcurrency,
		maxBodyBytes:          maxBodyBytes,
		readTimeout:           readTimeout,
		writeTimeout:          writeTimeout,
		responseHeaderTimeout: respTimeout,
		streamRequestBody:     streamBody,
	}, nil
}

// RoutePrefixWithSource registers a prefix route tagged with a specific subsystem source identifier.
func (r *Router) RoutePrefixWithSource(source string, targetType RouteType, host, prefix string, headers map[string]string, dirPath string, opts proxy.ProxyOptions) error {
	pr, err := r.compilePrefixRoute(source, PrefixRouteSpec{
		TargetType:            targetType,
		Host:                  host,
		Prefix:                prefix,
		Headers:               headers,
		DirPath:               dirPath,
		Opts:                  opts,
		MaxConcurrency:        opts.MaxConcurrency,
		MaxBodyBytes:          opts.MaxBodyBytes,
		ReadTimeout:           opts.ReadTimeout,
		WriteTimeout:          opts.WriteTimeout,
		ResponseHeaderTimeout: opts.ResponseHeaderTimeout,
		StreamRequestBody:     opts.StreamRequestBody,
	})
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefixRoutes = append(r.prefixRoutes, pr)
	sortPrefixRoutes(r.prefixRoutes)
	return nil
}

// RoutePrefix registers a prefix route that acts either as a static file server or an upstream reverse proxy, matching optional host and headers.
func (r *Router) RoutePrefix(targetType RouteType, host, prefix string, headers map[string]string, dirPath string, opts proxy.ProxyOptions) error {
	return r.RoutePrefixWithSource("config", targetType, host, prefix, headers, dirPath, opts)
}

// AddRoute registers a prefix route from a PrefixRouteSpec with optional handler or proxy options.
func (r *Router) AddRoute(spec PrefixRouteSpec) error {
	pr, err := r.compilePrefixRoute("config", spec)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefixRoutes = append(r.prefixRoutes, pr)
	sortPrefixRoutes(r.prefixRoutes)
	return nil
}

// ReplacePrefixRoutesBySource atomically replaces all prefix routes matching source with specs under write lock.
// Routes belonging to other sources are preserved in their exact relative order.
// If specs is empty, all routes belonging to source are cleanly pruned.
func (r *Router) ReplacePrefixRoutesBySource(source string, specs []PrefixRouteSpec) error {
	cleanSource := strings.TrimSpace(source)
	if cleanSource == "" {
		cleanSource = "config"
	}

	// 1. Pre-compile and pre-validate all specs outside lock.
	compiledRoutes := make([]prefixRoute, 0, len(specs))
	for _, spec := range specs {
		pr, err := r.compilePrefixRoute(cleanSource, spec)
		if err != nil {
			for _, cpr := range compiledRoutes {
				if cpr.proxy != nil {
					cpr.proxy.Close()
				}
			}
			return err
		}
		compiledRoutes = append(compiledRoutes, pr)
	}

	// 2. Under write lock, partition r.prefixRoutes and swap.
	var evicted []prefixRoute
	r.mu.Lock()
	retained := make([]prefixRoute, 0, len(r.prefixRoutes))
	for _, pr := range r.prefixRoutes {
		if pr.source == cleanSource {
			evicted = append(evicted, pr)
		} else {
			retained = append(retained, pr)
		}
	}
	r.prefixRoutes = append(retained, compiledRoutes...)
	sortPrefixRoutes(r.prefixRoutes)
	r.mu.Unlock()

	// 3. For evicted routes with matching source, invoke pr.proxy.Close() if pr.proxy != nil.
	for _, pr := range evicted {
		if pr.proxy != nil {
			pr.proxy.Close()
		}
	}

	return nil
}

// HandlePrefix registers an in-process handler for an incoming HTTP method and path prefix.
func (r *Router) HandlePrefix(method, prefix string, handler HandlerFunc) {
	r.HandlePrefixWithMatcher(method, "", prefix, nil, nil, handler)
}

// HandlePrefixWithMatcher registers an in-process prefix handler with method, host, header, and path matcher options.
func (r *Router) HandlePrefixWithMatcher(method, host, prefix string, headers map[string]string, matcher func(path string) bool, handler HandlerFunc) {
	cleanPrefix := "/" + strings.Trim(prefix, "/")
	if cleanPrefix == "/" {
		cleanPrefix = ""
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefixRoutes = append(r.prefixRoutes, prefixRoute{
		source:    "config",
		routeType: "handler",
		method:    strings.ToUpper(strings.TrimSpace(method)),
		host:      strings.ToLower(strings.TrimSpace(host)),
		prefix:    cleanPrefix,
		matcher:   matcher,
		headers:   headers,
		handler:   handler,
	})
	sortPrefixRoutes(r.prefixRoutes)
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

func (r *Router) createStaticHandler(cleanPrefix, absDir string, opts proxy.ProxyOptions) HandlerFunc {
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

		fileRelPath := relPath
		if fileRelPath == "" || fileRelPath == "/" {
			fileRelPath = "/index.html"
		}

		// Security: Prevent path traversal & symlink escape
		cleanRel := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(fileRelPath, "/")))
		targetPath := filepath.Join(absDir, cleanRel)

		relFromDir, err := filepath.Rel(absDir, targetPath)
		if err != nil || strings.HasPrefix(relFromDir, "..") || (strings.HasPrefix(relFromDir, ".") && len(relFromDir) > 1 && relFromDir[1] == '.') {
			res.SetStatus(http.StatusForbidden)
			res.Header.Set("Content-Type", "application/json")
			res.Header.Set("Connection", "close")
			_, _ = res.WriteString(`{"error":"403 Forbidden: Path Traversal Disallowed"}`)
			return
		}

		// Physical symlink target verification
		realAbsDir, err := filepath.EvalSymlinks(absDir)
		if err != nil {
			realAbsDir = absDir
		}
		if evalTarget, err := filepath.EvalSymlinks(targetPath); err == nil {
			relFromReal, err := filepath.Rel(realAbsDir, evalTarget)
			if err != nil || strings.HasPrefix(relFromReal, "..") || (strings.HasPrefix(relFromReal, ".") && len(relFromReal) > 1 && relFromReal[1] == '.') {
				res.SetStatus(http.StatusForbidden)
				res.Header.Set("Content-Type", "application/json")
				res.Header.Set("Connection", "close")
				_, _ = res.WriteString(`{"error":"403 Forbidden: Symlink Path Traversal Disallowed"}`)
				return
			}
		}

		serveFallback := func() {
			spaMode := opts.SPA || strings.TrimSpace(opts.Fallback) != ""
			if spaMode && filepath.Ext(relPath) == "" {
				fallbackName := filepath.Base(opts.Fallback)
				if fallbackName == "." || fallbackName == ".." || fallbackName == "" {
					fallbackName = "index.html"
				}
				fallbackPath := filepath.Join(absDir, fallbackName)

				relFB, err := filepath.Rel(absDir, fallbackPath)
				if err != nil || strings.HasPrefix(relFB, "..") || (strings.HasPrefix(relFB, ".") && len(relFB) > 1 && relFB[1] == '.') {
					res.SetStatus(http.StatusForbidden)
					res.Header.Set("Content-Type", "application/json")
					res.Header.Set("Connection", "close")
					_, _ = res.WriteString(`{"error":"403 Forbidden: Path Traversal Disallowed"}`)
					return
				}

				if evalFB, err := filepath.EvalSymlinks(fallbackPath); err == nil {
					relFBReal, err := filepath.Rel(realAbsDir, evalFB)
					if err != nil || strings.HasPrefix(relFBReal, "..") || (strings.HasPrefix(relFBReal, ".") && len(relFBReal) > 1 && relFBReal[1] == '.') {
						res.SetStatus(http.StatusForbidden)
						res.Header.Set("Content-Type", "application/json")
						res.Header.Set("Connection", "close")
						_, _ = res.WriteString(`{"error":"403 Forbidden: Symlink Path Traversal Disallowed"}`)
						return
					}
				}

				fbInfo, err := os.Stat(fallbackPath)
				if err == nil && !fbInfo.IsDir() {
					fbData, err := os.ReadFile(fallbackPath)
					if err == nil {
						res.SetStatus(http.StatusOK)
						res.Header.Set("Content-Type", "text/html; charset=utf-8")
						if req.Method != "HEAD" {
							_, _ = res.Write(fbData)
						}
						return
					}
				}
			}
			r.NotFound(req, res)
		}

		fileInfo, err := os.Stat(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				serveFallback()
				return
			}
			res.SetStatus(http.StatusInternalServerError)
			return
		}

		if fileInfo.IsDir() {
			targetPath = filepath.Join(targetPath, "index.html")
			fileInfo, err = os.Stat(targetPath)
			if err != nil || fileInfo.IsDir() {
				serveFallback()
				return
			}
		}

		data, err := os.ReadFile(targetPath)
		if err != nil {
			serveFallback()
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

// cleanRequestPath canonicalizes an incoming URL path, resolving dot segments (. and ..)
// and redundant slashes according to RFC 3986 / ADR-062, while preserving trailing slashes.
func cleanRequestPath(p string) string {
	if p == "" {
		return "/"
	}
	clean := path.Clean(p)
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	if strings.HasSuffix(p, "/") && clean != "/" && !strings.HasSuffix(clean, "/") {
		clean += "/"
	}
	return clean
}

// ServeHTTP dispatches the request to registered handlers through the middleware chain.
func (r *Router) ServeHTTP(req *httpparser.Request, res *httpparser.Response) {
	rawPath := ""
	unescaped := ""
	if req != nil {
		if req.RequestURI != "" {
			rawWire := req.RequestURI
			if idx := strings.IndexByte(rawWire, '?'); idx != -1 {
				rawWire = rawWire[:idx]
			}
			if idx := strings.IndexByte(rawWire, '#'); idx != -1 {
				rawWire = rawWire[:idx]
			}
			rawPath = rawWire
		} else {
			rawPath = req.Path
		}

		unescaped = rawPath
		for {
			u, err := url.PathUnescape(unescaped)
			if err != nil || u == unescaped {
				break
			}
			unescaped = u
		}

		req.Path = cleanRequestPath(req.Path)
	}
	r.mu.RLock()
	var targetHandler HandlerFunc
	var matchedPR *prefixRoute
	var matchedExact bool

	reqHost := extractHost(req)

	methodsMap, pathExists := r.routes[req.Path]
	if pathExists {
		entries, methodExists := methodsMap[req.Method]
		if methodExists {
			var fallbackEntry *routeEntry

			// Tier 1: Seek an exact match among explicit port route entries
			for i := range entries {
				if entries[i].host != "" && hasExplicitPort(entries[i].host) {
					if headersAndHostMatch(reqHost, req, entries[i].host, entries[i].headers) {
						targetHandler = entries[i].handler
						matchedExact = true
						break
					}
				}
				if entries[i].host == "" && len(entries[i].headers) == 0 {
					fallbackEntry = &entries[i]
				}
			}

			// Tier 2: If no explicit port route matched, evaluate domain-only routes with host specified
			if targetHandler == nil {
				for i := range entries {
					if entries[i].host != "" && !hasExplicitPort(entries[i].host) {
						if headersAndHostMatch(reqHost, req, entries[i].host, entries[i].headers) {
							targetHandler = entries[i].handler
							matchedExact = true
							break
						}
					}
				}
			}

			// Tier 3: If no domain route matched, evaluate header-constrained wildcard routes
			if targetHandler == nil {
				for i := range entries {
					if entries[i].host == "" && len(entries[i].headers) > 0 {
						if headersAndHostMatch(reqHost, req, entries[i].host, entries[i].headers) {
							targetHandler = entries[i].handler
							matchedExact = true
							break
						}
					}
				}
			}

			// Tier 4: Fall back to wildcard route if registered and no specific route matched
			if targetHandler == nil && fallbackEntry != nil {
				targetHandler = fallbackEntry.handler
				matchedExact = true
			}
		} else {
			targetHandler = r.MethodNotAllowed
		}
	}

	if targetHandler == nil {
		// Static Prefix Escape Guard (REQ-116 / ADR-116):
		// Detect if incoming path targeted a registered static prefix route,
		// but canonical path escapes the prefix boundary.
		var staticEscapeDetected bool
		for i := range r.prefixRoutes {
			pr := &r.prefixRoutes[i]
			if pr.routeType != string(RouteTypeStatic) || pr.prefix == "" {
				continue
			}
			prefix := "/" + strings.Trim(pr.prefix, "/")
			if prefix == "/" {
				continue
			}
			prefixSlash := prefix + "/"
			targeted := rawPath == prefix || strings.HasPrefix(rawPath, prefixSlash) ||
				unescaped == prefix || strings.HasPrefix(unescaped, prefixSlash)
			if targeted {
				cleanRaw := cleanRequestPath(rawPath)
				cleanUnesc := cleanRequestPath(unescaped)
				escapes := (!strings.HasPrefix(cleanRaw, prefixSlash) && cleanRaw != prefix) ||
					(!strings.HasPrefix(cleanUnesc, prefixSlash) && cleanUnesc != prefix) ||
					(req != nil && !strings.HasPrefix(req.Path, prefixSlash) && req.Path != prefix)
				if escapes {
					staticEscapeDetected = true
					targetHandler = func(req *httpparser.Request, res *httpparser.Response) {
						if res.Body == nil {
							res.Body = bytes.NewBuffer(nil)
						}
						res.SetStatus(http.StatusForbidden)
						res.Header.Set("Content-Type", "application/json")
						res.Header.Set("Connection", "close")
						_, _ = res.WriteString(`{"error":"403 Forbidden: Path Traversal Disallowed"}`)
					}
					break
				}
			}
		}

		if !staticEscapeDetected {
			// Check prefix routes (static file, upstream reverse proxy, or prefix handler routes)
			var fallbackPrefix *prefixRoute
			var methodMismatch bool
			for i := range r.prefixRoutes {
				pr := &r.prefixRoutes[i]
				if pr.prefix == "" || strings.HasPrefix(req.Path, pr.prefix+"/") || req.Path == pr.prefix {
					if pr.method != "" && !strings.EqualFold(pr.method, req.Method) {
						methodMismatch = true
						continue
					}
					if pr.matcher != nil && !pr.matcher(req.Path) {
						continue
					}
					if headersAndHostMatch(reqHost, req, pr.host, pr.headers) {
						targetHandler = pr.handler
						matchedPR = pr
						break
					}
					if pr.host == "" && len(pr.headers) == 0 && fallbackPrefix == nil {
						fallbackPrefix = pr
					}
				}
			}
			if targetHandler == nil && fallbackPrefix != nil {
				targetHandler = fallbackPrefix.handler
				matchedPR = fallbackPrefix
			}
			if targetHandler == nil {
				if methodMismatch {
					targetHandler = r.MethodNotAllowed
				} else {
					targetHandler = r.NotFound
				}
			}
		}
	}

	middlewares := append([]MiddlewareFunc(nil), r.middlewares...)
	r.mu.RUnlock()

	// Tag request context with matched route telemetry metadata
	if matchedPR != nil {
		SetMatchedRoute(req, matchedPR.prefix)
		SetRouteType(req, matchedPR.routeType)
		if matchedPR.routeType == string(RouteTypeStatic) {
			SetDestination(req, "static")
		}
	} else if matchedExact {
		SetMatchedRoute(req, req.Path)
		SetRouteType(req, "exact")
		SetDestination(req, "in-process")
	} else {
		SetDestination(req, "in-process")
	}

	// Enforce route bulkhead concurrency limits if not already acquired by caller (e.g. s.handleConn)
	if matchedPR != nil && matchedPR.maxConcurrency > 0 {
		alreadyAcquired := false
		if req != nil && req.Context() != nil {
			if v, ok := req.Context().Value(RouteSlotAcquiredContextKey).(bool); ok && v {
				alreadyAcquired = true
			}
		}
		if !alreadyAcquired {
			current := atomic.LoadInt64(&matchedPR.activeRequests)
			if current >= int64(matchedPR.maxConcurrency) {
				res.SetStatus(http.StatusServiceUnavailable)
				res.Header.Set("Retry-After", "5")
				res.Header.Set("Content-Type", "application/json")
				res.Header.Set("Connection", "close")
				if res.Body == nil {
					res.Body = bytes.NewBuffer(nil)
				} else {
					res.Body.Reset()
				}
				_, _ = res.WriteString(`{"error":"503 Service Unavailable: Route concurrency limit reached"}`)
				return
			}
			newVal := atomic.AddInt64(&matchedPR.activeRequests, 1)
			if newVal > int64(matchedPR.maxConcurrency) {
				atomic.AddInt64(&matchedPR.activeRequests, -1)
				res.SetStatus(http.StatusServiceUnavailable)
				res.Header.Set("Retry-After", "5")
				res.Header.Set("Content-Type", "application/json")
				res.Header.Set("Connection", "close")
				if res.Body == nil {
					res.Body = bytes.NewBuffer(nil)
				} else {
					res.Body.Reset()
				}
				_, _ = res.WriteString(`{"error":"503 Service Unavailable: Route concurrency limit reached"}`)
				return
			}
			var once sync.Once
			defer once.Do(func() {
				atomic.AddInt64(&matchedPR.activeRequests, -1)
			})
		}
	}

	// Chain middlewares in reverse order
	finalChain := targetHandler
	for i := len(middlewares) - 1; i >= 0; i-- {
		finalChain = middlewares[i](finalChain)
	}

	start := time.Now()
	finalChain(req, res)
	metrics.DefaultRegistry.RecordRequest(req.Method, fmt.Sprintf("%d", res.StatusCode), req.Path, time.Since(start).Seconds())
}

// ShouldRedirectHTTP checks whether an incoming HTTP request should be upgraded to HTTPS.
// It returns false if the request matches a configured route with redirect_http explicitly set to false.
// Otherwise, it returns true (defaulting to HTTPS redirection).
func (r *Router) ShouldRedirectHTTP(req *httpparser.Request) bool {
	if req == nil {
		return true
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	reqHost := extractHost(req)
	reqPath := cleanRequestPath(req.Path)

	for i := range r.prefixRoutes {
		pr := &r.prefixRoutes[i]
		if pr.prefix == "" || strings.HasPrefix(reqPath, pr.prefix+"/") || reqPath == pr.prefix {
			if pr.method != "" && !strings.EqualFold(pr.method, req.Method) {
				continue
			}
			if headersAndHostMatch(reqHost, req, pr.host, pr.headers) {
				if pr.redirectHTTP != nil && !*pr.redirectHTTP {
					return false
				}
				return true
			}
		}
	}
	return true
}

func hasExplicitPort(host string) bool {
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "[") {
		idx := strings.Index(host, "]")
		return idx != -1 && strings.Contains(host[idx+1:], ":")
	}
	return strings.Contains(host, ":")
}

func extractFullHostPort(req *httpparser.Request) string {
	if req == nil {
		return ""
	}
	h := strings.TrimSpace(req.Header.Get("Host"))
	if h == "" {
		return ""
	}
	if strings.HasPrefix(h, "[") {
		closeBracket := strings.Index(h, "]")
		if closeBracket != -1 {
			ipv6 := strings.ToLower(h[1:closeBracket])
			rest := h[closeBracket+1:]
			if strings.HasPrefix(rest, ":") {
				return "[" + ipv6 + "]:" + strings.TrimSpace(rest[1:])
			}
			return "[" + ipv6 + "]"
		}
	}
	colonIdx := strings.LastIndex(h, ":")
	if colonIdx != -1 {
		return strings.ToLower(strings.TrimSpace(h[:colonIdx])) + ":" + strings.TrimSpace(h[colonIdx+1:])
	}
	return strings.ToLower(h)
}

func extractHost(req *httpparser.Request) string {
	h := req.Header.Get("Host")
	if h == "" {
		return ""
	}
	h = strings.TrimSpace(h)
	if strings.HasPrefix(h, "[") {
		if idx := strings.Index(h, "]"); idx != -1 {
			return strings.ToLower(h[:idx+1])
		}
	}
	if idx := strings.LastIndex(h, ":"); idx != -1 {
		h = h[:idx]
	}
	return strings.ToLower(strings.TrimSpace(h))
}

func headersAndHostMatch(reqHost string, req *httpparser.Request, routeHost string, expectedHeaders map[string]string) bool {
	if routeHost != "" {
		cleanRouteHost := strings.ToLower(strings.TrimSpace(routeHost))
		if hasExplicitPort(cleanRouteHost) {
			// Port-specific route: must match full incoming authority (host and port)
			reqAuthority := extractFullHostPort(req)
			if reqAuthority != cleanRouteHost {
				return false
			}
		} else {
			// Domain-only route: matches port-stripped reqHost (wildcard port)
			if reqHost != cleanRouteHost {
				return false
			}
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

// RouteMatchInfo holds matched route metadata and resolved limits.
type RouteMatchInfo struct {
	Prefix                string
	RouteType             string
	MaxBodyBytes          int64
	MaxConcurrency        int
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	ResponseHeaderTimeout time.Duration
	StreamRequestBody     *bool
	routeRef              *prefixRoute
}

// LookupPrefixRoute evaluates route matching (path, host, method, headers) without consuming request body.
func (r *Router) LookupPrefixRoute(req *httpparser.Request) (*RouteMatchInfo, bool) {
	if r == nil || req == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	reqHost := extractHost(req)
	reqPath := cleanRequestPath(req.Path)

	var fallbackPrefix *prefixRoute
	for i := range r.prefixRoutes {
		pr := &r.prefixRoutes[i]
		if pr.prefix == "" || strings.HasPrefix(reqPath, pr.prefix+"/") || reqPath == pr.prefix {
			if pr.method != "" && !strings.EqualFold(pr.method, req.Method) {
				continue
			}
			if pr.matcher != nil && !pr.matcher(reqPath) {
				continue
			}
			if headersAndHostMatch(reqHost, req, pr.host, pr.headers) {
				return &RouteMatchInfo{
					Prefix:                pr.prefix,
					RouteType:             pr.routeType,
					MaxBodyBytes:          pr.maxBodyBytes,
					MaxConcurrency:        pr.maxConcurrency,
					ReadTimeout:           pr.readTimeout,
					WriteTimeout:          pr.writeTimeout,
					ResponseHeaderTimeout: pr.responseHeaderTimeout,
					StreamRequestBody:     pr.streamRequestBody,
					routeRef:              pr,
				}, true
			}
			if pr.host == "" && len(pr.headers) == 0 && fallbackPrefix == nil {
				fallbackPrefix = pr
			}
		}
	}
	if fallbackPrefix != nil {
		return &RouteMatchInfo{
			Prefix:                fallbackPrefix.prefix,
			RouteType:             fallbackPrefix.routeType,
			MaxBodyBytes:          fallbackPrefix.maxBodyBytes,
			MaxConcurrency:        fallbackPrefix.maxConcurrency,
			ReadTimeout:           fallbackPrefix.readTimeout,
			WriteTimeout:          fallbackPrefix.writeTimeout,
			ResponseHeaderTimeout: fallbackPrefix.responseHeaderTimeout,
			StreamRequestBody:     fallbackPrefix.streamRequestBody,
			routeRef:              fallbackPrefix,
		}, true
	}
	return nil, false
}

// TryAcquireRouteSlot attempts to reserve a concurrency slot on the matched route.
// Returns release callback and true if slot was acquired, or false if route is at capacity.
func (r *Router) TryAcquireRouteSlot(info *RouteMatchInfo) (release func(), acquired bool) {
	if info == nil || info.routeRef == nil || info.MaxConcurrency <= 0 {
		return func() {}, true // Unconstrained route
	}
	pr := info.routeRef
	current := atomic.LoadInt64(&pr.activeRequests)
	if current >= int64(info.MaxConcurrency) {
		return nil, false
	}
	newVal := atomic.AddInt64(&pr.activeRequests, 1)
	if newVal > int64(info.MaxConcurrency) {
		atomic.AddInt64(&pr.activeRequests, -1) // Rollback
		return nil, false
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			atomic.AddInt64(&pr.activeRequests, -1)
		})
	}, true
}

// MatchPrefixRoute finds the matching prefix route for an incoming request and returns its prefix, access log override, and security log override.
func (r *Router) MatchPrefixRoute(req *httpparser.Request) (prefix string, accessLog string, securityLog string, matched bool) {
	if r == nil || req == nil {
		return "", "", "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	reqHost := extractHost(req)
	reqPath := cleanRequestPath(req.Path)
	for i := range r.prefixRoutes {
		pr := &r.prefixRoutes[i]
		if pr.prefix == "" || strings.HasPrefix(reqPath, pr.prefix+"/") || reqPath == pr.prefix {
			if pr.method != "" && !strings.EqualFold(pr.method, req.Method) {
				continue
			}
			if pr.matcher != nil && !pr.matcher(reqPath) {
				continue
			}
			if headersAndHostMatch(reqHost, req, pr.host, pr.headers) {
				return pr.prefix, pr.accessLog, pr.securityLog, true
			}
		}
	}
	return "", "", "", false
}

// AccessLoggerMiddleware logs incoming requests to the configured LogManager, respecting route-level access log overrides and silencing.
func AccessLoggerMiddleware(logMgr *logging.LogManager, r *Router) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			start := time.Now()
			next(req, res)
			duration := time.Since(start)

			matchedPrefix := ""
			routeAccessLog := ""
			if r != nil {
				prefix, accLog, _, matched := r.MatchPrefixRoute(req)
				if matched {
					matchedPrefix = prefix
					routeAccessLog = accLog
				}
			}

			if logMgr != nil {
				bytesSent := int64(0)
				if res.Body != nil {
					bytesSent = int64(res.Body.Len())
				}
				proto := req.Proto
				if proto == "" {
					proto = "HTTP/1.1"
				}
				logPath := req.RequestURI
				if logPath == "" {
					logPath = req.Path
				}
				logMgr.LogAccess(logging.AccessLogEntry{
					Timestamp:   start,
					ClientIP:    logging.ExtractClientIP(req),
					Method:      req.Method,
					Path:        logPath,
					Protocol:    proto,
					StatusCode:  res.StatusCode,
					Duration:    duration,
					BytesSent:   bytesSent,
					UserAgent:   req.Header.Get("User-Agent"),
					Referer:     req.Header.Get("Referer"),
					Host:        req.Header.Get("Host"),
					RoutePrefix: matchedPrefix,
				}, routeAccessLog)
			}
		}
	}
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

// RouteSnapshot represents a thread-safe view of a registered prefix route.
type RouteSnapshot struct {
	Source  string            `json:"source,omitempty"`
	Type    string            `json:"type"`
	Host    string            `json:"host,omitempty"`
	Prefix  string            `json:"prefix"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// GetPrefixRoutes returns a thread-safe snapshot of all active prefix routes.
func (r *Router) GetPrefixRoutes() []RouteSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshots := make([]RouteSnapshot, 0, len(r.prefixRoutes))
	for _, pr := range r.prefixRoutes {
		rType := pr.routeType
		if rType == "" {
			rType = "upstream"
		}
		snapshots = append(snapshots, RouteSnapshot{
			Source:  pr.source,
			Type:    rType,
			Host:    pr.host,
			Prefix:  pr.prefix,
			Method:  pr.method,
			Headers: pr.headers,
		})
	}
	return snapshots
}

// RoutesSnapshot returns a thread-safe snapshot of all active prefix routes.
func (r *Router) RoutesSnapshot() []RouteSnapshot {
	return r.GetPrefixRoutes()
}

// RemovePrefixRoute removes matching host and prefix routes from the prefix routing table.
func (r *Router) RemovePrefixRoute(host, prefix string) {
	cleanHost := strings.ToLower(strings.TrimSpace(host))
	cleanPrefix := "/" + strings.Trim(prefix, "/")
	if cleanPrefix == "/" {
		cleanPrefix = ""
	}

	var evicted []prefixRoute
	r.mu.Lock()
	filtered := make([]prefixRoute, 0, len(r.prefixRoutes))
	for _, pr := range r.prefixRoutes {
		if pr.host == cleanHost && pr.prefix == cleanPrefix {
			evicted = append(evicted, pr)
			continue
		}
		filtered = append(filtered, pr)
	}
	r.prefixRoutes = filtered
	r.mu.Unlock()

	for _, pr := range evicted {
		if pr.proxy != nil {
			pr.proxy.Close()
		}
	}
}

// HasHost checks if a specific domain host is registered on any exact or prefix route.
func (r *Router) HasHost(host string) bool {
	if r == nil || host == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	h := strings.ToLower(strings.TrimSpace(host))
	for _, pr := range r.prefixRoutes {
		if pr.host != "" && strings.EqualFold(pr.host, h) {
			return true
		}
	}
	for _, methods := range r.routes {
		for _, entries := range methods {
			for _, entry := range entries {
				if entry.host != "" && strings.EqualFold(entry.host, h) {
					return true
				}
			}
		}
	}
	return false
}
