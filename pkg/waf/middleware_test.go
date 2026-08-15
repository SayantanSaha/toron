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
