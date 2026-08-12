package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"toron/pkg/httpparser"
)

// Algorithm defines the load balancing strategy name.
type Algorithm string

const (
	AlgorithmRoundRobin Algorithm = "round_robin"
	AlgorithmRandom     Algorithm = "random"
)

var (
	ErrNoTargetsAvailable = errors.New("proxy: no upstream targets available")
)

// LoadBalancer interface abstracts selecting an upstream target URL for a request.
type LoadBalancer interface {
	Next(req *httpparser.Request) (*url.URL, error)
	Algorithm() Algorithm
	Targets() []*url.URL
}

// RoundRobinBalancer selects upstream targets sequentially in a thread-safe circular order.
type RoundRobinBalancer struct {
	targets []*url.URL
	counter uint64
}

// NewRoundRobinBalancer creates a round-robin load balancer for target URLs.
func NewRoundRobinBalancer(targets []*url.URL) (*RoundRobinBalancer, error) {
	if len(targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}
	copied := make([]*url.URL, len(targets))
	copy(copied, targets)
	return &RoundRobinBalancer{
		targets: copied,
	}, nil
}

func (b *RoundRobinBalancer) Next(req *httpparser.Request) (*url.URL, error) {
	if len(b.targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}
	idx := atomic.AddUint64(&b.counter, 1) - 1
	return b.targets[idx%uint64(len(b.targets))], nil
}

func (b *RoundRobinBalancer) Algorithm() Algorithm {
	return AlgorithmRoundRobin
}

func (b *RoundRobinBalancer) Targets() []*url.URL {
	return b.targets
}

// NewLoadBalancer constructs a LoadBalancer for given targets and algorithm.
// Defaults to AlgorithmRoundRobin if algo is empty or "round_robin".
func NewLoadBalancer(algo Algorithm, targetURLs []*url.URL) (LoadBalancer, error) {
	if len(targetURLs) == 0 {
		return nil, ErrNoTargetsAvailable
	}

	normAlgo := Algorithm(strings.ToLower(strings.TrimSpace(string(algo))))
	switch normAlgo {
	case "", AlgorithmRoundRobin:
		return NewRoundRobinBalancer(targetURLs)
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

// NewLoadBalancerProxy creates a ReverseProxy instance that load balances requests across multiple target URL strings.
func NewLoadBalancerProxy(targetURLStrs []string, algo Algorithm, timeout time.Duration) (*ReverseProxy, error) {
	if len(targetURLStrs) == 0 {
		return nil, ErrNoTargetsAvailable
	}

	parsedURLs := make([]*url.URL, 0, len(targetURLStrs))
	for _, targetStr := range targetURLStrs {
		targetURL, err := url.Parse(targetStr)
		if err != nil {
			return nil, fmt.Errorf("proxy: invalid target URL %q: %w", targetStr, err)
		}
		parsedURLs = append(parsedURLs, targetURL)
	}

	lb, err := NewLoadBalancer(algo, parsedURLs)
	if err != nil {
		return nil, err
	}

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't auto-follow redirects, proxy them back
		},
	}

	return &ReverseProxy{
		TargetURL: parsedURLs[0],
		Balancer:  lb,
		Client:    client,
	}, nil
}

// ServeHTTP translates a Toron Request, proxies it to the upstream server, and writes the upstream response to Res.
func (p *ReverseProxy) ServeHTTP(req *httpparser.Request, res *httpparser.Response) {
	p.ServeHTTPWithPrefix(req, res, "")
}

// ServeHTTPWithPrefix proxies the request stripping an optional route prefix.
func (p *ReverseProxy) ServeHTTPWithPrefix(req *httpparser.Request, res *httpparser.Response, prefix string) {
	var targetURL *url.URL
	if p.Balancer != nil {
		selected, err := p.Balancer.Next(req)
		if err != nil {
			p.writeBadGateway(res, fmt.Sprintf("Load balancer error: %v", err))
			return
		}
		targetURL = selected
	} else {
		targetURL = p.TargetURL
	}

	if targetURL == nil {
		p.writeBadGateway(res, "No upstream target available")
		return
	}

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

	// Dispatch request to upstream
	outResp, err := p.Client.Do(outReq)
	if err != nil {
		p.writeBadGateway(res, fmt.Sprintf("Upstream unreachable (%s): %v", targetURL.String(), err))
		return
	}
	defer outResp.Body.Close()

	// Copy upstream status code
	res.SetStatus(outResp.StatusCode)

	// Copy upstream headers
	for key, values := range outResp.Header {
		for _, val := range values {
			res.Header.Add(key, val)
		}
	}

	// Copy upstream body
	if outResp.Body != nil {
		_, _ = io.Copy(res.Body, outResp.Body)
	}
}

func (p *ReverseProxy) writeBadGateway(res *httpparser.Response, msg string) {
	res.SetStatus(http.StatusBadGateway)
	res.Header.Set("Content-Type", "application/json")
	res.Body.Reset()
	_, _ = res.WriteString(fmt.Sprintf(`{"error":"502 Bad Gateway","message":%q}`, msg))
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
