package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/metrics"
)

// Algorithm defines the load balancing strategy name.
type Algorithm string

const (
	AlgorithmRoundRobin         Algorithm = "round_robin"
	AlgorithmRandom             Algorithm = "random"
	AlgorithmStickyCookie       Algorithm = "sticky_cookie"
	AlgorithmIPHash             Algorithm = "ip_hash"
	AlgorithmWeightedRoundRobin Algorithm = "weighted_round_robin"
	AlgorithmWeightedRandom     Algorithm = "weighted_random"
	AlgorithmLeastConn          Algorithm = "least_conn"
	AlgorithmWeightedLeastConn  Algorithm = "weighted_least_conn"
	AlgorithmLeastLatency       Algorithm = "least_latency"
)

// UpstreamContextKeyType is the private type for proxy context keys.
type UpstreamContextKeyType string

// UpstreamTargetContextKey is the context key for the selected upstream target socket address.
const UpstreamTargetContextKey UpstreamContextKeyType = "toron.upstream_target"

// GetUpstreamTarget retrieves the resolved upstream target host:port from ctx.
func GetUpstreamTarget(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(UpstreamTargetContextKey).(string); ok {
		return v
	}
	return ""
}

// SetUpstreamTarget stores the selected upstream target host:port in the request context.
func SetUpstreamTarget(req *httpparser.Request, target string) {
	if req != nil {
		req.SetContext(context.WithValue(req.Context(), UpstreamTargetContextKey, target))
	}
}

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

// 1. WeightedRoundRobinBalancer implements Nginx smooth weighted round-robin.
type WeightedRoundRobinBalancer struct {
	targets []*UpstreamTarget
	mu      sync.Mutex
}

func cloneUpstreamTarget(t *UpstreamTarget) *UpstreamTarget {
	if t == nil {
		return nil
	}
	return &UpstreamTarget{
		URL:                 t.URL,
		HealthCheckType:     t.HealthCheckType,
		HealthCheckPath:     t.HealthCheckPath,
		HealthCheckService:  t.HealthCheckService,
		HealthCheckInterval: t.HealthCheckInterval,
		ConsecutiveFailures: atomic.LoadInt32(&t.ConsecutiveFailures),
		MaxFailures:         t.MaxFailures,
		State:               t.GetState(),
		LastStateChange:     t.LastStateChange,
		CooldownPeriod:      t.CooldownPeriod,
		Weight:              t.Weight,
		EffectiveWeight:     t.EffectiveWeight,
		CurrentWeight:       t.CurrentWeight,
		ActiveConns:         atomic.LoadInt64(&t.ActiveConns),
		AvgLatencyUS:        atomic.LoadInt64(&t.AvgLatencyUS),
	}
}

func NewWeightedRoundRobinBalancer(targets []*UpstreamTarget) (*WeightedRoundRobinBalancer, error) {
	if len(targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}
	copied := make([]*UpstreamTarget, len(targets))
	for i, t := range targets {
		cp := cloneUpstreamTarget(t)
		if cp.Weight <= 0 {
			cp.Weight = 1
		}
		cp.EffectiveWeight = cp.Weight
		cp.CurrentWeight = 0
		copied[i] = cp
	}
	return &WeightedRoundRobinBalancer{targets: copied}, nil
}

func (b *WeightedRoundRobinBalancer) Next(req *httpparser.Request) (*UpstreamTarget, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}

	var best *UpstreamTarget
	totalWeight := 0

	for _, t := range b.targets {
		state := t.GetState()
		if state != StateClosed && state != StateHalfOpen {
			continue
		}

		t.CurrentWeight += t.EffectiveWeight
		totalWeight += t.EffectiveWeight

		if best == nil || t.CurrentWeight > best.CurrentWeight {
			best = t
		}
	}

	if best == nil {
		return nil, ErrNoHealthyUpstreamAvailable
	}

	best.CurrentWeight -= totalWeight
	return best, nil
}

func (b *WeightedRoundRobinBalancer) Algorithm() Algorithm       { return AlgorithmWeightedRoundRobin }
func (b *WeightedRoundRobinBalancer) Targets() []*UpstreamTarget { return b.targets }
func (b *WeightedRoundRobinBalancer) Stop() {
	for _, t := range b.targets {
		t.StopActiveHealthCheck()
	}
}

// 2. WeightedRandomBalancer implements weighted cumulative random selection.
type WeightedRandomBalancer struct {
	targets []*UpstreamTarget
	mu      sync.Mutex
	rnd     *rand.Rand
}

