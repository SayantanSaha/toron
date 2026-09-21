package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/router"
	"toron/pkg/waf"
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

	t.Run("Auto-Ban Endpoints: banned-ips, ban, unban", func(t *testing.T) {
		autoBanMgr, err := waf.NewAutoBanManager(waf.AutoBanConfig{
			Enabled:          true,
			MaxViolations:    3,
			Window:           1 * time.Minute,
			BanDuration:      1 * time.Hour,
			MaxTemporaryBans: 3,
			PersistenceFile:  filepath.Join(t.TempDir(), "banned_ips.json"),
		}, nil)
		if err != nil {
			t.Fatalf("failed to create auto-ban manager: %v", err)
		}
		defer autoBanMgr.Close()

		testRouter := router.New()
		RegisterInternalAPIRoutes(testRouter, InternalAPIConfig{
			Port:           8080,
			AutoBanManager: autoBanMgr,
		})

		// 1. Initially empty
		req1, _ := httpparser.NewRequest("GET", "/internal/api/security/banned-ips", "HTTP/1.1")
		res1 := httpparser.NewResponse()
		testRouter.ServeHTTP(req1, res1)
		if res1.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res1.StatusCode)
		}

		// 2. Ban IP
		banBody := `{"ip":"198.51.100.99","type":"temporary","duration":"30m","reason":"Manual test ban"}`
		req2, _ := httpparser.NewRequest("POST", "/internal/api/security/ban", "HTTP/1.1")
		req2.Body = io.NopCloser(strings.NewReader(banBody))
		res2 := httpparser.NewResponse()
		testRouter.ServeHTTP(req2, res2)
		if res2.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for ban, got %d: %s", res2.StatusCode, res2.Body.String())
		}

		// 3. Verify IP listed in banned-ips
		req3, _ := httpparser.NewRequest("GET", "/internal/api/security/banned-ips", "HTTP/1.1")
		res3 := httpparser.NewResponse()
		testRouter.ServeHTTP(req3, res3)
		var listPayload struct {
			Total     int `json:"total"`
			BannedIPs []struct {
				IP   string `json:"ip"`
				Type string `json:"type"`
			} `json:"banned_ips"`
		}
		if err := json.Unmarshal(res3.Body.Bytes(), &listPayload); err != nil {
			t.Fatalf("failed to parse banned-ips response: %v", err)
		}
		if listPayload.Total != 1 || listPayload.BannedIPs[0].IP != "198.51.100.99" {
			t.Fatalf("expected 1 banned IP 198.51.100.99, got: %+v", listPayload)
		}

		// 4. Unban IP
		unbanBody := `{"ip":"198.51.100.99"}`
		req4, _ := httpparser.NewRequest("POST", "/internal/api/security/unban", "HTTP/1.1")
		req4.Body = io.NopCloser(strings.NewReader(unbanBody))
		res4 := httpparser.NewResponse()
		testRouter.ServeHTTP(req4, res4)
		if res4.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for unban, got %d: %s", res4.StatusCode, res4.Body.String())
		}

		// 5. Verify IP is removed
		req5, _ := httpparser.NewRequest("GET", "/internal/api/security/banned-ips", "HTTP/1.1")
		res5 := httpparser.NewResponse()
		testRouter.ServeHTTP(req5, res5)
		var emptyPayload struct {
			Total int `json:"total"`
		}
		_ = json.Unmarshal(res5.Body.Bytes(), &emptyPayload)
		if emptyPayload.Total != 0 {
			t.Fatalf("expected 0 banned IPs after unban, got %d", emptyPayload.Total)
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

func TestProxyTest_NormalResponse_UnderLimit(t *testing.T) {
	upstreamPayload := strings.Repeat("A", 512)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Upstream-Header", "healthy")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(upstreamPayload))
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream URL: %v", err)
	}
	upstreamPort, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse upstream port: %v", err)
	}

	cfg := InternalAPIConfig{
		Port:                  upstreamPort,
		AllowedProxyTestPaths: []string{"/health"},
	}

	r := router.New()
	RegisterInternalAPIRoutes(r, cfg)

	reqBody := `{"path":"/health","method":"GET"}`
	req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Body = bytes.NewBufferString(reqBody)

	res := httpparser.NewResponse()
	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", res.StatusCode, res.Body.String())
	}

	var resOut ProxyTestResponse
	if err := json.Unmarshal(res.Body.Bytes(), &resOut); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if resOut.StatusCode != http.StatusOK {
		t.Errorf("expected resOut.StatusCode 200, got %d", resOut.StatusCode)
	}
	if resOut.StatusText != "OK" {
		t.Errorf("expected resOut.StatusText OK, got %s", resOut.StatusText)
	}
	if resOut.Headers["X-Upstream-Header"] != "healthy" {
		t.Errorf("expected X-Upstream-Header 'healthy', got %v", resOut.Headers["X-Upstream-Header"])
	}
	if len(resOut.Body) != 512 {
		t.Errorf("expected body length 512, got %d", len(resOut.Body))
	}
	if resOut.Body != upstreamPayload {
		t.Errorf("expected body to match upstream payload")
	}
	if resOut.Truncated {
		t.Errorf("expected Truncated == false, got true")
	}
}

