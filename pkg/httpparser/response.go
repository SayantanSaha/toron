package httpparser

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// Response represents an HTTP/1.1 response builder.
type Response struct {
	StatusCode int
	Header     Header
	Body       *bytes.Buffer
}

// NewResponse initializes an HTTP response with default status 200 OK.
func NewResponse() *Response {
	return &Response{
		StatusCode: http.StatusOK,
		Header:     make(Header),
		Body:       bytes.NewBuffer(nil),
	}
}

// SetStatus sets the HTTP response status code.
func (r *Response) SetStatus(code int) {
	r.StatusCode = code
}

// Write appends bytes to the response body buffer.
func (r *Response) Write(p []byte) (int, error) {
	return r.Body.Write(p)
}

// WriteString appends a string to the response body buffer.
func (r *Response) WriteString(s string) (int, error) {
	return r.Body.WriteString(s)
}

// Serialize converts the Response object into valid HTTP/1.1 wire bytes and writes atomically to w.
func (r *Response) Serialize(w io.Writer) error {
	statusText := http.StatusText(r.StatusCode)
	if statusText == "" {
		statusText = "Unknown"
	}

	// Ensure Content-Length is present if not already set
	if r.Header.Get("Content-Length") == "" {
		r.Header.Set("Content-Length", strconv.Itoa(r.Body.Len()))
	}
	if r.Header.Get("Content-Type") == "" {
		r.Header.Set("Content-Type", "text/plain; charset=utf-8")
	}

	var buf bytes.Buffer
	buf.Grow(256 + r.Body.Len())

	// Write Status Line
	fmt.Fprintf(&buf, "HTTP/1.1 %d %s\r\n", r.StatusCode, statusText)

	// Write Headers
	for key, values := range r.Header {
		for _, val := range values {
			fmt.Fprintf(&buf, "%s: %s\r\n", key, val)
		}
	}

	// Header/Body separator
	buf.WriteString("\r\n")

	// Payload Body
	if r.Body.Len() > 0 {
		buf.Write(r.Body.Bytes())
	}

	_, err := w.Write(buf.Bytes())
	return err
}
