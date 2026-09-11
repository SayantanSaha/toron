package httpparser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrBadRequest                  = errors.New("httpparser: bad request format")
	ErrHeaderTooLarge              = errors.New("httpparser: request header fields too large")
	ErrBodyTooLarge                = errors.New("httpparser: request payload body too large")
	ErrUnsupportedProtocol         = errors.New("httpparser: unsupported HTTP protocol version")
	ErrUnsupportedTransferEncoding = errors.New("httpparser: unsupported transfer encoding")
	ErrHTTP2ForbiddenHeader        = fmt.Errorf("%w: forbidden connection-specific header in HTTP/2 request (RFC 7540 §8.1.2.2)", ErrBadRequest)
	ErrHTTP2MultipleContentLength  = fmt.Errorf("%w: multiple or conflicting Content-Length headers in HTTP/2 request", ErrBadRequest)
	ErrHTTP2ContentLengthMismatch  = fmt.Errorf("%w: Content-Length does not match received body length in HTTP/2 request", ErrBadRequest)
	ErrHTTP2CRLFInjection          = fmt.Errorf("%w: CRLF or NUL injection detected in HTTP/2 request", ErrBadRequest)
)

// maxPooledBodySize defines the maximum body payload size (64 KB) handled by bodyBufferPool.
const maxPooledBodySize = 64 * 1024

var (
	lineBufferPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 1024))
		},
	}

	bodyBufferPool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, maxPooledBodySize)
			return &b
		},
	}
)

type pooledBodyReader struct {
	r      *bytes.Reader
	bufPtr *[]byte
	once   sync.Once
}

func newPooledBodyReader(b []byte, bufPtr *[]byte) *pooledBodyReader {
	return &pooledBodyReader{
		r:      bytes.NewReader(b),
		bufPtr: bufPtr,
	}
}

func (p *pooledBodyReader) Read(b []byte) (int, error) {
	return p.r.Read(b)
}

func (p *pooledBodyReader) Close() error {
	p.once.Do(func() {
		if p.bufPtr != nil {
			bodyBufferPool.Put(p.bufPtr)
			p.bufPtr = nil
		}
	})
	return nil
}

// validHeaderTokenTable defines RFC 7230 §3.2.6 token characters:
// token = 1*tchar
// tchar = "!" / "#" / "$" / "%" / "&" / "'" / "*" / "+" / "-" / "." /
//         "^" / "_" / "`" / "|" / "~" / DIGIT / ALPHA
var validHeaderTokenTable = func() [256]bool {
	var table [256]bool
	for c := '0'; c <= '9'; c++ {
		table[c] = true
	}
	for c := 'a'; c <= 'z'; c++ {
		table[c] = true
	}
	for c := 'A'; c <= 'Z'; c++ {
		table[c] = true
	}
	for _, c := range []byte("!#$%&'*+-.^_`|~") {
		table[c] = true
	}
	return table
}()

// ParserOptions holds security limit configurations for the parser.
type ParserOptions struct {
	MaxHeaderBytes int
	MaxBodyBytes   int64
}

// DefaultParserOptions returns safe default parsing limits.
func DefaultParserOptions() ParserOptions {
	return ParserOptions{
		MaxHeaderBytes: 8 * 1024,        // 8 KB max headers
		MaxBodyBytes:   4 * 1024 * 1024, // 4 MB max body
	}
}

