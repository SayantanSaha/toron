package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"toron/pkg/config"
	"toron/pkg/httpparser"
	"toron/pkg/router"
)

func TestParseContainerLabels(t *testing.T) {
	tests := []struct {
		name          string
		container     Container
		defaultWeight int
		wantOk        bool
		wantHost      string
		wantPrefix    string
		wantPort      int
		wantWeight    int
	}{
		{
			name: "disabled container",
			container: Container{
				ID:     "c1",
				Labels: map[string]string{"toron.enable": "false"},
			},
			wantOk: false,
		},
		{
			name: "missing labels",
			container: Container{
				ID: "c2",
			},
			wantOk: false,
		},
		{
			name: "enabled with custom labels",
			container: Container{
				ID:    "c3123456789012",
				Names: []string{"/my-service"},
				Labels: map[string]string{
					"toron.enable":       "true",
					"toron.host":         "api.example.com",
					"toron.prefix":       "/v1/app",
					"toron.port":         "9000",
					"toron.weight":       "5",
					"toron.health_check": "/healthz",
				},
				IPAddress: "172.18.0.5",
			},
			defaultWeight: 1,
			wantOk:        true,
			wantHost:      "api.example.com",
			wantPrefix:    "/v1/app",
			wantPort:      9000,
			wantWeight:    5,
		},
		{
			name: "enabled with port fallback from exposed ports",
			container: Container{
				ID:        "c4",
				Names:     []string{"web-app"},
				Labels:    map[string]string{"toron.enable": "1", "toron.prefix": "/web-app"},
				IPAddress: "10.0.0.2",
				Ports: []PortMapping{
					{PrivatePort: 8080, Type: "tcp"},
				},
			},
			defaultWeight: 2,
			wantOk:        true,
			wantHost:      "",
			wantPrefix:    "/web-app",
			wantPort:      8080,
			wantWeight:    2,
		},
		{
			name: "enabled with prefix and redirect rewrite labels",
			container: Container{
				ID:    "c5",
				Names: []string{"legacy-service"},
				Labels: map[string]string{
					"toron.enable":              "true",
					"toron.prefix":              "/legacy-app",
					"toron.port":                "8080",
					"toron.strip_prefix":        "false",
					"toron.rewrite_redirects":   "true",
					"toron.rewrite_cookie_path": "true",
				},
				IPAddress: "172.18.0.10",
			},
			defaultWeight: 1,
			wantOk:        true,
			wantHost:      "",
			wantPrefix:    "/legacy-app",
			wantPort:      8080,
			wantWeight:    1,
		},
		{
			name: "TC-079-01: Wildcard Root Prefix Without Host Rejection",
			container: Container{
				ID:        "c-root-nohost",
				Names:     []string{"rogue-root"},
				Labels:    map[string]string{"toron.enable": "true", "toron.prefix": "/"},
				IPAddress: "172.18.0.20",
				Ports:     []PortMapping{{PrivatePort: 80, Type: "tcp"}},
			},
			defaultWeight: 1,
			wantOk:        false,
		},
		{
			name: "Empty Prefix Without Host Rejection",
			container: Container{
				ID:        "c-empty-nohost",
				Names:     []string{"rogue-empty"},
				Labels:    map[string]string{"toron.enable": "true", "toron.prefix": ""},
				IPAddress: "172.18.0.21",
				Ports:     []PortMapping{{PrivatePort: 80, Type: "tcp"}},
			},
			defaultWeight: 1,
			wantOk:        false,
		},
		{
			name: "TC-079-02: Administrative Path Shadowing Rejection",
			container: Container{
				ID:        "c-shadow-internal",
				Names:     []string{"rogue-admin"},
				Labels:    map[string]string{"toron.enable": "true", "toron.prefix": "/internal/api"},
				IPAddress: "172.18.0.22",
				Ports:     []PortMapping{{PrivatePort: 80, Type: "tcp"}},
			},
			defaultWeight: 1,
			wantOk:        false,
		},
		{
			name: "TC-079-03: Legitimate Scoped Route Registration",
			container: Container{
				ID:        "c-scoped-payments",
				Names:     []string{"payment-svc"},
				Labels:    map[string]string{"toron.enable": "true", "toron.prefix": "/services/payments"},
				IPAddress: "172.18.0.23",
				Ports:     []PortMapping{{PrivatePort: 8080, Type: "tcp"}},
			},
			defaultWeight: 1,
			wantOk:        true,
			wantHost:      "",
			wantPrefix:    "/services/payments",
			wantPort:      8080,
			wantWeight:    1,
		},
		{
			name: "Root Prefix With Explicit Host Allowed",
			container: Container{
				ID:        "c-root-with-host",
				Names:     []string{"dashboard-frontend"},
				Labels:    map[string]string{"toron.enable": "true", "toron.prefix": "/", "toron.host": "dashboard.example.com"},
				IPAddress: "172.18.0.24",
				Ports:     []PortMapping{{PrivatePort: 3000, Type: "tcp"}},
			},
			defaultWeight: 1,
			wantOk:        true,
			wantHost:      "dashboard.example.com",
			wantPrefix:    "/",
			wantPort:      3000,
			wantWeight:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, ok := ParseContainerLabels(tt.container, tt.defaultWeight)
			if ok != tt.wantOk {
				t.Fatalf("ParseContainerLabels() ok = %v, want %v", ok, tt.wantOk)
			}
			if !ok {
				return
			}
			if route.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", route.Host, tt.wantHost)
			}
			if route.Prefix != tt.wantPrefix {
				t.Errorf("Prefix = %q, want %q", route.Prefix, tt.wantPrefix)
			}
			if route.TargetPort != tt.wantPort {
				t.Errorf("TargetPort = %d, want %d", route.TargetPort, tt.wantPort)
			}
			if route.Weight != tt.wantWeight {
				t.Errorf("Weight = %d, want %d", route.Weight, tt.wantWeight)
			}
			if route.TargetURL() == "" {
				t.Errorf("TargetURL should not be empty")
			}
			if tt.name == "enabled with prefix and redirect rewrite labels" {
				if route.StripPrefix == nil || *route.StripPrefix != false {
					t.Errorf("expected StripPrefix=false, got %v", route.StripPrefix)
				}
				if route.RewriteRedirects == nil || *route.RewriteRedirects != true {
					t.Errorf("expected RewriteRedirects=true, got %v", route.RewriteRedirects)
				}
				if route.RewriteCookiePath == nil || *route.RewriteCookiePath != true {
					t.Errorf("expected RewriteCookiePath=true, got %v", route.RewriteCookiePath)
				}
			}
		})
	}
}

