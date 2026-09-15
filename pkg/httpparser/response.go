package httpparser

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

var headerReplacer = strings.NewReplacer("\r", "", "\n", "")

func sanitizeHeader(s string) string {
	if strings.IndexByte(s, '\r') == -1 && strings.IndexByte(s, '\n') == -1 {
		return s
	}
	return headerReplacer.Replace(s)
}

// Thread-safe buffer pools for header serialization and stream copying
var bufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 8*1024))
	},
}

// responseBufPool aliases bufferPool for header serialization slab recycling (TC-125.8)
var responseBufPool = &bufferPool

// GetBuffer retrieves a recycled bytes.Buffer from bufferPool.
func GetBuffer() *bytes.Buffer {
	return bufferPool.Get().(*bytes.Buffer)
}

// PutBuffer resets and returns a bytes.Buffer to bufferPool.
func PutBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	buf.Reset()
	bufferPool.Put(buf)
}

var copyBufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*1024)
		return &b
	},
}

// GetCopyBuffer retrieves a recycled 32KB slice from copyBufferPool.
func GetCopyBuffer() *[]byte {
	return copyBufferPool.Get().(*[]byte)
}

// PutCopyBuffer returns a 32KB slice to copyBufferPool.
func PutCopyBuffer(b *[]byte) {
	if b == nil {
		return
	}
	copyBufferPool.Put(b)
}

// Response represents an HTTP/1.1 response builder.
type Response struct {
	StatusCode   int
	Header       Header
	Body         *bytes.Buffer
	UpgradedConn net.Conn
	StreamBody   io.ReadCloser
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

// Serialize converts the Response object into valid HTTP/1.1 wire bytes and writes to w.
func (r *Response) Serialize(w io.Writer) error {
	statusText := http.StatusText(r.StatusCode)
	if statusText == "" {
		statusText = "Unknown"
	}

	if r.StreamBody != nil {
		// Streaming response: do NOT set static Content-Length.
		if r.Header.Get("Content-Type") == "" {
			r.Header.Set("Content-Type", "text/plain; charset=utf-8")
		}
	} else if r.StatusCode != http.StatusSwitchingProtocols {
		// Ensure Content-Length is present if not already set
		if r.Header.Get("Content-Length") == "" {
			r.Header.Set("Content-Length", strconv.Itoa(r.Body.Len()))
		}
		if r.Header.Get("Content-Type") == "" {
			r.Header.Set("Content-Type", "text/plain; charset=utf-8")
		}
	}

	buf := GetBuffer()
	defer PutBuffer(buf)

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

	// Write headers directly to writer
	if _, err := w.Write(buf.Bytes()); err != nil {
		return err
	}

	// For streaming responses, body relay is delegated to the server socket loop
	if r.StreamBody != nil {
		return nil
	}

	// Payload Body for non-streaming responses: write directly without monolithic combined buffer allocation
	if r.Body.Len() > 0 {
		_, err := w.Write(r.Body.Bytes())
		return err
	}

	return nil
}
