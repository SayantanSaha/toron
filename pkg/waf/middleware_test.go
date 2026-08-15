package waf

import (
	"bytes"
	"net/url"
	"strings"
	"testing"

	"toron/pkg/httpparser"
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


