package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/router"
	"toron/pkg/server"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cfg := server.DefaultConfig()
	cfg.Addr = ":" + port

	r := router.New()

	// Attach Middlewares
	r.Use(router.LoggerMiddleware())
	r.Use(router.RecoveryMiddleware())

	// Register API Routes
	r.GET("/health", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"status":"ok"}`)
	})

	r.GET("/api/status", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"server":"Toron","version":"1.0.0","uptime":"healthy","engine":"event-driven"}`)
	})

	// Serve Static Site files from ./public directory
	r.Static("/", "./public")

	srv := server.New(cfg, r)

	// Graceful shutdown context listener
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("[TORON] Server listening on http://localhost%s...", cfg.Addr)
		log.Printf("[TORON] Serving static site from ./public...")
		if err := srv.ListenAndServe(); err != nil && err != server.ErrServerClosed {
			log.Fatalf("[TORON] Server fatal error: %v", err)
		}
	}()

	<-shutdownCtx.Done()
	log.Println("[TORON] Shutdown signal received. Shutting down gracefully...")

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(stopCtx); err != nil {
		log.Printf("[TORON] Error during graceful shutdown: %v", err)
	} else {
		log.Println("[TORON] Server stopped cleanly.")
	}
}