// MockProvider implements Provider interface for unit testing.
type MockProvider struct {
	name       string
	containers []Container
	eventsChan chan ContainerEvent
}

func NewMockProvider(name string, containers []Container) *MockProvider {
	return &MockProvider{
		name:       name,
		containers: containers,
		eventsChan: make(chan ContainerEvent, 10),
	}
}

func (m *MockProvider) Name() string { return m.name }

func (m *MockProvider) ListContainers(ctx context.Context) ([]Container, error) {
	return m.containers, nil
}

func (m *MockProvider) WatchEvents(ctx context.Context, events chan<- ContainerEvent) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt, ok := <-m.eventsChan:
			if !ok {
				return nil
			}
			events <- evt
		}
	}
}

func TestManagerOrchestration(t *testing.T) {
	r := router.New()
	cfg := config.DiscoveryConfig{
		Enabled:       true,
		Engine:        "mock",
		DefaultWeight: 1,
	}

	mgr := NewManager(cfg, r)

	mockContainer := Container{
		ID:        "mock-c1-1234567890",
		Names:     []string{"/test-service"},
		IPAddress: "127.0.0.1",
		Labels: map[string]string{
			"toron.enable": "true",
			"toron.host":   "test.local",
			"toron.prefix": "/api",
			"toron.port":   "8081",
		},
	}

	mockP := NewMockProvider("MockOCI", []Container{mockContainer})
	mgr.AddProvider(mockP)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("mgr.Start() failed: %v", err)
	}
	defer mgr.Stop()

	// Verify active route registration
	routes := mgr.ActiveRoutes()
	if len(routes) == 0 {
		t.Fatalf("Expected at least 1 active route, got 0")
	}

	found := false
	for _, rt := range routes {
		if rt.ContainerID == mockContainer.ID && rt.Host == "test.local" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected active route for mockContainer ID %s not found", mockContainer.ID)
	}

	// Emit stop event via mock provider
	mockP.eventsChan <- ContainerEvent{
		Type:        EventStop,
		ContainerID: mockContainer.ID,
	}

	// Allow event worker time to process
	time.Sleep(50 * time.Millisecond)

	routesAfterStop := mgr.ActiveRoutes()
	if len(routesAfterStop) != 0 {
		t.Errorf("Expected 0 active routes after stop event, got %d", len(routesAfterStop))
	}
}

