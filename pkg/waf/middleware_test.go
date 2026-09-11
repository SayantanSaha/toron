package waf

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"testing"

	"toron/pkg/httpparser"
	"toron/pkg/metrics"
)

func TestWAFMiddleware_EnforceAndDetection(t *testing.T) {
	// 1. Enforce mode test
	enforceCfg := DefaultConfig()
	enforceCfg.Mode = "enforce"
	engineEnforce, _ := NewEngine(enforceCfg)
	mwEnforce := NewWAFMiddleware(engineEnforce)

	nextCalled := false
	handler := mwEnforce(func(req *httpparser.Request, res *httpparser.Response) {
		nextCalled = true
	})

	u, _ := url.Parse("http://localhost/api?query=UNION+SELECT+1,2,3")
	req := &httpparser.Request{
		Method: "GET",
		Path:   u.Path,
		URL:    u,
		Header: make(httpparser.Header),
	}
	res := &httpparser.Response{
		Header: make(httpparser.Header),
		Body:   bytes.NewBuffer(nil),
	}

	handler(req, res)

	if nextCalled {
		t.Error("expected middleware to block request and NOT call next handler")
	}
	if res.StatusCode != 403 {
		t.Errorf("got status %d, want 403", res.StatusCode)
	}
	if !strings.Contains(res.Body.String(), "SQLI-001") {
		t.Errorf("expected response body to mention triggered rule SQLI-001, got: %s", res.Body.String())
	}

	// 2. Detection mode test
	detectCfg := DefaultConfig()
	detectCfg.Mode = "detection"
	engineDetect, _ := NewEngine(detectCfg)
	mwDetect := NewWAFMiddleware(engineDetect)

	nextCalledDetect := false
	handlerDetect := mwDetect(func(req *httpparser.Request, res *httpparser.Response) {
		nextCalledDetect = true
	})

	resDetect := &httpparser.Response{
		Header: make(httpparser.Header),
		Body:   bytes.NewBuffer(nil),
	}

	handlerDetect(req, resDetect)

	if !nextCalledDetect {
		t.Error("expected detection mode to pass request to next handler")
	}
	if resDetect.Header.Get("X-Toron-WAF-Anomaly-Score") == "" {
		t.Error("expected X-Toron-WAF-Anomaly-Score header to be set in detection mode")
	}
}

func TestWAFMiddleware_BenignPassThrough(t *testing.T) {
	cfg := DefaultConfig()
	engine, _ := NewEngine(cfg)
	mw := NewWAFMiddleware(engine)

	nextCalled := false
	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		nextCalled = true
		res.SetStatus(200)
		_, _ = res.WriteString("hello world")
	})

	u, _ := url.Parse("http://localhost/api/v1/health")
	req := &httpparser.Request{
		Method: "GET",
		Path:   u.Path,
		URL:    u,
		Header: make(httpparser.Header),
	}
	res := &httpparser.Response{
		Header: make(httpparser.Header),
		Body:   bytes.NewBuffer(nil),
	}

	handler(req, res)

	if !nextCalled {
		t.Error("expected benign request to pass through WAF middleware")
	}
	if res.StatusCode != 200 {
		t.Errorf("got status %d, want 200", res.StatusCode)
	}
}

