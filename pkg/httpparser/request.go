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
	return strings.ToLower(strings.TrimSpace(s))
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

// NewRequestFromStd converts a Go standard library http.Request into an httpparser.Request,
// preserving URL, QueryParams, Headers, and pseudo-headers.
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

