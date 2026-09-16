package httpparser

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Header represents HTTP request/response headers.
type Header map[string][]string

// Get returns the first header value associated with the given key.
func (h Header) Get(key string) string {
	if h == nil {
		return ""
	}
	values := h[canonicalKey(key)]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Values returns all values associated with the given key.
func (h Header) Values(key string) []string {
	if h == nil {
		return nil
	}
	return h[canonicalKey(key)]
}

// Set sets the header value associated with key to val.
func (h Header) Set(key, val string) {
	h[canonicalKey(key)] = []string{val}
}

// Add appends the header value associated with key.
func (h Header) Add(key, val string) {
	k := canonicalKey(key)
	h[k] = append(h[k], val)
}

// Del deletes the header values associated with key.
func (h Header) Del(key string) {
	if h != nil {
		delete(h, canonicalKey(key))
	}
}

func canonicalKey(s string) string {
	s = strings.TrimSpace(s)
	switch s {
	case "Accept", "accept":
		return "accept"
	case "Content-Type", "content-type":
		return "content-type"
	case "X-Accel-Buffering", "x-accel-buffering":
		return "x-accel-buffering"
	case "Cache-Control", "cache-control":
		return "cache-control"
	case "Accept-Encoding", "accept-encoding":
		return "accept-encoding"
	case "Content-Encoding", "content-encoding":
		return "content-encoding"
	case "Content-Length", "content-length":
		return "content-length"
	case "Connection", "connection":
		return "connection"
	case "Authorization", "authorization":
		return "authorization"
	case "Vary", "vary":
		return "vary"
	case "Pragma", "pragma":
		return "pragma"
	case "Host", "host":
		return "host"
	case "Upgrade", "upgrade":
		return "upgrade"
	case "Age", "age":
		return "age"
	case "X-Cache", "x-cache":
		return "x-cache"
	case "Set-Cookie", "set-cookie":
		return "set-cookie"
	case "Set-Cookie2", "set-cookie2":
		return "set-cookie2"
	case "Location", "location":
		return "location"
	case "TE", "te":
		return "te"
	case "Trailers", "trailers":
		return "trailers"
	case "Transfer-Encoding", "transfer-encoding":
		return "transfer-encoding"
	case "Proxy-Authenticate", "proxy-authenticate":
		return "proxy-authenticate"
	case "Proxy-Authorization", "proxy-authorization":
		return "proxy-authorization"
	case "Keep-Alive", "keep-alive":
		return "keep-alive"
	}
	return strings.ToLower(s)
}

// Request represents an HTTP/1.1 request.
type Request struct {
	Method        string
	RequestURI    string
	URL           *url.URL
	Path          string
	Proto         string
	Header        Header
	QueryParams   url.Values
	Body          io.Reader
	ContentLength int64
	RawConn       net.Conn
	RemoteAddr    string // Physical client network address ("IP:port") assigned at transport ingress
}

// IsWebSocketUpgrade returns true if the request contains WebSocket upgrade headers (HTTP/1.1) or RFC 8441 Extended CONNECT pseudo-headers (HTTP/2).
func (r *Request) IsWebSocketUpgrade() bool {
	if r == nil || r.Header == nil {
		return false
	}
	connHeader := strings.ToLower(r.Header.Get("Connection"))
	upgradeHeader := strings.ToLower(r.Header.Get("Upgrade"))
	if strings.Contains(connHeader, "upgrade") && upgradeHeader == "websocket" {
		return true
	}
	// RFC 8441 Extended CONNECT
	if r.Method == "CONNECT" && strings.ToLower(r.Header.Get(":protocol")) == "websocket" {
		return true
	}
	return false
}

// CloseBody closes the request body if it implements io.Closer.
func (r *Request) CloseBody() error {
	if r == nil || r.Body == nil {
		return nil
	}
	if closer, ok := r.Body.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// NewRequest creates a Request with initialized fields.
func NewRequest(method, reqURI, proto string) (*Request, error) {
	parsedURL, err := url.ParseRequestURI(reqURI)
	if err != nil {
		return nil, err
	}

	return &Request{
		Method:      strings.ToUpper(method),
		RequestURI:  reqURI,
		URL:         parsedURL,
		Path:        parsedURL.Path,
		Proto:       proto,
		Header:      make(Header),
		QueryParams: parsedURL.Query(),
		Body:        bytes.NewReader(nil),
	}, nil
}

// Query returns QueryParams, or lazily parses URL.Query() if QueryParams is nil.
func (r *Request) Query() url.Values {
	if r == nil {
		return nil
	}
	if r.QueryParams != nil {
		return r.QueryParams
	}
	if r.URL != nil {
		r.QueryParams = r.URL.Query()
		return r.QueryParams
	}
	return nil
}

// RemoteHost returns the host/IP portion of the physical connection address.
// It inspects RemoteAddr first, falling back to RawConn.RemoteAddr() if RemoteAddr is unset.
// Port numbers and IPv6 surrounding brackets are stripped safely.
// Returns an empty string if no valid host is present or if the receiver is nil.
func (r *Request) RemoteHost() string {
	if r == nil {
		return ""
	}
	addr := strings.TrimSpace(r.RemoteAddr)
	if addr == "" && r.RawConn != nil && r.RawConn.RemoteAddr() != nil {
		addr = strings.TrimSpace(r.RawConn.RemoteAddr().String())
	}
	if addr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	// Fallback for bare IPs or malformed addresses
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = addr[1 : len(addr)-1]
	}
	return addr
}

// RemoteIP parses and returns the physical client net.IP from RemoteHost().
// Supports both IPv4 and IPv6 representations. Returns nil if the host is empty or unparseable.
func (r *Request) RemoteIP() net.IP {
	if r == nil {
		return nil
	}
	host := r.RemoteHost()
	if host == "" {
		return nil
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	return net.ParseIP(host)
}

// NewRequestFromStd converts a Go standard library http.Request into an httpparser.Request,
// preserving URL, QueryParams, Headers, pseudo-headers, and physical RemoteAddr.
func NewRequestFromStd(r *http.Request) *Request {
	if r == nil {
		return nil
	}
	reqURI := r.RequestURI
	if reqURI == "" && r.URL != nil {
		reqURI = r.URL.RequestURI()
	}
	if reqURI == "" && r.URL != nil {
		reqURI = r.URL.Path
	}
	path := ""
	var queryParams url.Values
	if r.URL != nil {
		path = r.URL.Path
		queryParams = r.URL.Query()
	}
	req := &Request{
		Method:      r.Method,
		RequestURI:  reqURI,
		URL:         r.URL,
		Path:        path,
		Proto:       r.Proto,
		Header:      make(Header),
		QueryParams: queryParams,
		Body:        bytes.NewReader(nil),
		RemoteAddr:  r.RemoteAddr,
	}
	for k, vv := range r.Header {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	if r.Host != "" {
		req.Header.Set("Host", r.Host)
	}
	if protoHeader := r.Header.Get(":protocol"); protoHeader != "" {
		req.Header.Set(":protocol", protoHeader)
	}
	return req
}

// ValidateHTTP2Request validates semantic translation invariants on an incoming HTTP/2 request (RFC 7540 §8.1.2).
// It rejects forbidden connection-specific headers, conflicting Content-Length declarations, and CRLF/NUL injection.
func ValidateHTTP2Request(r *http.Request) error {
	if r == nil {
		return nil
	}

	// 1. CRLF and NUL character injection check on request-line elements
	if hasCRLFOrNUL(r.Method) || hasCRLFOrNUL(r.RequestURI) || hasCRLFOrNUL(r.Host) {
		return ErrHTTP2CRLFInjection
	}
	if r.URL != nil {
		if hasCRLFOrNUL(r.URL.Path) || hasCRLFOrNUL(r.URL.RawQuery) {
			return ErrHTTP2CRLFInjection
		}
	}

	// 2. CRLF and NUL check on headers, plus RFC 7540 §8.1.2.2 forbidden connection headers
	for k, vv := range r.Header {
		if hasCRLFOrNUL(k) {
			return ErrHTTP2CRLFInjection
		}
		for _, v := range vv {
			if hasCRLFOrNUL(v) {
				return ErrHTTP2CRLFInjection
			}
		}

		lk := strings.ToLower(k)
		switch lk {
		case "transfer-encoding", "connection", "keep-alive", "proxy-connection":
			return ErrHTTP2ForbiddenHeader
		case "upgrade":
			// RFC 8441: Upgrade is ONLY permitted if it's an extended CONNECT with :protocol
			hasProto := r.Header.Get(":protocol") != ""
			if !(r.Method == "CONNECT" && hasProto) {
				return ErrHTTP2ForbiddenHeader
			}
		}
	}

	// Also check r.TransferEncoding slice if populated by stdlib
	if len(r.TransferEncoding) > 0 {
		return ErrHTTP2ForbiddenHeader
	}

	// 3. Content-Length invariant validation (H2.CL)
	clHeaders := r.Header["Content-Length"]
	if len(clHeaders) > 1 {
		return ErrHTTP2MultipleContentLength
	}
	if len(clHeaders) == 1 {
		clStr := strings.TrimSpace(clHeaders[0])
		if strings.Contains(clStr, ",") {
			return ErrHTTP2MultipleContentLength
		}
		if clStr == "" {
			return ErrHTTP2MultipleContentLength
		}
		for _, ch := range clStr {
			if ch < '0' || ch > '9' {
				return ErrHTTP2MultipleContentLength
			}
		}
	}

	return nil
}

func hasCRLFOrNUL(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\r' || c == '\n' || c == 0x00 {
			return true
		}
	}
	return false
}
