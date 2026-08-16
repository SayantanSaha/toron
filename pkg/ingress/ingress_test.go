package ingress

import (
	"context"
	"encoding/json"
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
