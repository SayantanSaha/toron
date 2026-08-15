package proxy

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/metrics"
)

// Algorithm defines the load balancing strategy name.
type Algorithm string

const (
	AlgorithmRoundRobin   Algorithm = "round_robin"
	AlgorithmRandom       Algorithm = "random"
	AlgorithmStickyCookie Algorithm = "sticky_cookie"
	AlgorithmIPHash       Algorithm = "ip_hash"
)

var (
	ErrNoTargetsAvailable = errors.New("proxy: no upstream targets available")
)

// LoadBalancer interface abstracts selecting an upstream target node for a request.
type LoadBalancer interface {
	Next(req *httpparser.Request) (*UpstreamTarget, error)
	Algorithm() Algorithm
	Targets() []*UpstreamTarget
	Stop()
}

// RoundRobinBalancer selects healthy upstream targets sequentially in a thread-safe circular order.
type RoundRobinBalancer struct {
	targets []*UpstreamTarget
	counter uint64
}

// NewRoundRobinBalancer creates a round-robin load balancer for target nodes.
func NewRoundRobinBalancer(targets []*UpstreamTarget) (*RoundRobinBalancer, error) {
	if len(targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}
	copied := make([]*UpstreamTarget, len(targets))
	copy(copied, targets)
	return &RoundRobinBalancer{
		targets: copied,
	}, nil
}

func (b *RoundRobinBalancer) Next(req *httpparser.Request) (*UpstreamTarget, error) {
	if len(b.targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}

	n := len(b.targets)
	startIdx := int(atomic.AddUint64(&b.counter, 1) - 1)

	// Round-robin search for healthy (Closed or HalfOpen) target
	for i := 0; i < n; i++ {
		target := b.targets[(startIdx+i)%n]
		state := target.GetState()
		if state == StateClosed || state == StateHalfOpen {
			return target, nil
		}
	}

	return nil, ErrNoHealthyUpstreamAvailable
}

func (b *RoundRobinBalancer) Algorithm() Algorithm {
	return AlgorithmRoundRobin
}

func (b *RoundRobinBalancer) Targets() []*UpstreamTarget {
	return b.targets
}

func (b *RoundRobinBalancer) Stop() {
	for _, t := range b.targets {
		t.StopActiveHealthCheck()
	}
}

// NewLoadBalancer constructs a LoadBalancer for given targets and algorithm.
func NewLoadBalancer(algo Algorithm, targets []*UpstreamTarget) (LoadBalancer, error) {
	return NewLoadBalancerWithOptions(algo, targets, "")
}

// NewLoadBalancerWithOptions constructs a LoadBalancer supporting sticky session options.
func NewLoadBalancerWithOptions(algo Algorithm, targets []*UpstreamTarget, cookieName string) (LoadBalancer, error) {
	if len(targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}

	normAlgo := Algorithm(strings.ToLower(strings.TrimSpace(string(algo))))
	switch normAlgo {
	case "", AlgorithmRoundRobin:
		return NewRoundRobinBalancer(targets)
	case AlgorithmStickyCookie:
		return NewStickyCookieBalancer(targets, cookieName)
	case AlgorithmIPHash:
		return NewIPHashBalancer(targets)
	default:
		return nil, fmt.Errorf("proxy: unsupported load balancing algorithm %q", algo)
	}
}

// ReverseProxy handles proxying HTTP requests to upstream target URL(s).
type ReverseProxy struct {
	TargetURL *url.URL     // Single primary target (for backward compatibility)
	Balancer  LoadBalancer // Load balancer interface for target selection
	Client    *http.Client
}

// NewReverseProxy creates a ReverseProxy instance for a single target URL string.
func NewReverseProxy(targetURLStr string, timeout time.Duration) (*ReverseProxy, error) {
	return NewLoadBalancerProxy([]string{targetURLStr}, AlgorithmRoundRobin, timeout)
}

// ProxyOptions configures advanced proxy, health check, and circuit breaker settings.
type ProxyOptions struct {
	Targets             []string
	Algorithm           Algorithm
	Timeout             time.Duration
	HealthCheckType     string
	HealthCheckPath     string
	HealthCheckService  string
	HealthCheckInterval time.Duration
	MaxFailures         int
	CooldownPeriod      time.Duration
	RateLimit           string
	StickyCookieName    string
	Auth                any
	WAF                 any
}

// NewLoadBalancerProxy creates a ReverseProxy instance that load balances requests across multiple target URL strings.
func NewLoadBalancerProxy(targetURLStrs []string, algo Algorithm, timeout time.Duration) (*ReverseProxy, error) {
	return NewProxyWithOptions(ProxyOptions{
		Targets:   targetURLStrs,
		Algorithm: algo,
		Timeout:   timeout,
	})
}

