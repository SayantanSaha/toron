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
