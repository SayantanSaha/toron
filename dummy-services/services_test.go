package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestDummyServices_Launch10Ports(t *testing.T) {
	startPort := 9801 // Use high offset port range for tests
	count := 10

	cluster := NewCluster(startPort, count)
	cluster.StartAll()
	defer cluster.StopAll()

	// Wait briefly for listeners to bind
	time.Sleep(100 * time.Millisecond)

	for i := 0; i < count; i++ {
		port := startPort + i
		expectedName := fmt.Sprintf("dummy-service-%d", i+1)
		url := fmt.Sprintf("http://127.0.0.1:%d/test-path", port)

		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("failed to query dummy service on port %d: %v", port, err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("port %d: expected 200 OK, got %d", port, resp.StatusCode)
		}

		var payload ResponsePayload
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("port %d: failed to decode JSON: %v", port, err)
		}
		_ = resp.Body.Close()

		if payload.Service != expectedName {
			t.Errorf("port %d: expected service name %q, got %q", port, expectedName, payload.Service)
		}
		if payload.Port != port {
			t.Errorf("port %d: expected port %d, got %d", port, port, payload.Port)
		}
		if payload.Path != "/test-path" {
			t.Errorf("port %d: expected path '/test-path', got %q", port, payload.Path)
		}
	}
}
