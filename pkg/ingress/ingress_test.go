package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"toron/pkg/config"
	"toron/pkg/router"
)

func TestIngressJSONDecoding(t *testing.T) {
	jsonPayload := `{
		"apiVersion": "networking.k8s.io/v1",
		"kind": "Ingress",
		"metadata": {
			"name": "demo-ingress",
			"namespace": "default",
			"annotations": {
				"kubernetes.io/ingress.class": "toron"
			}
		},
		"spec": {
			"ingressClassName": "toron",
			"rules": [
				{
					"host": "api.k8s.local",
					"http": {
						"paths": [
							{
								"path": "/v1",
								"pathType": "Prefix",
								"backend": {
									"service": {
										"name": "user-svc",
										"port": {
											"number": 8080
										}
									}
								}
							}
						]
					}
				}
			]
		}
	}`

	var ing Ingress
	if err := json.Unmarshal([]byte(jsonPayload), &ing); err != nil {
		t.Fatalf("Failed to decode Ingress JSON: %v", err)
	}

	if ing.Metadata.Name != "demo-ingress" {
		t.Errorf("Metadata.Name = %q, want %q", ing.Metadata.Name, "demo-ingress")
	}
	if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != "toron" {
		t.Errorf("IngressClassName mismatch")
	}
	if len(ing.Spec.Rules) != 1 || ing.Spec.Rules[0].Host != "api.k8s.local" {
		t.Errorf("Ingress Rule Host mismatch")
	}
}

