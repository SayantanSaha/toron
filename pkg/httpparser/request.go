package httpparser

import (
	"bytes"
	"io"
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

// Set sets the header value associated with key to val.
func (h Header) Set(key, val string) {
	h[canonicalKey(key)] = []string{val}
}

// Add appends the header value associated with key.
func (h Header) Add(key, val string) {
	k := canonicalKey(key)
	h[k] = append(h[k], val)
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
