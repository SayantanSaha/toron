package server

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/metrics"
	"toron/pkg/router"
	"toron/pkg/version"
	"toron/pkg/waf"
)

// RouteInfo represents routing metadata for internal route listings.
type RouteInfo struct {
	Type          string            `json:"type"`
	Host          string            `json:"host,omitempty"`
	Prefix        string            `json:"prefix"`
	Headers       map[string]string `json:"headers,omitempty"`
	Dir           string            `json:"dir,omitempty"`
	Algorithm     string            `json:"algorithm,omitempty"`
	Targets       []string          `json:"targets,omitempty"`
	Source        string            `json:"source,omitempty"`
	ContainerName string            `json:"container_name,omitempty"`
}

// InternalAPIConfig configures /internal/api/ route parameters without importing pkg/config.
type InternalAPIConfig struct {
	Port                   int                `json:"port"`
	WorkerPoolSize         int                `json:"worker_pool_size"`
	ProxyEnabled           bool               `json:"proxy_enabled"`
	Routes                 []RouteInfo        `json:"routes"`
	StaticEnabled          bool               `json:"static_enabled"`
	StaticPrefix           string             `json:"static_prefix"`
	StaticDir              string             `json:"static_dir"`
	WAFEnabled             bool               `json:"waf_enabled"`
	WAFMode                string             `json:"waf_mode"`
	WAFAnomalyThreshold    int                `json:"waf_anomaly_threshold"`
	WAFRulesCount          int                `json:"waf_rules_count"`
	WAFCustomRulesCount    int                `json:"waf_custom_rules_count"`
	WAFAllowedIPs          []string           `json:"waf_allowed_ips"`
	WAFDeniedIPs           []string           `json:"waf_denied_ips"`
	CORSEnabled            bool               `json:"cors_enabled"`
	CORSAllowedOrigins     []string           `json:"cors_allowed_origins"`
	SecurityHeadersEnabled bool               `json:"security_headers_enabled"`
	MTLSEnabled            bool               `json:"mtls_enabled"`
	DiscoveryEnabled       bool               `json:"discovery_enabled"`
	DiscoveryFunc          func() []RouteInfo `json:"-"`
	AuditLogger            *waf.AuditLogger   `json:"-"`
	AdminAuthEnabled       bool               `json:"admin_auth_enabled"`
	AdminToken             string             `json:"admin_token"`
	AdminAPIKeys           []string           `json:"admin_api_keys"`
	AdminUsername          string             `json:"admin_username"`
	AdminPassword          string             `json:"admin_password"`
	AdminUsers             map[string]string  `json:"admin_users"`
	AdminSubnets           []string           `json:"admin_subnets"`
	TrustedProxies         []string           `json:"trusted_proxies"`
	AllowedProxyTestPaths  []string           `json:"allowed_proxy_test_paths"`
	MaxProxyTestResponseBytes int64           `json:"max_proxy_test_response_bytes,omitempty"`
}

// UpstreamNodeHealth describes the health state of an individual upstream service node.
type UpstreamNodeHealth struct {
	ID        int     `json:"id"`
	Port      int     `json:"port"`
	Name      string  `json:"name"`
	Route     string  `json:"route"`
	Algo      string  `json:"algo"`
	Status    string  `json:"status"`
	LatencyMS float64 `json:"latency_ms"`
	HTTPCode  int     `json:"http_code"`
}

// ProxyTestRequest defines incoming request body for /internal/api/proxy-test.
type ProxyTestRequest struct {
	Path    string            `json:"path"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
}

// ProxyTestResponse defines the output structure of /internal/api/proxy-test.
type ProxyTestResponse struct {
	StatusCode int               `json:"status_code"`
	StatusText string            `json:"status_text"`
	LatencyMS  float64           `json:"latency_ms"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	Truncated  bool              `json:"truncated,omitempty"`
}

