package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
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
	if appCfg.Server.Cache.Enabled {
		r.Use(router.NewCacheMiddleware(router.CacheConfig{
			Enabled:        appCfg.Server.Cache.Enabled,
			DefaultTTL:     appCfg.Server.Cache.DefaultTTL,
			MaxEntries:     appCfg.Server.Cache.MaxEntries,
			MaxPayloadSize: appCfg.Server.Cache.MaxPayloadSize,
		}))
	}
	if appCfg.Server.Compression.Enabled {
		r.Use(router.NewCompressionMiddleware(router.CompressionConfig{
			Enabled:   appCfg.Server.Compression.Enabled,
			MinLength: appCfg.Server.Compression.MinLength,
			Level:     appCfg.Server.Compression.Level,
			Encodings: appCfg.Server.Compression.Encodings,
			Types:     appCfg.Server.Compression.Types,
		}))
	}
	if appCfg.Server.Auth.Type != "" {
		r.Use(router.NewAuthMiddleware(router.AuthConfig{
			Type: router.AuthType(appCfg.Server.Auth.Type),
			JWT: router.JWTConfig{
				Secret:   appCfg.Server.Auth.JWT.Secret,
				Issuer:   appCfg.Server.Auth.JWT.Issuer,
				Audience: appCfg.Server.Auth.JWT.Audience,
			},
			APIKey: router.APIKeyConfig{
				Keys:   appCfg.Server.Auth.APIKey.Keys,
				Header: appCfg.Server.Auth.APIKey.Header,
				Query:  appCfg.Server.Auth.APIKey.Query,
			},
			Basic: router.BasicAuthConfig{
				Users: appCfg.Server.Auth.Basic.Users,
				Realm: appCfg.Server.Auth.Basic.Realm,
			},
			Excluded: appCfg.Server.Auth.Excluded,
		}))
	}

	// Register Internal Management API Routes (/internal/api/)
	internalRoutes := make([]server.RouteInfo, 0)
	var staticEnabled bool
	var staticPrefix, staticDir string

	if appCfg.Proxy.Enabled {
		for _, pr := range appCfg.Proxy.Routes {
			rInfo := server.RouteInfo{
				Type:    pr.GetType(),
				Host:    pr.GetHost(),
				Prefix:  pr.Prefix,
				Headers: pr.Headers,
			}
			if pr.IsStatic() {
				rInfo.Dir = pr.GetDir()
				staticEnabled = true
				if staticPrefix == "" {
					staticPrefix = pr.Prefix
					staticDir = pr.GetDir()
				}
			} else {
				rInfo.Algorithm = pr.GetAlgorithm()
				rInfo.Targets = pr.GetTargets()
			}
			internalRoutes = append(internalRoutes, rInfo)
		}
	}
	if !staticEnabled && appCfg.Static.Enabled {
		staticEnabled = true
		staticPrefix = appCfg.Static.Prefix
		staticDir = appCfg.Static.Dir
	}

	internalCfg := server.InternalAPIConfig{
		Port:           appCfg.Server.Port,
		WorkerPoolSize: appCfg.Server.WorkerPoolSize,
		ProxyEnabled:   appCfg.Proxy.Enabled,
		Routes:         internalRoutes,
		StaticEnabled:  staticEnabled,
		StaticPrefix:   staticPrefix,
		StaticDir:      staticDir,
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

	// Register Routing Rules (Static site routes & Upstream reverse proxy routes) if enabled
	if appCfg.Proxy.Enabled {
		for _, pr := range appCfg.Proxy.Routes {
			host := pr.GetHost()
			if pr.IsStatic() {
				log.Printf("[TORON] Configuring Static Route: host %q, prefix %q (headers: %v) -> dir %q", host, pr.Prefix, pr.Headers, pr.GetDir())
				authCfg := router.AuthConfig{
					Type: router.AuthType(pr.Auth.Type),
					JWT: router.JWTConfig{
						Secret:   pr.Auth.JWT.Secret,
						Issuer:   pr.Auth.JWT.Issuer,
						Audience: pr.Auth.JWT.Audience,
					},
					APIKey: router.APIKeyConfig{
						Keys:   pr.Auth.APIKey.Keys,
						Header: pr.Auth.APIKey.Header,
						Query:  pr.Auth.APIKey.Query,
					},
					Basic: router.BasicAuthConfig{
						Users: pr.Auth.Basic.Users,
						Realm: pr.Auth.Basic.Realm,
					},
					Excluded: pr.Auth.Excluded,
				}
				if err := r.RoutePrefix(router.RouteTypeStatic, host, pr.Prefix, pr.Headers, pr.GetDir(), proxy.ProxyOptions{RateLimit: pr.RateLimit, Auth: authCfg}); err != nil {
					log.Fatalf("[TORON] Invalid static route configuration for prefix %q: %v", pr.Prefix, err)
				}
			} else if pr.IsTCP() {
				port := pr.GetListenPort()
				targets := pr.GetTargets()
				log.Printf("[TORON] Configuring Layer 4 TCP Stream Proxy: listen_port %d -> targets %v", port, targets)
				tcpProxy, err := proxy.NewTCPProxy(targets, 5*time.Second)
				if err != nil {
					log.Printf("[TORON] Failed to create TCP proxy for port %d: %v", port, err)
					continue
				}
				ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
				if err != nil {
					log.Printf("[TORON] Failed to listen TCP on port %d: %v", port, err)
					continue
				}
				go func(p *proxy.TCPProxy, l net.Listener) {
					_ = p.Serve(l)
				}(tcpProxy, ln)
			} else if pr.IsUDP() {
				port := pr.GetListenPort()
				targets := pr.GetTargets()
				log.Printf("[TORON] Configuring Layer 4 UDP Datagram Proxy: listen_port %d -> targets %v", port, targets)
				udpProxy, err := proxy.NewUDPProxy(targets, 5*time.Second)
				if err != nil {
					log.Printf("[TORON] Failed to create UDP proxy for port %d: %v", port, err)
					continue
				}
				addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
				if err != nil {
					log.Printf("[TORON] Failed to resolve UDP addr for port %d: %v", port, err)
					continue
				}
				conn, err := net.ListenUDP("udp", addr)
				if err != nil {
					log.Printf("[TORON] Failed to listen UDP on port %d: %v", port, err)
					continue
				}
				go func(p *proxy.UDPProxy, c *net.UDPConn) {
					_ = p.Serve(c)
				}(udpProxy, conn)
			} else {
				targets := pr.GetTargets()
				algo := pr.GetAlgorithm()
				log.Printf("[TORON] Configuring Reverse Proxy Route: host %q, prefix %q (headers: %v) -> targets %v [algo: %s, healthCheck: %q]", host, pr.Prefix, pr.Headers, targets, algo, pr.HealthCheckPath)
				authCfg := router.AuthConfig{
					Type: router.AuthType(pr.Auth.Type),
					JWT: router.JWTConfig{
						Secret:   pr.Auth.JWT.Secret,
						Issuer:   pr.Auth.JWT.Issuer,
						Audience: pr.Auth.JWT.Audience,
					},
					APIKey: router.APIKeyConfig{
						Keys:   pr.Auth.APIKey.Keys,
						Header: pr.Auth.APIKey.Header,
						Query:  pr.Auth.APIKey.Query,
					},
					Basic: router.BasicAuthConfig{
						Users: pr.Auth.Basic.Users,
						Realm: pr.Auth.Basic.Realm,
					},
					Excluded: pr.Auth.Excluded,
				}
				opts := proxy.ProxyOptions{
					Targets:             targets,
					Algorithm:           proxy.Algorithm(algo),
					Timeout:             10 * time.Second,
					HealthCheckPath:     pr.HealthCheckPath,
					HealthCheckInterval: pr.HealthCheckInterval,
					MaxFailures:         pr.ConsecutiveFailures,
					CooldownPeriod:      pr.CooldownPeriod,
					RateLimit:           pr.RateLimit,
					StickyCookieName:    pr.StickyCookieName,
					Auth:                authCfg,
				}
				if err := r.RoutePrefix(router.RouteTypeUpstream, host, pr.Prefix, pr.Headers, "", opts); err != nil {
					log.Fatalf("[TORON] Invalid proxy load balancer configuration for targets %v: %v", targets, err)
				}
			}
		}
	}

	// Legacy static asset fallback if configured and no static routes present
	if appCfg.Static.Enabled && !staticEnabled {
		log.Printf("[TORON] Serving static assets from %s under prefix %q...", appCfg.Static.Dir, appCfg.Static.Prefix)
		_ = r.RoutePrefix(router.RouteTypeStatic, "", appCfg.Static.Prefix, nil, appCfg.Static.Dir, proxy.ProxyOptions{})
	}

	srv := server.New(srvCfg, r)

	// Graceful shutdown context listener
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if appCfg.Server.TLS.Enabled {
			log.Printf("[TORON] HTTPS Server listening on https://localhost%s (TLS enabled)...", srvCfg.Addr)
			if err := srv.ListenAndServeTLS(appCfg.Server.TLS.CertFile, appCfg.Server.TLS.KeyFile); err != nil && err != server.ErrServerClosed {
				log.Fatalf("[TORON] HTTPS Server fatal error: %v", err)
			}
		} else {
			log.Printf("[TORON] HTTP Server listening on http://localhost%s...", srvCfg.Addr)
			if err := srv.ListenAndServe(); err != nil && err != server.ErrServerClosed {
				log.Fatalf("[TORON] HTTP Server fatal error: %v", err)
			}
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