func TestProxyTest_OversizedResponse_Truncated(t *testing.T) {
	const payloadSize = 2500 * 1024 // 2.5 MB
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		buf := make([]byte, 32*1024)
		for i := range buf {
			buf[i] = 'X'
		}
		written := 0
		for written < payloadSize {
			n := len(buf)
			if payloadSize-written < n {
				n = payloadSize - written
			}
			_, _ = w.Write(buf[:n])
			written += n
		}
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream URL: %v", err)
	}
	upstreamPort, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse upstream port: %v", err)
	}

	cfg := InternalAPIConfig{
		Port:                  upstreamPort,
		AllowedProxyTestPaths: []string{"/health"},
	}

	r := router.New()
	RegisterInternalAPIRoutes(r, cfg)

	reqBody := `{"path":"/health","method":"GET"}`
	req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Body = bytes.NewBufferString(reqBody)

	res := httpparser.NewResponse()
	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", res.StatusCode, res.Body.String())
	}

	var resOut ProxyTestResponse
	if err := json.Unmarshal(res.Body.Bytes(), &resOut); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if resOut.StatusCode != http.StatusOK {
		t.Errorf("expected resOut.StatusCode 200, got %d", resOut.StatusCode)
	}
	if !resOut.Truncated {
		t.Errorf("expected resOut.Truncated == true, got false")
	}
	if len(resOut.Body) != 1048576 {
		t.Errorf("expected clamped body length 1048576 (1 MB), got %d", len(resOut.Body))
	}
	if !strings.HasPrefix(resOut.Body, "XXXX") {
		t.Errorf("expected body to have prefix 'XXXX'")
	}
}