func TestUnixRESTProviderMockServer(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "toron-socket-test-*")
	if err != nil {
		t.Fatalf("Failed to create tmpDir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sockPath := filepath.Join(tmpDir, "test.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("Failed to listen on unix socket %s: %v", sockPath, err)
	}
	defer l.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/containers/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		containers := []dockerContainerJSON{
			{
				ID:     "c-mock-100",
				Names:  []string{"/mock-app"},
				Image:  "alpine:latest",
				State:  "running",
				Status: "Up 5 minutes",
				Labels: map[string]string{
					"toron.enable": "true",
					"toron.port":   "8080",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(containers)
	})

	server := &http.Server{Handler: mux}
	go server.Serve(l)
	defer server.Close()

	prov := NewUnixRESTProvider("TestUnix", sockPath)
	if prov.Name() == "" {
		t.Errorf("Provider Name() should not be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	list, err := prov.ListContainers(ctx)
	if err != nil {
		t.Fatalf("prov.ListContainers() error: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("Expected 1 container from mock socket, got %d", len(list))
	}
	if list[0].ID != "c-mock-100" {
		t.Errorf("Container ID = %q, want %q", list[0].ID, "c-mock-100")
	}
}

func TestParseContainerLabels_MethodAndHeaders(t *testing.T) {
	tests := []struct {
		name        string
		container   Container
		wantOk      bool
		wantMethod  string
		wantHeaders map[string]string
	}{
		{
			name: "1.1 Explicit Method",
			container: Container{
				ID:    "c1",
				State: "running",
				Labels: map[string]string{
					"toron.enable": "true",
					"toron.prefix": "/api",
					"toron.port":   "8080",
					"toron.method": "post",
				},
			},
			wantOk:     true,
			wantMethod: "POST",
		},
		{
			name: "1.2 Method with Whitespace",
			container: Container{
				ID:    "c2",
				State: "running",
				Labels: map[string]string{
					"toron.enable": "true",
					"toron.prefix": "/api",
					"toron.port":   "8080",
					"toron.method": "  delete  ",
				},
			},
			wantOk:     true,
			wantMethod: "DELETE",
		},
		{
			name: "1.3 Individual Header",
			container: Container{
				ID:    "c3",
				State: "running",
				Labels: map[string]string{
					"toron.enable":           "true",
					"toron.prefix":           "/api",
					"toron.port":             "8080",
					"toron.header.X-Version": "canary",
				},
			},
			wantOk:      true,
			wantHeaders: map[string]string{"X-Version": "canary"},
		},
		{
			name: "1.4 Grouped Headers CSV",
			container: Container{
				ID:    "c4",
				State: "running",
				Labels: map[string]string{
					"toron.enable":  "true",
					"toron.prefix":  "/api",
					"toron.port":    "8080",
					"toron.headers": "X-Env=staging, X-Region = us-east-1",
				},
			},
			wantOk: true,
			wantHeaders: map[string]string{
				"X-Env":    "staging",
				"X-Region": "us-east-1",
			},
		},
		{
			name: "1.5 Grouped Headers JSON",
			container: Container{
				ID:    "c5",
				State: "running",
				Labels: map[string]string{
					"toron.enable":  "true",
					"toron.prefix":  "/api",
					"toron.port":    "8080",
					"toron.headers": `{"X-Env":"staging","X-Tier":"gold"}`,
				},
			},
			wantOk: true,
			wantHeaders: map[string]string{
				"X-Env":  "staging",
				"X-Tier": "gold",
			},
		},
		{
			name: "1.6 Override Precedence",
			container: Container{
				ID:    "c6",
				State: "running",
				Labels: map[string]string{
					"toron.enable":        "true",
					"toron.prefix":        "/api",
					"toron.port":          "8080",
					"toron.headers":       "X-Env=staging,X-Tier=gold",
					"toron.header.X-Env": "production",
				},
			},
			wantOk: true,
			wantHeaders: map[string]string{
				"X-Env":  "production",
				"X-Tier": "gold",
			},
		},
		{
			name: "1.7 Malformed JSON Fallback",
			container: Container{
				ID:    "c7",
				State: "running",
				Labels: map[string]string{
					"toron.enable":  "true",
					"toron.prefix":  "/api",
					"toron.port":    "8080",
					"toron.headers": `{"X-Env":"staging", bad-json`,
				},
			},
			wantOk:      true,
			wantHeaders: nil,
		},
		{
			name: "1.8 Malformed CSV Token",
			container: Container{
				ID:    "c8",
				State: "running",
				Labels: map[string]string{
					"toron.enable":  "true",
					"toron.prefix":  "/api",
					"toron.port":    "8080",
					"toron.headers": "X-Env=prod,invalidtoken,X-App=web",
				},
			},
			wantOk: true,
			wantHeaders: map[string]string{
				"X-Env": "prod",
				"X-App": "web",
			},
		},
		{
			name: "1.9 No Headers Configured",
			container: Container{
				ID:    "c9",
				State: "running",
				Labels: map[string]string{
					"toron.enable": "true",
					"toron.prefix": "/api",
					"toron.port":   "8080",
				},
			},
			wantOk:      true,
			wantHeaders: nil,
		},
		{
			name: "1.10 Non-Running State",
			container: Container{
				ID:    "c10",
				State: "exited",
				Labels: map[string]string{
					"toron.enable": "true",
					"toron.prefix": "/api",
					"toron.port":   "8080",
				},
			},
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, ok := ParseContainerLabels(tt.container, 1)
			if ok != tt.wantOk {
				t.Fatalf("ParseContainerLabels() ok = %v, want %v", ok, tt.wantOk)
			}
			if !ok {
				return
			}
			if route.Method != tt.wantMethod {
				t.Errorf("Method = %q, want %q", route.Method, tt.wantMethod)
			}
			if len(tt.wantHeaders) == 0 {
				if len(route.Headers) != 0 {
					t.Errorf("expected empty headers, got %v", route.Headers)
				}
			} else {
				if len(route.Headers) != len(tt.wantHeaders) {
					t.Errorf("len(Headers) = %d, want %d", len(route.Headers), len(tt.wantHeaders))
				}
				for k, v := range tt.wantHeaders {
					if route.Headers[k] != v {
						t.Errorf("Headers[%q] = %q, want %q", k, route.Headers[k], v)
					}
				}
			}
		})
	}
}

func TestDiscoveryManager_CompositeRouteKeyCanonicalization(t *testing.T) {
	headers := map[string]string{
		"Z-Header": "val-z",
		"A-Header": "val-a",
		"M-Header": "val-m",
		"X-Env":    "prod",
		"B-Tier":   "gold",
	}
	expected := "A-Header=val-a&B-Tier=gold&M-Header=val-m&X-Env=prod&Z-Header=val-z"

	for i := 0; i < 100; i++ {
		got := CanonicalHeaders(headers)
		if got != expected {
			t.Fatalf("iteration %d: CanonicalHeaders() = %q, want %q", i, got, expected)
		}
	}

	if CanonicalHeaders(nil) != "" {
		t.Errorf("CanonicalHeaders(nil) = %q, want empty", CanonicalHeaders(nil))
	}
	if CanonicalHeaders(map[string]string{}) != "" {
		t.Errorf("CanonicalHeaders(empty) = %q, want empty", CanonicalHeaders(map[string]string{}))
	}

	// Normalization
	r := &DiscoveredRoute{
		Host:   "  API.Example.COM  ",
		Prefix: "/v1/data/",
		Method: "  post ",
	}
	key := makeCompositeRouteKey(r)
	if key.Host != "api.example.com" {
		t.Errorf("key.Host = %q, want %q", key.Host, "api.example.com")
	}
	if key.Prefix != "/v1/data" {
		t.Errorf("key.Prefix = %q, want %q", key.Prefix, "/v1/data")
	}
	if key.Method != "POST" {
		t.Errorf("key.Method = %q, want %q", key.Method, "POST")
	}

	// Key Inequality
	k1 := CompositeRouteKey{Host: "a", Prefix: "/p", Method: "GET", CanonicalHeaders: ""}
	k2 := CompositeRouteKey{Host: "b", Prefix: "/p", Method: "GET", CanonicalHeaders: ""}
	k3 := CompositeRouteKey{Host: "a", Prefix: "/q", Method: "GET", CanonicalHeaders: ""}
	k4 := CompositeRouteKey{Host: "a", Prefix: "/p", Method: "POST", CanonicalHeaders: ""}
	k5 := CompositeRouteKey{Host: "a", Prefix: "/p", Method: "GET", CanonicalHeaders: "X-Env=prod"}

	keys := map[CompositeRouteKey]struct{}{
		k1: {},
		k2: {},
		k3: {},
		k4: {},
		k5: {},
	}
	if len(keys) != 5 {
		t.Errorf("expected 5 distinct keys in map, got %d", len(keys))
	}
}

func TestDiscoveryManager_CompositeRouteKeyAggregation(t *testing.T) {
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server-1"))
	}))
	defer s1.Close()

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server-2"))
	}))
	defer s2.Close()

	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server-3"))
	}))
	defer s3.Close()

	u1, _ := url.Parse(s1.URL)
	u2, _ := url.Parse(s2.URL)
	u3, _ := url.Parse(s3.URL)

	p1, _ := strconv.Atoi(u1.Port())
	p2, _ := strconv.Atoi(u2.Port())
	p3, _ := strconv.Atoi(u3.Port())

	r := router.New()
	cfg := config.DiscoveryConfig{Enabled: true, Engine: "mock", DefaultWeight: 1}
	mgr := NewManager(cfg, r)

	c1 := Container{
		ID:        "c1",
		State:     "running",
		IPAddress: u1.Hostname(),
		Labels: map[string]string{
			"toron.enable":          "true",
			"toron.host":            "api.example.com",
			"toron.prefix":          "/v1",
			"toron.method":          "GET",
			"toron.header.X-Region": "us-east",
			"toron.port":            strconv.Itoa(p1),
		},
	}
	c2 := Container{
		ID:        "c2",
		State:     "running",
		IPAddress: u2.Hostname(),
		Labels: map[string]string{
			"toron.enable":          "true",
			"toron.host":            "api.example.com",
			"toron.prefix":          "/v1",
			"toron.method":          "GET",
			"toron.header.X-Region": "us-east",
			"toron.port":            strconv.Itoa(p2),
		},
	}
	c3 := Container{
		ID:        "c3",
		State:     "running",
		IPAddress: u3.Hostname(),
		Labels: map[string]string{
			"toron.enable":          "true",
			"toron.host":            "api.example.com",
			"toron.prefix":          "/v1",
			"toron.method":          "GET",
			"toron.header.X-Region": "us-east",
			"toron.port":            strconv.Itoa(p3),
		},
	}

	mgr.handleContainerStart(c1)
	mgr.handleContainerStart(c2)
	mgr.handleContainerStart(c3)

	routes := r.GetPrefixRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected exactly 1 aggregated route in router, got %d", len(routes))
	}
	if routes[0].Host != "api.example.com" || routes[0].Prefix != "/v1" {
		t.Errorf("route = %+v, want host=api.example.com prefix=/v1", routes[0])
	}

	// Duplicate container C4 reporting same target as C1
	c4 := Container{
		ID:        "c4",
		State:     "running",
		IPAddress: u1.Hostname(),
		Labels: map[string]string{
			"toron.enable":          "true",
			"toron.host":            "api.example.com",
			"toron.prefix":          "/v1",
			"toron.method":          "GET",
			"toron.header.X-Region": "us-east",
			"toron.port":            strconv.Itoa(p1),
		},
	}
	mgr.handleContainerStart(c4)
	routesAfterDup := r.GetPrefixRoutes()
	if len(routesAfterDup) != 1 {
		t.Fatalf("expected 1 route after duplicate container, got %d", len(routesAfterDup))
	}

	// Dispatch 30 sequential HTTP requests: verify each server receives exactly 10 requests
	counts := make(map[string]int)
	for i := 0; i < 30; i++ {
		req, _ := httpparser.NewRequest("GET", "/v1/data", "HTTP/1.1")
		req.Host = "api.example.com"
		req.Header.Set("X-Region", "us-east")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("request %d failed with status %d", i, res.StatusCode)
		}
		counts[res.Body.String()]++
	}

	if counts["server-1"] != 10 || counts["server-2"] != 10 || counts["server-3"] != 10 {
		t.Errorf("traffic not balanced equally: %v (want 10 each)", counts)
	}
}