// NewProxyWithOptions creates a ReverseProxy using ProxyOptions.
func NewProxyWithOptions(opts ProxyOptions) (*ReverseProxy, error) {
	if len(opts.Targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}

	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	client := &http.Client{
		Timeout: opts.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	upstreamTargets := make([]*UpstreamTarget, 0, len(opts.Targets))
	for _, targetStr := range opts.Targets {
		parsedURL, err := url.Parse(targetStr)
		if err != nil {
			return nil, fmt.Errorf("proxy: invalid target URL %q: %w", targetStr, err)
		}

		targetNode := NewUpstreamTargetWithHealth(parsedURL, opts.HealthCheckType, opts.HealthCheckPath, opts.HealthCheckService, opts.MaxFailures, opts.CooldownPeriod)
		if opts.HealthCheckPath != "" || strings.ToLower(opts.HealthCheckType) == "grpc" {
			targetNode.StartActiveHealthCheck(client, opts.HealthCheckInterval)
		}
		upstreamTargets = append(upstreamTargets, targetNode)
	}

	lb, err := NewLoadBalancerWithOptions(opts.Algorithm, upstreamTargets, opts.StickyCookieName)
	if err != nil {
		return nil, err
	}

	return &ReverseProxy{
		TargetURL: upstreamTargets[0].URL,
		Balancer:  lb,
		Client:    client,
	}, nil
}

// Close stops active background health checks.
func (p *ReverseProxy) Close() {
	if p.Balancer != nil {
		p.Balancer.Stop()
	}
}

// ServeHTTP translates a Toron Request, proxies it to the upstream server, and writes the upstream response to Res.
func (p *ReverseProxy) ServeHTTP(req *httpparser.Request, res *httpparser.Response) {
	p.ServeHTTPWithPrefix(req, res, "")
}

// ServeHTTPWithPrefix proxies the request stripping an optional route prefix.
func (p *ReverseProxy) ServeHTTPWithPrefix(req *httpparser.Request, res *httpparser.Response, prefix string) {
	var targetNode *UpstreamTarget
	if p.Balancer != nil {
		selected, err := p.Balancer.Next(req)
		if err != nil {
			if errors.Is(err, ErrNoHealthyUpstreamAvailable) {
				p.writeServiceUnavailable(res, "503 Service Unavailable: All upstream targets are unhealthy or circuit open")
			} else {
				p.writeBadGateway(res, fmt.Sprintf("Load balancer error: %v", err))
			}
			return
		}
		targetNode = selected
	} else if p.TargetURL != nil {
		targetNode = NewUpstreamTarget(p.TargetURL, "", 3, 10*time.Second)
	}

	if targetNode == nil || targetNode.URL == nil {
		p.writeBadGateway(res, "No upstream target available")
		return
	}

	targetURL := targetNode.URL
	relPath := req.Path
	if prefix != "" {
		relPath = strings.TrimPrefix(req.Path, prefix)
	}
	if relPath == "" {
		relPath = "/"
	}
	if !strings.HasPrefix(relPath, "/") {
		relPath = "/" + relPath
	}

	outURL := *targetURL
	outURL.Path = singleJoiningSlash(targetURL.Path, relPath)
	outURL.RawQuery = req.QueryParams.Encode()

	if req.IsWebSocketUpgrade() {
		p.serveWebSocketProxy(req, res, targetNode, outURL, prefix)
		return
	}

	var bodyReader io.Reader
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil && len(bodyBytes) > 0 {
			bodyReader = bytes.NewReader(bodyBytes)
		}
	}

	outReq, err := http.NewRequest(req.Method, outURL.String(), bodyReader)
	if err != nil {
		p.writeBadGateway(res, fmt.Sprintf("Failed to construct proxy request: %v", err))
		return
	}

	// Copy original request headers
	for key, values := range req.Header {
		for _, val := range values {
			outReq.Header.Add(key, val)
		}
	}

	// Inject X-Forwarded-* headers
	outReq.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	outReq.Header.Set("X-Forwarded-Proto", "http")
	if clientIP := req.Header.Get("X-Real-IP"); clientIP != "" {
		outReq.Header.Set("X-Forwarded-For", clientIP)
	}

	// Propagate / Inject W3C traceparent header
	incomingTrace := req.Header.Get("traceparent")
	outReq.Header.Set("traceparent", metrics.EnsureW3CTraceparent(incomingTrace))

	// Dispatch request to upstream
	outResp, err := p.Client.Do(outReq)
	if err != nil {
		targetNode.RecordFailure()
		p.writeBadGateway(res, fmt.Sprintf("Upstream unreachable (%s): %v", targetURL.String(), err))
		return
	}
	defer outResp.Body.Close()

	if outResp.StatusCode >= 500 {
		targetNode.RecordFailure()
	} else {
		targetNode.RecordSuccess()
	}

	// Copy upstream status code
	res.SetStatus(outResp.StatusCode)

	// Copy upstream headers
	for key, values := range outResp.Header {
		for _, val := range values {
			res.Header.Add(key, val)
		}
	}

	// Inject sticky session cookie if sticky_cookie load balancer is active
	if stickyBalancer, ok := p.Balancer.(*StickyCookieBalancer); ok {
		cookieVal := TargetHash(targetNode.URL.String())
		res.Header.Set("Set-Cookie", fmt.Sprintf("%s=%s; Path=/; HttpOnly", stickyBalancer.CookieName(), cookieVal))
	}

	// Copy upstream body
	if outResp.Body != nil {
		_, _ = io.Copy(res.Body, outResp.Body)
	}

	// Copy upstream trailers after reading body (e.g. grpc-status, grpc-message)
	for key, values := range outResp.Trailer {
		for _, val := range values {
			res.Header.Add(key, val)
		}
	}
}