func TestProxyTest_InfiniteStream_BoundedTermination(t *testing.T) {
	serverDone := make(chan struct{})
	var closeOnce sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		chunk := bytes.Repeat([]byte("0123456789abcdef"), 4096) // 64 KB chunk
		for {
			select {
			case <-r.Context().Done():
				closeOnce.Do(func() { close(serverDone) })
				return
			default:
				_, err := w.Write(chunk)
				if err != nil {
					closeOnce.Do(func() { close(serverDone) })
					return
				}
				if ok {
					flusher.Flush()
				}
				time.Sleep(1 * time.Millisecond)
			}
		}
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream URL: %v", err)
	}
	upstreamPort, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse upstream port: %v", err)
	}

	cfg := InternalAPIConfig{
		Port:                  upstreamPort,
		AllowedProxyTestPaths: []string{"/health"},
	}

	r := router.New()
	RegisterInternalAPIRoutes(r, cfg)

	initialGoroutines := runtime.NumGoroutine()

	reqBody := `{"path":"/health","method":"GET"}`
	req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Body = bytes.NewBufferString(reqBody)

	res := httpparser.NewResponse()

	start := time.Now()
	r.ServeHTTP(req, res)
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("probe took too long (%v), expected termination < 3s", elapsed)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", res.StatusCode, res.Body.String())
	}

	var resOut ProxyTestResponse
	if err := json.Unmarshal(res.Body.Bytes(), &resOut); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if resOut.StatusCode != http.StatusOK {
		t.Errorf("expected resOut.StatusCode 200, got %d", resOut.StatusCode)
	}
	if !resOut.Truncated {
		t.Errorf("expected resOut.Truncated == true, got false")
	}
	if len(resOut.Body) != 1048576 {
		t.Errorf("expected clamped body length 1048576 (1 MB), got %d", len(resOut.Body))
	}

	select {
	case <-serverDone:
		// Upstream server successfully detected client termination
	case <-time.After(3 * time.Second):
		t.Fatalf("upstream server did not receive socket closure from client")
	}

	// Wait briefly for goroutine count to settle
	for i := 0; i < 50; i++ {
		if runtime.NumGoroutine() <= initialGoroutines+5 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestProxyTest_CustomConfiguredLimit(t *testing.T) {
	const payloadSize = 500 * 1024 // 500 KB
	upstreamPayload := strings.Repeat("C", payloadSize)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(upstreamPayload))
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream URL: %v", err)
	}
	upstreamPort, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse upstream port: %v", err)
	}

	cfg := InternalAPIConfig{
		Port:                      upstreamPort,
		AllowedProxyTestPaths:     []string{"/health"},
		MaxProxyTestResponseBytes: 256 * 1024, // 256 KB limit
	}

	r := router.New()
	RegisterInternalAPIRoutes(r, cfg)

	reqBody := `{"path":"/health","method":"GET"}`
	req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Body = bytes.NewBufferString(reqBody)

	res := httpparser.NewResponse()
	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", res.StatusCode, res.Body.String())
	}

	var resOut ProxyTestResponse
	if err := json.Unmarshal(res.Body.Bytes(), &resOut); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if resOut.StatusCode != http.StatusOK {
		t.Errorf("expected resOut.StatusCode 200, got %d", resOut.StatusCode)
	}
	if !resOut.Truncated {
		t.Errorf("expected resOut.Truncated == true, got false")
	}
	if len(resOut.Body) != 256*1024 {
		t.Errorf("expected clamped body length 262144 (256 KB), got %d", len(resOut.Body))
	}
}

func TestProxyTest_DefaultFallback_ZeroOrNegativeLimit(t *testing.T) {
	const payloadSize = 1500 * 1024 // 1.5 MB
	upstreamPayload := strings.Repeat("D", payloadSize)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(upstreamPayload))
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream URL: %v", err)
	}
	upstreamPort, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse upstream port: %v", err)
	}

	testCases := []struct {
		name  string
		limit int64
	}{
		{"Scenario 5.1: Zero limit defaults to 1 MB", 0},
		{"Scenario 5.2: Negative -1 defaults to 1 MB", -1},
		{"Scenario 5.3: Large negative defaults to 1 MB", -1048576},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := InternalAPIConfig{
				Port:                      upstreamPort,
				AllowedProxyTestPaths:     []string{"/health"},
				MaxProxyTestResponseBytes: tc.limit,
			}

			r := router.New()
			RegisterInternalAPIRoutes(r, cfg)

			reqBody := `{"path":"/health","method":"GET"}`
			req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			req.Body = bytes.NewBufferString(reqBody)

			res := httpparser.NewResponse()
			r.ServeHTTP(req, res)

			if res.StatusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d (body: %s)", res.StatusCode, res.Body.String())
			}

			var resOut ProxyTestResponse
			if err := json.Unmarshal(res.Body.Bytes(), &resOut); err != nil {
				t.Fatalf("failed to parse response JSON: %v", err)
			}

			if resOut.StatusCode != http.StatusOK {
				t.Errorf("expected resOut.StatusCode 200, got %d", resOut.StatusCode)
			}
			if !resOut.Truncated {
				t.Errorf("expected resOut.Truncated == true, got false")
			}
			if len(resOut.Body) != 1048576 {
				t.Errorf("expected clamped body length 1048576 (1 MB), got %d", len(resOut.Body))
			}
		})
	}
}

