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
	var routesPath string
	var testConfig bool
	flag.StringVar(&configPath, "config", "", "Path to YAML server configuration file (e.g. -config config.yaml)")
	flag.StringVar(&configPath, "c", "", "Path to YAML server configuration file (short alias)")
	flag.StringVar(&routesPath, "routes", "", "Path to YAML proxy routing configuration file (e.g. -routes routes.yaml)")
	flag.StringVar(&routesPath, "r", "", "Path to YAML proxy routing configuration file (short alias)")
	flag.BoolVar(&testConfig, "test-config", false, "Test configuration files syntax and exit without running server")
	flag.BoolVar(&testConfig, "t", false, "Test configuration files syntax and exit (short alias)")
	flag.Parse()

	// If no flag provided, check if default files exist in current working directory
	if configPath == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			configPath = "config.yaml"
		}
	}
	if routesPath == "" {
		if _, err := os.Stat("routes.yaml"); err == nil {
			routesPath = "routes.yaml"
		}
	}

	appCfg, err := config.LoadFromFiles(configPath, routesPath)
	if err != nil {
		if testConfig {
			log.Printf("[TORON] Configuration syntax ERROR: %v", err)
			os.Exit(1)
		}
		log.Fatalf("[TORON] Configuration error: %v", err)
	}

	// Dry-run mode: test configuration syntax and exit
	if testConfig {
		if valErr := config.ValidateConfig(appCfg); valErr != nil {
			log.Printf("[TORON] Configuration syntax ERROR: %v", valErr)
			os.Exit(1)
		}
		log.Printf("[TORON] Configuration syntax OK: %s and %s are valid.", configPath, routesPath)
		os.Exit(0)
	}

	if configPath != "" {
		log.Printf("[TORON] Loaded server configuration from %s", configPath)
	} else {
		log.Println("[TORON] No configuration file specified. Using built-in defaults.")
	}

	if routesPath != "" {
		log.Printf("[TORON] Loaded routing configuration from %s", routesPath)
	}

	srvCfg := appCfg.ToServerConfig()
	r := router.New()

	// Attach Middlewares
	r.Use(router.LoggerMiddleware())
	r.Use(router.RecoveryMiddleware())

	// Register Internal Management API Routes (/internal/api/)
	internalRoutes := make([]server.RouteInfo, 0)
	if appCfg.Proxy.Enabled {
		for _, pr := range appCfg.Proxy.Routes {
			internalRoutes = append(internalRoutes, server.RouteInfo{
				Host:      pr.GetHost(),
				Prefix:    pr.Prefix,
				Headers:   pr.Headers,
				Algorithm: pr.GetAlgorithm(),
				Targets:   pr.GetTargets(),
			})
		}
	}
	internalCfg := server.InternalAPIConfig{
		Port:           appCfg.Server.Port,
		WorkerPoolSize: appCfg.Server.WorkerPoolSize,
		ProxyEnabled:   appCfg.Proxy.Enabled,
		Routes:         internalRoutes,
		StaticEnabled:  appCfg.Static.Enabled,
		StaticPrefix:   appCfg.Static.Prefix,
		StaticDir:      appCfg.Static.Dir,
	}
	server.RegisterInternalAPIRoutes(r, internalCfg)

	// Register API Routes
	r.GET("/health", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"status":"ok"}`)
	})

	r.GET("/api/status", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"server":"Toron","version":"1.0.0","uptime":"healthy","engine":"event-driven"}`)
	})

	// Register Reverse Proxy routes (with domain routing, header routing, load balancing, health check & circuit breaker support) if enabled
	if appCfg.Proxy.Enabled {
		for _, pr := range appCfg.Proxy.Routes {
			targets := pr.GetTargets()
			algo := pr.GetAlgorithm()
			host := pr.GetHost()
			log.Printf("[TORON] Configuring Reverse Proxy: host %q, prefix %q (headers: %v) -> targets %v [algo: %s, healthCheck: %q]", host, pr.Prefix, pr.Headers, targets, algo, pr.HealthCheckPath)
			opts := proxy.ProxyOptions{
				Targets:             targets,
				Algorithm:           proxy.Algorithm(algo),
				Timeout:             10 * time.Second,
				HealthCheckPath:     pr.HealthCheckPath,
				HealthCheckInterval: pr.HealthCheckInterval,
				MaxFailures:         pr.ConsecutiveFailures,
				CooldownPeriod:      pr.CooldownPeriod,
			}
			if err := r.ProxyWithOptions(host, pr.Prefix, pr.Headers, opts); err != nil {
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