func TestWAFMiddleware_IPACL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DeniedIPs = []string{"198.51.100.0/24"}
	cfg.AllowedIPs = []string{"10.0.0.0/8", "127.0.0.1"}
	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	mw := NewWAFMiddleware(engine)
	nextCalled := false
	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		nextCalled = true
		res.SetStatus(200)
	})

	// 1. Test Denied IP
	u, _ := url.Parse("http://localhost/test")
	reqDenied := &httpparser.Request{
		Method: "GET",
		Path:   u.Path,
		URL:    u,
		Header: make(httpparser.Header),
	}
	reqDenied.Header.Set("X-Forwarded-For", "198.51.100.45")
	resDenied := &httpparser.Response{
		Header: make(httpparser.Header),
		Body:   bytes.NewBuffer(nil),
	}
	handler(reqDenied, resDenied)

	if nextCalled {
		t.Error("expected denied IP to be blocked")
	}
	if resDenied.StatusCode != 403 {
		t.Errorf("expected status 403, got %d", resDenied.StatusCode)
	}
	if !strings.Contains(resDenied.Body.String(), "denied") {
		t.Errorf("expected body to mention denied reason, got: %s", resDenied.Body.String())
	}

	// 2. Test Allowed IP
	nextCalled = false
	reqAllowed := &httpparser.Request{
		Method: "GET",
		Path:   u.Path,
		URL:    u,
		Header: make(httpparser.Header),
	}
	reqAllowed.Header.Set("X-Forwarded-For", "10.1.2.3")
	resAllowed := &httpparser.Response{
		Header: make(httpparser.Header),
		Body:   bytes.NewBuffer(nil),
	}
	handler(reqAllowed, resAllowed)

	if !nextCalled {
		t.Error("expected allowed IP to pass")
	}
	if resAllowed.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resAllowed.StatusCode)
	}

	// 3. Test Disallowed IP (not in allowlist)
	nextCalled = false
	reqDisallowed := &httpparser.Request{
		Method: "GET",
		Path:   u.Path,
		URL:    u,
		Header: make(httpparser.Header),
	}
	reqDisallowed.Header.Set("X-Forwarded-For", "172.16.0.1")
	resDisallowed := &httpparser.Response{
		Header: make(httpparser.Header),
		Body:   bytes.NewBuffer(nil),
	}
	handler(reqDisallowed, resDisallowed)

	if nextCalled {
		t.Error("expected disallowed IP to be blocked")
	}
	if resDisallowed.StatusCode != 403 {
		t.Errorf("expected status 403, got %d", resDisallowed.StatusCode)
	}
}

func TestWAFMiddleware_DisabledRules(t *testing.T) {
	// Engine with SQLI-001 disabled
	cfg := DefaultConfig()
	cfg.DisabledRules = []string{"SQLI-001"}
	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	mw := NewWAFMiddleware(engine)
	nextCalled := false
	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		nextCalled = true
		res.SetStatus(200)
	})

	u, _ := url.Parse("http://localhost/api?q=UNION+SELECT+1,2,3")
	req := &httpparser.Request{
		Method: "GET",
		Path:   u.Path,
		URL:    u,
		Header: make(httpparser.Header),
	}
	res := &httpparser.Response{
		Header: make(httpparser.Header),
		Body:   bytes.NewBuffer(nil),
	}
	handler(req, res)

	if !nextCalled {
		t.Error("expected disabled rule payload to pass through")
	}
	if res.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}
}

func TestWAFMiddleware_TelemetryAndAuditLog(t *testing.T) {
	var logBuf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Mode = "enforce"
	engine, _ := NewEngine(cfg)
	engine.SetAuditLogger(NewAuditLoggerWithWriter(&logBuf))

	mw := NewWAFMiddleware(engine)
	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(200)
	})

	u, _ := url.Parse("http://localhost/api/users?q=%3Cscript%3Ealert('xss')%3C/script%3E")
	req := &httpparser.Request{
		Method: "GET",
		Path:   u.Path,
		URL:    u,
		Header: make(httpparser.Header),
	}
	req.Header.Set("X-Forwarded-For", "203.0.113.88")
	res := &httpparser.Response{
		Header: make(httpparser.Header),
		Body:   bytes.NewBuffer(nil),
	}

	handler(req, res)

	if res.StatusCode != 403 {
		t.Errorf("expected status 403, got %d", res.StatusCode)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "waf_block") {
		t.Errorf("expected log output to contain waf_block, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "203.0.113.88") {
		t.Errorf("expected log output to contain client IP 203.0.113.88, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "XSS-001") {
		t.Errorf("expected log output to contain XSS-001, got: %s", logOutput)
	}
}

func TestWAFMiddleware_NilClientIP_AllowlistEnforced(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = "enforce"
	cfg.Enabled = true
	cfg.AllowedIPs = []string{"10.0.0.0/8"}
	cfg.DeniedIPs = []string{"198.51.100.0/24"}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	mw := NewWAFMiddleware(engine)
	nextCalled := false
	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		nextCalled = true
		res.SetStatus(200)
	})

	u, _ := url.Parse("http://localhost/api/protected")
	req := &httpparser.Request{
		Method:     "GET",
		Path:       u.Path,
		URL:        u,
		Header:     make(httpparser.Header),
		RemoteAddr: "",
		RawConn:    nil,
	}
	res := &httpparser.Response{
		Header: make(httpparser.Header),
		Body:   bytes.NewBuffer(nil),
	}

	// Capture metrics baseline
	initialSummary := metrics.DefaultRegistry.GetSummaryJSON()
	var initialBlocked uint64
	if wafSummary, ok := initialSummary["waf"].(map[string]interface{}); ok {
		if catMap, ok := wafSummary["blocked_by_category"].(map[string]uint64); ok {
			initialBlocked = catMap["ip_acl"]
		}
	}

	handler(req, res)

	if nextCalled {
		t.Errorf("expected downstream handler not to be called when client IP is nil under allowlist")
	}
	if res.StatusCode != 403 {
		t.Errorf("got status %d, want 403", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("got Content-Type %q, want %q", ct, "application/json")
	}

	expectedBody := `{"error":"Forbidden","message":"client IP could not be determined and allowed IP list is enforced"}`
	if res.Body.String() != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, res.Body.String())
	}

	// Check metrics increment
	afterSummary := metrics.DefaultRegistry.GetSummaryJSON()
	var afterBlocked uint64
	if wafSummary, ok := afterSummary["waf"].(map[string]interface{}); ok {
		if catMap, ok := wafSummary["blocked_by_category"].(map[string]uint64); ok {
			afterBlocked = catMap["ip_acl"]
		}
	}
	if afterBlocked != initialBlocked+1 {
		t.Errorf("expected ip_acl blocked count to increment from %d to %d, got %d", initialBlocked, initialBlocked+1, afterBlocked)
	}

	promOutput := metrics.DefaultRegistry.ExportPrometheus()
	expectedMetricKey := `toron_waf_blocked_requests_total{category="ip_acl",route="/api/protected"}`
	if !strings.Contains(promOutput, expectedMetricKey) {
		t.Errorf("expected prometheus metrics to contain %q", expectedMetricKey)
	}
}