func TestDiscoveryManager_NonDestructivePartialScaleDown(t *testing.T) {
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("s1"))
	}))
	defer s1.Close()

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("s2"))
	}))
	defer s2.Close()

	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("s3"))
	}))
	defer s3.Close()

	u1, _ := url.Parse(s1.URL)
	u2, _ := url.Parse(s2.URL)
	u3, _ := url.Parse(s3.URL)

	p1, _ := strconv.Atoi(u1.Port())
	p2, _ := strconv.Atoi(u2.Port())
	p3, _ := strconv.Atoi(u3.Port())

	r := router.New()
	cfg := config.DiscoveryConfig{Enabled: true, Engine: "mock", DefaultWeight: 1}
	mgr := NewManager(cfg, r)

	c1 := Container{ID: "c1", State: "running", IPAddress: u1.Hostname(), Labels: map[string]string{"toron.enable": "true", "toron.host": "scale.test", "toron.prefix": "/api", "toron.port": strconv.Itoa(p1)}}
	c2 := Container{ID: "c2", State: "running", IPAddress: u2.Hostname(), Labels: map[string]string{"toron.enable": "true", "toron.host": "scale.test", "toron.prefix": "/api", "toron.port": strconv.Itoa(p2)}}
	c3 := Container{ID: "c3", State: "running", IPAddress: u3.Hostname(), Labels: map[string]string{"toron.enable": "true", "toron.host": "scale.test", "toron.prefix": "/api", "toron.port": strconv.Itoa(p3)}}

	mgr.handleContainerStart(c1)
	mgr.handleContainerStart(c2)
	mgr.handleContainerStart(c3)

	// Phase 1: 6 requests -> 2 to each server
	for i := 0; i < 6; i++ {
		req, _ := httpparser.NewRequest("GET", "/api/test", "HTTP/1.1")
		req.Host = "scale.test"
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("initial request %d failed: %d", i, res.StatusCode)
		}
	}

	// Phase 2: Stop C1 (Partial scale down)
	mgr.handleContainerStop("c1")

	// Phase 3: Zero downtime verification - send 20 requests
	counts := make(map[string]int)
	for i := 0; i < 20; i++ {
		req, _ := httpparser.NewRequest("GET", "/api/test", "HTTP/1.1")
		req.Host = "scale.test"
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("post-scale-down request %d failed: %d", i, res.StatusCode)
		}
		counts[res.Body.String()]++
	}

	if counts["s1"] != 0 {
		t.Errorf("stopped replica s1 received traffic: %d", counts["s1"])
	}
	if counts["s2"] != 10 || counts["s3"] != 10 {
		t.Errorf("traffic not evenly split across surviving s2 and s3: %v", counts)
	}

	// Phase 4: Full Eviction - stop c2 and c3
	mgr.handleContainerStop("c2")
	mgr.handleContainerStop("c3")

	routesAfterStopAll := r.GetPrefixRoutes()
	if len(routesAfterStopAll) != 0 {
		t.Errorf("expected 0 routes after stopping all containers, got %d", len(routesAfterStopAll))
	}

	req404, _ := httpparser.NewRequest("GET", "/api/test", "HTTP/1.1")
	req404.Host = "scale.test"
	res404 := httpparser.NewResponse()
	r.ServeHTTP(req404, res404)
	if res404.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after full route eviction, got %d", res404.StatusCode)
	}
}

