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
	"net/url"
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
	proxies         map[string]*proxy.ReverseProxy // origin -> *proxy.ReverseProxy
	clientTLS       *tls.Config
	ingressServer   *http.Server
	egressServer    *http.Server
	ingressListener net.Listener
	egressListener  net.Listener
	cancel          context.CancelFunc
	running         bool
}

// NewProxyEngine constructs a Service Mesh Sidecar ProxyEngine instance.
func NewProxyEngine(cfg config.SidecarConfig, routers ...*router.Router) (*ProxyEngine, error) {
	var r *router.Router
	if len(routers) > 0 {
		r = routers[0]
	}

	if cfg.IngressPort <= 0 {
		cfg.IngressPort = 15006
	}
	if cfg.EgressPort <= 0 {
		cfg.EgressPort = 15001
	}
	if cfg.AppPort <= 0 {
		cfg.AppPort = 8080
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 10 * 1024 * 1024
	}

	clientTLS, err := BuildClientTLSConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("sidecar client tls config failed: %w", err)
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
		proxies:   make(map[string]*proxy.ReverseProxy),
		clientTLS: clientTLS,
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

// normalizeTargetOrigin normalizes a raw target URL to a canonical origin scheme://host[:port],
// stripping any paths, query strings, and fragments, and converting scheme and host to lowercase.
func normalizeTargetOrigin(rawURL string) (string, error) {
	targetURL := strings.TrimSpace(rawURL)
	if targetURL == "" {
		return "", fmt.Errorf("empty target URL")
	}

	lower := strings.ToLower(targetURL)
	if strings.HasPrefix(targetURL, "//") {
		targetURL = "http:" + targetURL
	} else if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		targetURL = "http://" + targetURL
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("invalid target URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)

	if host == "" {
		return "", fmt.Errorf("missing host in target URL: %s", rawURL)
	}

	return fmt.Sprintf("%s://%s", scheme, host), nil
}

// getOrCreateProxy retrieves an existing ReverseProxy for the canonical origin,
// or instantiates and caches a new one using thread-safe double-checked locking.
func (p *ProxyEngine) getOrCreateProxy(targetURL string) (*proxy.ReverseProxy, error) {
	origin, err := normalizeTargetOrigin(targetURL)
	if err != nil {
		return nil, err
	}

	// Fast path: read lock
	p.mu.RLock()
	px, ok := p.proxies[origin]
	p.mu.RUnlock()
	if ok && px != nil {
		return px, nil
	}

	// Slow path: write lock
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-checked locking
	if px, ok = p.proxies[origin]; ok && px != nil {
		return px, nil
	}

	var clientTLS *tls.Config
	if p.clientTLS != nil {
		clientTLS = p.clientTLS.Clone()
	}
	opts := proxy.ProxyOptions{
		Targets:         []string{origin},
		TLSClientConfig: clientTLS,
	}

	newPx, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		return nil, err
	}

	if p.proxies == nil {
		p.proxies = make(map[string]*proxy.ReverseProxy)
	}
	p.proxies[origin] = newPx
	return newPx, nil
}

func (p *ProxyEngine) proxyToURL(w http.ResponseWriter, r *http.Request, targetURL string) {
	px, err := p.getOrCreateProxy(targetURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Sidecar proxy error: %v", err), http.StatusBadGateway)
		return
	}

	// Adapt stdlib http.Request to Toron Request
	toronReq := httpparser.NewRequestFromStd(r)
	if targetParsed, err := url.Parse(targetURL); err == nil {
		if (toronReq.Path == "" || toronReq.Path == "/") && targetParsed.Path != "" && targetParsed.Path != "/" {
			toronReq.Path = targetParsed.Path
		}
		if toronReq.URL != nil && toronReq.URL.RawQuery == "" && targetParsed.RawQuery != "" {
			toronReq.URL.RawQuery = targetParsed.RawQuery
		}
	}

	maxBody := p.cfg.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = 10 * 1024 * 1024
	}

	if r.Body != nil && r.Method != "GET" && r.Method != "HEAD" {
		defer r.Body.Close()

		// a. Fast-fail check: If r.ContentLength > maxBody && r.ContentLength > 0
		if r.ContentLength > maxBody && r.ContentLength > 0 {
			log.Printf("[SIDECAR] 413 Payload Too Large: %s %s declared Content-Length %d exceeds limit %d", r.Method, r.URL.Path, r.ContentLength, maxBody)
			http.Error(w, fmt.Sprintf("Payload Too Large: request Content-Length %d exceeds limit of %d bytes", r.ContentLength, maxBody), http.StatusRequestEntityTooLarge)
			r.Body.Close()
			return
		}

		if r.ContentLength == 0 {
			toronReq.ContentLength = 0
		} else {
			// b. Stream over-read bounded ingestion
			bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
			if err != nil {
				http.Error(w, fmt.Sprintf("Bad Request: %v", err), http.StatusBadRequest)
				return
			}
			if int64(len(bodyBytes)) > maxBody {
				log.Printf("[SIDECAR] 413 Payload Too Large: %s %s request body exceeds limit %d", r.Method, r.URL.Path, maxBody)
				http.Error(w, fmt.Sprintf("Payload Too Large: request body exceeds limit of %d bytes", maxBody), http.StatusRequestEntityTooLarge)
				return
			}
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
	if toronRes.StreamBody != nil {
		defer toronRes.StreamBody.Close()
		_, _ = io.Copy(w, toronRes.StreamBody)
	} else {
		_, _ = w.Write(toronRes.Body.Bytes())
	}
}

// Stop terminates active sidecar proxy servers and cleans up all cached reverse proxies.
func (p *ProxyEngine) Stop() {
	p.mu.Lock()
	wasRunning := p.running
	p.running = false
	if p.cancel != nil {
		p.cancel()
	}

	for _, px := range p.proxies {
		if px != nil {
			px.Close()
		}
	}
	p.proxies = make(map[string]*proxy.ReverseProxy)
	p.mu.Unlock()

	if !wasRunning {
		return
	}

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