func TestWAFMiddleware_NilClientIP_DenylistOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = "enforce"
	cfg.Enabled = true
	cfg.AllowedIPs = nil
	cfg.DeniedIPs = []string{"198.51.100.0/24"}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	mw := NewWAFMiddleware(engine)
	nextCalled := false
	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		nextCalled = true
		res.SetStatus(200)
		_, _ = res.WriteString("success")
	})

	u, _ := url.Parse("http://localhost/api/test")
	req := &httpparser.Request{
		Method:     "GET",
		Path:       u.Path,
		URL:        u,
		Header:     make(httpparser.Header),
		RemoteAddr: "",
		RawConn:    nil,
	}
	res := &httpparser.Response{
		Header: make(httpparser.Header),
		Body:   bytes.NewBuffer(nil),
	}

	handler(req, res)

	if !nextCalled {
		t.Errorf("expected downstream handler to be called for nil IP in denylist-only configuration")
	}
	if res.StatusCode != 200 {
		t.Errorf("got status %d, want 200", res.StatusCode)
	}
	if res.Body.String() != "success" {
		t.Errorf("got body %q, want %q", res.Body.String(), "success")
	}
}

func TestWAFMiddleware_MalformedClientIP_AllowlistEnforced(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = "enforce"
	cfg.Enabled = true
	cfg.AllowedIPs = []string{"10.0.0.0/8"}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}
	mw := NewWAFMiddleware(engine)

	scenarios := []struct {
		name       string
		remoteAddr string
		xff        string
		xri        string
	}{
		{name: "Malformed RemoteAddr with port", remoteAddr: "not-an-ip:9999"},
		{name: "Malformed RemoteAddr IPv6 colons", remoteAddr: ":::invalid"},
		{name: "Bare hostname in RemoteAddr", remoteAddr: "hostname-without-ip"},
		{name: "XFF unknown keyword", xff: "unknown"},
		{name: "XFF localhost hostname", xff: "localhost"},
		{name: "XFF out-of-range octets", xff: "999.999.999.999"},
		{name: "XFF arbitrary garbage", xff: "garbage-header"},
		{name: "X-Real-IP corrupt string", xri: "invalid-real-ip"},
		{name: "X-Real-IP unclosed bracket IPv6", xri: "[bad-ipv6"},
	}

	const expectedBody = `{"error":"Forbidden","message":"client IP could not be determined and allowed IP list is enforced"}`

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			u, _ := url.Parse("http://localhost/api/data")
			req := &httpparser.Request{
				Method:     "GET",
				Path:       u.Path,
				URL:        u,
				Header:     make(httpparser.Header),
				RemoteAddr: sc.remoteAddr,
			}
			if sc.xff != "" {
				req.Header.Set("X-Forwarded-For", sc.xff)
			}
			if sc.xri != "" {
				req.Header.Set("X-Real-IP", sc.xri)
			}

			// Verify ExtractClientIP evaluates to nil
			extracted := ExtractClientIP(req)
			if extracted != nil {
				t.Fatalf("expected ExtractClientIP to return nil for malformed input %v, got %v", sc, extracted)
			}

			nextCalled := false
			handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
				nextCalled = true
				res.SetStatus(200)
			})

			res := &httpparser.Response{
				Header: make(httpparser.Header),
				Body:   bytes.NewBuffer(nil),
			}

			handler(req, res)

			if nextCalled {
				t.Errorf("expected handler not to be called for malformed client IP")
			}
			if res.StatusCode != 403 {
				t.Errorf("got status %d, want 403", res.StatusCode)
			}
			if ct := res.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("got Content-Type %q, want %q", ct, "application/json")
			}
			if res.Body.String() != expectedBody {
				t.Errorf("got body %q, want %q", res.Body.String(), expectedBody)
			}
		})
	}
}

