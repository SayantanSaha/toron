package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"toron/pkg/config"
	"toron/pkg/discovery"
	"toron/pkg/httpparser"
	"toron/pkg/ingress"
	"toron/pkg/logging"
	"toron/pkg/proxy"
	"toron/pkg/router"
	"toron/pkg/server"
	"toron/pkg/sidecar"
	"toron/pkg/transcoder"
	"toron/pkg/version"
	"toron/pkg/waf"
)

func main() {
	var configPath string
	var routesPath string
	var testConfig bool
	var showVersion bool
	flag.StringVar(&configPath, "config", "", "Path to YAML server configuration file (e.g. -config config.yaml)")
	flag.StringVar(&configPath, "c", "", "Path to YAML server configuration file (short alias)")
	flag.StringVar(&routesPath, "routes", "", "Path to YAML proxy routing configuration file (e.g. -routes routes.yaml)")
	flag.StringVar(&routesPath, "r", "", "Path to YAML proxy routing configuration file (short alias)")
	flag.BoolVar(&testConfig, "test-config", false, "Test configuration files syntax and exit without running server")
	flag.BoolVar(&testConfig, "t", false, "Test configuration files syntax and exit (short alias)")
	flag.BoolVar(&showVersion, "version", false, "Print Toron version and build information and exit")
	flag.BoolVar(&showVersion, "v", false, "Print Toron version (short alias)")
	flag.Parse()

	if showVersion {
		fmt.Println(version.Full())
		os.Exit(0)
	}

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

	// Initialize Multi-Stream Logging Manager
	logMgr, err := logging.NewLogManager(logging.Config{
		Level:       appCfg.Logging.Level,
		Format:      appCfg.Logging.Format,
		ServerLog:   appCfg.Logging.ServerLog,
		AccessLog:   appCfg.Logging.AccessLog,
		SecurityLog: appCfg.Logging.SecurityLog,
	})
	if err != nil {
		log.Printf("[TORON] Warning: Failed to initialize LogManager: %v", err)
	} else {
		defer logMgr.Close()
		if logMgr.ServerSink() != nil {
			log.SetOutput(logMgr.ServerSink())
			log.Printf("[TORON] Server logs redirected to %s", appCfg.Logging.ServerLog)
		}
	}

	srvCfg := appCfg.ToServerConfig()
	r := router.New()

	// Attach Middlewares
	if logMgr != nil {
		r.Use(router.AccessLoggerMiddleware(logMgr, r))
	} else {
		r.Use(router.LoggerMiddleware())
	}
	r.Use(router.RecoveryMiddleware())
	var globalWafEngine *waf.WAFEngine
	if appCfg.Server.WAF.Enabled {
		globalWafCfg := appCfg.Server.WAF
		if appCfg.Logging.SecurityLog != "" && appCfg.Logging.SecurityLog != "stdout" && globalWafCfg.AuditLog.Output == "stdout" {
			globalWafCfg.AuditLog.Output = appCfg.Logging.SecurityLog
		}
		if appCfg.Proxy.Enabled {
			for _, pr := range appCfg.Proxy.Routes {
				if pr.WAF.Enabled || len(pr.WAF.AllowedIPs) > 0 || len(pr.WAF.DeniedIPs) > 0 || len(pr.WAF.DisabledRules) > 0 || len(pr.WAF.CustomRules) > 0 || pr.WAF.Mode != "" {
					if pr.Prefix != "" {
						globalWafCfg.Excluded = append(globalWafCfg.Excluded, pr.Prefix)
					}
				}
			}
		}
		if engine, wafErr := waf.NewEngine(globalWafCfg); wafErr == nil {
			globalWafEngine = engine
			if logMgr != nil && logMgr.SecuritySink() != nil {
				globalWafEngine.SetAuditLogger(waf.NewAuditLoggerWithWriter(logMgr.SecuritySink()))
			}
			wafMw := waf.NewWAFMiddleware(globalWafEngine)
			r.Use(func(next router.HandlerFunc) router.HandlerFunc {
				return func(req *httpparser.Request, res *httpparser.Response) {
					wafMw(waf.HandlerFunc(next))(req, res)
				}
			})
		}
	}
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
	if appCfg.Server.CORS.Enabled {
		r.Use(router.NewCORSMiddleware(router.CORSConfig{
			Enabled:          appCfg.Server.CORS.Enabled,
			AllowOrigins:     appCfg.Server.CORS.AllowOrigins,
			AllowMethods:     appCfg.Server.CORS.AllowMethods,
			AllowHeaders:     appCfg.Server.CORS.AllowHeaders,
			ExposeHeaders:    appCfg.Server.CORS.ExposeHeaders,
			AllowCredentials: appCfg.Server.CORS.AllowCredentials,
			MaxAge:           appCfg.Server.CORS.MaxAge,
		}))
	}
	if appCfg.Server.SecurityHeaders.Enabled {
		r.Use(router.NewSecurityHeadersMiddleware(router.SecurityHeadersConfig{
			Enabled:            appCfg.Server.SecurityHeaders.Enabled,
			HSTS:               appCfg.Server.SecurityHeaders.HSTS,
			ContentTypeOptions: appCfg.Server.SecurityHeaders.ContentTypeOptions,
			FrameOptions:       appCfg.Server.SecurityHeaders.FrameOptions,
			ReferrerPolicy:     appCfg.Server.SecurityHeaders.ReferrerPolicy,
			CSP:                appCfg.Server.SecurityHeaders.CSP,
			PermissionsPolicy:  appCfg.Server.SecurityHeaders.PermissionsPolicy,
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

	wafMode := "enforce"
	if appCfg.Server.WAF.Mode != "" {
		wafMode = appCfg.Server.WAF.Mode
	}
	wafRulesCount := 10
	if globalWafEngine != nil {
		wafRulesCount = len(globalWafEngine.Rules())
	}
	var auditLogger *waf.AuditLogger
	if globalWafEngine != nil {
		auditLogger = globalWafEngine.AuditLogger()
	}

	// Initialize OCI Container Auto-Discovery Engine if enabled
	var discMgr *discovery.Manager
	if appCfg.Discovery.Enabled {
		discMgr = discovery.NewManager(appCfg.Discovery, r)
		if err := discMgr.Start(context.Background()); err != nil {
			log.Printf("[TORON] Failed to start OCI Container Auto-Discovery: %v", err)
		} else {
			defer discMgr.Stop()
		}
	}

	internalCfg := server.InternalAPIConfig{
		Port:                   appCfg.Server.Port,
		WorkerPoolSize:         appCfg.Server.WorkerPoolSize,
		ProxyEnabled:           appCfg.Proxy.Enabled,
		Routes:                 internalRoutes,
		StaticEnabled:          staticEnabled,
		StaticPrefix:           staticPrefix,
		StaticDir:              staticDir,
		WAFEnabled:             appCfg.Server.WAF.Enabled,
		WAFMode:                wafMode,
		WAFAnomalyThreshold:    appCfg.Server.WAF.AnomalyThreshold,
		WAFRulesCount:          wafRulesCount,
		WAFCustomRulesCount:    len(appCfg.Server.WAF.CustomRules),
		WAFAllowedIPs:          appCfg.Server.WAF.AllowedIPs,
		WAFDeniedIPs:           appCfg.Server.WAF.DeniedIPs,
		CORSEnabled:            appCfg.Server.CORS.Enabled,
		CORSAllowedOrigins:     appCfg.Server.CORS.AllowOrigins,
		SecurityHeadersEnabled: appCfg.Server.SecurityHeaders.Enabled,
		MTLSEnabled:            appCfg.Server.TLS.Enabled,
		DiscoveryEnabled:       appCfg.Discovery.Enabled,
		DiscoveryFunc: func() []server.RouteInfo {
			if discMgr == nil {
				return nil
			}
			active := discMgr.ActiveRoutes()
			res := make([]server.RouteInfo, 0, len(active))
			for _, dr := range active {
				res = append(res, server.RouteInfo{
					Type:          "upstream",
					Host:          dr.Host,
					Prefix:        dr.Prefix,
					Targets:       []string{dr.TargetURL()},
					Source:        "oci",
					ContainerName: dr.ContainerName,
				})
			}
			return res
		},
		AuditLogger: auditLogger,
	}
	server.RegisterInternalAPIRoutes(r, internalCfg)

	// Register API Routes
	r.GET("/health", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"status":"ok"}`)
	})

	r.GET("/api/status", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(fmt.Sprintf(`{"server":"Toron","version":%q,"uptime":"healthy","engine":"event-driven"}`, version.Get()))
	})

	// Initialize SNIRegistry for dynamic per-host TLS & mTLS dispatching
	sniRegistry := server.NewSNIRegistry(nil)
	if appCfg.Proxy.Enabled {
		for _, pr := range appCfg.Proxy.Routes {
			if pr.TLS.CertFile != "" && pr.GetHost() != "" {
				if err := sniRegistry.RegisterRouteTLS(pr.GetHost(), server.RouteTLSConfig{
					CertFile:   pr.TLS.CertFile,
					KeyFile:    pr.TLS.KeyFile,
					CAFile:     pr.TLS.CAFile,
					ClientAuth: pr.TLS.ClientAuth,
					MinVersion: pr.TLS.MinVersion,
				}); err != nil {
					log.Printf("[TORON] Warning: failed to register TLS profile for host %q: %v", pr.GetHost(), err)
				} else {
					log.Printf("[TORON] Registered per-host TLS profile for host %q (client_auth: %s, min_version: %s)", pr.GetHost(), pr.TLS.ClientAuth, pr.TLS.MinVersion)
				}
			}
		}
	}
	srvCfg.SNIRegistry = sniRegistry

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
				if err := r.RoutePrefix(router.RouteTypeStatic, host, pr.Prefix, pr.Headers, pr.GetDir(), proxy.ProxyOptions{
					RateLimit:    pr.RateLimit,
					Auth:         authCfg,
					WAF:          pr.WAF,
					SPA:          pr.SPA,
					Fallback:     pr.Fallback,
					RedirectHTTP: pr.GetRedirectHTTP(),
					AccessLog:    pr.AccessLog,
					SecurityLog:  pr.SecurityLog,
				}); err != nil {
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
					HealthCheckType:     pr.HealthCheckType,
					HealthCheckPath:     pr.HealthCheckPath,
					HealthCheckService:  pr.HealthCheckService,
					HealthCheckInterval: pr.HealthCheckInterval,
					MaxFailures:         pr.ConsecutiveFailures,
					CooldownPeriod:      pr.CooldownPeriod,
					RateLimit:           pr.RateLimit,
					StickyCookieName:    pr.StickyCookieName,
					StripPrefix:         pr.StripPrefix,
					RewriteRedirects:    pr.RewriteRedirects,
					RewriteCookiePath:   pr.RewriteCookiePath,
					Auth:                authCfg,
					WAF:                 pr.WAF,
					RedirectHTTP:        pr.GetRedirectHTTP(),
					AccessLog:           pr.AccessLog,
					SecurityLog:         pr.SecurityLog,
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

	// Initialize Kubernetes Ingress Controller Engine if enabled
	if appCfg.Ingress.Enabled {
		ingCtrl, err := ingress.NewController(appCfg.Ingress, r)
		if err != nil {
			log.Printf("[TORON] K8s Ingress Controller initialization warning: %v", err)
		} else {
			if err := ingCtrl.Start(context.Background()); err != nil {
				log.Printf("[TORON] Failed to start K8s Ingress Controller: %v", err)
			}
			defer ingCtrl.Stop()
		}
	}

	// Initialize Service Mesh Sidecar Mode Engine if enabled
	if appCfg.Sidecar.Enabled {
		sidecarEng, err := sidecar.NewProxyEngine(appCfg.Sidecar, r)
		if err != nil {
			log.Printf("[TORON] Service Mesh Sidecar initialization warning: %v", err)
		} else {
			if err := sidecarEng.Start(context.Background()); err != nil {
				log.Printf("[TORON] Failed to start Service Mesh Sidecar: %v", err)
			}
			defer sidecarEng.Stop()
		}
	}

	// Initialize REST-to-gRPC Transcoding Engine if enabled
	if appCfg.Transcoder.Enabled {
		_, err := transcoder.NewEngine(appCfg.Transcoder, r)
		if err != nil {
			log.Printf("[TORON] REST-to-gRPC Transcoder initialization warning: %v", err)
		}
	}

	// Initialize ConfigWatcher for zero-downtime server and WAF hot reloading
	var configWatcher *config.ConfigWatcher
	if configPath != "" {
		cw, err := config.NewConfigWatcher(configPath, routesPath, func(newCfg *config.AppConfig) {
			if globalWafEngine != nil && newCfg.Server.WAF.Enabled {
				globalWafCfg := newCfg.Server.WAF
				if newCfg.Proxy.Enabled {
					for _, pr := range newCfg.Proxy.Routes {
						if pr.WAF.Enabled || len(pr.WAF.AllowedIPs) > 0 || len(pr.WAF.DeniedIPs) > 0 || len(pr.WAF.DisabledRules) > 0 || len(pr.WAF.CustomRules) > 0 || pr.WAF.Mode != "" {
							if pr.Prefix != "" {
								globalWafCfg.Excluded = append(globalWafCfg.Excluded, pr.Prefix)
							}
						}
					}
				}
				if reloadErr := globalWafEngine.Reload(globalWafCfg); reloadErr != nil {
					log.Printf("[TORON] Hot reload error updating WAF engine: %v", reloadErr)
				} else {
					log.Printf("[TORON] Hot reloaded WAF engine successfully (%d custom rules, mode=%s)", len(globalWafCfg.CustomRules), globalWafCfg.Mode)
				}
			}
		})
		if err != nil {
			log.Printf("[TORON] Warning: Failed to start ConfigWatcher for %s: %v", configPath, err)
		} else {
			configWatcher = cw
			log.Printf("[TORON] Started ConfigWatcher for zero-downtime hot reloading on %s", configPath)
		}
	}

	srv := server.New(srvCfg, r)

	// Graceful shutdown context listener
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Listen for SIGHUP signal to trigger atomic log file reopening (system logrotate daily rotation)
	hupChan := make(chan os.Signal, 1)
	signal.Notify(hupChan, syscall.SIGHUP)
	go func() {
		for range hupChan {
			log.Println("[TORON] SIGHUP signal received: reopening all active log files for system logrotate...")
			if logMgr != nil {
				if err := logMgr.Reopen(); err != nil {
					log.Printf("[TORON] Error reopening log files on SIGHUP: %v", err)
				} else {
					log.Println("[TORON] Successfully reopened all log files.")
				}
			}
		}
	}()

	var redirectSrv *server.Server
	if appCfg.Server.TLS.Enabled && appCfg.Server.HTTPRedirect.Enabled {
		redirPort := appCfg.Server.HTTPRedirect.Port
		if redirPort <= 0 {
			redirPort = 80
		}
		redirCfg := srvCfg
		redirCfg.Addr = fmt.Sprintf(":%d", redirPort)
		redirCfg.TLSEnabled = false
		redirCfg.HTTP3Enabled = false
		redirCfg.HTTPRedirectEnabled = true
		redirCfg.HTTPRedirectPort = redirPort
		redirCfg.HTTPSPort = appCfg.Server.Port
		if redirCfg.HTTPSPort <= 0 {
			redirCfg.HTTPSPort = 443
		}

		redirectSrv = server.New(redirCfg, r)
		go func() {
			log.Printf("[TORON] HTTP Redirect Server listening on http://localhost:%d (upgrading cleartext to HTTPS :%d)...", redirPort, redirCfg.HTTPSPort)
			if err := redirectSrv.ListenAndServe(); err != nil && err != server.ErrServerClosed {
				log.Printf("[TORON] HTTP Redirect Server error: %v", err)
			}
		}()
	}

	go func() {
		if appCfg.Server.TLS.Enabled {
			if appCfg.Server.HTTP3.Enabled {
				h3Port := appCfg.Server.HTTP3.Port
				if h3Port <= 0 {
					h3Port = 8443
				}
				go func() {
					log.Printf("[TORON] HTTP/3 QUIC Server listening on UDP :%d...", h3Port)
					if err := srv.ListenAndServeH3(appCfg.Server.TLS.CertFile, appCfg.Server.TLS.KeyFile); err != nil && err != server.ErrServerClosed && !errors.Is(err, server.ErrServerClosed) {
						log.Printf("[TORON] HTTP/3 QUIC Server error: %v", err)
					}
				}()
			}

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

	if redirectSrv != nil {
		_ = redirectSrv.Shutdown(stopCtx)
	}

	if configWatcher != nil {
		_ = configWatcher.Stop()
	}

	if err := srv.Shutdown(stopCtx); err != nil {
		log.Printf("[TORON] Error during graceful shutdown: %v", err)
	} else {
		log.Println("[TORON] Server stopped cleanly.")
	}
}
