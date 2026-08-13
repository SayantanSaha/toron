package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"toron/pkg/config"
)

func TestRouteWatcher_HotReload(t *testing.T) {
	tempDir := t.TempDir()
	routesFile := filepath.Join(tempDir, "routes.yaml")

	initialContent := `routes:
  - type: "static"
    prefix: "/test"
    dir: "./public"
`
	if err := os.WriteFile(routesFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write routes.yaml: %v", err)
	}

	reloadCh := make(chan []config.ProxyRouteConfig, 1)

	watcher, err := config.NewRouteWatcher(routesFile, func(routes []config.ProxyRouteConfig) {
		reloadCh <- routes
	})
	if err != nil {
		t.Fatalf("failed to create RouteWatcher: %v", err)
	}
	defer watcher.Stop()

	// Update routes.yaml content
	updatedContent := `routes:
  - type: "upstream"
    prefix: "/api"
    target: "http://localhost:9001"
  - type: "static"
    prefix: "/test"
    dir: "./public"
`
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(routesFile, []byte(updatedContent), 0644); err != nil {
		t.Fatalf("failed to update routes.yaml: %v", err)
	}

	select {
	case routes := <-reloadCh:
		if len(routes) != 2 {
			t.Errorf("expected 2 reloaded routes, got %d", len(routes))
		}
	case <-time.After(2 * time.Second):
		t.Errorf("timeout waiting for route hot reload event")
	}
}