func TestWAFMiddleware_AuditLogging_NilIP_Blocked(t *testing.T) {
	var logBuf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Mode = "enforce"
	cfg.Enabled = true
	cfg.AllowedIPs = []string{"10.0.0.0/8"}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}
	engine.SetAuditLogger(NewAuditLoggerWithWriter(&logBuf))

	mw := NewWAFMiddleware(engine)
	nextCalled := false
	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		nextCalled = true
		res.SetStatus(200)
	})

	u, _ := url.Parse("http://localhost/internal/admin")
	req := &httpparser.Request{
		Method:     "POST",
		Path:       u.Path,
		URL:        u,
		Header:     make(httpparser.Header),
		RemoteAddr: "",
		RawConn:    nil,
	}
	res := &httpparser.Response{
		Header: make(httpparser.Header),
		Body:   bytes.NewBuffer(nil),
	}

	handler(req, res)

	if nextCalled {
		t.Errorf("expected handler not to be called")
	}
	if res.StatusCode != 403 {
		t.Errorf("got status %d, want 403", res.StatusCode)
	}

	logLine := strings.TrimSpace(logBuf.String())
	if logLine == "" {
		t.Fatalf("expected audit log output, got empty buffer")
	}

	var event SecurityEvent
	if err := json.Unmarshal([]byte(logLine), &event); err != nil {
		t.Fatalf("failed to unmarshal audit log event: %v; raw log: %s", err, logLine)
	}

	if event.Event != "ip_acl_block" {
		t.Errorf("got event %q, want %q", event.Event, "ip_acl_block")
	}
	if event.ClientIP != "" {
		t.Errorf("got client_ip %q, want empty string", event.ClientIP)
	}
	if event.Method != "POST" {
		t.Errorf("got method %q, want %q", event.Method, "POST")
	}
	if event.Path != "/internal/admin" {
		t.Errorf("got path %q, want %q", event.Path, "/internal/admin")
	}
	if event.Category != "ip_acl" {
		t.Errorf("got category %q, want %q", event.Category, "ip_acl")
	}
	if event.Action != "blocked" {
		t.Errorf("got action %q, want %q", event.Action, "blocked")
	}
	if event.Location != "remote_addr" {
		t.Errorf("got location %q, want %q", event.Location, "remote_addr")
	}
}

