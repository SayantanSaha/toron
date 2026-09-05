package httpparser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var (
	ErrBadRequest                  = errors.New("httpparser: bad request format")
	ErrHeaderTooLarge              = errors.New("httpparser: request header fields too large")
	ErrBodyTooLarge                = errors.New("httpparser: request payload body too large")
	ErrUnsupportedProtocol         = errors.New("httpparser: unsupported HTTP protocol version")
	ErrUnsupportedTransferEncoding = errors.New("httpparser: unsupported transfer encoding")
)

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

	bufr := bufio.NewReader(r)

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
		if k == "" || strings.ContainsAny(k, " \t\r\n") {
			return nil, fmt.Errorf("%w: whitespace in header field-name", ErrBadRequest)
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
		bodyBuf := make([]byte, clInt)
		if _, err := io.ReadFull(bufr, bodyBuf); err != nil {
			return nil, fmt.Errorf("%w: unexpected EOF reading body", ErrBadRequest)
		}
		req.Body = bytes.NewReader(bodyBuf)
	}

	return req, nil
}

func readLineBounded(r *bufio.Reader, maxBytes int) (string, error) {
	var buf strings.Builder
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
