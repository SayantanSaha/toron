package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SayantanSaha/toron/internal/config"
	"github.com/SayantanSaha/toron/internal/listener"
	"github.com/SayantanSaha/toron/internal/router"
	"github.com/SayantanSaha/toron/internal/utils"
)

func main() {
	log.Println("[Toron] Starting server...")

	cfgFile := "config.yaml"
	cfg, err := config.Load(cfgFile)
	if err != nil {
		log.Fatalf("[Toron] Failed to load config: %v", err)
	}

	r := router.NewRouter(cfg.Routes)
	l := listener.NewHTTPListener(cfg.Server, r)

	// Optional HTTP to HTTPS redirector
	if cfg.Server.UseTLS && cfg.Server.RedirectHTTP {
		go func() {
			log.Printf("[Toron] Starting HTTP redirector on %s", cfg.Server.HTTPRedirectPort)
			handler := utils.NewHTTPSRedirectHandler(cfg.Server.Address)
			err := http.ListenAndServe(cfg.Server.HTTPRedirectPort, handler)
			if err != nil {
				log.Printf("[Toron] HTTP redirector failed: %v", err)
			}
		}()
	}

	// Start main HTTPS or HTTP server
	go func() {
		if err := l.Start(); err != nil {
			log.Fatalf("[Toron] Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("[Toron] Caught signal: %s. Shutting down...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := l.Stop(ctx); err != nil {
		log.Fatalf("[Toron] Error during shutdown: %v", err)
	}

	log.Println("[Toron] Shutdown complete.")
}