func TestWAFMiddleware_SinglePassExtraction_TelemetryParity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = "enforce"
	cfg.Enabled = true
	cfg.AllowedIPs = []string{"10.0.0.0/8"}
	cfg.DeniedIPs = []string{"10.99.99.99/32"}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	var logBuf bytes.Buffer
	engine.SetAuditLogger(NewAuditLoggerWithWriter(&logBuf))
	mw := NewWAFMiddleware(engine)

	// Flow 7A: Valid Allowed IP
	t.Run("Allowed IP Flow", func(t *testing.T) {
		logBuf.Reset()
		u, _ := url.Parse("http://localhost/api/resource")
		req := &httpparser.Request{
			Method:     "GET",
			Path:       u.Path,
			URL:        u,
			Header:     make(httpparser.Header),
			RemoteAddr: "10.1.2.3:55555",
		}
		res := &httpparser.Response{
			Header: make(httpparser.Header),
			Body:   bytes.NewBuffer(nil),
		}

		nextCalled := false
		handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
			nextCalled = true
			res.SetStatus(200)
			_, _ = res.WriteString("ok")
		})

		handler(req, res)

		if !nextCalled {
			t.Errorf("expected allowed IP to reach downstream handler")
		}
		if res.StatusCode != 200 {
			t.Errorf("got status %d, want 200", res.StatusCode)
		}
	})

	// Flow 7B: Valid Denied IP
	t.Run("Denied IP Flow", func(t *testing.T) {
		logBuf.Reset()
		u, _ := url.Parse("http://localhost/api/resource")
		req := &httpparser.Request{
			Method:     "GET",
			Path:       u.Path,
			URL:        u,
			Header:     make(httpparser.Header),
			RemoteAddr: "10.99.99.99:55555",
		}
		res := &httpparser.Response{
			Header: make(httpparser.Header),
			Body:   bytes.NewBuffer(nil),
		}

		nextCalled := false
		handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
			nextCalled = true
			res.SetStatus(200)
		})

		handler(req, res)

		if nextCalled {
			t.Errorf("expected denied IP not to reach downstream handler")
		}
		if res.StatusCode != 403 {
			t.Errorf("got status %d, want 403", res.StatusCode)
		}
		expectedReason := "IP address 10.99.99.99 matches denied CIDR 10.99.99.99/32"
		if !strings.Contains(res.Body.String(), expectedReason) {
			t.Errorf("expected body to contain %q, got %q", expectedReason, res.Body.String())
		}

		logLine := strings.TrimSpace(logBuf.String())
		var event SecurityEvent
		if err := json.Unmarshal([]byte(logLine), &event); err != nil {
			t.Fatalf("failed to unmarshal audit log event: %v", err)
		}
		if event.ClientIP != "10.99.99.99" {
			t.Errorf("got audit client_ip %q, want %q", event.ClientIP, "10.99.99.99")
		}
		if event.Event != "ip_acl_block" {
			t.Errorf("got audit event %q, want %q", event.Event, "ip_acl_block")
		}
	})

	// Flow 7C: Unidentifiable Client IP
	t.Run("Nil Client IP Flow", func(t *testing.T) {
		logBuf.Reset()
		u, _ := url.Parse("http://localhost/api/resource")
		req := &httpparser.Request{
			Method:     "GET",
			Path:       u.Path,
			URL:        u,
			Header:     make(httpparser.Header),
			RemoteAddr: "",
		}
		res := &httpparser.Response{
			Header: make(httpparser.Header),
			Body:   bytes.NewBuffer(nil),
		}

		nextCalled := false
		handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
			nextCalled = true
			res.SetStatus(200)
		})

		handler(req, res)

		if nextCalled {
			t.Errorf("expected nil IP not to reach downstream handler")
		}
		if res.StatusCode != 403 {
			t.Errorf("got status %d, want 403", res.StatusCode)
		}
		expectedReason := "client IP could not be determined and allowed IP list is enforced"
		if !strings.Contains(res.Body.String(), expectedReason) {
			t.Errorf("expected body to contain %q, got %q", expectedReason, res.Body.String())
		}

		logLine := strings.TrimSpace(logBuf.String())
		var event SecurityEvent
		if err := json.Unmarshal([]byte(logLine), &event); err != nil {
			t.Fatalf("failed to unmarshal audit log event: %v", err)
		}
		if event.ClientIP != "" {
			t.Errorf("got audit client_ip %q, want empty string", event.ClientIP)
		}
		if event.Event != "ip_acl_block" {
			t.Errorf("got audit event %q, want %q", event.Event, "ip_acl_block")
		}
	})
}