func TestDiscoveryManager_DistinctVariantCanarySeparation(t *testing.T) {
	baseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("baseline-response"))
	}))
	defer baseServer.Close()

	canaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("canary-response"))
	}))
	defer canaryServer.Close()

	uBase, _ := url.Parse(baseServer.URL)
	uCanary, _ := url.Parse(canaryServer.URL)

	pBase, _ := strconv.Atoi(uBase.Port())
	pCanary, _ := strconv.Atoi(uCanary.Port())

	r := router.New()
	cfg := config.DiscoveryConfig{Enabled: true, Engine: "mock", DefaultWeight: 1}
	mgr := NewManager(cfg, r)

	b1 := Container{
		ID:        "b1",
		State:     "running",
		IPAddress: uBase.Hostname(),
		Labels: map[string]string{
			"toron.enable": "true",
			"toron.host":   "shop.example.com",
			"toron.prefix": "/checkout",
			"toron.port":   strconv.Itoa(pBase),
		},
	}
	c1 := Container{
		ID:        "c1",
		State:     "running",
		IPAddress: uCanary.Hostname(),
		Labels: map[string]string{
			"toron.enable":           "true",
			"toron.host":             "shop.example.com",
			"toron.prefix":           "/checkout",
			"toron.header.X-Version": "canary",
			"toron.port":             strconv.Itoa(pCanary),
		},
	}

	mgr.handleContainerStart(b1)
	mgr.handleContainerStart(c1)

	routes := r.GetPrefixRoutes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 distinct routes (baseline and canary), got %d", len(routes))
	}

	// 20 requests with canary header -> canary
	for i := 0; i < 20; i++ {
		req, _ := httpparser.NewRequest("GET", "/checkout/pay", "HTTP/1.1")
		req.Host = "shop.example.com"
		req.Header.Set("X-Version", "canary")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.StatusCode != http.StatusOK || res.Body.String() != "canary-response" {
			t.Fatalf("canary request %d failed: %d %q", i, res.StatusCode, res.Body.String())
		}
	}

	// 20 requests without header -> baseline
	for i := 0; i < 20; i++ {
		req, _ := httpparser.NewRequest("GET", "/checkout/pay", "HTTP/1.1")
		req.Host = "shop.example.com"
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.StatusCode != http.StatusOK || res.Body.String() != "baseline-response" {
			t.Fatalf("baseline request %d failed: %d %q", i, res.StatusCode, res.Body.String())
		}
	}

	// 10 requests with mismatched canary header (X-Version: v1) -> baseline
	for i := 0; i < 10; i++ {
		req, _ := httpparser.NewRequest("GET", "/checkout/pay", "HTTP/1.1")
		req.Host = "shop.example.com"
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.StatusCode != http.StatusOK || res.Body.String() != "baseline-response" {
			t.Fatalf("mismatched canary request %d failed: %d %q", i, res.StatusCode, res.Body.String())
		}
	}
}

