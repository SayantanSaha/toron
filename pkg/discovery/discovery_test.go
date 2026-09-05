package discovery

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"toron/pkg/config"
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