func NewWeightedRandomBalancer(targets []*UpstreamTarget) (*WeightedRandomBalancer, error) {
	if len(targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}
	copied := make([]*UpstreamTarget, len(targets))
	for i, t := range targets {
		cp := cloneUpstreamTarget(t)
		if cp.Weight <= 0 {
			cp.Weight = 1
		}
		copied[i] = cp
	}
	return &WeightedRandomBalancer{
		targets: copied,
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func (b *WeightedRandomBalancer) Next(req *httpparser.Request) (*UpstreamTarget, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	healthy := make([]*UpstreamTarget, 0, len(b.targets))
	totalWeight := 0
	for _, t := range b.targets {
		state := t.GetState()
		if state == StateClosed || state == StateHalfOpen {
			healthy = append(healthy, t)
			w := t.Weight
			if w <= 0 {
				w = 1
			}
			totalWeight += w
		}
	}

	if len(healthy) == 0 {
		return nil, ErrNoHealthyUpstreamAvailable
	}

	r := b.rnd.Intn(totalWeight)
	for _, t := range healthy {
		w := t.Weight
		if w <= 0 {
			w = 1
		}
		if r < w {
			return t, nil
		}
		r -= w
	}

	return healthy[0], nil
}

func (b *WeightedRandomBalancer) Algorithm() Algorithm       { return AlgorithmWeightedRandom }
func (b *WeightedRandomBalancer) Targets() []*UpstreamTarget { return b.targets }
func (b *WeightedRandomBalancer) Stop() {
	for _, t := range b.targets {
		t.StopActiveHealthCheck()
	}
}

// 3. LeastConnBalancer selects target handling fewest active connections.
type LeastConnBalancer struct {
	targets []*UpstreamTarget
}

func NewLeastConnBalancer(targets []*UpstreamTarget) (*LeastConnBalancer, error) {
	if len(targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}
	copied := make([]*UpstreamTarget, len(targets))
	copy(copied, targets)
	return &LeastConnBalancer{targets: copied}, nil
}

func (b *LeastConnBalancer) Next(req *httpparser.Request) (*UpstreamTarget, error) {
	var best *UpstreamTarget
	var minConns int64 = -1

	for _, t := range b.targets {
		state := t.GetState()
		if state == StateClosed || state == StateHalfOpen {
			conns := t.GetActiveConns()
			if minConns == -1 || conns < minConns {
				minConns = conns
				best = t
			}
		}
	}

	if best == nil {
		return nil, ErrNoHealthyUpstreamAvailable
	}
	return best, nil
}

func (b *LeastConnBalancer) Algorithm() Algorithm       { return AlgorithmLeastConn }
func (b *LeastConnBalancer) Targets() []*UpstreamTarget { return b.targets }
func (b *LeastConnBalancer) Stop() {
	for _, t := range b.targets {
		t.StopActiveHealthCheck()
	}
}

// 4. WeightedLeastConnBalancer selects target with minimal ActiveConns / Weight.
type WeightedLeastConnBalancer struct {
	targets []*UpstreamTarget
}

func NewWeightedLeastConnBalancer(targets []*UpstreamTarget) (*WeightedLeastConnBalancer, error) {
	if len(targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}
	copied := make([]*UpstreamTarget, len(targets))
	for i, t := range targets {
		cp := cloneUpstreamTarget(t)
		if cp.Weight <= 0 {
			cp.Weight = 1
		}
		copied[i] = cp
	}
	return &WeightedLeastConnBalancer{targets: copied}, nil
}

func (b *WeightedLeastConnBalancer) Next(req *httpparser.Request) (*UpstreamTarget, error) {
	var best *UpstreamTarget
	var minRatio float64 = -1.0

	for _, t := range b.targets {
		state := t.GetState()
		if state == StateClosed || state == StateHalfOpen {
			w := t.Weight
			if w <= 0 {
				w = 1
			}
			ratio := float64(t.GetActiveConns()) / float64(w)
			if minRatio < 0 || ratio < minRatio {
				minRatio = ratio
				best = t
			}
		}
	}

	if best == nil {
		return nil, ErrNoHealthyUpstreamAvailable
	}
	return best, nil
}

func (b *WeightedLeastConnBalancer) Algorithm() Algorithm       { return AlgorithmWeightedLeastConn }
func (b *WeightedLeastConnBalancer) Targets() []*UpstreamTarget { return b.targets }
func (b *WeightedLeastConnBalancer) Stop() {
	for _, t := range b.targets {
		t.StopActiveHealthCheck()
	}
}

// 5. LeastLatencyBalancer selects target with minimal EMA latency.
type LeastLatencyBalancer struct {
	targets []*UpstreamTarget
}

func NewLeastLatencyBalancer(targets []*UpstreamTarget) (*LeastLatencyBalancer, error) {
	if len(targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}
	copied := make([]*UpstreamTarget, len(targets))
	copy(copied, targets)
	return &LeastLatencyBalancer{targets: copied}, nil
}

func (b *LeastLatencyBalancer) Next(req *httpparser.Request) (*UpstreamTarget, error) {
	var best *UpstreamTarget
	var minLatency int64 = -1

	for _, t := range b.targets {
		state := t.GetState()
		if state == StateClosed || state == StateHalfOpen {
			lat := t.GetAvgLatencyUS()
			if minLatency == -1 || lat < minLatency {
				minLatency = lat
				best = t
			}
		}
	}

	if best == nil {
		return nil, ErrNoHealthyUpstreamAvailable
	}
	return best, nil
}

func (b *LeastLatencyBalancer) Algorithm() Algorithm       { return AlgorithmLeastLatency }
func (b *LeastLatencyBalancer) Targets() []*UpstreamTarget { return b.targets }
func (b *LeastLatencyBalancer) Stop() {
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
	case AlgorithmRandom:
		return NewWeightedRandomBalancer(targets)
	case AlgorithmStickyCookie:
		return NewStickyCookieBalancer(targets, cookieName)
	case AlgorithmIPHash:
		return NewIPHashBalancer(targets)
	case AlgorithmWeightedRoundRobin:
		return NewWeightedRoundRobinBalancer(targets)
	case AlgorithmWeightedRandom:
		return NewWeightedRandomBalancer(targets)
	case AlgorithmLeastConn:
		return NewLeastConnBalancer(targets)
	case AlgorithmWeightedLeastConn:
		return NewWeightedLeastConnBalancer(targets)
	case AlgorithmLeastLatency:
		return NewLeastLatencyBalancer(targets)
	default:
		return nil, fmt.Errorf("proxy: unsupported load balancing algorithm %q", algo)
	}
}

// ProxyTLSConfig captures TLS configuration for upstream reverse proxy connections.
type ProxyTLSConfig struct {
	CAFile             string
	CertFile           string
	KeyFile            string
	InsecureSkipVerify bool
}

// ReverseProxy handles proxying HTTP requests to upstream target URL(s).
type ReverseProxy struct {
	TargetURL              *url.URL     // Single primary target (for backward compatibility)
	Balancer               LoadBalancer // Load balancer interface for target selection
	Client                 *http.Client
	StripPrefix            bool
	RewriteRedirects       bool
	RewriteCookiePath      bool
	InsecureSkipVerify     bool
	TLSCACertPool          *x509.CertPool
	trustedProxies         []*net.IPNet
	propagateUpstreamClose bool
	tracing                bool
	streamResponse         bool
	routeHasCompression    bool
	routeHasCache          bool
	maxPayloadSize         int
	modeMu                 sync.RWMutex
	inboundChunkedMode     string
	maxBodyBytes           int64
}

// InboundChunkedMode returns the configured inbound chunked handling mode ("normalize", "passthrough", "reject").
func (p *ReverseProxy) InboundChunkedMode() string {
	if p == nil {
		return "normalize"
	}
	p.modeMu.RLock()
	defer p.modeMu.RUnlock()
	if p.inboundChunkedMode == "" {
		return "normalize"
	}
	return p.inboundChunkedMode
}

// SetInboundChunkedMode updates the inbound chunked handling mode.
func (p *ReverseProxy) SetInboundChunkedMode(mode string) {
	if p == nil {
		return
	}
	p.modeMu.Lock()
	defer p.modeMu.Unlock()
	p.inboundChunkedMode = mode
}

// MaxBodyBytes returns the maximum inbound payload bytes permitted before returning 413.
func (p *ReverseProxy) MaxBodyBytes() int64 {
	if p == nil {
		return 0
	}
	return atomic.LoadInt64(&p.maxBodyBytes)
}

// SetMaxBodyBytes updates the maximum inbound payload bytes permitted.
func (p *ReverseProxy) SetMaxBodyBytes(limit int64) {
	if p == nil {
		return
	}
	atomic.StoreInt64(&p.maxBodyBytes, limit)
}

// IsStreamResponseEnabled returns whether streaming response is enabled on this reverse proxy.
func (p *ReverseProxy) IsStreamResponseEnabled() bool {
	if p == nil {
		return false
	}
	return p.streamResponse
}

// GetTransport returns the underlying *http.Transport for telemetry and testing verification.
func (p *ReverseProxy) GetTransport() *http.Transport {
	if p.Client != nil {
		if tr, ok := p.Client.Transport.(*http.Transport); ok {
			return tr
		}
	}
	return nil
}

// NewReverseProxy creates a ReverseProxy instance for a single target URL string.
func NewReverseProxy(targetURLStr string, timeout time.Duration) (*ReverseProxy, error) {
	return NewLoadBalancerProxy([]string{targetURLStr}, AlgorithmRoundRobin, timeout)
}

// ProxyTransportConfig configures upstream transport parameters including connection pooling,
// timeouts, compression, egress routing, and protocol negotiation.
type ProxyTransportConfig struct {
	Profile                string
	MaxIdleConns           int
	MaxIdleConnsPerHost    int
	MaxConnsPerHost        int
	IdleConnTimeout        time.Duration
	DisableCompression     *bool
	UseEnvProxy            *bool
	ProxyURL               string
	PropagateUpstreamClose *bool
	ForceAttemptHTTP2      *bool
	Tracing                *bool
	StreamResponse         *bool
	ResponseHeaderTimeout  time.Duration
	InboundChunkedMode     string
}

// DefaultProxyTransportConfig returns the canonical transport configuration for the given profile.
// Default profile is "raw_speed".
func DefaultProxyTransportConfig(profile string) ProxyTransportConfig {
	p := strings.ToLower(strings.TrimSpace(profile))
	if p == "balanced" || p == "standard" {
		f := false
		t := true
		return ProxyTransportConfig{
			Profile:                "balanced",
			MaxIdleConns:           1000,
			MaxIdleConnsPerHost:    100,
			MaxConnsPerHost:        200,
			IdleConnTimeout:        30 * time.Second,
			DisableCompression:     &f,
			UseEnvProxy:            &t,
			PropagateUpstreamClose: &t,
			ForceAttemptHTTP2:      &t,
			Tracing:                &t,
			StreamResponse:         &t,
			ResponseHeaderTimeout:  10 * time.Second,
			InboundChunkedMode:     "normalize",
		}
	}
	t := true
	f := false
	return ProxyTransportConfig{
		Profile:                "raw_speed",
		MaxIdleConns:           10000,
		MaxIdleConnsPerHost:    1000,
		MaxConnsPerHost:        0,
		IdleConnTimeout:        90 * time.Second,
		DisableCompression:     &t,
		UseEnvProxy:            &f,
		PropagateUpstreamClose: &f,
		ForceAttemptHTTP2:      &f,
		Tracing:                &f,
		StreamResponse:         &t,
		ResponseHeaderTimeout:  10 * time.Second,
		InboundChunkedMode:     "normalize",
	}
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
	StripPrefix         *bool
	RewriteRedirects    *bool
	RewriteCookiePath   *bool
	Auth                any
	WAF                 any
	SPA                 bool
	Fallback            string
	RedirectHTTP        *bool
	AccessLog           string
	SecurityLog         string
	TLS                 ProxyTLSConfig
	InsecureSkipVerify  bool
	TLSCACertPool       *x509.CertPool
	TLSClientConfig     *tls.Config
	TrustedProxies      []string
	RouteHasCompression bool
	RouteHasCache       bool
	Transport           ProxyTransportConfig
	MaxPayloadSize      int
	InboundChunkedMode  string
	MaxBodyBytes        int64
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

	var caPool *x509.CertPool
	if opts.TLSCACertPool != nil {
		caPool = opts.TLSCACertPool
	} else if opts.TLS.CAFile != "" {
		caData, err := os.ReadFile(opts.TLS.CAFile)
		if err != nil {
			return nil, fmt.Errorf("proxy: failed to read CAFile %q: %w", opts.TLS.CAFile, err)
		}
		caPool = x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caData) {
			return nil, fmt.Errorf("proxy: failed to parse CA certificates from %q", opts.TLS.CAFile)
		}
	}

	insecureSkipVerify := opts.InsecureSkipVerify || opts.TLS.InsecureSkipVerify

	// Resolve transport settings
	baseProfile := opts.Transport.Profile
	tc := DefaultProxyTransportConfig(baseProfile)
	if opts.Transport.MaxIdleConns > 0 {
		tc.MaxIdleConns = opts.Transport.MaxIdleConns
	}
	if opts.Transport.MaxIdleConnsPerHost > 0 {
		tc.MaxIdleConnsPerHost = opts.Transport.MaxIdleConnsPerHost
	}
	if opts.Transport.MaxConnsPerHost > 0 {
		tc.MaxConnsPerHost = opts.Transport.MaxConnsPerHost
	}
	if opts.Transport.IdleConnTimeout > 0 {
		tc.IdleConnTimeout = opts.Transport.IdleConnTimeout
	}
	if opts.Transport.DisableCompression != nil {
		tc.DisableCompression = opts.Transport.DisableCompression
	}
	if opts.Transport.UseEnvProxy != nil {
		tc.UseEnvProxy = opts.Transport.UseEnvProxy
	}
	if opts.Transport.ProxyURL != "" {
		tc.ProxyURL = opts.Transport.ProxyURL
	}
	if opts.Transport.PropagateUpstreamClose != nil {
		tc.PropagateUpstreamClose = opts.Transport.PropagateUpstreamClose
	}
	if opts.Transport.ForceAttemptHTTP2 != nil {
		tc.ForceAttemptHTTP2 = opts.Transport.ForceAttemptHTTP2
	}
	if opts.Transport.Tracing != nil {
		tc.Tracing = opts.Transport.Tracing
	}
	if opts.Transport.StreamResponse != nil {
		tc.StreamResponse = opts.Transport.StreamResponse
	}
	if opts.Transport.ResponseHeaderTimeout > 0 {
		tc.ResponseHeaderTimeout = opts.Transport.ResponseHeaderTimeout
	}
	if tc.ResponseHeaderTimeout <= 0 {
		tc.ResponseHeaderTimeout = 10 * time.Second
	}

	var proxyFunc func(*http.Request) (*url.URL, error)
	if tc.ProxyURL != "" {
		parsedProxyURL, err := url.Parse(tc.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("proxy: invalid proxy_url %q: %w", tc.ProxyURL, err)
		}
		proxyFunc = http.ProxyURL(parsedProxyURL)
	} else if tc.UseEnvProxy != nil && *tc.UseEnvProxy {
		proxyFunc = http.ProxyFromEnvironment
	} else {
		proxyFunc = nil
	}

	disableCompression := true
	if tc.DisableCompression != nil {
		disableCompression = *tc.DisableCompression
	}

	forceAttemptH2 := false
	if tc.ForceAttemptHTTP2 != nil {
		forceAttemptH2 = *tc.ForceAttemptHTTP2
	}

	tr := &http.Transport{
		Proxy: proxyFunc,
		DialContext: (&net.Dialer{
			Timeout:   opts.Timeout,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     forceAttemptH2,
		MaxIdleConns:          tc.MaxIdleConns,
		MaxIdleConnsPerHost:   tc.MaxIdleConnsPerHost,
		MaxConnsPerHost:       tc.MaxConnsPerHost,
		IdleConnTimeout:       tc.IdleConnTimeout,
		ResponseHeaderTimeout: tc.ResponseHeaderTimeout,
		DisableCompression:    disableCompression,
		WriteBufferSize:       64 * 1024,
		ReadBufferSize:        64 * 1024,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
			RootCAs:            caPool,
		},
	}

	if opts.TLSClientConfig != nil {
		tr.TLSClientConfig = opts.TLSClientConfig.Clone()
	}

	client := &http.Client{
		Timeout:   0,
		Transport: tr,
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

	stripPrefix := true
	if opts.StripPrefix != nil {
		stripPrefix = *opts.StripPrefix
	}
	rewriteRedirects := true
	if opts.RewriteRedirects != nil {
		rewriteRedirects = *opts.RewriteRedirects
	}
	rewriteCookiePath := true
	if opts.RewriteCookiePath != nil {
		rewriteCookiePath = *opts.RewriteCookiePath
	}

	var trustedProxies []*net.IPNet
	for _, tp := range opts.TrustedProxies {
		tp = strings.TrimSpace(tp)
		if tp == "" {
			continue
		}
		if !strings.Contains(tp, "/") {
			if ip := net.ParseIP(tp); ip != nil {
				if ip.To4() != nil {
					tp += "/32"
				} else {
					tp += "/128"
				}
			}
		}
		_, ipNet, err := net.ParseCIDR(tp)
		if err == nil && ipNet != nil {
			trustedProxies = append(trustedProxies, ipNet)
		}
	}

	propagateUpstreamClose := false
	if tc.PropagateUpstreamClose != nil {
		propagateUpstreamClose = *tc.PropagateUpstreamClose
	}

	tracing := false
	if tc.Tracing != nil {
		tracing = *tc.Tracing
	}

	streamResponse := true
	if opts.Transport.StreamResponse != nil {
		streamResponse = *opts.Transport.StreamResponse
	} else if tc.StreamResponse != nil {
		streamResponse = *tc.StreamResponse
	}

	maxPayloadSize := opts.MaxPayloadSize
	if maxPayloadSize <= 0 {
		maxPayloadSize = 1024 * 1024 // 1 MB default
	}

	inboundMode := opts.InboundChunkedMode
	if inboundMode == "" {
		inboundMode = opts.Transport.InboundChunkedMode
	}
	if inboundMode == "" {
		inboundMode = "normalize"
	}

	return &ReverseProxy{
		TargetURL:              upstreamTargets[0].URL,
		Balancer:               lb,
		Client:                 client,
		StripPrefix:            stripPrefix,
		RewriteRedirects:       rewriteRedirects,
		RewriteCookiePath:      rewriteCookiePath,
		InsecureSkipVerify:     insecureSkipVerify,
		TLSCACertPool:          caPool,
		trustedProxies:         trustedProxies,
		propagateUpstreamClose: propagateUpstreamClose,
		tracing:                tracing,
		streamResponse:         streamResponse,
		routeHasCompression:    opts.RouteHasCompression,
		routeHasCache:          opts.RouteHasCache,
		maxPayloadSize:         maxPayloadSize,
		inboundChunkedMode:     inboundMode,
		maxBodyBytes:           opts.MaxBodyBytes,
	}, nil
}

// Close stops active background health checks and closes all idle transport connections.
func (p *ReverseProxy) Close() {
	if p.Balancer != nil {
		p.Balancer.Stop()
	}
	if p.Client != nil {
		if tr, ok := p.Client.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
}

var hopByHopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"trailers":            true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

type earlyCancelingReader struct {
	r      io.Reader
	cancel context.CancelFunc
}

func (e *earlyCancelingReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		if e.cancel != nil {
			e.cancel()
		}
	}
	return n, err
}

func (e *earlyCancelingReader) Close() error {
	if c, ok := e.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

type tlsConn interface {
	ConnectionState() tls.ConnectionState
}

func isTLSConnection(conn net.Conn) bool {
	if conn == nil {
		return false
	}
	if _, ok := conn.(*tls.Conn); ok {
		return true
	}
	if _, ok := conn.(tlsConn); ok {
		return true
	}
	return false
}

func isPeerTrusted(remoteAddr net.Addr, trusted []*net.IPNet) (string, bool) {
	if remoteAddr == nil {
		return "", false
	}
	host, _, err := net.SplitHostPort(remoteAddr.String())
	if err != nil {
		host = remoteAddr.String()
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return host, false
	}
	for _, cidr := range trusted {
		if cidr.Contains(ip) {
			return host, true
		}
	}
	return host, false
}

// mergeTargetAndClientQuery merges pre-configured target URL query parameters with
// client-supplied query parameters, ensuring target parameters strictly take precedence
// and cannot be polluted or overridden by client parameters (HTTP Parameter Pollution guard, REQ-072).
func mergeTargetAndClientQuery(targetRawQuery, clientQuery string) string {
	if targetRawQuery == "" {
		return clientQuery
	}
	if clientQuery == "" {
		return targetRawQuery
	}

	targetKeys := make(map[string]struct{})
	if parsedTarget, err := url.ParseQuery(targetRawQuery); err == nil {
		for k := range parsedTarget {
			targetKeys[k] = struct{}{}
		}
	} else {
		for _, part := range strings.Split(targetRawQuery, "&") {
			if part == "" {
				continue
			}
			rawKey := part
			if idx := strings.Index(part, "="); idx != -1 {
				rawKey = part[:idx]
			}
			if unescaped, err := url.QueryUnescape(rawKey); err == nil {
				targetKeys[unescaped] = struct{}{}
			} else {
				targetKeys[rawKey] = struct{}{}
			}
		}
	}

	var filteredClientParts []string
	for _, part := range strings.Split(clientQuery, "&") {
		if part == "" {
			continue
		}

		collides := false
		for _, subpart := range strings.Split(part, ";") {
			rawKey := subpart
			if idx := strings.Index(subpart, "="); idx != -1 {
				rawKey = subpart[:idx]
			}
			unescapedKey, err := url.QueryUnescape(rawKey)
			if err != nil {
				unescapedKey = rawKey
			}

			if _, exists := targetKeys[unescapedKey]; exists {
				collides = true
				break
			}
			if _, exists := targetKeys[rawKey]; exists {
				collides = true
				break
			}
		}

		if collides {
			continue
		}

		filteredClientParts = append(filteredClientParts, part)
	}

	if len(filteredClientParts) == 0 {
		return targetRawQuery
	}

	return targetRawQuery + "&" + strings.Join(filteredClientParts, "&")
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
			SetUpstreamTarget(req, "unavailable")
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
		SetUpstreamTarget(req, "unavailable")
		p.writeBadGateway(res, "No upstream target available")
		return
	}

	SetUpstreamTarget(req, targetNode.URL.Host)

	targetNode.IncActiveConns()
	startTime := time.Now()
	defer func() {
		targetNode.DecActiveConns()
		targetNode.RecordLatency(time.Since(startTime))
	}()

	targetURL := targetNode.URL
	outURL := *targetURL
	outURL.Path = JoinProxyPath(targetURL.Path, req.Path, prefix, p.StripPrefix)

	clientQuery := ""
	if req.URL != nil && req.URL.RawQuery != "" {
		clientQuery = req.URL.RawQuery
	} else if len(req.QueryParams) > 0 {
		clientQuery = req.QueryParams.Encode()
	} else if req.Query() != nil && len(req.Query()) > 0 {
		clientQuery = req.Query().Encode()
	}

	outURL.RawQuery = mergeTargetAndClientQuery(targetURL.RawQuery, clientQuery)

	if req.IsWebSocketUpgrade() {
		p.serveWebSocketProxy(req, res, targetNode, outURL, prefix)
		return
	}

	isChunked := (req.ContentLength == -1 && req.Body != nil) || (req.Header.Get("Transfer-Encoding") != "")
	effectiveMode := p.InboundChunkedMode()

	var bodyReader io.Reader
	var cleanupBuf func()
	var cancelFunc context.CancelFunc
	reqCtx := req.Context()

	if isChunked {
		if strings.EqualFold(effectiveMode, "reject") {
			res.SetStatus(http.StatusNotImplemented)
			res.Header.Set("Connection", "close")
			res.Header.Set("Content-Type", "application/json")
			_, _ = res.WriteString(`{"error":"501 Not Implemented: Inbound chunked transfer encoding is disabled"}`)
			return
		} else if strings.EqualFold(effectiveMode, "passthrough") {
			var passthroughCancel context.CancelFunc
			reqCtx, passthroughCancel = context.WithCancel(req.Context())
			cancelFunc = passthroughCancel
			bodyReader = &earlyCancelingReader{
				r:      req.Body,
				cancel: passthroughCancel,
			}
		} else { // "normalize" mode
			decodedBytes, err := io.ReadAll(req.Body)
			if err != nil {
				res.Header.Set("Connection", "close")
				res.Header.Set("Content-Type", "application/json")
				switch {
				case errors.Is(err, httpparser.ErrBodyTooLarge):
					res.SetStatus(http.StatusRequestEntityTooLarge)
					_, _ = res.WriteString(`{"error":"413 Payload Too Large"}`)
				case errors.Is(err, httpparser.ErrHeaderTooLarge):
					res.SetStatus(http.StatusRequestHeaderFieldsTooLarge)
					_, _ = res.WriteString(`{"error":"431 Request Header Fields Too Large"}`)
				default:
					res.SetStatus(http.StatusBadRequest)
					_, _ = res.WriteString(`{"error":"400 Bad Request"}`)
				}
				return
			}
			exactCL := int64(len(decodedBytes))
			if exactCL > 0 {
				if exactCL <= 65536 {
					bufPtr := httpparser.GetBodyBuffer()
					copy(*bufPtr, decodedBytes)
					bodyReader = bytes.NewReader((*bufPtr)[:exactCL])
					cleanupBuf = func() {
						httpparser.PutBodyBuffer(bufPtr)
					}
				} else {
					bodyReader = bytes.NewReader(decodedBytes)
				}
			}
		}
	} else if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil && len(bodyBytes) > 0 {
			bodyReader = bytes.NewReader(bodyBytes)
		}
	}

	if cleanupBuf != nil {
		defer cleanupBuf()
	}
	if cancelFunc != nil {
		defer cancelFunc()
	}

	outReq, err := http.NewRequestWithContext(reqCtx, req.Method, outURL.String(), bodyReader)
	if err != nil {
		p.writeBadGateway(res, fmt.Sprintf("Failed to construct proxy request: %v", err))
		return
	}

	// Build custom hop-by-hop tokens from Connection header (RFC 7230 §6.1)
	customHopByHop := make(map[string]bool)
	if connHdr := req.Header.Get("Connection"); connHdr != "" {
		for _, tok := range strings.Split(connHdr, ",") {
			tok = strings.ToLower(strings.TrimSpace(tok))
			if tok != "" {
				customHopByHop[tok] = true
			}
		}
	}

	// Copy original request headers, stripping RFC 7230 §6.1 hop-by-hop headers and HTTP/2 pseudo-headers
	for key, values := range req.Header {
		if strings.HasPrefix(key, ":") {
			continue // Strip HTTP/2 pseudo-headers (:protocol, :path, :authority, etc.)
		}
		lowerKey := strings.ToLower(key)
		if hopByHopHeaders[lowerKey] || customHopByHop[lowerKey] {
			continue
		}
		for _, val := range values {
			outReq.Header.Add(key, val)
		}
	}

	// Forward decoded trailers from chunked request
	if cr, ok := req.Body.(*httpparser.ChunkedBodyReader); ok {
		for k, vals := range cr.Trailers() {
			for _, v := range vals {
				outReq.Header.Add(k, v)
			}
		}
	}

	if isChunked {
		if strings.EqualFold(effectiveMode, "passthrough") {
			outReq.ContentLength = -1
			outReq.TransferEncoding = []string{"chunked"}
			outReq.Header.Del("Content-Length")
			outReq.Header.Del("Transfer-Encoding")
		} else { // "normalize"
			exactCL := int64(0)
			if r, ok := bodyReader.(*bytes.Reader); ok {
				exactCL = int64(r.Len())
			}
			outReq.ContentLength = exactCL
			outReq.Header.Set("Content-Length", strconv.FormatInt(exactCL, 10))
			outReq.Header.Del("Transfer-Encoding")
		}
	} else if req.ContentLength > 0 {
		outReq.ContentLength = req.ContentLength
	}

	// Derive client peer IP and trust status
	peerIP := ""
	isTrusted := false
	if host := req.RemoteHost(); host != "" {
		peerIP = host
		if ip := req.RemoteIP(); ip != nil {
			peerIP = ip.String()
			for _, cidr := range p.trustedProxies {
				if cidr.Contains(ip) {
					isTrusted = true
					break
				}
			}
		}
	}

	// Inject X-Forwarded-* headers with verified connection state integrity (ADR-066 / CWE-345)
	outReq.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))

	// Derive protocol from physical connection or HTTP/2 & HTTP/3 indicators
	isTLS := isTLSConnection(req.RawConn)
	proto := "http"
	if isTLS || strings.EqualFold(req.Proto, "https") || req.Proto == "HTTP/2.0" || req.Proto == "HTTP/3.0" || strings.HasPrefix(req.Proto, "HTTP/2") || strings.HasPrefix(req.Proto, "HTTP/3") {
		proto = "https"
	}

	clientProto := req.Header.Get("X-Forwarded-Proto")
	if isTrusted && clientProto != "" {
		proto = clientProto
	} else if req.RawConn == nil && req.RemoteAddr == "" {
		// Mock testing fallback when both RawConn and RemoteAddr are absent
		if clientProto != "" {
			proto = clientProto
		}
	}
	outReq.Header.Set("X-Forwarded-Proto", proto)

	if peerIP != "" {
		if isTrusted {
			existingXFF := req.Header.Get("X-Forwarded-For")
			if existingXFF != "" {
				outReq.Header.Set("X-Forwarded-For", existingXFF+", "+peerIP)
			} else {
				outReq.Header.Set("X-Forwarded-For", peerIP)
			}
			if xri := req.Header.Get("X-Real-IP"); xri != "" {
				outReq.Header.Set("X-Real-IP", xri)
			} else {
				outReq.Header.Set("X-Real-IP", peerIP)
			}
		} else {
			// Untrusted peer: discard client-supplied XFF and X-Real-IP; strictly set to verified peerIP
			outReq.Header.Set("X-Forwarded-For", peerIP)
			outReq.Header.Set("X-Real-IP", peerIP)
		}
	} else if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		outReq.Header.Set("X-Forwarded-For", xff)
	} else if clientIP := req.Header.Get("X-Real-IP"); clientIP != "" {
		outReq.Header.Set("X-Forwarded-For", clientIP)
	}

	if prefix != "" {
		outReq.Header.Set("X-Forwarded-Prefix", prefix)
	}

	// Propagate / Inject W3C traceparent header if tracing is enabled
	incomingTrace := req.Header.Get("traceparent")
	if p.tracing {
		outReq.Header.Set("traceparent", metrics.EnsureW3CTraceparent(incomingTrace))
	} else if incomingTrace != "" {
		outReq.Header.Set("traceparent", incomingTrace)
	}

	// Dispatch request to upstream
	outResp, err := p.Client.Do(outReq)
	if err != nil {
		targetNode.RecordFailure()
		p.writeBadGateway(res, fmt.Sprintf("Upstream unreachable (%s): %v", targetURL.String(), err))
		return
	}

	contentType := strings.ToLower(outResp.Header.Get("Content-Type"))
	isStreamingMIME := strings.HasPrefix(contentType, "text/event-stream")
	isUnbuffered := strings.EqualFold(strings.TrimSpace(outResp.Header.Get("X-Accel-Buffering")), "no")

	maxPayloadSize := 1048576 // 1MB default
	if p.maxPayloadSize > 0 {
		maxPayloadSize = p.maxPayloadSize
	}

	canStream := false
	if p.streamResponse {
		if !p.routeHasCompression && !p.routeHasCache {
			canStream = true
		} else if isStreamingMIME || isUnbuffered {
			canStream = true
		} else if outResp.ContentLength > int64(maxPayloadSize) || outResp.ContentLength < 0 || strings.EqualFold(outResp.Header.Get("Transfer-Encoding"), "chunked") {
			// Dynamic clamp: bypass compression/cache for oversized/chunked bodies
			canStream = true
		} else {
			// 0 <= ContentLength <= maxPayloadSize: buffer for compression/cache
			canStream = false
		}
	}

	if outResp.StatusCode >= 500 {
		targetNode.RecordFailure()
	} else {
		targetNode.RecordSuccess()
	}

	// Copy upstream status code
	res.SetStatus(outResp.StatusCode)

	upstreamClosed := false
	if strings.EqualFold(outResp.Header.Get("Connection"), "close") || outResp.Close {
		upstreamClosed = true
	}

	// Copy upstream headers (filtering RFC 7230 hop-by-hop headers)
	for key, values := range outResp.Header {
		lowerKey := strings.ToLower(key)
		if hopByHopHeaders[lowerKey] {
			continue
		}
		for _, val := range values {
			res.Header.Add(key, val)
		}
	}

	// Propagate upstream close if enabled
	if p.propagateUpstreamClose && upstreamClosed {
		res.Header.Set("Connection", "close")
	}

	// Intercept and rewrite 3xx redirects (Location header) if enabled
	if p.RewriteRedirects && prefix != "" && outResp.StatusCode >= 300 && outResp.StatusCode < 400 {
		if loc := res.Header.Get("Location"); loc != "" {
			res.Header.Set("Location", RewriteRedirectLocation(loc, prefix, targetURL, req))
		}
	}

	// Intercept and rewrite Set-Cookie Path if enabled
	if p.RewriteCookiePath && prefix != "" {
		if cookies, ok := res.Header["set-cookie"]; ok && len(cookies) > 0 {
			rewritten := make([]string, len(cookies))
			for i, c := range cookies {
				rewritten[i] = RewriteCookiePath(c, prefix)
			}
			res.Header["set-cookie"] = rewritten
		}
	}

	// Inject sticky session cookie if sticky_cookie load balancer is active
	if stickyBalancer, ok := p.Balancer.(*StickyCookieBalancer); ok {
		cookieVal := TargetHash(targetNode.URL.String())
		res.Header.Set("Set-Cookie", fmt.Sprintf("%s=%s; Path=/; HttpOnly", stickyBalancer.CookieName(), cookieVal))
	}

	if canStream {
		res.StreamBody = outResp.Body
		return
	}

	defer outResp.Body.Close()

	// Copy upstream body using pooled copy buffer slab bounded by maxPayloadSize + 1
	if outResp.Body != nil {
		bufPtr := httpparser.GetCopyBuffer()
		limitReader := io.LimitReader(outResp.Body, int64(maxPayloadSize)+1)
		n, _ := io.CopyBuffer(res.Body, limitReader, *bufPtr)
		httpparser.PutCopyBuffer(bufPtr)
		if n > int64(maxPayloadSize) {
			targetNode.RecordFailure()
			res.Body.Reset()
			p.writeBadGateway(res, "Upstream payload exceeded maximum allowed buffer limit")
			return
		}
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

// JoinProxyPath resolves the upstream destination path by joining targetPath with reqPath,
// taking prefix stripping and target subpath preservation into account, while
// enforcing path canonicalization and directory traversal guards (ADR-062).
func JoinProxyPath(targetPath, reqPath, prefix string, stripPrefix bool) string {
	if !stripPrefix || prefix == "" {
		cleanReq := path.Clean(reqPath)
		if !strings.HasPrefix(cleanReq, "/") {
			cleanReq = "/" + cleanReq
		}
		if targetPath == "" || targetPath == "/" {
			if strings.HasSuffix(reqPath, "/") && cleanReq != "/" && !strings.HasSuffix(cleanReq, "/") {
				cleanReq += "/"
			}
			return cleanReq
		}

		cleanTarget := path.Clean(targetPath)
		joined := singleJoiningSlash(cleanTarget, cleanReq)
		cleanJoined := path.Clean(joined)
		if !strings.HasPrefix(cleanJoined, cleanTarget) {
			cleanJoined = singleJoiningSlash(cleanTarget, strings.TrimPrefix(cleanJoined, "/"))
		}
		if (strings.HasSuffix(reqPath, "/") || strings.HasSuffix(targetPath, "/")) && !strings.HasSuffix(cleanJoined, "/") {
			cleanJoined += "/"
		}
		return cleanJoined
	}

	trimmed := strings.TrimPrefix(reqPath, prefix)

	// Target defines an explicit subpath (e.g., "/postback", "/v1/api")
	if targetPath != "" && targetPath != "/" {
		if trimmed == "" {
			// Exact prefix match without trailing slash (e.g. req="/kite/postback", prefix="/kite/postback")
			return targetPath
		}
		if trimmed == "/" {
			// Explicit trailing slash requested by client (e.g. req="/kite/postback/", prefix="/kite/postback")
			if strings.HasSuffix(targetPath, "/") {
				return targetPath
			}
			return targetPath + "/"
		}

		cleanTrimmed := path.Clean("/" + strings.TrimPrefix(trimmed, "/"))
		if cleanTrimmed == "/" {
			if strings.HasSuffix(reqPath, "/") || strings.HasSuffix(targetPath, "/") {
				if !strings.HasSuffix(targetPath, "/") {
					return targetPath + "/"
				}
			}
			return targetPath
		}

		cleanTarget := path.Clean(targetPath)
		joined := singleJoiningSlash(cleanTarget, cleanTrimmed)
		cleanJoined := path.Clean(joined)
		if !strings.HasPrefix(cleanJoined, cleanTarget) {
			cleanJoined = singleJoiningSlash(cleanTarget, strings.TrimPrefix(cleanJoined, "/"))
		}
		if strings.HasSuffix(reqPath, "/") && !strings.HasSuffix(cleanJoined, "/") {
			cleanJoined += "/"
		}
		return cleanJoined
	}

	// Target has no subpath (e.g. targetPath is "" or "/")
	if trimmed == "" || trimmed == "/" {
		return "/"
	}
	cleanTrimmed := path.Clean("/" + strings.TrimPrefix(trimmed, "/"))
	if cleanTrimmed == "/" {
		return "/"
	}
	if strings.HasSuffix(reqPath, "/") && !strings.HasSuffix(cleanTrimmed, "/") {
		cleanTrimmed += "/"
	}
	return cleanTrimmed
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
		serverName := outURL.Hostname()
		tlsConfig := &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: p.InsecureSkipVerify,
			RootCAs:            p.TLSCACertPool,
		}
		timeout := p.Client.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		dialer := &tls.Dialer{
			NetDialer: &net.Dialer{
				Timeout: timeout,
			},
			Config: tlsConfig,
		}
		upstreamConn, dialErr = dialer.Dial("tcp", host)
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
		if strings.HasPrefix(k, ":") {
			continue // Strip HTTP/2 pseudo-headers from HTTP/1.1 wire serialization
		}
		for _, v := range vv {
			fmt.Fprintf(&reqBuf, "%s: %s\r\n", k, v)
		}
	}
	if req.Header.Get("Host") == "" {
		fmt.Fprintf(&reqBuf, "Host: %s\r\n", outURL.Host)
	}
	if prefix != "" && req.Header.Get("X-Forwarded-Prefix") == "" {
		fmt.Fprintf(&reqBuf, "X-Forwarded-Prefix: %s\r\n", prefix)
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

func (c *proxyPrefixConn) Read(b []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(b, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(b)
}

// RewriteRedirectLocation adjusts a 3xx redirect Location header to prepend the proxy route prefix.
// It handles:
// 1. Relative paths: "/dashboard" -> "/api/dashboard"
// 2. Target-internal URLs: "http://127.0.0.1:9001/dashboard" -> "/api/dashboard" (or public URL)
// 3. External URLs: "https://accounts.google.com/oauth" -> preserved as-is
func RewriteRedirectLocation(loc, prefix string, targetURL *url.URL, req *httpparser.Request) string {
	if loc == "" || prefix == "" {
		return loc
	}

	cleanPrefix := "/" + strings.Trim(prefix, "/")

	// Case 1: Relative Path starting with "/"
	if strings.HasPrefix(loc, "/") {
		if loc == cleanPrefix || strings.HasPrefix(loc, cleanPrefix+"/") {
			return loc
		}
		return singleJoiningSlash(cleanPrefix, loc)
	}

	// Case 2: Parse as URL
	u, err := url.Parse(loc)
	if err != nil {
		return loc
	}

	// If no scheme and no host, it's relative
	if u.Scheme == "" && u.Host == "" {
		if u.Path == cleanPrefix || strings.HasPrefix(u.Path, cleanPrefix+"/") {
			return loc
		}
		u.Path = singleJoiningSlash(cleanPrefix, u.Path)
		return u.String()
	}

	// Check if redirect target matches the upstream backend host
	if targetURL != nil && strings.EqualFold(u.Host, targetURL.Host) {
		newPath := u.Path
		if newPath != cleanPrefix && !strings.HasPrefix(newPath, cleanPrefix+"/") {
			newPath = singleJoiningSlash(cleanPrefix, newPath)
		}
		u.Path = newPath

		// Map host to client request Host / X-Forwarded-Host if available
		clientHost := req.Header.Get("X-Forwarded-Host")
		if clientHost == "" {
			clientHost = req.Header.Get("Host")
		}
		if clientHost != "" {
			u.Host = clientHost
			clientProto := req.Header.Get("X-Forwarded-Proto")
			if clientProto != "" {
				u.Scheme = clientProto
			}
		} else {
			// Return relative path + query + fragment
			rel := u.Path
			if u.RawQuery != "" {
				rel += "?" + u.RawQuery
			}
			if u.Fragment != "" {
				rel += "#" + u.Fragment
			}
			return rel
		}
		return u.String()
	}

	// Case 3: External third-party redirect (e.g. OAuth, external CDN) -> untouched
	return loc
}

// RewriteCookiePath adjusts the Path attribute in a Set-Cookie header string to be scoped under prefix.
// Example: "session=xyz; Path=/; HttpOnly" -> "session=xyz; Path=/api; HttpOnly"
func RewriteCookiePath(cookieStr, prefix string) string {
	if cookieStr == "" || prefix == "" {
		return cookieStr
	}

	cleanPrefix := "/" + strings.Trim(prefix, "/")
	parts := strings.Split(cookieStr, ";")
	foundPath := false

	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(trimmed), "path=") {
			foundPath = true
			oldPath := strings.TrimSpace(trimmed[5:])
			var newPath string
			if oldPath == "" || oldPath == "/" {
				newPath = cleanPrefix
			} else if oldPath == cleanPrefix || strings.HasPrefix(oldPath, cleanPrefix+"/") {
				newPath = oldPath
			} else {
				newPath = singleJoiningSlash(cleanPrefix, oldPath)
			}
			parts[i] = " Path=" + newPath
		}
	}

	if !foundPath {
		// If no Path attribute was explicitly specified, append Path=<cleanPrefix>
		return cookieStr + "; Path=" + cleanPrefix
	}

	return strings.Join(parts, ";")
}
