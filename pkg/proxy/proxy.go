package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"toron/pkg/httpparser"
)

// ReverseProxy handles proxying HTTP requests to an upstream target URL.
type ReverseProxy struct {
	TargetURL *url.URL
	Client    *http.Client
}

// NewReverseProxy creates a ReverseProxy instance for a target URL string.
func NewReverseProxy(targetURLStr string, timeout time.Duration) (*ReverseProxy, error) {
	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		return nil, fmt.Errorf("proxy: invalid target URL %q: %w", targetURLStr, err)
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
		TargetURL: targetURL,
		Client:    client,
	}, nil
}

// ServeHTTP translates a Toron Request, proxies it to the upstream server, and writes the upstream response to Res.
func (p *ReverseProxy) ServeHTTP(req *httpparser.Request, res *httpparser.Response) {
	p.ServeHTTPWithPrefix(req, res, "")
}

// ServeHTTPWithPrefix proxies the request stripping an optional route prefix.
func (p *ReverseProxy) ServeHTTPWithPrefix(req *httpparser.Request, res *httpparser.Response, prefix string) {
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

	outURL := *p.TargetURL
	outURL.Path = singleJoiningSlash(p.TargetURL.Path, relPath)
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
		p.writeBadGateway(res, fmt.Sprintf("Upstream unreachable (%s): %v", p.TargetURL.String(), err))
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
