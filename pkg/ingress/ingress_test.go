package ingress

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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"toron/pkg/config"
	"toron/pkg/httpparser"
	"toron/pkg/proxy"
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

func TestIngressController_BoundedRouteTable(t *testing.T) {
	mux := http.NewServeMux()
	ingClass := "toron"

	ingresses := IngressList{
		Kind:       "IngressList",
		APIVersion: "networking.k8s.io/v1",
		Items: []Ingress{
			{
				Metadata: ObjectMeta{Name: "app1-ing", Namespace: "default"},
				Spec: IngressSpec{
					IngressClassName: &ingClass,
					Rules: []IngressRule{
						{
							Host: "app1.cluster.local",
							HTTP: &HTTPIngressRuleValue{
								Paths: []HTTPIngressPath{
									{
										Path: "/app1",
										Backend: IngressBackend{
											Service: &IngressServiceBackend{
												Name: "svc-1",
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
			{
				Metadata: ObjectMeta{Name: "app2-ing", Namespace: "default"},
				Spec: IngressSpec{
					IngressClassName: &ingClass,
					Rules: []IngressRule{
						{
							Host: "app2.cluster.local",
							HTTP: &HTTPIngressRuleValue{
								Paths: []HTTPIngressPath{
									{
										Path: "/app2",
										Backend: IngressBackend{
											Service: &IngressServiceBackend{
												Name: "svc-2",
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
			{
				Metadata: ObjectMeta{Name: "app3-ing", Namespace: "default"},
				Spec: IngressSpec{
					IngressClassName: &ingClass,
					Rules: []IngressRule{
						{
							Host: "app3.cluster.local",
							HTTP: &HTTPIngressRuleValue{
								Paths: []HTTPIngressPath{
									{
										Path: "/app3",
										Backend: IngressBackend{
											Service: &IngressServiceBackend{
												Name: "svc-3",
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

	mux.HandleFunc("/apis/networking.k8s.io/v1/ingresses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ingresses)
	})

	eps := map[string]Endpoints{
		"svc-1": {
			Metadata: ObjectMeta{Name: "svc-1", Namespace: "default"},
			Subsets: []EndpointSubset{
				{
					Addresses: []EndpointAddress{{IP: "10.244.0.10"}},
					Ports:     []EndpointPort{{Port: 8080}},
				},
			},
		},
		"svc-2": {
			Metadata: ObjectMeta{Name: "svc-2", Namespace: "default"},
			Subsets: []EndpointSubset{
				{
					Addresses: []EndpointAddress{{IP: "10.244.0.20"}},
					Ports:     []EndpointPort{{Port: 8080}},
				},
			},
		},
		"svc-3": {
			Metadata: ObjectMeta{Name: "svc-3", Namespace: "default"},
			Subsets: []EndpointSubset{
				{
					Addresses: []EndpointAddress{{IP: "10.244.0.30"}},
					Ports:     []EndpointPort{{Port: 8080}},
				},
			},
		},
	}

	for svcName, ep := range eps {
		svc := svcName
		endpoint := ep
		mux.HandleFunc("/api/v1/namespaces/default/endpoints/"+svc, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(endpoint)
		})
	}

	server := httptest.NewServer(mux)
	defer server.Close()

	r := router.New()
	// Pre-register 1 static route to verify coexistence
	tempDir := t.TempDir()
	err := r.RoutePrefix(router.RouteTypeStatic, "", "/static", nil, tempDir, proxy.ProxyOptions{})
	if err != nil {
		t.Fatalf("RoutePrefix static failed: %v", err)
	}

	cfg := config.IngressConfig{
		Enabled:       true,
		IngressClass:  "toron",
		KubeAPIServer: server.URL,
	}

	ctrl, err := NewController(cfg, r)
	if err != nil {
		t.Fatalf("NewController failed: %v", err)
	}

	ctx := context.Background()

	// Execute 50 consecutive synchronization iterations
	for i := 1; i <= 50; i++ {
		ctrl.syncIngresses(ctx)
		routes := r.GetPrefixRoutes()
		var k8sRoutes int
		for _, rt := range routes {
			if rt.Source == "k8s-ingress" {
				k8sRoutes++
			}
		}
		if k8sRoutes != 3 {
			t.Fatalf("Iteration %d: expected exactly 3 k8s-ingress routes, got %d", i, k8sRoutes)
		}
		if len(routes) != 4 {
			t.Fatalf("Iteration %d: expected exactly 4 total routes (3 k8s + 1 static), got %d", i, len(routes))
		}
	}
}

func TestIngressController_ZombieRoutePruning(t *testing.T) {
	backendA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("service-a payload"))
	}))
	defer backendA.Close()

	backendB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("service-b payload"))
	}))
	defer backendB.Close()

	uA, _ := url.Parse(backendA.URL)
	hostA, portStrA, _ := net.SplitHostPort(uA.Host)
	portA, _ := strconv.Atoi(portStrA)

	uB, _ := url.Parse(backendB.URL)
	hostB, portStrB, _ := net.SplitHostPort(uB.Host)
	portB, _ := strconv.Atoi(portStrB)

	var mu sync.Mutex
	ingClass := "toron"
	currentIngresses := make(map[string]Ingress)
	currentEndpoints := make(map[string]Endpoints)

	mux := http.NewServeMux()
	mux.HandleFunc("/apis/networking.k8s.io/v1/ingresses", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		list := IngressList{
			Kind:       "IngressList",
			APIVersion: "networking.k8s.io/v1",
		}
		for _, ing := range currentIngresses {
			list.Items = append(list.Items, ing)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

	mux.HandleFunc("/api/v1/namespaces/default/endpoints/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		svcName := strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/default/endpoints/")
		if ep, ok := currentEndpoints[svcName]; ok {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ep)
			return
		}
		http.NotFound(w, r)
	})

	apiServer := httptest.NewServer(mux)
	defer apiServer.Close()

	r := router.New()
	cfg := config.IngressConfig{
		Enabled:       true,
		IngressClass:  "toron",
		KubeAPIServer: apiServer.URL,
	}

	ctrl, err := NewController(cfg, r)
	if err != nil {
		t.Fatalf("NewController failed: %v", err)
	}
	ctx := context.Background()

	// Phase 1: Deploy Ingress for api.example.com/service-a
	mu.Lock()
	currentIngresses["service-a"] = Ingress{
		Metadata: ObjectMeta{Name: "service-a", Namespace: "default"},
		Spec: IngressSpec{
			IngressClassName: &ingClass,
			Rules: []IngressRule{
				{
					Host: "api.example.com",
					HTTP: &HTTPIngressRuleValue{
						Paths: []HTTPIngressPath{
							{
								Path: "/service-a",
								Backend: IngressBackend{
									Service: &IngressServiceBackend{
										Name: "svc-a",
										Port: ServiceBackendPort{Number: portA},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	currentEndpoints["svc-a"] = Endpoints{
		Metadata: ObjectMeta{Name: "svc-a", Namespace: "default"},
		Subsets: []EndpointSubset{
			{
				Addresses: []EndpointAddress{{IP: hostA}},
				Ports:     []EndpointPort{{Port: portA}},
			},
		},
	}
	mu.Unlock()

	ctrl.syncIngresses(ctx)

	// Send HTTP request to /service-a
	reqA, _ := httpparser.NewRequest("GET", "/service-a", "HTTP/1.1")
	reqA.Header.Set("Host", "api.example.com")
	resA := httpparser.NewResponse()
	r.ServeHTTP(reqA, resA)
	bodyA := resA.BodyString()
	if resA.StatusCode != http.StatusOK || !strings.Contains(bodyA, "service-a payload") {
		t.Fatalf("Phase 1: expected 200 service-a payload, got %d %q", resA.StatusCode, bodyA)
	}
	if len(r.GetPrefixRoutes()) != 1 {
		t.Fatalf("Phase 1: expected 1 route, got %d", len(r.GetPrefixRoutes()))
	}

	// Phase 2: Ingress Deletion in K8s
	mu.Lock()
	delete(currentIngresses, "service-a")
	delete(currentEndpoints, "svc-a")
	mu.Unlock()

	ctrl.syncIngresses(ctx)

	// Phase 3: Zombie Elimination Verification
	if len(r.GetPrefixRoutes()) != 0 {
		t.Fatalf("Phase 3: expected 0 prefix routes after deletion, got %d", len(r.GetPrefixRoutes()))
	}
	if len(ctrl.ActiveRoutes()) != 0 {
		t.Fatalf("Phase 3: expected 0 active routes in controller, got %d", len(ctrl.ActiveRoutes()))
	}

	resADeleted := httpparser.NewResponse()
	r.ServeHTTP(reqA, resADeleted)
	if resADeleted.StatusCode != http.StatusNotFound {
		t.Fatalf("Phase 3: expected 404 Not Found after deletion, got %d", resADeleted.StatusCode)
	}

	// Phase 4: Partial Route Deletion with Multiple Ingresses
	mu.Lock()
	currentIngresses["ing-alpha"] = Ingress{
		Metadata: ObjectMeta{Name: "ing-alpha", Namespace: "default"},
		Spec: IngressSpec{
			IngressClassName: &ingClass,
			Rules: []IngressRule{
				{
					Host: "api.example.com",
					HTTP: &HTTPIngressRuleValue{
						Paths: []HTTPIngressPath{
							{
								Path: "/alpha",
								Backend: IngressBackend{
									Service: &IngressServiceBackend{
										Name: "svc-alpha",
										Port: ServiceBackendPort{Number: portA},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	currentEndpoints["svc-alpha"] = Endpoints{
		Metadata: ObjectMeta{Name: "svc-alpha", Namespace: "default"},
		Subsets: []EndpointSubset{
			{
				Addresses: []EndpointAddress{{IP: hostA}},
				Ports:     []EndpointPort{{Port: portA}},
			},
		},
	}

	currentIngresses["ing-beta"] = Ingress{
		Metadata: ObjectMeta{Name: "ing-beta", Namespace: "default"},
		Spec: IngressSpec{
			IngressClassName: &ingClass,
			Rules: []IngressRule{
				{
					Host: "api.example.com",
					HTTP: &HTTPIngressRuleValue{
						Paths: []HTTPIngressPath{
							{
								Path: "/beta",
								Backend: IngressBackend{
									Service: &IngressServiceBackend{
										Name: "svc-beta",
										Port: ServiceBackendPort{Number: portB},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	currentEndpoints["svc-beta"] = Endpoints{
		Metadata: ObjectMeta{Name: "svc-beta", Namespace: "default"},
		Subsets: []EndpointSubset{
			{
				Addresses: []EndpointAddress{{IP: hostB}},
				Ports:     []EndpointPort{{Port: portB}},
			},
		},
	}
	mu.Unlock()

	ctrl.syncIngresses(ctx)
	if len(r.GetPrefixRoutes()) != 2 {
		t.Fatalf("Phase 4: expected 2 routes, got %d", len(r.GetPrefixRoutes()))
	}

	// Delete /alpha only
	mu.Lock()
	delete(currentIngresses, "ing-alpha")
	delete(currentEndpoints, "svc-alpha")
	mu.Unlock()

	ctrl.syncIngresses(ctx)
	if len(r.GetPrefixRoutes()) != 1 {
		t.Fatalf("Phase 4: expected 1 route after deleting alpha, got %d", len(r.GetPrefixRoutes()))
	}

	reqAlpha, _ := httpparser.NewRequest("GET", "/alpha", "HTTP/1.1")
	reqAlpha.Header.Set("Host", "api.example.com")
	resAlpha := httpparser.NewResponse()
	r.ServeHTTP(reqAlpha, resAlpha)
	if resAlpha.StatusCode != http.StatusNotFound {
		t.Fatalf("Phase 4: expected 404 for deleted /alpha, got %d", resAlpha.StatusCode)
	}

	reqBeta, _ := httpparser.NewRequest("GET", "/beta", "HTTP/1.1")
	reqBeta.Header.Set("Host", "api.example.com")
	resBeta := httpparser.NewResponse()
	r.ServeHTTP(reqBeta, resBeta)
	bodyBeta := resBeta.BodyString()
	if resBeta.StatusCode != http.StatusOK || !strings.Contains(bodyBeta, "service-b payload") {
		t.Fatalf("Phase 4: expected 200 for remaining /beta, got %d %q", resBeta.StatusCode, bodyBeta)
	}
}

func TestIngressController_MultiPodLoadBalancing(t *testing.T) {
	pod1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pod-1"))
	}))
	defer pod1.Close()

	pod2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pod-2"))
	}))
	defer pod2.Close()

	pod3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pod-3"))
	}))
	defer pod3.Close()

	u1, _ := url.Parse(pod1.URL)
	host1, portStr1, _ := net.SplitHostPort(u1.Host)
	port1, _ := strconv.Atoi(portStr1)

	u2, _ := url.Parse(pod2.URL)
	host2, portStr2, _ := net.SplitHostPort(u2.Host)
	port2, _ := strconv.Atoi(portStr2)

	u3, _ := url.Parse(pod3.URL)
	host3, portStr3, _ := net.SplitHostPort(u3.Host)
	port3, _ := strconv.Atoi(portStr3)

	ingClass := "toron"
	mux := http.NewServeMux()
	mux.HandleFunc("/apis/networking.k8s.io/v1/ingresses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		list := IngressList{
			Kind:       "IngressList",
			APIVersion: "networking.k8s.io/v1",
			Items: []Ingress{
				{
					Metadata: ObjectMeta{Name: "app-ingress", Namespace: "default"},
					Spec: IngressSpec{
						IngressClassName: &ingClass,
						Rules: []IngressRule{
							{
								Host: "lb.example.com",
								HTTP: &HTTPIngressRuleValue{
									Paths: []HTTPIngressPath{
										{
											Path: "/app",
											Backend: IngressBackend{
												Service: &IngressServiceBackend{
													Name: "app-svc",
													Port: ServiceBackendPort{Number: port1},
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

	mux.HandleFunc("/api/v1/namespaces/default/endpoints/app-svc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 3 pod endpoints across 3 subsets with their respective ports
		ep := Endpoints{
			Metadata: ObjectMeta{Name: "app-svc", Namespace: "default"},
			Subsets: []EndpointSubset{
				{
					Addresses: []EndpointAddress{{IP: host1}},
					Ports:     []EndpointPort{{Port: port1}},
				},
				{
					Addresses: []EndpointAddress{{IP: host2}},
					Ports:     []EndpointPort{{Port: port2}},
				},
				{
					Addresses: []EndpointAddress{{IP: host3}},
					Ports:     []EndpointPort{{Port: port3}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(ep)
	})

	apiServer := httptest.NewServer(mux)
	defer apiServer.Close()

	r := router.New()
	cfg := config.IngressConfig{
		Enabled:       true,
		IngressClass:  "toron",
		KubeAPIServer: apiServer.URL,
	}

	ctrl, err := NewController(cfg, r)
	if err != nil {
		t.Fatalf("NewController failed: %v", err)
	}

	ctx := context.Background()
	ctrl.syncIngresses(ctx)

	// Verify route table: exactly ONE route registered for lb.example.com /app
	routes := r.GetPrefixRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected exactly 1 prefix route registered for multi-pod service, got %d", len(routes))
	}
	if routes[0].Host != "lb.example.com" || routes[0].Prefix != "/app" {
		t.Errorf("unexpected route: %+v", routes[0])
	}

	// Dispatch 30 sequential HTTP requests
	podCounts := make(map[string]int)
	for i := 0; i < 30; i++ {
		req, _ := httpparser.NewRequest("GET", "/app", "HTTP/1.1")
		req.Header.Set("Host", "lb.example.com")
		req.RemoteAddr = "192.0.2.1:54321"
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("Request %d failed with status %d", i, res.StatusCode)
		}
		podCounts[res.BodyString()]++
	}

	// Verify round-robin distribution: exactly 10 requests per pod
	if podCounts["pod-1"] != 10 {
		t.Errorf("pod-1 count = %d, want 10", podCounts["pod-1"])
	}
	if podCounts["pod-2"] != 10 {
		t.Errorf("pod-2 count = %d, want 10", podCounts["pod-2"])
	}
	if podCounts["pod-3"] != 10 {
		t.Errorf("pod-3 count = %d, want 10", podCounts["pod-3"])
	}
}

func TestIngressController_EndpointUpdateNoShadowing(t *testing.T) {
	serverOld := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("old-pod"))
	}))

	serverNew := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("new-pod"))
	}))
	defer serverNew.Close()

	uOld, _ := url.Parse(serverOld.URL)
	hostOld, portStrOld, _ := net.SplitHostPort(uOld.Host)
	portOld, _ := strconv.Atoi(portStrOld)

	uNew, _ := url.Parse(serverNew.URL)
	hostNew, portStrNew, _ := net.SplitHostPort(uNew.Host)
	portNew, _ := strconv.Atoi(portStrNew)

	var mu sync.Mutex
	currentHost := hostOld
	currentPort := portOld

	ingClass := "toron"
	mux := http.NewServeMux()
	mux.HandleFunc("/apis/networking.k8s.io/v1/ingresses", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		p := currentPort
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		list := IngressList{
			Kind:       "IngressList",
			APIVersion: "networking.k8s.io/v1",
			Items: []Ingress{
				{
					Metadata: ObjectMeta{Name: "rollout-ing", Namespace: "default"},
					Spec: IngressSpec{
						IngressClassName: &ingClass,
						Rules: []IngressRule{
							{
								Host: "rollout.example.com",
								HTTP: &HTTPIngressRuleValue{
									Paths: []HTTPIngressPath{
										{
											Path: "/app",
											Backend: IngressBackend{
												Service: &IngressServiceBackend{
													Name: "rollout-svc",
													Port: ServiceBackendPort{Number: p},
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

	mux.HandleFunc("/api/v1/namespaces/default/endpoints/rollout-svc", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		h := currentHost
		p := currentPort
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		ep := Endpoints{
			Metadata: ObjectMeta{Name: "rollout-svc", Namespace: "default"},
			Subsets: []EndpointSubset{
				{
					Addresses: []EndpointAddress{{IP: h}},
					Ports:     []EndpointPort{{Port: p}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(ep)
	})

	apiServer := httptest.NewServer(mux)
	defer apiServer.Close()

	r := router.New()
	cfg := config.IngressConfig{
		Enabled:       true,
		IngressClass:  "toron",
		KubeAPIServer: apiServer.URL,
	}

	ctrl, err := NewController(cfg, r)
	if err != nil {
		t.Fatalf("NewController failed: %v", err)
	}

	ctx := context.Background()

	// Initial Deployment
	ctrl.syncIngresses(ctx)

	for i := 0; i < 5; i++ {
		req, _ := httpparser.NewRequest("GET", "/app", "HTTP/1.1")
		req.Header.Set("Host", "rollout.example.com")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		bodyOld := res.BodyString()
		if res.StatusCode != http.StatusOK || !strings.Contains(bodyOld, "old-pod") {
			t.Fatalf("Phase 1: request %d expected old-pod, got %d %q", i, res.StatusCode, bodyOld)
		}
	}

	// Terminate old pod server
	serverOld.Close()

	// Update endpoints to new pod
	mu.Lock()
	currentHost = hostNew
	currentPort = portNew
	mu.Unlock()

	ctrl.syncIngresses(ctx)

	// Cutover Verification: 10 consecutive requests should hit new-pod
	for i := 0; i < 10; i++ {
		req, _ := httpparser.NewRequest("GET", "/app", "HTTP/1.1")
		req.Header.Set("Host", "rollout.example.com")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		bodyNew := res.BodyString()
		if res.StatusCode != http.StatusOK || !strings.Contains(bodyNew, "new-pod") {
			t.Fatalf("Phase 2: request %d expected new-pod, got %d %q", i, res.StatusCode, bodyNew)
		}
	}

	routes := r.GetPrefixRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected exactly 1 route after cutover, got %d", len(routes))
	}
}

func TestIngressController_ConcurrentSyncAndRouting_RaceClean(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-1-ok"))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-2-ok"))
	}))
	defer backend2.Close()

	u1, _ := url.Parse(backend1.URL)
	h1, p1Str, _ := net.SplitHostPort(u1.Host)
	p1, _ := strconv.Atoi(p1Str)

	u2, _ := url.Parse(backend2.URL)
	h2, p2Str, _ := net.SplitHostPort(u2.Host)
	p2, _ := strconv.Atoi(p2Str)

	var mu sync.Mutex
	includeV2 := true
	ingClass := "toron"

	mux := http.NewServeMux()
	mux.HandleFunc("/apis/networking.k8s.io/v1/ingresses", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		v2 := includeV2
		mu.Unlock()

		items := []Ingress{
			{
				Metadata: ObjectMeta{Name: "ing-v1", Namespace: "default"},
				Spec: IngressSpec{
					IngressClassName: &ingClass,
					Rules: []IngressRule{
						{
							Host: "race.example.com",
							HTTP: &HTTPIngressRuleValue{
								Paths: []HTTPIngressPath{
									{
										Path: "/v1",
										Backend: IngressBackend{
											Service: &IngressServiceBackend{
												Name: "svc-v1",
												Port: ServiceBackendPort{Number: p1},
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

		if v2 {
			items = append(items, Ingress{
				Metadata: ObjectMeta{Name: "ing-v2", Namespace: "default"},
				Spec: IngressSpec{
					IngressClassName: &ingClass,
					Rules: []IngressRule{
						{
							Host: "race.example.com",
							HTTP: &HTTPIngressRuleValue{
								Paths: []HTTPIngressPath{
									{
										Path: "/v2",
										Backend: IngressBackend{
											Service: &IngressServiceBackend{
												Name: "svc-v2",
												Port: ServiceBackendPort{Number: p2},
											},
										},
									},
								},
							},
						},
					},
				},
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(IngressList{
			Kind:       "IngressList",
			APIVersion: "networking.k8s.io/v1",
			Items:      items,
		})
	})

	mux.HandleFunc("/api/v1/namespaces/default/endpoints/svc-v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Endpoints{
			Metadata: ObjectMeta{Name: "svc-v1", Namespace: "default"},
			Subsets: []EndpointSubset{
				{
					Addresses: []EndpointAddress{{IP: h1}},
					Ports:     []EndpointPort{{Port: p1}},
				},
			},
		})
	})

	mux.HandleFunc("/api/v1/namespaces/default/endpoints/svc-v2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Endpoints{
			Metadata: ObjectMeta{Name: "svc-v2", Namespace: "default"},
			Subsets: []EndpointSubset{
				{
					Addresses: []EndpointAddress{{IP: h2}},
					Ports:     []EndpointPort{{Port: p2}},
				},
			},
		})
	})

	apiServer := httptest.NewServer(mux)
	defer apiServer.Close()

	r := router.New()
	// Add a static/config route to verify route isolation under concurrent churn
	tempDir := t.TempDir()
	staticFile := filepath.Join(tempDir, "index.html")
	_ = os.WriteFile(staticFile, []byte("static content"), 0644)
	_ = r.RoutePrefix(router.RouteTypeStatic, "", "/static", nil, tempDir, proxy.ProxyOptions{})

	cfg := config.IngressConfig{
		Enabled:       true,
		IngressClass:  "toron",
		KubeAPIServer: apiServer.URL,
	}

	ctrl, err := NewController(cfg, r)
	if err != nil {
		t.Fatalf("NewController failed: %v", err)
	}

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	ctx := context.Background()
	// Perform initial sync so initial routes are established
	ctrl.syncIngresses(ctx)

	// 5 background churn goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					mu.Lock()
					includeV2 = !includeV2
					mu.Unlock()

					ctrl.syncIngresses(ctx)
					time.Sleep(10 * time.Millisecond)
				}
			}
		}(i)
	}

	// 20 high-volume HTTP client goroutines
	var requestCount int64
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			paths := []string{"/v1", "/v2", "/static/index.html", "/deleted"}
			idx := 0
			for {
				select {
				case <-stopCh:
					return
				default:
					p := paths[idx%len(paths)]
					idx++
					req, _ := httpparser.NewRequest("GET", p, "HTTP/1.1")
					req.Header.Set("Host", "race.example.com")
					res := httpparser.NewResponse()
					r.ServeHTTP(req, res)

					atomic.AddInt64(&requestCount, 1)

					// Verify valid response status
					switch p {
					case "/v1":
						// /v1 should always return 200 OK
						if res.StatusCode != http.StatusOK {
							t.Errorf("expected 200 for /v1, got %d", res.StatusCode)
						}
					case "/static/index.html":
						// /static should always return 200 OK (isolated from k8s churn)
						if res.StatusCode != http.StatusOK {
							t.Errorf("expected 200 for /static, got %d", res.StatusCode)
						}
					case "/deleted":
						// /deleted should always return 404
						if res.StatusCode != http.StatusNotFound {
							t.Errorf("expected 404 for /deleted, got %d", res.StatusCode)
						}
					case "/v2":
						// /v2 may be 200 or 404 depending on churn, but never 500 or corrupt
						if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNotFound {
							t.Errorf("unexpected status for /v2: %d", res.StatusCode)
						}
					}
				}
			}
		}(i)
	}

	time.Sleep(500 * time.Millisecond)
	close(stopCh)
	wg.Wait()

	if atomic.LoadInt64(&requestCount) == 0 {
		t.Fatal("expected positive request count, got 0")
	}
}