func TestProxyTest_OversizedRequestBody_Rejection(t *testing.T) {
	var upstreamCalls int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&upstreamCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream URL: %v", err)
	}
	upstreamPort, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse upstream port: %v", err)
	}

	cfg := InternalAPIConfig{
		Port:                  upstreamPort,
		AllowedProxyTestPaths: []string{"/health"},
	}

	r := router.New()
	RegisterInternalAPIRoutes(r, cfg)

	baseJSON := `{"path":"/health","method":"GET"}`

	t.Run("Scenario 6.1: Exactly 64 KB valid JSON accepted", func(t *testing.T) {
		atomic.StoreInt64(&upstreamCalls, 0)
		padding := strings.Repeat(" ", 64*1024-len(baseJSON))
		body := baseJSON + padding

		req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Body = bytes.NewBufferString(body)

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200 for 64 KB body, got %d (body: %s)", res.StatusCode, res.Body.String())
		}
		if calls := atomic.LoadInt64(&upstreamCalls); calls != 1 {
			t.Errorf("expected 1 upstream call, got %d", calls)
		}
	})

	t.Run("Scenario 6.2: 64 KB + 1 byte rejected with 400 Bad Request", func(t *testing.T) {
		atomic.StoreInt64(&upstreamCalls, 0)
		padding := strings.Repeat(" ", 64*1024+1-len(baseJSON))
		body := baseJSON + padding

		req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Body = bytes.NewBufferString(body)

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected status 400 for 65537-byte body, got %d", res.StatusCode)
		}
		if !strings.Contains(res.Body.String(), "Request body exceeds maximum allowed size of 64KB") {
			t.Errorf("expected error message to contain 'Request body exceeds maximum allowed size of 64KB', got %s", res.Body.String())
		}
		if calls := atomic.LoadInt64(&upstreamCalls); calls != 0 {
			t.Errorf("expected 0 upstream calls on oversized body, got %d", calls)
		}
	})

	t.Run("Scenario 6.3: Massive 1 MB JSON body rejected with 400 Bad Request", func(t *testing.T) {
		atomic.StoreInt64(&upstreamCalls, 0)
		padding := strings.Repeat(" ", 1024*1024-len(baseJSON))
		body := baseJSON + padding

		req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Body = bytes.NewBufferString(body)

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected status 400 for 1 MB body, got %d", res.StatusCode)
		}
		if !strings.Contains(res.Body.String(), "Request body exceeds maximum allowed size of 64KB") {
			t.Errorf("expected error message to contain 'Request body exceeds maximum allowed size of 64KB', got %s", res.Body.String())
		}
		if calls := atomic.LoadInt64(&upstreamCalls); calls != 0 {
			t.Errorf("expected 0 upstream calls, got %d", calls)
		}
	})

	t.Run("Scenario 6.4: Empty request body rejected with 400 Bad Request", func(t *testing.T) {
		atomic.StoreInt64(&upstreamCalls, 0)

		req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Body = bytes.NewBufferString("")

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected status 400 for empty body, got %d", res.StatusCode)
		}
		if !strings.Contains(res.Body.String(), "Missing or invalid request body") {
			t.Errorf("expected error message to contain 'Missing or invalid request body', got %s", res.Body.String())
		}
		if calls := atomic.LoadInt64(&upstreamCalls); calls != 0 {
			t.Errorf("expected 0 upstream calls, got %d", calls)
		}
	})

	t.Run("Scenario 6.5: Malformed JSON rejected with 400 Bad Request", func(t *testing.T) {
		atomic.StoreInt64(&upstreamCalls, 0)

		req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Body = bytes.NewBufferString(`{"path": /health}`)

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected status 400 for malformed JSON, got %d", res.StatusCode)
		}
		if !strings.Contains(res.Body.String(), "Invalid JSON body") {
			t.Errorf("expected error message to contain 'Invalid JSON body', got %s", res.Body.String())
		}
		if calls := atomic.LoadInt64(&upstreamCalls); calls != 0 {
			t.Errorf("expected 0 upstream calls, got %d", calls)
		}
	})
}

