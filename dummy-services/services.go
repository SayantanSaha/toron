package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// ServiceConfig defines configuration for a single dummy service.
type ServiceConfig struct {
	Name string `json:"service"`
	Port int    `json:"port"`
}

// DummyServer wraps an individual dummy HTTP server instance.
type DummyServer struct {
	Config ServiceConfig
	server *http.Server
}

// ResponsePayload defines the JSON response returned by dummy services.
type ResponsePayload struct {
	Service string              `json:"service"`
	Port    int                 `json:"port"`
	Path    string              `json:"path"`
	Method  string              `json:"method"`
	Headers map[string][]string `json:"headers"`
}

// NewDummyServer creates a server instance for a given port.
func NewDummyServer(serviceName string, port int) *DummyServer {
	cfg := ServiceConfig{
		Name: serviceName,
		Port: port,
	}

	mux := http.NewServeMux()
	ds := &DummyServer{
		Config: cfg,
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Dummy-Server", cfg.Name)

		if r.URL.Path == "/error" || r.URL.Path == "/500" || r.Header.Get("X-Simulate-Error") != "" || r.URL.Query().Get("fail") == "true" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"500 Internal Server Error","service":%q,"port":%d}`, cfg.Name, cfg.Port)))
			return
		}

		payload := ResponsePayload{
			Service: cfg.Name,
			Port:    cfg.Port,
			Path:    r.URL.Path,
			Method:  r.Method,
			Headers: r.Header,
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(payload)
	})

	ds.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return ds
}

// Start listens and serves on the configured port.
func (ds *DummyServer) Start() error {
	listener, err := net.Listen("tcp", ds.server.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", ds.server.Addr, err)
	}
	return ds.server.Serve(listener)
}

// Stop gracefully stops the server.
func (ds *DummyServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return ds.server.Shutdown(ctx)
}

// ServiceCluster manages multiple dummy servers.
type ServiceCluster struct {
	Servers []*DummyServer
}

// NewCluster creates 10 dummy servers running on ports startPort through startPort+count-1.
func NewCluster(startPort int, count int) *ServiceCluster {
	cluster := &ServiceCluster{}
	for i := 0; i < count; i++ {
		port := startPort + i
		name := fmt.Sprintf("dummy-service-%d", i+1)
		cluster.Servers = append(cluster.Servers, NewDummyServer(name, port))
	}
	return cluster
}

// StartAll starts all servers asynchronously.
func (sc *ServiceCluster) StartAll() {
	for _, srv := range sc.Servers {
		s := srv
		go func() {
			if err := s.Start(); err != nil && err != http.ErrServerClosed {
				log.Printf("[%s] Server error: %v", s.Config.Name, err)
			}
		}()
	}
}

// StopAll gracefully shuts down all servers in the cluster.
func (sc *ServiceCluster) StopAll() {
	var wg sync.WaitGroup
	for _, srv := range sc.Servers {
		s := srv
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Stop()
		}()
	}
	wg.Wait()
}