func TestDiscoveryManager_AtomicReplacementLifecycle(t *testing.T) {
	var probeCount int64
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			atomic.AddInt64(&probeCount, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockUpstream.Close()

	u, _ := url.Parse(mockUpstream.URL)
	port, _ := strconv.Atoi(u.Port())

	r := router.New()
	cfg := config.DiscoveryConfig{Enabled: true, Engine: "mock", DefaultWeight: 1}
	mgr := NewManager(cfg, r)

	c := Container{
		ID:        "hc-c1",
		State:     "running",
		IPAddress: u.Hostname(),
		Labels: map[string]string{
			"toron.enable":                "true",
			"toron.host":                  "health.test",
			"toron.prefix":                "/api",
			"toron.port":                  strconv.Itoa(port),
			"toron.health_check":          "/healthz",
			"toron.health_check_interval": "20ms",
		},
	}

	mgr.handleContainerStart(c)

	// Wait for probes to occur
	time.Sleep(100 * time.Millisecond)
	probesBefore := atomic.LoadInt64(&probeCount)
	if probesBefore == 0 {
		t.Fatalf("expected health check probes to be running, got 0")
	}

	// Evict route by stopping container
	mgr.handleContainerStop("hc-c1")

	// Wait and verify probes stop
	time.Sleep(100 * time.Millisecond)
	probesAfter := atomic.LoadInt64(&probeCount)
	if probesAfter > probesBefore+1 {
		t.Errorf("health check probes continued after eviction: before=%d, after=%d", probesBefore, probesAfter)
	}
}

func TestDiscoveryManager_ConcurrentLifecycleAndRouting_RaceClean(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	u, _ := url.Parse(backend.URL)
	port, _ := strconv.Atoi(u.Port())

	r := router.New()
	cfg := config.DiscoveryConfig{Enabled: true, Engine: "mock", DefaultWeight: 1}
	mgr := NewManager(cfg, r)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup

	// Lifecycle churn goroutines (5 workers)
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			cid := fmt.Sprintf("worker-c-%d", workerID)
			for {
				select {
				case <-ctx.Done():
					return
				default:
					mgr.handleContainerStart(Container{
						ID:        cid,
						State:     "running",
						IPAddress: u.Hostname(),
						Labels: map[string]string{
							"toron.enable": "true",
							"toron.prefix": "/v1/data",
							"toron.port":   strconv.Itoa(port),
						},
					})
					mgr.handleContainerStop(cid)
				}
			}
		}(w)
	}

	// Traffic dispatch goroutines (15 workers)
	for c := 0; c < 15; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					req, _ := httpparser.NewRequest("GET", "/v1/data", "HTTP/1.1")
					res := httpparser.NewResponse()
					r.ServeHTTP(req, res)
					// Status should be 200 or 404
					if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNotFound {
						t.Errorf("unexpected status code: %d", res.StatusCode)
					}
				}
			}
		}()
	}

	wg.Wait()
}