func TestProxyTest_SocketDrainAndConnectionReuse(t *testing.T) {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport.CloseIdleConnections()
		defer transport.CloseIdleConnections()
	}

	var connMutex sync.Mutex
	activeConns := make(map[string]int)
	payload := bytes.Repeat([]byte("B"), 1050*1024) // 1.05 MB
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connMutex.Lock()
		activeConns[r.RemoteAddr]++
		connMutex.Unlock()
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream URL: %v", err)
	}
	upstreamPort, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse upstream port: %v", err)
	}

	cfg := InternalAPIConfig{
		Port:                  upstreamPort,
		AllowedProxyTestPaths: []string{"/health"},
	}

	r := router.New()
	RegisterInternalAPIRoutes(r, cfg)

	for i := 0; i < 10; i++ {
		req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
		if err != nil {
			t.Fatalf("req %d: failed to create request: %v", i, err)
		}
		req.Body = bytes.NewBufferString(`{"path":"/health","method":"GET"}`)

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("req %d: expected status 200, got %d (body: %s)", i, res.StatusCode, res.Body.String())
		}

		var resOut ProxyTestResponse
		if err := json.Unmarshal(res.Body.Bytes(), &resOut); err != nil {
			t.Fatalf("req %d: failed to parse response JSON: %v", i, err)
		}

		if !resOut.Truncated {
			t.Errorf("req %d: expected Truncated == true, got false", i)
		}
		if len(resOut.Body) != 1048576 {
			t.Errorf("req %d: expected body len 1048576, got %d", i, len(resOut.Body))
		}
	}

	connMutex.Lock()
	distinctConns := len(activeConns)
	connMutex.Unlock()

	if distinctConns >= 10 {
		t.Errorf("expected keep-alive connection reuse (< 10 distinct TCP conns), got %d distinct conns", distinctConns)
	}
}