func (p *ReverseProxy) writeBadGateway(res *httpparser.Response, msg string) {
	res.SetStatus(http.StatusBadGateway)
	res.Header.Set("Content-Type", "application/json")
	res.Body.Reset()
	_, _ = res.WriteString(fmt.Sprintf(`{"error":"502 Bad Gateway","message":%q}`, msg))
}

func (p *ReverseProxy) writeServiceUnavailable(res *httpparser.Response, msg string) {
	res.SetStatus(http.StatusServiceUnavailable)
	res.Header.Set("Content-Type", "application/json")
	res.Body.Reset()
	_, _ = res.WriteString(fmt.Sprintf(`{"error":"503 Service Unavailable","message":%q}`, msg))
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func (p *ReverseProxy) serveWebSocketProxy(req *httpparser.Request, res *httpparser.Response, targetNode *UpstreamTarget, outURL url.URL, prefix string) {
	host := outURL.Host
	if !strings.Contains(host, ":") {
		if outURL.Scheme == "https" || outURL.Scheme == "wss" {
			host = host + ":443"
		} else {
			host = host + ":80"
		}
	}

	var upstreamConn net.Conn
	var dialErr error
	if outURL.Scheme == "https" || outURL.Scheme == "wss" {
		upstreamConn, dialErr = tls.Dial("tcp", host, &tls.Config{InsecureSkipVerify: true})
	} else {
		timeout := p.Client.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		upstreamConn, dialErr = net.DialTimeout("tcp", host, timeout)
	}

	if dialErr != nil {
		targetNode.RecordFailure()
		p.writeBadGateway(res, fmt.Sprintf("Upstream WebSocket target unreachable (%s): %v", outURL.String(), dialErr))
		return
	}

	reqURI := outURL.RequestURI()
	if reqURI == "" {
		reqURI = "/"
	}

	var reqBuf bytes.Buffer
	fmt.Fprintf(&reqBuf, "%s %s HTTP/1.1\r\n", req.Method, reqURI)
	for k, vv := range req.Header {
		for _, v := range vv {
			fmt.Fprintf(&reqBuf, "%s: %s\r\n", k, v)
		}
	}
	if req.Header.Get("Host") == "" {
		fmt.Fprintf(&reqBuf, "Host: %s\r\n", outURL.Host)
	}
	reqBuf.WriteString("\r\n")

	if _, err := upstreamConn.Write(reqBuf.Bytes()); err != nil {
		upstreamConn.Close()
		targetNode.RecordFailure()
		p.writeBadGateway(res, fmt.Sprintf("Failed to write WebSocket handshake to upstream: %v", err))
		return
	}

	reader := bufio.NewReader(upstreamConn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		upstreamConn.Close()
		targetNode.RecordFailure()
		p.writeBadGateway(res, fmt.Sprintf("Failed to read WebSocket upgrade response from upstream: %v", err))
		return
	}

	if !strings.Contains(statusLine, "101") {
		upstreamConn.Close()
		targetNode.RecordFailure()
		p.writeBadGateway(res, fmt.Sprintf("Upstream rejected WebSocket upgrade: %s", strings.TrimSpace(statusLine)))
		return
	}

	res.SetStatus(http.StatusSwitchingProtocols)
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" || line == "\n" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			res.Header.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}

	targetNode.RecordSuccess()

	if reader.Buffered() > 0 {
		bufBytes := make([]byte, reader.Buffered())
		_, _ = reader.Read(bufBytes)
		res.UpgradedConn = &proxyPrefixConn{Conn: upstreamConn, prefix: bufBytes}
	} else {
		res.UpgradedConn = upstreamConn
	}
}

type proxyPrefixConn struct {
	net.Conn
	prefix []byte
}

func (c *proxyPrefixConn) Read(b []byte) (n int, err error) {
	if len(c.prefix) > 0 {
		n = copy(b, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(b)
}
