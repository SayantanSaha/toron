package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
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
	Data    any                 `json:"data,omitempty"`
}

// NewDummyServer creates a server instance for a given port with comprehensive feature endpoints.
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
		// 1. gRPC Health Check Probing Endpoint (/grpc.health.v1.Health/Check)
		if strings.HasSuffix(r.URL.Path, "/grpc.health.v1.Health/Check") {
			w.Header().Set("Content-Type", "application/grpc")
			w.Header().Set("Trailer", "grpc-status, grpc-message")
			w.Header().Set("grpc-status", "0")
			w.Header().Set("grpc-message", "OK")

			// Protobuf HealthCheckResponse { ServingStatus = SERVING (1) } -> tag: 0x08, val: 0x01
			payload := []byte{0x08, 0x01}
			frame := make([]byte, 5+len(payload))
			frame[0] = 0x00 // uncompressed
			binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
			copy(frame[5:], payload)

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(frame)
			return
		}

		// 2. Standard Health Check Endpoint
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"ok","service":%q,"port":%d}`, cfg.Name, cfg.Port)))
			return
		}

		// 3. Cacheable Response Endpoint with RFC 7234 max-age
		if r.URL.Path == "/cacheable" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "public, max-age=60")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"service":%q,"port":%d,"cached_at":%q,"ttl":"60s"}`, cfg.Name, cfg.Port, time.Now().UTC().Format(time.RFC3339Nano))))
			return
		}

		// 4. Large Payload for Zstd / Brotli / Gzip / Deflate Compression Testing (>2KB)
		if r.URL.Path == "/large" {
			w.Header().Set("Content-Type", "application/json")
			items := make([]map[string]any, 50)
			for i := 0; i < 50; i++ {
				items[i] = map[string]any{
					"id":          i + 1,
					"service":     cfg.Name,
					"port":        cfg.Port,
					"title":       fmt.Sprintf("Compressible Payload Item #%d", i+1),
					"description": "High entropy repeating text designed to thoroughly benchmark and test Zstandard, Brotli, Gzip, and Deflate streaming response compression algorithms in Toron.",
					"timestamp":   time.Now().UTC().Format(time.RFC3339),
				}
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total": 50,
				"items": items,
			})
			return
		}

		// 5. Error simulation endpoint for Circuit Breaker testing
		if r.URL.Path == "/error" || r.URL.Path == "/500" || r.Header.Get("X-Simulate-Error") != "" || r.URL.Query().Get("fail") == "true" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"500 Internal Server Error","service":%q,"port":%d}`, cfg.Name, cfg.Port)))
			return
		}

		// 6. Generic Echo Response Endpoint
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Dummy-Server", cfg.Name)

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

// NewCluster creates dummy servers running on ports startPort through startPort+count-1.
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
