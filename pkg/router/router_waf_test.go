package router_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"toron/pkg/httpparser"
	"toron/pkg/proxy"
	"toron/pkg/router"
	"toron/pkg/waf"
)

func TestRouter_RouteLevelWAF_IPACL(t *testing.T) {
	r := router.New()

	// Upstream test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"upstream success"}`))
	}))
	defer ts.Close()

	// Register route with route-level WAF restricting to 10.0.0.0/8 and denying 10.99.0.0/16
	routeWaf := waf.WAFConfig{
		Enabled:    true,
		Mode:       "enforce",
		AllowedIPs: []string{"10.0.0.0/8"},
		DeniedIPs:  []string{"10.99.0.0/16"},
	}

	opts := proxy.ProxyOptions{
		Targets:   []string{ts.URL},
		Algorithm: proxy.AlgorithmRoundRobin,
		WAF:       routeWaf,
	}

	if err := r.RoutePrefix(router.RouteTypeUpstream, "", "/secure/api", nil, "", opts); err != nil {
		t.Fatalf("failed to register route: %v", err)
	}

	// 1. Request from allowed IP (10.1.2.3)
	req1, _ := httpparser.NewRequest("GET", "/secure/api/test", "HTTP/1.1")
	req1.Header.Set("X-Forwarded-For", "10.1.2.3")
	res1 := &httpparser.Response{Header: make(httpparser.Header), Body: bytes.NewBuffer(nil)}
	r.ServeHTTP(req1, res1)
	if res1.StatusCode != 200 {
		t.Errorf("expected allowed IP to get 200, got %d (body: %s)", res1.StatusCode, res1.Body.String())
	}

	// 2. Request from denied subnet (10.99.1.1)
	req2, _ := httpparser.NewRequest("GET", "/secure/api/test", "HTTP/1.1")
	req2.Header.Set("X-Forwarded-For", "10.99.1.1")
	res2 := &httpparser.Response{Header: make(httpparser.Header), Body: bytes.NewBuffer(nil)}
	r.ServeHTTP(req2, res2)
	if res2.StatusCode != 403 {
		t.Errorf("expected denied IP to get 403, got %d", res2.StatusCode)
	}

	// 3. Request from IP outside allowlist (192.168.1.5)
	req3, _ := httpparser.NewRequest("GET", "/secure/api/test", "HTTP/1.1")
	req3.Header.Set("X-Forwarded-For", "192.168.1.5")
	res3 := &httpparser.Response{Header: make(httpparser.Header), Body: bytes.NewBuffer(nil)}
	r.ServeHTTP(req3, res3)
	if res3.StatusCode != 403 {
		t.Errorf("expected non-allowed IP to get 403, got %d", res3.StatusCode)
	}
}

func TestRouter_RouteLevelWAF_DisabledRules(t *testing.T) {
	r := router.New()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"upstream reached"}`))
	}))
	defer ts.Close()

	// Route 1: Default WAF (blocks SQLi)
	opts1 := proxy.ProxyOptions{
		Targets:   []string{ts.URL},
		Algorithm: proxy.AlgorithmRoundRobin,
		WAF: waf.WAFConfig{
			Enabled: true,
			Mode:    "enforce",
		},
	}
	_ = r.RoutePrefix(router.RouteTypeUpstream, "", "/strict", nil, "", opts1)

	// Route 2: WAF with SQLI-001 disabled
	opts2 := proxy.ProxyOptions{
		Targets:   []string{ts.URL},
		Algorithm: proxy.AlgorithmRoundRobin,
		WAF: waf.WAFConfig{
			Enabled:       true,
			Mode:          "enforce",
			DisabledRules: []string{"SQLI-001"},
		},
	}
	_ = r.RoutePrefix(router.RouteTypeUpstream, "", "/legacy", nil, "", opts2)

	// Test Route 1: Should block UNION SELECT (SQLI-001)
	req1, _ := httpparser.NewRequest("GET", "/strict/data?query=UNION+SELECT+1,2,3", "HTTP/1.1")
	res1 := &httpparser.Response{Header: make(httpparser.Header), Body: bytes.NewBuffer(nil)}
	r.ServeHTTP(req1, res1)
	if res1.StatusCode != 403 {
		t.Errorf("expected /strict to block SQLI-001 with 403, got %d", res1.StatusCode)
	}

	// Test Route 2: Should allow UNION SELECT (SQLI-001)
	req2, _ := httpparser.NewRequest("GET", "/legacy/data?query=UNION+SELECT+1,2,3", "HTTP/1.1")
	res2 := &httpparser.Response{Header: make(httpparser.Header), Body: bytes.NewBuffer(nil)}
	r.ServeHTTP(req2, res2)
	if res2.StatusCode != 200 {
		t.Errorf("expected /legacy to permit disabled rule SQLI-001, got %d (body: %s)", res2.StatusCode, res2.Body.String())
	}
}

func TestRouter_RouteLevelWAF_DetectionMode(t *testing.T) {
	r := router.New()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"staging received"}`))
	}))
	defer ts.Close()

	// Route in detection mode
	opts := proxy.ProxyOptions{
		Targets:   []string{ts.URL},
		Algorithm: proxy.AlgorithmRoundRobin,
		WAF: waf.WAFConfig{
			Enabled: true,
			Mode:    "detection",
		},
	}
	_ = r.RoutePrefix(router.RouteTypeUpstream, "", "/staging", nil, "", opts)

	req, _ := httpparser.NewRequest("GET", "/staging/test?q=%3Cscript%3Ealert(1)%3C/script%3E", "HTTP/1.1")
	res := &httpparser.Response{Header: make(httpparser.Header), Body: bytes.NewBuffer(nil)}
	r.ServeHTTP(req, res)

	if res.StatusCode != 200 {
		t.Errorf("expected detection mode to pass to upstream (status 200), got %d", res.StatusCode)
	}
	if res.Header.Get("X-Toron-WAF-Anomaly-Score") == "" {
		t.Error("expected X-Toron-WAF-Anomaly-Score to be present in detection mode")
	}
	body := res.BodyString()
	if !strings.Contains(body, "staging received") {
		t.Errorf("expected upstream body, got %s", body)
	}
}