func TestProxyTest_ConcurrentProbes_RaceClean(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(strings.Repeat("A", 512)))
		case "/large":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytes.Repeat([]byte("B"), 2*1024*1024))
		case "/stream":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytes.Repeat([]byte("C"), 1500*1024))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream URL: %v", err)
	}
	upstreamPort, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse upstream port: %v", err)
	}

	cfg := InternalAPIConfig{
		Port:                  upstreamPort,
		AllowedProxyTestPaths: []string{"/health", "/large", "/stream"},
	}

	r := router.New()
	RegisterInternalAPIRoutes(r, cfg)

	var wg sync.WaitGroup
	for worker := 0; worker < 20; worker++ {
		wg.Add(1)
		go func(wID int) {
			defer wg.Done()
			for iter := 0; iter < 5; iter++ {
				mode := (wID*5 + iter) % 4
				switch mode {
				case 0:
					// Small body under limit
					req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
					if err != nil {
						t.Errorf("worker %d iter %d: failed to create request: %v", wID, iter, err)
						return
					}
					req.Body = bytes.NewBufferString(`{"path":"/health","method":"GET"}`)
					res := httpparser.NewResponse()
					r.ServeHTTP(req, res)

					if res.StatusCode != http.StatusOK {
						t.Errorf("worker %d iter %d: expected 200, got %d", wID, iter, res.StatusCode)
						return
					}
					var out ProxyTestResponse
					if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
						t.Errorf("worker %d iter %d: failed to unmarshal: %v", wID, iter, err)
						return
					}
					if out.Truncated {
						t.Errorf("worker %d iter %d: expected Truncated == false", wID, iter)
					}
					if len(out.Body) != 512 {
						t.Errorf("worker %d iter %d: expected body len 512, got %d", wID, iter, len(out.Body))
					}
				case 1:
					// Large 2 MB body
					req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
					if err != nil {
						t.Errorf("worker %d iter %d: failed to create request: %v", wID, iter, err)
						return
					}
					req.Body = bytes.NewBufferString(`{"path":"/large","method":"GET"}`)
					res := httpparser.NewResponse()
					r.ServeHTTP(req, res)

					if res.StatusCode != http.StatusOK {
						t.Errorf("worker %d iter %d: expected 200, got %d", wID, iter, res.StatusCode)
						return
					}
					var out ProxyTestResponse
					if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
						t.Errorf("worker %d iter %d: failed to unmarshal: %v", wID, iter, err)
						return
					}
					if !out.Truncated {
						t.Errorf("worker %d iter %d: expected Truncated == true", wID, iter)
					}
					if len(out.Body) != 1048576 {
						t.Errorf("worker %d iter %d: expected body len 1048576, got %d", wID, iter, len(out.Body))
					}
				case 2:
					// Stream 1.5 MB body
					req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
					if err != nil {
						t.Errorf("worker %d iter %d: failed to create request: %v", wID, iter, err)
						return
					}
					req.Body = bytes.NewBufferString(`{"path":"/stream","method":"GET"}`)
					res := httpparser.NewResponse()
					r.ServeHTTP(req, res)

					if res.StatusCode != http.StatusOK {
						t.Errorf("worker %d iter %d: expected 200, got %d", wID, iter, res.StatusCode)
						return
					}
					var out ProxyTestResponse
					if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
						t.Errorf("worker %d iter %d: failed to unmarshal: %v", wID, iter, err)
						return
					}
					if !out.Truncated {
						t.Errorf("worker %d iter %d: expected Truncated == true", wID, iter)
					}
					if len(out.Body) != 1048576 {
						t.Errorf("worker %d iter %d: expected body len 1048576, got %d", wID, iter, len(out.Body))
					}
				case 3:
					// Oversized inbound request (>64 KB)
					padding := strings.Repeat(" ", 65*1024)
					reqBody := fmt.Sprintf(`{"path":"/health","method":"GET"}%s`, padding)
					req, err := httpparser.NewRequest("POST", "/internal/api/proxy-test", "HTTP/1.1")
					if err != nil {
						t.Errorf("worker %d iter %d: failed to create request: %v", wID, iter, err)
						return
					}
					req.Body = bytes.NewBufferString(reqBody)
					res := httpparser.NewResponse()
					r.ServeHTTP(req, res)

					if res.StatusCode != http.StatusBadRequest {
						t.Errorf("worker %d iter %d: expected 400, got %d", wID, iter, res.StatusCode)
					}
				}
			}
		}(worker)
	}
	wg.Wait()
}

func TestInternalAPI_LogsAndTracing(t *testing.T) {
	cfg := InternalAPIConfig{
		Port:           8080,
		WorkerPoolSize: 128,
		ProxyEnabled:   true,
	}

	r := router.New()
	RegisterInternalAPIRoutes(r, cfg)

	// Record sample traces into GlobalTraceBuffer
	GlobalTraceBuffer.RecordTrace(TraceLogEntry{
		Method: "GET",
		Path:   "/v1/orders/123",
		Route:  "orders",
		Short:  "api /v1/orders",
		Status: 200,
		MS:     12.4,
		Up:     "10.0.1.11:8080",
		Trace:  "trace12345678",
		IP:     "192.168.1.50",
		Bytes:  1024,
		Spans: []TraceSpan{
			{Name: "Accept & Parse", Mod: "listener.http", D: 0.2},
			{Name: "Upstream Proxy", Mod: "proxy.reverse", D: 12.2},
		},
	})

	t.Run("GET /internal/api/logs", func(t *testing.T) {
		req, err := httpparser.NewRequest("GET", "/internal/api/logs", "HTTP/1.1")
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

		logs, ok := payload["logs"].([]interface{})
		if !ok || len(logs) == 0 {
			t.Errorf("expected non-empty logs array in /internal/api/logs")
		}
	})
}

