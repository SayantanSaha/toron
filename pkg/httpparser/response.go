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

func appendSanitizedHeader(buf []byte, s string) []byte {
	if strings.IndexByte(s, '\r') == -1 && strings.IndexByte(s, '\n') == -1 {
		return append(buf, s...)
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\r' && c != '\n' {
			buf = append(buf, c)
		}
	}
	return buf
}

// Pre-computed status lines for common HTTP response codes to achieve zero-allocation serialization.
var (
	statusLine200 = []byte("HTTP/1.1 200 OK\r\n")
	statusLine204 = []byte("HTTP/1.1 204 No Content\r\n")
	statusLine301 = []byte("HTTP/1.1 301 Moved Permanently\r\n")
	statusLine302 = []byte("HTTP/1.1 302 Found\r\n")
	statusLine304 = []byte("HTTP/1.1 304 Not Modified\r\n")
	statusLine400 = []byte("HTTP/1.1 400 Bad Request\r\n")
	statusLine401 = []byte("HTTP/1.1 401 Unauthorized\r\n")
	statusLine403 = []byte("HTTP/1.1 403 Forbidden\r\n")
	statusLine404 = []byte("HTTP/1.1 404 Not Found\r\n")
	statusLine500 = []byte("HTTP/1.1 500 Internal Server Error\r\n")
	statusLine502 = []byte("HTTP/1.1 502 Bad Gateway\r\n")
	statusLine503 = []byte("HTTP/1.1 503 Service Unavailable\r\n")
)

func appendStatusLine(buf []byte, code int) []byte {
	switch code {
	case 200:
		return append(buf, statusLine200...)
	case 204:
		return append(buf, statusLine204...)
	case 301:
		return append(buf, statusLine301...)
	case 302:
		return append(buf, statusLine302...)
	case 304:
		return append(buf, statusLine304...)
	case 400:
		return append(buf, statusLine400...)
	case 401:
		return append(buf, statusLine401...)
	case 403:
		return append(buf, statusLine403...)
	case 404:
		return append(buf, statusLine404...)
	case 500:
		return append(buf, statusLine500...)
	case 502:
		return append(buf, statusLine502...)
	case 503:
		return append(buf, statusLine503...)
	default:
		buf = append(buf, "HTTP/1.1 "...)
		buf = strconv.AppendInt(buf, int64(code), 10)
		buf = append(buf, ' ')
		statusText := http.StatusText(code)
		if statusText == "" {
			statusText = "Unknown"
		}
		buf = append(buf, statusText...)
		buf = append(buf, '\r', '\n')
		return buf
	}
}

// Thread-safe buffer pools for header serialization and stream copying
var bufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 8*1024))
	},
}

// responseBufPool manages recycled 4KB byte buffer slabs for HTTP response serialization (TASK-150 / ADR-127).
var responseBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 4096)
		return &b
	},
}

// GetResponseBuffer acquires a 4KB byte slice slab from responseBufPool.
func GetResponseBuffer() *[]byte {
	return getResponseBuf()
}

// PutResponseBuffer resets length to 0 and returns a byte slice slab to responseBufPool.
func PutResponseBuffer(b *[]byte) {
	putResponseBuf(b)
}

// GetResponseBuf is an alias for GetResponseBuffer.
func GetResponseBuf() *[]byte {
	return getResponseBuf()
}

// PutResponseBuf is an alias for PutResponseBuffer.
func PutResponseBuf(b *[]byte) {
	putResponseBuf(b)
}

func getResponseBuf() *[]byte {
	b := responseBufPool.Get().(*[]byte)
	*b = (*b)[:0]
	return b
}

func putResponseBuf(b *[]byte) {
	if b == nil {
		return
	}
	*b = (*b)[:0]
	responseBufPool.Put(b)
}

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
	bufPtr := getResponseBuf()
	defer putResponseBuf(bufPtr)

	buf := *bufPtr

	// 1. Format Status Line into pooled slab
	buf = appendStatusLine(buf, r.StatusCode)

	// 2. Format Headers into pooled slab with CRLF Injection Protection
	hasContentLength := false
	hasContentType := false

	for key, values := range r.Header {
		if strings.EqualFold(key, "Content-Length") {
			hasContentLength = true
			if r.StreamBody != nil {
				// Content-Length must be strictly omitted for streaming responses
				continue
			}
		}
		if strings.EqualFold(key, "Content-Type") {
			hasContentType = true
		}
		for _, val := range values {
			buf = appendSanitizedHeader(buf, key)
			buf = append(buf, ':', ' ')
			buf = appendSanitizedHeader(buf, val)
			buf = append(buf, '\r', '\n')
		}
	}

	if !hasContentType {
		buf = append(buf, "Content-Type: text/plain; charset=utf-8\r\n"...)
	}

	if !hasContentLength && r.StreamBody == nil && r.StatusCode != http.StatusSwitchingProtocols {
		buf = append(buf, "Content-Length: "...)
		bodyLen := 0
		if r.Body != nil {
			bodyLen = r.Body.Len()
		}
		buf = strconv.AppendInt(buf, int64(bodyLen), 10)
		buf = append(buf, '\r', '\n')
	}

	// 3. Header/Body separator
	buf = append(buf, '\r', '\n')

	// Update slice pointer before writing
	*bufPtr = buf

	// 4. Dual-Write Step 1: Write header block to wire
	if _, err := w.Write(buf); err != nil {
		return err
	}

	// 5. Streaming Hand-Off: If StreamBody != nil, delegate body writing to server loop
	if r.StreamBody != nil {
		return nil
	}

	// 6. Dual-Write Step 2: Write payload directly to wire without intermediate concatenation
	if r.Body != nil && r.Body.Len() > 0 {
		_, err := w.Write(r.Body.Bytes())
		return err
	}

	return nil
}