func TestWAFMiddleware_NilClientIP_Concurrency(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = "enforce"
	cfg.Enabled = true
	cfg.AllowedIPs = []string{"10.0.0.0/8"}
	cfg.DeniedIPs = []string{"198.51.100.0/24"}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	safeLogWriter := &concurrentSafeBuffer{}
	engine.SetAuditLogger(NewAuditLoggerWithWriter(safeLogWriter))

	mw := NewWAFMiddleware(engine)
	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(200)
		_, _ = res.WriteString("ok")
	})

	const numGoroutines = 100
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				u, _ := url.Parse("http://localhost/api/test")
				req := &httpparser.Request{
					Method: "GET",
					Path:   u.Path,
					URL:    u,
					Header: make(httpparser.Header),
				}
				res := &httpparser.Response{
					Header: make(httpparser.Header),
					Body:   bytes.NewBuffer(nil),
				}

				switch {
				case gid < 25:
					// Unidentifiable IP
					req.RemoteAddr = ""
					handler(req, res)
					if res.StatusCode != 403 {
						t.Errorf("gid %d: expected 403 for unidentifiable IP, got %d", gid, res.StatusCode)
					}
					if !strings.Contains(res.Body.String(), "client IP could not be determined") {
						t.Errorf("gid %d: unexpected body %s", gid, res.Body.String())
					}
				case gid < 50:
					// Malformed IP
					req.RemoteAddr = "corrupt:addr"
					handler(req, res)
					if res.StatusCode != 403 {
						t.Errorf("gid %d: expected 403 for malformed IP, got %d", gid, res.StatusCode)
					}
					if !strings.Contains(res.Body.String(), "client IP could not be determined") {
						t.Errorf("gid %d: unexpected body %s", gid, res.Body.String())
					}
				case gid < 75:
					// Allowed IP
					req.RemoteAddr = "10.0.1.5:8080"
					handler(req, res)
					if res.StatusCode != 200 {
						t.Errorf("gid %d: expected 200 for allowed IP, got %d", gid, res.StatusCode)
					}
				default:
					// Denied IP
					req.RemoteAddr = "198.51.100.20:8080"
					handler(req, res)
					if res.StatusCode != 403 {
						t.Errorf("gid %d: expected 403 for denied IP, got %d", gid, res.StatusCode)
					}
					if !strings.Contains(res.Body.String(), "matches denied CIDR") {
						t.Errorf("gid %d: unexpected body %s", gid, res.Body.String())
					}
				}
			}
		}(g)
	}

	wg.Wait()
}

func TestWAFMiddleware_ConcurrentRaceClean(t *testing.T) {
	TestWAFMiddleware_NilClientIP_Concurrency(t)
}

type concurrentSafeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *concurrentSafeBuffer) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// TC-107-01: Verification of Connection: close Header on WAF Rejection Paths
func TestWAFMiddleware_ConnectionCloseOnSecurityRejection(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = "enforce"
	cfg.Enabled = true
	cfg.AllowedIPs = []string{"10.0.0.0/8"}
	cfg.DeniedIPs = []string{"198.51.100.0/24"}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	mw := NewWAFMiddleware(engine)
	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(200)
		_, _ = res.WriteString("ok")
	})

	// 1. IP ACL Denial
	t.Run("IP_ACL_Block", func(t *testing.T) {
		req := &httpparser.Request{
			Method:     "GET",
			Path:       "/api/resource",
			RemoteAddr: "198.51.100.55:12345",
		}
		res := httpparser.NewResponse()
		handler(req, res)
		if res.StatusCode != 403 {
			t.Fatalf("expected 403, got %d", res.StatusCode)
		}
		if res.Header.Get("Connection") != "close" {
			t.Errorf("expected Connection: close on IP ACL block, got %q", res.Header.Get("Connection"))
		}
	})

	// 2. Protocol Integrity Violation (Null Byte in URI)
	t.Run("Protocol_Integrity_Block", func(t *testing.T) {
		req := &httpparser.Request{
			Method:     "GET",
			Path:       "/api/test\x00/admin",
			RemoteAddr: "10.0.0.1:12345",
		}
		res := httpparser.NewResponse()
		handler(req, res)
		if res.StatusCode != 400 {
			t.Fatalf("expected 400, got %d", res.StatusCode)
		}
		if res.Header.Get("Connection") != "close" {
			t.Errorf("expected Connection: close on protocol violation, got %q", res.Header.Get("Connection"))
		}
	})

	// 3. Layer 7 OWASP Threat Block (Path Traversal payload)
	t.Run("L7_OWASP_Block", func(t *testing.T) {
		u, _ := url.Parse("http://localhost/api/test?file=../../../../etc/passwd")
		req := &httpparser.Request{
			Method:     "GET",
			Path:       u.Path,
			URL:        u,
			RemoteAddr: "10.0.0.1:12345",
		}
		res := httpparser.NewResponse()
		handler(req, res)
		if res.StatusCode != 403 {
			t.Fatalf("expected 403, got %d", res.StatusCode)
		}
		if res.Header.Get("Connection") != "close" {
			t.Errorf("expected Connection: close on L7 threat block, got %q", res.Header.Get("Connection"))
		}
	})
}