// ParseRequest parses an HTTP/1.1 request from an io.Reader according to ParserOptions limits.
func ParseRequest(r io.Reader, opts ParserOptions) (*Request, error) {
	if opts.MaxHeaderBytes <= 0 {
		opts.MaxHeaderBytes = 8 * 1024
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = 4 * 1024 * 1024
	}

	var bufr *bufio.Reader
	if br, ok := r.(*bufio.Reader); ok {
		bufr = br
	} else {
		bufr = bufio.NewReader(r)
	}

	// Read Request Line (e.g. "GET /index.html HTTP/1.1\r\n")
	requestLine, err := readLineBounded(bufr, opts.MaxHeaderBytes)
	if err != nil {
		if errors.Is(err, ErrHeaderTooLarge) {
			return nil, ErrHeaderTooLarge
		}
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}

	parts := strings.Split(strings.TrimRight(requestLine, "\r\n"), " ")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: invalid request line", ErrBadRequest)
	}

	method, reqURI, proto := parts[0], parts[1], parts[2]
	if !strings.HasPrefix(proto, "HTTP/1.") {
		return nil, ErrUnsupportedProtocol
	}

	req, err := NewRequest(method, reqURI, proto)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid URI", ErrBadRequest)
	}

	// Read Headers
	headerBytesCount := len(requestLine)
	for {
		line, err := readLineBounded(bufr, opts.MaxHeaderBytes-headerBytesCount)
		if err != nil {
			return nil, err
		}

		lineTrimmed := strings.TrimRight(line, "\r\n")
		if lineTrimmed == "" {
			break // End of HTTP headers
		}

		headerBytesCount += len(line)
		if headerBytesCount > opts.MaxHeaderBytes {
			return nil, ErrHeaderTooLarge
		}

		colonIdx := strings.IndexByte(lineTrimmed, ':')
		if colonIdx == -1 {
			return nil, fmt.Errorf("%w: malformed header line", ErrBadRequest)
		}

		k := lineTrimmed[:colonIdx]
		if k == "" {
			return nil, fmt.Errorf("%w: whitespace in header field-name", ErrBadRequest)
		}
		for i := 0; i < len(k); i++ {
			b := k[i]
			if !validHeaderTokenTable[b] {
				if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
					return nil, fmt.Errorf("%w: whitespace in header field-name", ErrBadRequest)
				}
				return nil, fmt.Errorf("%w: invalid header field-name token grammar", ErrBadRequest)
			}
		}
		v := strings.TrimSpace(lineTrimmed[colonIdx+1:])
		req.Header.Add(k, v)
	}

	// HTTP Request Smuggling Prevention (RFC 7230 §3.3.3)
	clValues := req.Header.Values("Content-Length")
	if req.Header.Get("Transfer-Encoding") != "" {
		if len(clValues) > 0 {
			return nil, fmt.Errorf("%w: conflicting Content-Length and Transfer-Encoding headers", ErrBadRequest)
		}
		return nil, fmt.Errorf("%w: chunked or custom transfer-encoding is not supported", ErrUnsupportedTransferEncoding)
	}

	// Determine Body Length & Enforce RFC 7230 §3.3.2 (Multiple/Conflicting Content-Length)
	if len(clValues) > 0 {
		var clInt int64 = -1
		for _, clRaw := range clValues {
			parts := strings.Split(clRaw, ",")
			for _, part := range parts {
				p := strings.TrimSpace(part)
				if p == "" {
					return nil, fmt.Errorf("%w: empty Content-Length token", ErrBadRequest)
				}
				val, err := strconv.ParseInt(p, 10, 64)
				if err != nil || val < 0 {
					return nil, fmt.Errorf("%w: invalid Content-Length: %q", ErrBadRequest, p)
				}
				if clInt != -1 && val != clInt {
					return nil, fmt.Errorf("%w: conflicting Content-Length values (%d vs %d)", ErrBadRequest, clInt, val)
				}
				clInt = val
			}
		}

		if clInt > opts.MaxBodyBytes {
			return nil, ErrBodyTooLarge
		}
		req.ContentLength = clInt
		req.Header.Set("Content-Length", strconv.FormatInt(clInt, 10))

		// Read Body payload
		if clInt <= maxPooledBodySize {
			bufPtr := bodyBufferPool.Get().(*[]byte)
			buf := *bufPtr
			if _, err := io.ReadFull(bufr, buf[:clInt]); err != nil {
				bodyBufferPool.Put(bufPtr)
				return nil, fmt.Errorf("%w: unexpected EOF reading body", ErrBadRequest)
			}
			req.Body = newPooledBodyReader(buf[:clInt], bufPtr)
		} else {
			bodyBuf := make([]byte, clInt)
			if _, err := io.ReadFull(bufr, bodyBuf); err != nil {
				return nil, fmt.Errorf("%w: unexpected EOF reading body", ErrBadRequest)
			}
			req.Body = io.NopCloser(bytes.NewReader(bodyBuf))
		}
	}

	return req, nil
}

func readLineBounded(r *bufio.Reader, maxBytes int) (string, error) {
	buf := lineBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer lineBufferPool.Put(buf)

	for {
		b, err := r.ReadByte()
		if err != nil {
			return buf.String(), err
		}
		buf.WriteByte(b)
		if buf.Len() > maxBytes {
			return "", ErrHeaderTooLarge
		}
		if b == '\n' {
			break
		}
	}
	return buf.String(), nil
}
