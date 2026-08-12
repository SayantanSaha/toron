package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"toron/pkg/config"
	"toron/pkg/httpparser"
	"toron/pkg/proxy"
	"toron/pkg/router"
	"toron/pkg/server"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to YAML configuration file (e.g. -config config.yaml)")
	flag.StringVar(&configPath, "c", "", "Path to YAML configuration file (short alias)")
	flag.Parse()

	// If no flag provided, check if config.yaml exists in current working directory
	if configPath == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			configPath = "config.yaml"
		}
	}

	appCfg, err := config.LoadFromFile(configPath)
	if err != nil {
		log.Fatalf("[TORON] Configuration error: %v", err)
	}

	if configPath != "" {
		log.Printf("[TORON] Loaded configuration from %s", configPath)
	} else {
		log.Println("[TORON] No configuration file specified. Using built-in defaults.")
	}

	srvCfg := appCfg.ToServerConfig()
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

	// Register Reverse Proxy routes (with header routing, load balancing, health check & circuit breaker support) if enabled
	if appCfg.Proxy.Enabled {
		for _, pr := range appCfg.Proxy.Routes {
			targets := pr.GetTargets()
			algo := pr.GetAlgorithm()
			log.Printf("[TORON] Configuring Reverse Proxy: prefix %q (headers: %v) -> targets %v [algo: %s, healthCheck: %q]", pr.Prefix, pr.Headers, targets, algo, pr.HealthCheckPath)
			opts := proxy.ProxyOptions{
				Targets:             targets,
				Algorithm:           proxy.Algorithm(algo),
				Timeout:             10 * time.Second,
				HealthCheckPath:     pr.HealthCheckPath,
				HealthCheckInterval: pr.HealthCheckInterval,
				MaxFailures:         pr.ConsecutiveFailures,
				CooldownPeriod:      pr.CooldownPeriod,
			}
			if err := r.ProxyWithOptions(pr.Prefix, pr.Headers, opts); err != nil {
				log.Fatalf("[TORON] Invalid proxy load balancer configuration for targets %v: %v", targets, err)
			}
		}
	}

	// Serve Static Site files if enabled in config
	if appCfg.Static.Enabled {
		log.Printf("[TORON] Serving static assets from %s under prefix %q...", appCfg.Static.Dir, appCfg.Static.Prefix)
		r.Static(appCfg.Static.Prefix, appCfg.Static.Dir)
	}

	srv := server.New(srvCfg, r)

	// Graceful shutdown context listener
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("[TORON] Server listening on http://localhost%s...", srvCfg.Addr)
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
