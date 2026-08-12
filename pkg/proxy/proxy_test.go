package proxy_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/proxy"
)

func TestReverseProxy_ForwardRequest(t *testing.T) {
	// Start mock upstream HTTP server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users" {
			t.Errorf("expected upstream path /api/users, got %q", r.URL.Path)
		}
		if r.Header.Get("X-Forwarded-Host") != "toron.local" {
			t.Errorf("expected X-Forwarded-Host toron.local, got %q", r.Header.Get("X-Forwarded-Host"))
		}

		bodyBytes, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"echo":%q}`, string(bodyBytes))
	}))
	defer upstreamServer.Close()

	px, err := proxy.NewReverseProxy(upstreamServer.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create reverse proxy: %v", err)
	}

	req, err := httpparser.NewRequest("POST", "/api/users", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}
	req.Header.Set("Host", "toron.local")
	req.Body = strings.NewReader("hello upstream")

	res := httpparser.NewResponse()

	px.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", res.StatusCode)
	}
	if res.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json Content-Type, got %q", res.Header.Get("Content-Type"))
	}
	expectedBody := `{"echo":"hello upstream"}`
	if res.Body.String() != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, res.Body.String())
	}
}

func TestReverseProxy_BadGateway(t *testing.T) {
	// Target an offline port
	px, err := proxy.NewReverseProxy("http://127.0.0.1:59999", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
	res := httpparser.NewResponse()

	px.ServeHTTP(req, res)

	if res.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502 Bad Gateway, got %d", res.StatusCode)
	}
}

func TestReverseProxy_RoundRobinLoadBalancing(t *testing.T) {
	var count1, count2 int

	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count1++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server-1"))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count2++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server-2"))
	}))
	defer server2.Close()

	targets := []string{server1.URL, server2.URL}
	px, err := proxy.NewLoadBalancerProxy(targets, proxy.AlgorithmRoundRobin, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create load balancer proxy: %v", err)
	}

	if px.Balancer.Algorithm() != proxy.AlgorithmRoundRobin {
		t.Errorf("expected round_robin algorithm, got %s", px.Balancer.Algorithm())
	}

	// Make 4 requests, expecting round-robin distribution: s1, s2, s1, s2
	expectedResponses := []string{"server-1", "server-2", "server-1", "server-2"}
	for i, expected := range expectedResponses {
		req, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Errorf("request %d: expected 200 OK, got %d", i, res.StatusCode)
		}
		if res.Body.String() != expected {
			t.Errorf("request %d: expected body %q, got %q", i, expected, res.Body.String())
		}
	}

	if count1 != 2 || count2 != 2 {
		t.Errorf("expected 2 requests each, got server1=%d, server2=%d", count1, count2)
	}
}

func TestLoadBalancer_Validation(t *testing.T) {
	// Empty targets
	_, err := proxy.NewLoadBalancerProxy(nil, proxy.AlgorithmRoundRobin, time.Second)
	if err == nil {
		t.Error("expected error for empty target URLs")
	}

	// Unsupported algorithm
	_, err = proxy.NewLoadBalancerProxy([]string{"http://localhost:8080"}, proxy.Algorithm("unknown_algo"), time.Second)
	if err == nil {
		t.Error("expected error for unsupported algorithm")
	}
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("healthy"))
	}))
	defer healthyServer.Close()

	offlinePort := "http://127.0.0.1:59998"

	opts := proxy.ProxyOptions{
		Targets:             []string{healthyServer.URL, offlinePort},
		Algorithm:           proxy.AlgorithmRoundRobin,
		Timeout:             500 * time.Millisecond,
		MaxFailures:         2,
		CooldownPeriod:      200 * time.Millisecond,
		HealthCheckPath:     "/health",
		HealthCheckInterval: 50 * time.Millisecond,
	}

	px, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		t.Fatalf("failed to create proxy with options: %v", err)
	}
	defer px.Close()

	// Wait for background active health checker to trip offline server to Open
	time.Sleep(150 * time.Millisecond)

	// Make 4 requests - all should automatically route to healthyServer because offline target is Open/tripped
	for i := 0; i < 4; i++ {
		req, _ := httpparser.NewRequest("GET", "/", "HTTP/1.1")
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Errorf("request %d: expected 200 OK from healthy node, got %d", i, res.StatusCode)
		}
		if res.Body.String() != "healthy" {
			t.Errorf("request %d: expected body 'healthy', got %q", i, res.Body.String())
		}
	}
}