func TestTranslateIngress(t *testing.T) {
	ingClass := "toron"
	ing := Ingress{
		Metadata: ObjectMeta{
			Name:      "test-ing",
			Namespace: "prod",
		},
		Spec: IngressSpec{
			IngressClassName: &ingClass,
			Rules: []IngressRule{
				{
					Host: "app.example.com",
					HTTP: &HTTPIngressRuleValue{
						Paths: []HTTPIngressPath{
							{
								Path:     "/api",
								PathType: "Prefix",
								Backend: IngressBackend{
									Service: &IngressServiceBackend{
										Name: "app-service",
										Port: ServiceBackendPort{Number: 9000},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	epMap := map[string]*Endpoints{
		"prod/app-service": {
			Metadata: ObjectMeta{Name: "app-service", Namespace: "prod"},
			Subsets: []EndpointSubset{
				{
					Addresses: []EndpointAddress{
						{IP: "10.244.1.15"},
						{IP: "10.244.1.16"},
					},
					Ports: []EndpointPort{
						{Port: 9000},
					},
				},
			},
		},
	}

	routes, ok := TranslateIngress(ing, "toron", epMap)
	if !ok {
		t.Fatalf("TranslateIngress returned false, expected true")
	}

	if len(routes) != 2 {
		t.Fatalf("Expected 2 discovered routes for 2 endpoint IPs, got %d", len(routes))
	}

	if routes[0].Host != "app.example.com" {
		t.Errorf("Host = %q, want app.example.com", routes[0].Host)
	}
	if routes[0].TargetPort != 9000 {
		t.Errorf("TargetPort = %d, want 9000", routes[0].TargetPort)
	}
	if routes[0].TargetIP != "10.244.1.15" && routes[0].TargetIP != "10.244.1.16" {
		t.Errorf("Unexpected TargetIP %q", routes[0].TargetIP)
	}
}

func TestK8sMockAPIServerClient(t *testing.T) {
	mux := http.NewServeMux()
	ingClass := "toron"

	mux.HandleFunc("/apis/networking.k8s.io/v1/ingresses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		list := IngressList{
			Kind:       "IngressList",
			APIVersion: "networking.k8s.io/v1",
			Items: []Ingress{
				{
					Metadata: ObjectMeta{Name: "mock-ing", Namespace: "default"},
					Spec: IngressSpec{
						IngressClassName: &ingClass,
						Rules: []IngressRule{
							{
								Host: "mock.cluster.local",
								HTTP: &HTTPIngressRuleValue{
									Paths: []HTTPIngressPath{
										{
											Path: "/v1",
											Backend: IngressBackend{
												Service: &IngressServiceBackend{
													Name: "mock-svc",
													Port: ServiceBackendPort{Number: 8080},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(list)
	})

	mux.HandleFunc("/api/v1/namespaces/default/endpoints/mock-svc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ep := Endpoints{
			Metadata: ObjectMeta{Name: "mock-svc", Namespace: "default"},
			Subsets: []EndpointSubset{
				{
					Addresses: []EndpointAddress{{IP: "10.96.0.50"}},
					Ports:     []EndpointPort{{Port: 8080}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(ep)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.IngressConfig{
		Enabled:       true,
		IngressClass:  "toron",
		KubeAPIServer: server.URL,
	}

	r := router.New()
	ctrl, err := NewController(cfg, r)
	if err != nil {
		t.Fatalf("NewController failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("ctrl.Start() failed: %v", err)
	}
	defer ctrl.Stop()

	routes := ctrl.ActiveRoutes()
	if len(routes) == 0 {
		t.Fatalf("Expected at least 1 active route from mock apiserver, got 0")
	}

	if routes[0].Host != "mock.cluster.local" {
		t.Errorf("Route Host = %q, want mock.cluster.local", routes[0].Host)
	}
	if routes[0].TargetIP != "10.96.0.50" {
		t.Errorf("Route TargetIP = %q, want 10.96.0.50", routes[0].TargetIP)
	}
}

// TC-084-01: Context cancellation during blocked channel send
func TestTC084_WatchIngresses_ContextCancellationOnSend(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/apis/networking.k8s.io/v1/ingresses", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") == "true" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			for i := 0; i < 50; i++ {
				_, _ = w.Write([]byte(`{"type":"ADDED","object":{"metadata":{"name":"test"}}}` + "\n"))
				if ok {
					flusher.Flush()
				}
				time.Sleep(2 * time.Millisecond)
			}
			<-r.Context().Done()
			return
		}
		http.NotFound(w, r)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.IngressConfig{
		Enabled:       true,
		IngressClass:  "toron",
		KubeAPIServer: server.URL,
	}

	cli, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Tiny buffer with NO consumer
	events := make(chan K8sWatchEvent, 2)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- cli.WatchIngresses(ctx, events)
	}()

	// Give it a brief moment to send 2 items and block on 3rd
	time.Sleep(20 * time.Millisecond)

	// Cancel context; WatchIngresses MUST unblock immediately
	cancel()

	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("WatchIngresses deadlocked / failed to cancel within 300ms")
	}
}

// TC-084-02: Consumer worker continuously drains events (>100 events) without deadlocking
func TestTC084_Controller_ConsumerDrainsOver100Events(t *testing.T) {
	ingClass := "toron"
	mux := http.NewServeMux()

	mux.HandleFunc("/apis/networking.k8s.io/v1/ingresses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("watch") == "true" {
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			// Emit 150 events (exceeds channel capacity of 100)
			for i := 1; i <= 150; i++ {
				_, _ = w.Write([]byte(fmt.Sprintf(`{"type":"MODIFIED","object":{"metadata":{"name":"ing-%d"}}}`+"\n", i)))
				if ok {
					flusher.Flush()
				}
			}
			<-r.Context().Done()
			return
		}

		// List endpoint
		list := IngressList{
			Kind:       "IngressList",
			APIVersion: "networking.k8s.io/v1",
			Items: []Ingress{
				{
					Metadata: ObjectMeta{Name: "stream-ing", Namespace: "default"},
					Spec: IngressSpec{
						IngressClassName: &ingClass,
						Rules: []IngressRule{
							{
								Host: "stream.cluster.local",
								HTTP: &HTTPIngressRuleValue{
									Paths: []HTTPIngressPath{
										{
											Path: "/stream",
											Backend: IngressBackend{
												Service: &IngressServiceBackend{
													Name: "stream-svc",
													Port: ServiceBackendPort{Number: 8080},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(list)
	})

	mux.HandleFunc("/api/v1/namespaces/default/endpoints/stream-svc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ep := Endpoints{
			Metadata: ObjectMeta{Name: "stream-svc", Namespace: "default"},
			Subsets: []EndpointSubset{
				{
					Addresses: []EndpointAddress{{IP: "10.96.1.100"}},
					Ports:     []EndpointPort{{Port: 8080}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(ep)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.IngressConfig{
		Enabled:       true,
		IngressClass:  "toron",
		KubeAPIServer: server.URL,
	}

	r := router.New()
	ctrl, err := NewController(cfg, r)
	if err != nil {
		t.Fatalf("NewController failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("ctrl.Start() failed: %v", err)
	}

	// Wait for consumer to process events and debounce
	time.Sleep(100 * time.Millisecond)

	routes := ctrl.ActiveRoutes()
	if len(routes) == 0 {
		t.Fatalf("Expected active routes after stream ingestion, got 0")
	}

	// Verify Stop completes without deadlock
	stopDone := make(chan struct{})
	go func() {
		ctrl.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		// Clean stop!
	case <-time.After(300 * time.Millisecond):
		t.Fatal("ctrl.Stop() deadlocked after consuming >100 events")
	}
}

// TC-084-03: Zero-deadlock shutdown under continuous high-frequency event streaming
func TestTC084_Controller_ZeroDeadlockShutdownUnderLoad(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/apis/networking.k8s.io/v1/ingresses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("watch") == "true" {
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			for {
				select {
				case <-r.Context().Done():
					return
				default:
					_, err := w.Write([]byte(`{"type":"MODIFIED","object":{"metadata":{"name":"flood"}}}` + "\n"))
					if err != nil {
						return
					}
					if ok {
						flusher.Flush()
					}
				}
			}
		}
		list := IngressList{Kind: "IngressList", APIVersion: "networking.k8s.io/v1"}
		_ = json.NewEncoder(w).Encode(list)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.IngressConfig{
		Enabled:       true,
		IngressClass:  "toron",
		KubeAPIServer: server.URL,
	}

	r := router.New()
	ctrl, err := NewController(cfg, r)
	if err != nil {
		t.Fatalf("NewController failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("ctrl.Start() failed: %v", err)
	}

	// Let the flood stream for a moment
	time.Sleep(30 * time.Millisecond)

	// Stop must finish quickly without hanging
	stopDone := make(chan struct{})
	go func() {
		ctrl.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		// Successful shutdown
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ctrl.Stop() hung or deadlocked under active flood stream")
	}
}

// TC-085: Verification of Ingress Route Scoping and Sensitive Endpoint Boundary Protection (SEC-24)
func TestTC085_TranslateIngress_SecurityGuards(t *testing.T) {
	ingClass := "toron"
	epMap := map[string]*Endpoints{
		"test-ns/svc": {
			Metadata: ObjectMeta{Name: "svc", Namespace: "test-ns"},
			Subsets: []EndpointSubset{
				{
					Addresses: []EndpointAddress{{IP: "10.0.0.1"}},
					Ports:     []EndpointPort{{Port: 8080}},
				},
			},
		},
	}

	createIng := func(host, pathStr string) Ingress {
		return Ingress{
			Metadata: ObjectMeta{Name: "test-ing", Namespace: "test-ns"},
			Spec: IngressSpec{
				IngressClassName: &ingClass,
				Rules: []IngressRule{
					{
						Host: host,
						HTTP: &HTTPIngressRuleValue{
							Paths: []HTTPIngressPath{
								{
									Path: pathStr,
									Backend: IngressBackend{
										Service: &IngressServiceBackend{
											Name: "svc",
											Port: ServiceBackendPort{Number: 8080},
										},
									},
								},
							},
						},
					},
				},
			},
		}
	}

	// 1. TC-085-01: Rejection of unhosted root path ("" or "/")
	_, ok := TranslateIngress(createIng("", "/"), "toron", epMap)
	if ok {
		t.Errorf("expected unhosted root path '/' to be rejected, got ok=true")
	}
	_, ok = TranslateIngress(createIng("", ""), "toron", epMap)
	if ok {
		t.Errorf("expected unhosted empty path '' to be rejected, got ok=true")
	}

	// 2. TC-085-02: Hosted root path ("/" with host) is allowed
	routes, ok := TranslateIngress(createIng("web.example.com", "/"), "toron", epMap)
	if !ok || len(routes) == 0 {
		t.Fatalf("expected hosted root path '/' to be accepted, got ok=%v", ok)
	}
	if routes[0].Host != "web.example.com" || routes[0].Prefix != "/" {
		t.Errorf("expected host 'web.example.com' and prefix '/', got host=%q, prefix=%q",
			routes[0].Host, routes[0].Prefix)
	}

	// 3. TC-085-03: Rejection of /internal and /internal/* shadowing
	for _, p := range []string{"/internal", "/internal/dashboard", "/internal/api/status"} {
		_, ok := TranslateIngress(createIng("web.example.com", p), "toron", epMap)
		if ok {
			t.Errorf("expected path %q to be rejected as protected internal path, got ok=true", p)
		}
	}

	// 4. TC-085-04: Rejection of /api/status shadowing
	for _, p := range []string{"/api/status", "/api/status/routes"} {
		_, ok := TranslateIngress(createIng("web.example.com", p), "toron", epMap)
		if ok {
			t.Errorf("expected path %q to be rejected as protected api status path, got ok=true", p)
		}
	}

	// 5. TC-085-05: Rejection of unhosted /health and /metrics
	_, ok = TranslateIngress(createIng("", "/health"), "toron", epMap)
	if ok {
		t.Errorf("expected unhosted /health to be rejected, got ok=true")
	}
	_, ok = TranslateIngress(createIng("", "/metrics"), "toron", epMap)
	if ok {
		t.Errorf("expected unhosted /metrics to be rejected, got ok=true")
	}

	// 6. TC-085-06: Path traversal canonicalization
	// /app/../internal resolves to /internal -> MUST be rejected
	_, ok = TranslateIngress(createIng("web.example.com", "/app/../internal"), "toron", epMap)
	if ok {
		t.Errorf("expected traversal path '/app/../internal' to be rejected after canonicalization, got ok=true")
	}

	// /internal/../app resolves to /app -> MUST be accepted
	routes, ok = TranslateIngress(createIng("web.example.com", "/internal/../app"), "toron", epMap)
	if !ok || len(routes) == 0 {
		t.Errorf("expected traversal path '/internal/../app' to resolve to '/app' and be accepted, got ok=%v", ok)
	} else if routes[0].Prefix != "/app" {
		t.Errorf("expected canonical prefix '/app', got %q", routes[0].Prefix)
	}
}


