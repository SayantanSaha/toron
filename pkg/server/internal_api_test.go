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
		if !ok || len(upstreams) != 1 {
			t.Fatalf("expected 1 dynamically discovered upstream, got %v", payload["upstreams"])
		}

		firstNode, isMap := upstreams[0].(map[string]interface{})
		if !isMap || firstNode["port"] != float64(9001) {
			t.Fatalf("expected discovered upstream on port 9001, got %v", upstreams[0])
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

	t.Run("POST /internal/api/proxy-test SSRF Rejection", func(t *testing.T) {
		ssrfPayloads := []string{
			`{"path":"http://evil.com/ssrf","method":"GET"}`,
			`{"path":"//evil.com/ssrf","method":"GET"}`,
			`{"path":"https://169.254.169.254/latest/meta-data","method":"GET"}`,
		}

		for _, body := range ssrfPayloads {
			req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			req.Body = bytes.NewBufferString(body)

			res := httpparser.NewResponse()
			r.ServeHTTP(req, res)

			if res.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400 Bad Request for SSRF payload %s, got %d", body, res.StatusCode)
			}
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
