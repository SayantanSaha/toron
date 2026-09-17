package httpparser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
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

// GetBodyBuffer acquires a pooled 64KB byte slice pointer from bodyBufferPool.
func GetBodyBuffer() *[]byte {
	return bodyBufferPool.Get().(*[]byte)
}

// PutBodyBuffer recycles a pooled 64KB byte slice pointer into bodyBufferPool.
func PutBodyBuffer(b *[]byte) {
	if b != nil {
		bodyBufferPool.Put(b)
	}
}

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
//
//	"^" / "_" / "`" / "|" / "~" / DIGIT / ALPHA
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
		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return nil, err
		}
		var netErr net.Error
		if errors.As(err, &netErr) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}

	requestLineTrimmed := trimLineEnding(requestLine)
	if strings.ContainsAny(requestLineTrimmed, "\r\n") {
		return nil, fmt.Errorf("%w: bare CR or LF in request line", ErrBadRequest)
	}

	parts := strings.Split(requestLineTrimmed, " ")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: invalid request line", ErrBadRequest)
	}

	method, reqURI, proto := parts[0], parts[1], parts[2]
	if proto != "HTTP/1.1" && proto != "HTTP/1.0" {
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

		lineTrimmed := trimLineEnding(line)
		if lineTrimmed == "" {
			break // End of HTTP headers
		}

		if strings.ContainsAny(lineTrimmed, "\r\n") {
			return nil, fmt.Errorf("%w: bare CR or LF in header line", ErrBadRequest)
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
		afterColon := lineTrimmed[colonIdx+1:]
		if strings.EqualFold(k, "Transfer-Encoding") && strings.HasPrefix(afterColon, "\t") {
			return nil, fmt.Errorf("%w: horizontal tab after colon in Transfer-Encoding header", ErrBadRequest)
		}
		for i := 0; i < len(afterColon); i++ {
			b := afterColon[i]
			if (b < 0x20 && b != '\t') || b == 0x7f {
				return nil, fmt.Errorf("%w: control character in header value", ErrBadRequest)
			}
		}
		v := strings.TrimSpace(afterColon)
		req.Header.Add(k, v)
	}

	// HTTP Request Smuggling Prevention (RFC 9112 §6.3 / RFC 7230 §3.3.3)
	clValues := req.Header.Values("Content-Length")
	teValues := req.Header.Values("Transfer-Encoding")
	if len(teValues) > 0 && len(clValues) > 0 {
		return nil, fmt.Errorf("%w: conflicting Content-Length and Transfer-Encoding headers", ErrBadRequest)
	}

	if len(teValues) > 0 {
		var codings []string
		for _, teRaw := range teValues {
			trimmed := strings.TrimSpace(teRaw)
			if trimmed == "" {
				return nil, fmt.Errorf("%w: empty Transfer-Encoding header", ErrBadRequest)
			}
			parts := strings.Split(trimmed, ",")
			for _, part := range parts {
				c := strings.ToLower(strings.TrimSpace(part))
				if c == "" {
					return nil, fmt.Errorf("%w: empty transfer-coding in Transfer-Encoding header", ErrBadRequest)
				}
				codings = append(codings, c)
			}
		}

		if len(codings) == 0 {
			return nil, fmt.Errorf("%w: empty Transfer-Encoding header", ErrBadRequest)
		}

		finalCoding := codings[len(codings)-1]
		if finalCoding != "chunked" {
			return nil, fmt.Errorf("%w: final transfer-coding is %q (must be chunked)", ErrUnsupportedTransferEncoding, finalCoding)
		}

		chunkedCount := 0
		for _, c := range codings {
			if c == "chunked" {
				chunkedCount++
			} else {
				return nil, fmt.Errorf("%w: unsupported transfer-coding %q", ErrUnsupportedTransferEncoding, c)
			}
		}

		if chunkedCount > 1 {
			return nil, fmt.Errorf("%w: chunked transfer-coding applied more than once", ErrBadRequest)
		}

		req.ContentLength = -1
		var closer io.Closer
		if c, ok := r.(io.Closer); ok {
			closer = c
		}
		req.Body = newChunkedBodyReader(bufr, closer, opts.MaxBodyBytes, req.Header)
		return req, nil
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

// trimLineEnding strips exactly one trailing CRLF or LF line ending without stripping bare CR/LF characters.
func trimLineEnding(line string) string {
	if strings.HasSuffix(line, "\r\n") {
		return line[:len(line)-2]
	}
	if strings.HasSuffix(line, "\n") {
		return line[:len(line)-1]
	}
	return line
}
