package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/metrics"
	"toron/pkg/router"
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
}

// RegisterInternalAPIRoutes registers /internal/api/ management routes on the router.
func RegisterInternalAPIRoutes(r *router.Router, cfg InternalAPIConfig) {
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.WorkerPoolSize == 0 {
		cfg.WorkerPoolSize = 128
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
	r.GET("/internal/api/status", func(req *httpparser.Request, res *httpparser.Response) {
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
			"version":          "1.5.0",
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
	})

	// 1b. GET /internal/api/metrics
	r.GET("/internal/api/metrics", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		data, _ := json.Marshal(metrics.DefaultRegistry.GetSummaryJSON())
		_, _ = res.Write(data)
	})

	// 2. GET /internal/api/routes
	r.GET("/internal/api/routes", func(req *httpparser.Request, res *httpparser.Response) {
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
	})

	// 3. GET /internal/api/upstreams/health
	r.GET("/internal/api/upstreams/health", func(req *httpparser.Request, res *httpparser.Response) {
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
	})

	// 4. POST /internal/api/proxy-test
	r.POST("/internal/api/proxy-test", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")

		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil || len(bodyBytes) == 0 {
			res.SetStatus(http.StatusBadRequest)
			_, _ = res.WriteString(`{"error":"400 Bad Request","message":"Missing or invalid request body"}`)
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

		// Security: Prevent SSRF & Authority Overrides
		parsedPath, err := url.Parse(testReq.Path)
		if err != nil || parsedPath.Scheme != "" || parsedPath.Host != "" || parsedPath.User != nil {
			res.SetStatus(http.StatusBadRequest)
			_, _ = res.WriteString(`{"error":"400 Bad Request","message":"Invalid or disallowed proxy test target path"}`)
			return
		}

		cleanPath := parsedPath.Path
		if !strings.HasPrefix(cleanPath, "/") {
			cleanPath = "/" + cleanPath
		}
		if parsedPath.RawQuery != "" {
			cleanPath += "?" + parsedPath.RawQuery
		}

		serverPort := cfg.Port
		targetURL := fmt.Sprintf("http://127.0.0.1:%d%s", serverPort, cleanPath)

		httpReq, err := http.NewRequest(strings.ToUpper(testReq.Method), targetURL, nil)
		if err != nil {
			res.SetStatus(http.StatusInternalServerError)
			_, _ = res.WriteString(fmt.Sprintf(`{"error":"500 Internal Error","message":%q}`, err.Error()))
			return
		}

		for k, v := range testReq.Headers {
			httpReq.Header.Set(k, v)
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

		respBodyBytes, _ := io.ReadAll(httpResp.Body)
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
		}

		data, _ := json.Marshal(resOut)
		_, _ = res.Write(data)
	})

	// 5. GET /internal/api/security/incidents
	r.GET("/internal/api/security/incidents", func(req *httpparser.Request, res *httpparser.Response) {
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
	})
}

