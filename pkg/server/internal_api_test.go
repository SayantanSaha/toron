package server

import (
	"bytes"
	"encoding/json"
	"net"
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
			`{"path":"/secret-admin-area","method":"GET"}`,
			`{"path":"/internal/api/status","method":"GET"}`,
			`{"path":"/health","method":"DELETE"}`,
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
				t.Errorf("expected 400 Bad Request for disallowed payload %s, got %d", body, res.StatusCode)
			}
		}
	})

	t.Run("POST /internal/api/proxy-test Rejects Sensitive Headers", func(t *testing.T) {
		disallowedHeaderPayloads := []string{
			`{"path":"/health","method":"GET","headers":{"X-Authenticated-User":"root"}}`,
			`{"path":"/health","method":"GET","headers":{"X-Admin":"true"}}`,
			`{"path":"/health","method":"GET","headers":{"x-user":"admin"}}`,
		}

		for _, body := range disallowedHeaderPayloads {
			req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			req.Body = bytes.NewBufferString(body)

			res := httpparser.NewResponse()
			r.ServeHTTP(req, res)

			if res.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400 Bad Request when attempting to inject sensitive header %s, got %d", body, res.StatusCode)
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

type mockAddr struct {
	addr string
}

func (m *mockAddr) Network() string { return "tcp" }
func (m *mockAddr) String() string  { return m.addr }

type mockConn struct {
	net.Conn
	remoteAddr net.Addr
}

func (m *mockConn) RemoteAddr() net.Addr { return m.remoteAddr }

func TestInternalAPI_Authentication(t *testing.T) {
	cfg := InternalAPIConfig{
		Port:             8080,
		AdminAuthEnabled: true,
		AdminToken:       "super-admin-secret-999",
		AdminAPIKeys:     []string{"secondary-key-456"},
		AdminUsername:    "admin",
		AdminPassword:    "secure-pass-789",
	}

	r := router.New()
	RegisterInternalAPIRoutes(r, cfg)

	t.Run("Unauthenticated request returns 401", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/api/status", "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", res.StatusCode)
		}
		if res.Header.Get("WWW-Authenticate") == "" {
			t.Error("expected WWW-Authenticate header in 401 response")
		}
	})

	t.Run("Invalid token returns 401", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/api/status", "HTTP/1.1")
		req.Header.Set("X-Toron-Admin-Key", "wrong-secret")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", res.StatusCode)
		}
	})

	t.Run("Valid token via X-Toron-Admin-Key returns 200", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/api/status", "HTTP/1.1")
		req.Header.Set("X-Toron-Admin-Key", "super-admin-secret-999")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}
	})

	t.Run("Valid token via Authorization Bearer returns 200", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/api/metrics", "HTTP/1.1")
		req.Header.Set("Authorization", "Bearer super-admin-secret-999")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}
	})

	t.Run("Valid secondary key via Bearer returns 200", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/api/routes", "HTTP/1.1")
		req.Header.Set("Authorization", "Bearer secondary-key-456")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}
	})

	t.Run("Valid Basic Auth returns 200", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/api/status", "HTTP/1.1")
		// base64("admin:secure-pass-789") -> "YWRtaW46c2VjdXJlLXBhc3MtNzg5"
		req.Header.Set("Authorization", "Basic YWRtaW46c2VjdXJlLXBhc3MtNzg5")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}
	})
}

func TestInternalAPI_SubnetRestriction(t *testing.T) {
	cfg := InternalAPIConfig{
		Port:         8080,
		AdminSubnets: []string{"127.0.0.1/32", "10.0.0.0/16"},
	}

	r := router.New()
	RegisterInternalAPIRoutes(r, cfg)

	t.Run("Disallowed source IP returns 403 Forbidden", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/api/status", "HTTP/1.1")
		req.RawConn = &mockConn{remoteAddr: &mockAddr{addr: "198.51.100.25:52341"}}
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for IP outside subnet, got %d", res.StatusCode)
		}
	})

	t.Run("Allowed localhost IP returns 200 OK", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/api/status", "HTTP/1.1")
		req.RawConn = &mockConn{remoteAddr: &mockAddr{addr: "127.0.0.1:49152"}}
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK for allowed localhost IP, got %d", res.StatusCode)
		}
	})

	t.Run("Allowed private LAN subnet IP returns 200 OK", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/api/status", "HTTP/1.1")
		req.RawConn = &mockConn{remoteAddr: &mockAddr{addr: "10.0.4.12:51234"}}
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK for allowed 10.0.0.0/16 IP, got %d", res.StatusCode)
		}
	})
}
