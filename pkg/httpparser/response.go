package httpparser

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

var headerReplacer = strings.NewReplacer("\r", "", "\n", "")

func sanitizeHeader(s string) string {
	if strings.IndexByte(s, '\r') == -1 && strings.IndexByte(s, '\n') == -1 {
		return s
	}
	return headerReplacer.Replace(s)
}

// Response represents an HTTP/1.1 response builder.
type Response struct {
	StatusCode   int
	Header       Header
	Body         *bytes.Buffer
	UpgradedConn net.Conn
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

	if r.StatusCode != http.StatusSwitchingProtocols {
		// Ensure Content-Length is present if not already set
		if r.Header.Get("Content-Length") == "" {
			r.Header.Set("Content-Length", strconv.Itoa(r.Body.Len()))
		}
		if r.Header.Get("Content-Type") == "" {
			r.Header.Set("Content-Type", "text/plain; charset=utf-8")
		}
	}

	var buf bytes.Buffer
	buf.Grow(256 + r.Body.Len())

	// Write Status Line
	buf.WriteString("HTTP/1.1 ")
	buf.WriteString(strconv.Itoa(r.StatusCode))
	buf.WriteString(" ")
	buf.WriteString(statusText)
	buf.WriteString("\r\n")

	// Write Headers with CRLF Injection Sanitization
	for key, values := range r.Header {
		cleanKey := sanitizeHeader(key)
		for _, val := range values {
			cleanVal := sanitizeHeader(val)
			buf.WriteString(cleanKey)
			buf.WriteString(": ")
			buf.WriteString(cleanVal)
			buf.WriteString("\r\n")
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