// validateAdminAuth verifies administrative credentials against configured token, API keys, or basic auth.
func validateAdminAuth(req *httpparser.Request, cfg InternalAPIConfig) bool {
	// 1. Check X-Toron-Admin-Key header
	if key := req.Header.Get("X-Toron-Admin-Key"); key != "" {
		if cfg.AdminToken != "" && subtle.ConstantTimeCompare([]byte(key), []byte(cfg.AdminToken)) == 1 {
			return true
		}
		for _, k := range cfg.AdminAPIKeys {
			if subtle.ConstantTimeCompare([]byte(key), []byte(k)) == 1 {
				return true
			}
		}
	}

	// 2. Check Authorization header
	authHeader := req.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 {
			scheme := strings.ToLower(parts[0])
			cred := strings.TrimSpace(parts[1])

			if scheme == "bearer" {
				if cfg.AdminToken != "" && subtle.ConstantTimeCompare([]byte(cred), []byte(cfg.AdminToken)) == 1 {
					return true
				}
				for _, k := range cfg.AdminAPIKeys {
					if subtle.ConstantTimeCompare([]byte(cred), []byte(k)) == 1 {
						return true
					}
				}
			} else if scheme == "basic" {
				decoded, err := base64.StdEncoding.DecodeString(cred)
				if err == nil {
					userPass := strings.SplitN(string(decoded), ":", 2)
					if len(userPass) == 2 {
						u, p := userPass[0], userPass[1]
						if cfg.AdminUsername != "" &&
							subtle.ConstantTimeCompare([]byte(u), []byte(cfg.AdminUsername)) == 1 &&
							subtle.ConstantTimeCompare([]byte(p), []byte(cfg.AdminPassword)) == 1 {
							return true
						}
						if cfg.AdminUsers != nil {
							if expectedPass, exists := cfg.AdminUsers[u]; exists {
								if subtle.ConstantTimeCompare([]byte(p), []byte(expectedPass)) == 1 {
									return true
								}
							}
						}
					}
				}
			}
		}
	}

	return false
}

