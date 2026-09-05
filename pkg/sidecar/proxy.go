package sidecar

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"toron/pkg/config"
	"toron/pkg/httpparser"
	"toron/pkg/proxy"
	"toron/pkg/router"
)

// ProxyEngine manages sidecar proxy listeners and weighted traffic splitters.
type ProxyEngine struct {
	mu              sync.RWMutex
	cfg             config.SidecarConfig
	router          *router.Router
	splitters       map[string]*WeightedSplitter // prefix -> splitter
	ingressServer   *http.Server
	egressServer    *http.Server
	ingressListener net.Listener
	egressListener  net.Listener
	cancel          context.CancelFunc
	running         bool
}

// NewProxyEngine constructs a Service Mesh Sidecar ProxyEngine instance.
func NewProxyEngine(cfg config.SidecarConfig, r *router.Router) (*ProxyEngine, error) {
	if cfg.IngressPort <= 0 {
		cfg.IngressPort = 15006
	}
	if cfg.EgressPort <= 0 {
		cfg.EgressPort = 15001
	}
	if cfg.AppPort <= 0 {
		cfg.AppPort = 8080
	}

	splitters := make(map[string]*WeightedSplitter)
	for _, route := range cfg.TrafficSplits {
		var targets []SplitTarget
		for _, b := range route.Backends {
			targets = append(targets, SplitTarget{URL: b.Target, Weight: b.Weight})
		}
		if len(targets) > 0 {
			sp, err := NewWeightedSplitter(targets)
			if err == nil {
				prefix := "/" + strings.Trim(route.Prefix, "/")
				if prefix == "/" {
					prefix = ""
				}
				splitters[prefix] = sp
			}
		}
	}

	return &ProxyEngine{
		cfg:       cfg,
		router:    r,
		splitters: splitters,
	}, nil
}

// Start begins ingress and egress sidecar proxy listeners.
func (p *ProxyEngine) Start(parentCtx context.Context) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(parentCtx)
	p.cancel = cancel
	p.running = true
	p.mu.Unlock()

	mode := strings.ToLower(p.cfg.Mode)
	if mode == "" {
		mode = "dual"
	}

	// Start Ingress Proxy Listener (Pod Inbound Traffic)
	if mode == "ingress" || mode == "dual" {
		if err := p.startIngressListener(ctx); err != nil {
			return err
		}
	}

	// Start Egress Proxy Listener (Pod Outbound Traffic)
	if mode == "egress" || mode == "dual" {
		if err := p.startEgressListener(ctx); err != nil {
			return err
		}
	}

	log.Printf("[SIDECAR] Toron Service Mesh Sidecar running (Mode: %s, IngressPort: %d, EgressPort: %d, AppPort: %d)",
		mode, p.cfg.IngressPort, p.cfg.EgressPort, p.cfg.AppPort)
	return nil
}

func (p *ProxyEngine) startIngressListener(ctx context.Context) error {
	addr := fmt.Sprintf("0.0.0.0:%d", p.cfg.IngressPort)
	serverTLS, err := BuildServerTLSConfig(p.cfg)
	if err != nil {
		return err
	}

	var l net.Listener
	if serverTLS != nil {
		var err error
		l, err = tls.Listen("tcp", addr, serverTLS)
		if err != nil {
			return fmt.Errorf("sidecar ingress mTLS listen failed on %s: %w", addr, err)
		}
	} else {
		var err error
		l, err = net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("sidecar ingress listen failed on %s: %w", addr, err)
		}
	}

	p.ingressListener = l
	p.ingressServer = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Decrypt mTLS and forward plain HTTP request to local app container (127.0.0.1:AppPort)
			appURL := fmt.Sprintf("http://127.0.0.1:%d", p.cfg.AppPort)
			p.proxyToURL(w, r, appURL)
		}),
	}

	go func() {
		if err := p.ingressServer.Serve(l); err != nil && err != http.ErrServerClosed {
			log.Printf("[SIDECAR] Ingress proxy serve error: %v", err)
		}
	}()

	return nil
}

func (p *ProxyEngine) startEgressListener(ctx context.Context) error {
	addr := fmt.Sprintf("0.0.0.0:%d", p.cfg.EgressPort)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("sidecar egress listen failed on %s: %w", addr, err)
	}

	p.egressListener = l
	p.egressServer = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Select target URL using weighted traffic splitter
			targetURL := ""
			p.mu.RLock()
			for prefix, sp := range p.splitters {
				if prefix == "" || strings.HasPrefix(r.URL.Path, prefix) {
					targetURL = sp.Select()
					break
				}
			}
			p.mu.RUnlock()

			if targetURL == "" {
				// Default to request Host/URL
				targetURL = r.URL.String()
			}

			p.proxyToURL(w, r, targetURL)
		}),
	}

	go func() {
		if err := p.egressServer.Serve(l); err != nil && err != http.ErrServerClosed {
			log.Printf("[SIDECAR] Egress proxy serve error: %v", err)
		}
	}()

	return nil
}

func (p *ProxyEngine) proxyToURL(w http.ResponseWriter, r *http.Request, targetURL string) {
	opts := proxy.ProxyOptions{
		Targets: []string{targetURL},
	}

	px, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		http.Error(w, fmt.Sprintf("Sidecar proxy error: %v", err), http.StatusBadGateway)
		return
	}

	// Adapt stdlib http.Request to Toron Request
	toronReq := httpparser.NewRequestFromStd(r)
	if r.Body != nil && r.Method != "GET" && r.Method != "HEAD" {
		defer r.Body.Close()
		const maxSidecarBody = 10 * 1024 * 1024 // 10 MB limit
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxSidecarBody+1))
		if err == nil {
			toronReq.Body = bytes.NewReader(bodyBytes)
			toronReq.ContentLength = int64(len(bodyBytes))
		}
	}
	toronRes := httpparser.NewResponse()

	px.ServeHTTP(toronReq, toronRes)

	// Write back to stdlib ResponseWriter
	for k, vv := range toronRes.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(toronRes.StatusCode)
	_, _ = w.Write(toronRes.Body.Bytes())
}

// Stop terminates active sidecar proxy servers.
func (p *ProxyEngine) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	if p.cancel != nil {
		p.cancel()
	}
	p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if p.ingressServer != nil {
		_ = p.ingressServer.Shutdown(ctx)
	}
	if p.egressServer != nil {
		_ = p.egressServer.Shutdown(ctx)
	}
	log.Printf("[SIDECAR] Toron Service Mesh Sidecar stopped")
}
