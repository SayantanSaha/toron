package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"toron/pkg/httpparser"
	"toron/pkg/router"
)

func TestInternalAPIRoutes(t *testing.T) {
	cfg := InternalAPIConfig{
		Port:           8080,
		WorkerPoolSize: 128,
		ProxyEnabled:   true,
		Routes: []RouteInfo{
			{Prefix: "/api", Headers: map[string]string{"X-Version": "v2"}, Algorithm: "round_robin", Targets: []string{"http://localhost:9001"}},
		},
		StaticEnabled: true,
		StaticPrefix:  "/",
		StaticDir:     "./public",
	}

	r := router.New()
	RegisterInternalAPIRoutes(r, cfg)

	t.Run("GET /internal/api/status", func(t *testing.T) {
		req, err := httpparser.NewRequest("GET", "/internal/api/status", "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", res.StatusCode)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to parse JSON response: %v", err)
		}

		if payload["server"] != "Toron" {
			t.Errorf("expected server Toron, got %v", payload["server"])
		}
	})

	t.Run("GET /internal/api/routes", func(t *testing.T) {
		req, err := httpparser.NewRequest("GET", "/internal/api/routes", "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", res.StatusCode)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to parse JSON response: %v", err)
		}

		if _, ok := payload["routes"]; !ok {
			t.Error("expected routes key in payload")
		}
	})

	t.Run("GET /internal/api/upstreams/health", func(t *testing.T) {
		req, err := httpparser.NewRequest("GET", "/internal/api/upstreams/health", "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", res.StatusCode)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to parse JSON response: %v", err)
		}

		upstreams, ok := payload["upstreams"].([]interface{})
		if !ok || len(upstreams) != 10 {
			t.Fatalf("expected 10 upstreams, got %v", payload["upstreams"])
		}
	})

	t.Run("POST /internal/api/proxy-test", func(t *testing.T) {
		// Mock local server for proxy test endpoint
		testSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"mock_ok"}`))
		}))
		defer testSrv.Close()

		reqBody := `{"path":"/health","method":"GET","headers":{"X-Version":"v2"}}`
		req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Body = bytes.NewBufferString(reqBody)

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", res.StatusCode)
		}
	})

	t.Run("GET /internal/api/security/incidents", func(t *testing.T) {
		req, err := httpparser.NewRequest("GET", "/internal/api/security/incidents", "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", res.StatusCode)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to parse JSON response: %v", err)
		}

		if _, ok := payload["incidents"]; !ok {
			t.Error("expected incidents key in payload")
		}
	})
}