// RegisterInternalAPIRoutes registers /internal/api/ management routes on the router.
func RegisterInternalAPIRoutes(r *router.Router, cfg InternalAPIConfig) {
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.WorkerPoolSize == 0 {
		cfg.WorkerPoolSize = 128
	}

	// Parse configured administrative subnets
	var parsedSubnets []*net.IPNet
	for _, s := range cfg.AdminSubnets {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			if ip := net.ParseIP(s); ip != nil {
				if ip.To4() != nil {
					s = s + "/32"
				} else {
					s = s + "/128"
				}
			}
		}
		_, ipNet, err := net.ParseCIDR(s)
		if err == nil && ipNet != nil {
			parsedSubnets = append(parsedSubnets, ipNet)
		}
	}

	// Parse configured trusted proxies
	var parsedTrustedProxies []*net.IPNet
	for _, tp := range cfg.TrustedProxies {
		tp = strings.TrimSpace(tp)
		if tp == "" {
			continue
		}
		if !strings.Contains(tp, "/") {
			if ip := net.ParseIP(tp); ip != nil {
				if ip.To4() != nil {
					tp = tp + "/32"
				} else {
					tp = tp + "/128"
				}
			}
		}
		_, ipNet, err := net.ParseCIDR(tp)
		if err == nil && ipNet != nil {
			parsedTrustedProxies = append(parsedTrustedProxies, ipNet)
		}
	}

	authRequired := cfg.AdminAuthEnabled || cfg.AdminToken != "" || len(cfg.AdminAPIKeys) > 0 || cfg.AdminUsername != "" || len(cfg.AdminUsers) > 0

	// Security middleware guard wrapping all internal API routes
	wrapHandler := func(h router.HandlerFunc) router.HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			// Subnet check
			if len(parsedSubnets) > 0 {
				var clientIP net.IP
				physicalIP := req.RemoteIP()
				if physicalIP != nil {
					peerIsTrusted := false
					for _, tpNet := range parsedTrustedProxies {
						if tpNet.Contains(physicalIP) {
							peerIsTrusted = true
							break
						}
					}
					if peerIsTrusted && req.Header != nil {
						if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
							parts := strings.Split(xff, ",")
							raw := strings.TrimSpace(parts[0])
							if host, _, err := net.SplitHostPort(raw); err == nil {
								clientIP = net.ParseIP(host)
							} else {
								clientIP = net.ParseIP(raw)
							}
						}
					}
					if clientIP == nil {
						clientIP = physicalIP
					}
				}

				allowed := false
				if clientIP != nil {
					for _, subnet := range parsedSubnets {
						if subnet.Contains(clientIP) {
							allowed = true
							break
						}
					}
				}
				if !allowed {
					res.SetStatus(http.StatusForbidden)
					res.Header.Set("Content-Type", "application/json")
					_, _ = res.WriteString(`{"error":"403 Forbidden","message":"Access denied by administrative subnet policy"}`)
					return
				}
			}

			// Auth check
			if authRequired {
				if !validateAdminAuth(req, cfg) {
					res.SetStatus(http.StatusUnauthorized)
					res.Header.Set("Content-Type", "application/json")
					res.Header.Set("WWW-Authenticate", `Bearer realm="Toron Management", Basic realm="Toron Management"`)
					_, _ = res.WriteString(`{"error":"401 Unauthorized","message":"Authentication required for internal management API"}`)
					return
				}
			}

			h(req, res)
		}
	}

	// Helper to get aggregated routes (static/proxy config + dynamic OCI discovery)
	getAggregatedRoutes := func() []RouteInfo {
		routesList := append([]RouteInfo{}, cfg.Routes...)
		if cfg.DiscoveryFunc != nil {
			for _, ociR := range cfg.DiscoveryFunc() {
				routesList = append(routesList, ociR)
			}
		}
		if r != nil {
			activeSnapshots := r.GetPrefixRoutes()
			for _, snap := range activeSnapshots {
				found := false
				for _, existing := range routesList {
					if existing.Host == snap.Host && existing.Prefix == snap.Prefix {
						found = true
						break
					}
				}
				if !found && snap.Prefix != "" && snap.Prefix != "/metrics" && !strings.HasPrefix(snap.Prefix, "/internal/") {
					routesList = append(routesList, RouteInfo{
						Type:    snap.Type,
						Host:    snap.Host,
						Prefix:  snap.Prefix,
						Headers: snap.Headers,
					})
				}
			}
		}
		return routesList
	}

	// 1. GET /internal/api/status
	r.GET("/internal/api/status", wrapHandler(func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		wafAllowed := cfg.WAFAllowedIPs
		if wafAllowed == nil {
			wafAllowed = make([]string, 0)
		}
		wafDenied := cfg.WAFDeniedIPs
		if wafDenied == nil {
			wafDenied = make([]string, 0)
		}
		corsOrigins := cfg.CORSAllowedOrigins
		if corsOrigins == nil {
			corsOrigins = make([]string, 0)
		}

		var ociCount int
		if cfg.DiscoveryFunc != nil {
			ociCount = len(cfg.DiscoveryFunc())
		}

		payload := map[string]interface{}{
			"server":           "Toron",
			"version":          version.Get(),
			"uptime":           "healthy",
			"engine":           "event-driven",
			"port":             cfg.Port,
			"worker_pool_size": cfg.WorkerPoolSize,
			"metrics":          metrics.DefaultRegistry.GetSummaryJSON(),
			"discovery": map[string]interface{}{
				"enabled":          cfg.DiscoveryEnabled,
				"containers_count": ociCount,
			},
			"security": map[string]interface{}{
				"waf_enabled":            cfg.WAFEnabled,
				"waf_mode":               cfg.WAFMode,
				"waf_anomaly_threshold":  cfg.WAFAnomalyThreshold,
				"waf_rules_count":        cfg.WAFRulesCount,
				"waf_custom_rules_count": cfg.WAFCustomRulesCount,
				"waf_allowed_ips":        wafAllowed,
				"waf_denied_ips":         wafDenied,
				"cors_enabled":           cfg.CORSEnabled,
				"cors_allowed_origins":   corsOrigins,
				"security_headers":       cfg.SecurityHeadersEnabled,
				"mtls_enabled":           cfg.MTLSEnabled,
			},
		}
		data, _ := json.Marshal(payload)
		_, _ = res.Write(data)
	}))

	// 1b. GET /internal/api/metrics
	r.GET("/internal/api/metrics", wrapHandler(func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		data, _ := json.Marshal(metrics.DefaultRegistry.GetSummaryJSON())
		_, _ = res.Write(data)
	}))

	// 2. GET /internal/api/routes
	r.GET("/internal/api/routes", wrapHandler(func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")

		routesList := getAggregatedRoutes()

		payload := map[string]interface{}{
			"proxy_enabled": cfg.ProxyEnabled,
			"routes":        routesList,
			"static": map[string]interface{}{
				"enabled": cfg.StaticEnabled,
				"prefix":  cfg.StaticPrefix,
				"dir":     cfg.StaticDir,
			},
		}
		data, _ := json.Marshal(payload)
		_, _ = res.Write(data)
	}))

	// 3. GET /internal/api/upstreams/health
	r.GET("/internal/api/upstreams/health", wrapHandler(func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")

		type targetEntry struct {
			id        int
			port      int
			name      string
			route     string
			algo      string
			targetURL string
		}

		var targetEntries []targetEntry
		seenTargets := make(map[string]bool)
		targetID := 1

		for _, rInfo := range getAggregatedRoutes() {
			if len(rInfo.Targets) == 0 {
				continue
			}
			algo := rInfo.Algorithm
			if algo == "" {
				algo = "Round-Robin"
			}
			for _, t := range rInfo.Targets {
				cleanTarget := strings.TrimSpace(t)
				if cleanTarget == "" {
					continue
				}
				key := fmt.Sprintf("%s|%s", rInfo.Prefix, cleanTarget)
				if seenTargets[key] {
					continue
				}
				seenTargets[key] = true

				port := 80
				u, err := url.Parse(cleanTarget)
				if err == nil && u.Port() != "" {
					if p, pErr := url.Parse("http://" + u.Host); pErr == nil {
						_ = p
					}
					var pVal int
					if _, scanErr := fmt.Sscanf(u.Port(), "%d", &pVal); scanErr == nil && pVal > 0 {
						port = pVal
					}
				}

				name := cleanTarget
				if u != nil && u.Host != "" {
					name = u.Host
				}

				routeLabel := rInfo.Prefix
				if rInfo.Host != "" {
					routeLabel = fmt.Sprintf("%s%s", rInfo.Host, rInfo.Prefix)
				}

				targetEntries = append(targetEntries, targetEntry{
					id:        targetID,
					port:      port,
					name:      name,
					route:     routeLabel,
					algo:      algo,
					targetURL: cleanTarget,
				})
				targetID++
			}
		}

		results := make([]UpstreamNodeHealth, len(targetEntries))
		var wg sync.WaitGroup
		client := &http.Client{Timeout: 1500 * time.Millisecond}

		for i, entry := range targetEntries {
			wg.Add(1)
			go func(idx int, target targetEntry) {
				defer wg.Done()
				start := time.Now()
				probeURL := target.targetURL
				if !strings.HasPrefix(probeURL, "http://") && !strings.HasPrefix(probeURL, "https://") {
					probeURL = "http://" + probeURL
				}

				resp, err := client.Get(probeURL)
				latency := float64(time.Since(start).Microseconds()) / 1000.0

				node := UpstreamNodeHealth{
					ID:        target.id,
					Port:      target.port,
					Name:      target.name,
					Route:     target.route,
					Algo:      target.algo,
					LatencyMS: latency,
				}

				if err != nil {
					node.Status = "UNREACHABLE"
					node.HTTPCode = 0
				} else {
					_ = resp.Body.Close()
					node.HTTPCode = resp.StatusCode
					if resp.StatusCode < 400 {
						node.Status = "HEALTHY"
					} else if resp.StatusCode >= 500 {
						node.Status = fmt.Sprintf("OPEN (%d ERR)", resp.StatusCode)
					} else {
						node.Status = fmt.Sprintf("STATUS %d", resp.StatusCode)
					}
				}
				results[idx] = node
			}(i, entry)
		}

		wg.Wait()

		healthyCount := 0
		for _, n := range results {
			if n.Status == "HEALTHY" || n.Status == "CLOSED" || n.Status == "OK" {
				healthyCount++
			}
		}

		payload := map[string]interface{}{
			"timestamp":     time.Now().Format(time.RFC3339),
			"total_nodes":   len(results),
			"healthy_nodes": healthyCount,
			"upstreams":     results,
		}
		data, _ := json.Marshal(payload)
		_, _ = res.Write(data)
	}))

	// 4. POST /internal/api/proxy-test
	r.POST("/internal/api/proxy-test", wrapHandler(func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")

		const maxRequestBodyBytes = 64 * 1024 // 64 KB
		bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, maxRequestBodyBytes+1))
		if err != nil || len(bodyBytes) == 0 {
			res.SetStatus(http.StatusBadRequest)
			_, _ = res.WriteString(`{"error":"400 Bad Request","message":"Missing or invalid request body"}`)
			return
		}
		if int64(len(bodyBytes)) > maxRequestBodyBytes {
			res.SetStatus(http.StatusBadRequest)
			_, _ = res.WriteString(`{"error":"400 Bad Request","message":"Request body exceeds maximum allowed size of 64KB"}`)
			return
		}

		var testReq ProxyTestRequest
		if err := json.Unmarshal(bodyBytes, &testReq); err != nil {
			res.SetStatus(http.StatusBadRequest)
			_, _ = res.WriteString(`{"error":"400 Bad Request","message":"Invalid JSON body"}`)
			return
		}

		if testReq.Path == "" {
			testReq.Path = "/health"
		}
		if testReq.Method == "" {
			testReq.Method = "GET"
		}

		// Security: Constrain methods to safe diagnostic methods
		methodUpper := strings.ToUpper(testReq.Method)
		if methodUpper != "GET" && methodUpper != "HEAD" && methodUpper != "POST" {
			res.SetStatus(http.StatusBadRequest)
			_, _ = res.WriteString(`{"error":"400 Bad Request","message":"Method not allowed for proxy test probe"}`)
			return
		}

		// Security: Prevent SSRF & Authority Overrides
		parsedPath, err := url.Parse(testReq.Path)
		if err != nil || parsedPath.Scheme != "" || parsedPath.Host != "" || parsedPath.User != nil {
			res.SetStatus(http.StatusBadRequest)
			_, _ = res.WriteString(`{"error":"400 Bad Request","message":"Invalid or disallowed proxy test target path"}`)
			return
		}

		cleanPath := path.Clean(parsedPath.Path)
		if !strings.HasPrefix(cleanPath, "/") {
			cleanPath = "/" + cleanPath
		}

		// Strictly forbid internal management routes
		if strings.HasPrefix(cleanPath, "/internal") {
			res.SetStatus(http.StatusBadRequest)
			_, _ = res.WriteString(`{"error":"400 Bad Request","message":"Access to internal management routes via proxy-test is forbidden"}`)
			return
		}

		// Whitelist check: only allow safe diagnostic endpoints and configured non-internal routes
		allowedPaths := map[string]bool{
			"/health":     true,
			"/api/status": true,
		}
		for _, p := range cfg.AllowedProxyTestPaths {
			allowedPaths[p] = true
		}
		for _, r := range cfg.Routes {
			if r.Prefix != "" && !strings.HasPrefix(r.Prefix, "/internal") {
				allowedPaths[r.Prefix] = true
			}
		}

		if !allowedPaths[cleanPath] {
			res.SetStatus(http.StatusBadRequest)
			_, _ = res.WriteString(`{"error":"400 Bad Request","message":"Target path is not in the allowed diagnostic whitelist"}`)
			return
		}

		// Security: Reject sensitive identity header injections
		disallowedHeaders := map[string]bool{
			"x-authenticated-user": true,
			"x-admin":              true,
			"x-user":               true,
			"x-remote-user":        true,
		}
		for k := range testReq.Headers {
			if disallowedHeaders[strings.ToLower(k)] {
				res.SetStatus(http.StatusBadRequest)
				_, _ = res.WriteString(`{"error":"400 Bad Request","message":"Disallowed sensitive header in proxy test request"}`)
				return
			}
		}

		if parsedPath.RawQuery != "" {
			cleanPath += "?" + parsedPath.RawQuery
		}

		serverPort := cfg.Port
		targetURL := fmt.Sprintf("http://127.0.0.1:%d%s", serverPort, cleanPath)

		httpReq, err := http.NewRequest(methodUpper, targetURL, nil)
		if err != nil {
			res.SetStatus(http.StatusInternalServerError)
			_, _ = res.WriteString(fmt.Sprintf(`{"error":"500 Internal Error","message":%q}`, err.Error()))
			return
		}

		// Strip forwarded and hop-by-hop identity headers
		stripHeaders := map[string]bool{
			"x-forwarded-for":   true,
			"x-forwarded-host":  true,
			"x-forwarded-proto": true,
			"authorization":     true,
			"cookie":            true,
		}
		for k, v := range testReq.Headers {
			if !stripHeaders[strings.ToLower(k)] {
				httpReq.Header.Set(k, v)
			}
		}

		client := &http.Client{Timeout: 5 * time.Second}
		start := time.Now()
		httpResp, err := client.Do(httpReq)
		latency := float64(time.Since(start).Microseconds()) / 1000.0

		if err != nil {
			resOut := ProxyTestResponse{
				StatusCode: 502,
				StatusText: "Bad Gateway",
				LatencyMS:  latency,
				Headers:    map[string]string{"Content-Type": "application/json"},
				Body:       fmt.Sprintf(`{"error":"Connection to local server failed: %s"}`, err.Error()),
			}
			data, _ := json.Marshal(resOut)
			_, _ = res.Write(data)
			return
		}
		defer httpResp.Body.Close()

		maxResponseBytes := cfg.MaxProxyTestResponseBytes
		if maxResponseBytes <= 0 {
			maxResponseBytes = 1024 * 1024 // 1 MB default
		}

		respBodyBytes, _ := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes+1))
		truncated := false
		if int64(len(respBodyBytes)) > maxResponseBytes {
			respBodyBytes = respBodyBytes[:maxResponseBytes]
			truncated = true
			_, _ = io.Copy(io.Discard, io.LimitReader(httpResp.Body, 64*1024))
		}

		respHeaders := make(map[string]string)
		for k, v := range httpResp.Header {
			if len(v) > 0 {
				respHeaders[k] = v[0]
			}
		}

		resOut := ProxyTestResponse{
			StatusCode: httpResp.StatusCode,
			StatusText: http.StatusText(httpResp.StatusCode),
			LatencyMS:  latency,
			Headers:    respHeaders,
			Body:       string(respBodyBytes),
			Truncated:  truncated,
		}

		data, _ := json.Marshal(resOut)
		_, _ = res.Write(data)
	}))

	// 5. GET /internal/api/security/incidents
	r.GET("/internal/api/security/incidents", wrapHandler(func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		events := []waf.SecurityEvent{}
		if cfg.AuditLogger != nil {
			events = cfg.AuditLogger.GetRecentEvents(50)
		}
		if events == nil {
			events = make([]waf.SecurityEvent, 0)
		}
		payload := map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"total":     len(events),
			"incidents": events,
		}
		data, _ := json.Marshal(payload)
		_, _ = res.Write(data)
	}))
}
