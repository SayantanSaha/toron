package main

import (
	"context"
	"testing"
	"time"

	"github.com/SayantanSaha/toron/internal/config"
	"github.com/SayantanSaha/toron/internal/router"
)

func TestStartServer_BasicRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cfg := config.ServerConfig{
		Address: ":0", // let OS assign a free port
		UseTLS:  false,
	}

	routes := []router.Route{
		{Path: "/test", Backend: "http://localhost:9999"},
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel() // trigger shutdown
	}()

	err := StartServer(ctx, cfg, routes)
	if err != nil {
		t.Errorf("server failed to start/stop cleanly: %v", err)
	}
}
