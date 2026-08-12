package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/router"
)

// RouteInfo represents routing metadata for internal route listings.
type RouteInfo struct {
	Host      string            `json:"host,omitempty"`
	Prefix    string            `json:"prefix"`
	Headers   map[string]string `json:"headers,omitempty"`
	Algorithm string            `json:"algorithm"`
	Targets   []string          `json:"targets"`
}

// InternalAPIConfig configures /internal/api/ route parameters without importing pkg/config.
type InternalAPIConfig struct {
	Port           int         `json:"port"`
	WorkerPoolSize int         `json:"worker_pool_size"`
	ProxyEnabled   bool        `json:"proxy_enabled"`
	Routes         []RouteInfo `json:"routes"`
	StaticEnabled  bool        `json:"static_enabled"`
	StaticPrefix   string      `json:"static_prefix"`
	StaticDir      string      `json:"static_dir"`
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

	// 1. GET /internal/api/status
	r.GET("/internal/api/status", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		payload := map[string]interface{}{
			"server":           "Toron",
			"version":          "1.0.0",
			"uptime":           "healthy",
			"engine":           "event-driven",
			"port":             cfg.Port,
			"worker_pool_size": cfg.WorkerPoolSize,
		}
		data, _ := json.Marshal(payload)
		_, _ = res.Write(data)
	})

	// 2. GET /internal/api/routes
	r.GET("/internal/api/routes", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")

		routesList := cfg.Routes
		if routesList == nil {
			routesList = make([]RouteInfo, 0)
		}

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

		upstreamsDef := []struct {
			id    int
			port  int
			name  string
			route string
			algo  string
		}{
			{1, 9001, "Dummy Service 1", "/api (v2)", "Round-Robin"},
			{2, 9002, "Dummy Service 2", "/api (v2)", "Round-Robin"},
			{3, 9003, "Dummy Service 3", "/api (v2)", "Round-Robin"},
			{4, 9004, "Dummy Service 4", "/api (v1)", "Single Target"},
			{5, 9005, "Dummy Service 5", "/services/cluster", "Round-Robin"},
			{6, 9006, "Dummy Service 6", "/services/cluster", "Round-Robin"},
			{7, 9007, "Dummy Service 7", "/services/cluster", "Round-Robin"},
			{8, 9008, "Dummy Service 8", "/services/auth", "Single Target"},
			{9, 9009, "Dummy Service 9", "/services/analytics", "Round-Robin"},
			{10, 9010, "Dummy Service 10", "/services/analytics", "Round-Robin"},
		}

		results := make([]UpstreamNodeHealth, len(upstreamsDef))
		var wg sync.WaitGroup
		client := &http.Client{Timeout: 2 * time.Second}

		for i, def := range upstreamsDef {
			wg.Add(1)
			go func(idx int, targetDef struct {
				id    int
				port  int
				name  string
				route string
				algo  string
			}) {
				defer wg.Done()
				start := time.Now()
				targetURL := fmt.Sprintf("http://localhost:%d/", targetDef.port)

				resp, err := client.Get(targetURL)
				latency := float64(time.Since(start).Microseconds()) / 1000.0

				node := UpstreamNodeHealth{
					ID:        targetDef.id,
					Port:      targetDef.port,
					Name:      targetDef.name,
					Route:     targetDef.route,
					Algo:      targetDef.algo,
					LatencyMS: latency,
				}

				if err != nil {
					node.Status = "UNREACHABLE"
					node.HTTPCode = 0
				} else {
					_ = resp.Body.Close()
					node.HTTPCode = resp.StatusCode
					if resp.StatusCode == http.StatusOK {
						node.Status = "CLOSED"
					} else if resp.StatusCode >= 500 {
						node.Status = fmt.Sprintf("OPEN (%d ERR)", resp.StatusCode)
					} else {
						node.Status = fmt.Sprintf("STATUS %d", resp.StatusCode)
					}
				}
				results[idx] = node
			}(i, def)
		}

		wg.Wait()

		payload := map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
			"upstreams": results,
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
		if !strings.HasPrefix(testReq.Path, "/") {
			testReq.Path = "/" + testReq.Path
		}
		if testReq.Method == "" {
			testReq.Method = "GET"
		}

		serverPort := cfg.Port
		targetURL := fmt.Sprintf("http://127.0.0.1:%d%s", serverPort, testReq.Path)

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
}
