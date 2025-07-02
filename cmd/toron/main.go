package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SayantanSaha/toron/internal/config"
	"github.com/SayantanSaha/toron/internal/listener"
	"github.com/SayantanSaha/toron/internal/router"
)

func main() {
	log.Println("[Toron] Starting server...")
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[Toron] Failed to load config: %v", err)
	}

	routes := convertRoutes(cfg.Routes)

	r := router.NewRouter(routes)
	l := listener.NewHTTPListener(cfg.Server, r)

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

func convertRoutes(cfgRoutes []config.Route) []config.Route {
	routes := make([]config.Route, len(cfgRoutes))
	for i, r := range cfgRoutes {
		routes[i] = config.Route{
			Path:        r.Path,
			Backend:     r.Backend,
			StripPrefix: r.StripPrefix,
			MatchType:   r.MatchType,
		}
	}
	return routes
}

func StartServer(ctx context.Context, cfg config.ServerConfig, cfgRoutes []config.Route) error {
	routes := convertRoutes(cfgRoutes)
	r := router.NewRouter(routes)
	l := listener.NewHTTPListener(cfg, r)

	go func() {
		if err := l.Start(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Toron] Server error: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return l.Stop(shutdownCtx)
}
