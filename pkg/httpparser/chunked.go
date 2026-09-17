package httpparser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
)

const (
	// MaxChunkExtensionBytes defines the maximum cumulative bytes permitted for chunk extensions on a chunk-size line.
	MaxChunkExtensionBytes = 256

	// MaxTrailerBytes defines the maximum cumulative size permitted for all trailer headers combined (4 KB).
	MaxTrailerBytes = 4096

	// MaxDrainBytes defines the maximum bytes drained upon Close() to preserve keep-alive (64 KB).
	MaxDrainBytes = 65536
)

type chunkedReaderState int

const (
	stateChunkSize chunkedReaderState = iota
	stateChunkData
	stateChunkCRLF
	stateTrailerSection
	stateDone
)

// ChunkedBodyReader implements a zero-tolerance streaming chunked transfer-coding decoder (RFC 9112 §7.1).
type ChunkedBodyReader struct {
	r              *bufio.Reader
	closer         io.Closer
	state          chunkedReaderState
	chunkRemaining int64
	totalBodyRead  int64
	maxBodyBytes   int64
	reqHeader      Header
	err            error
	closed         bool
	trailers       http.Header
}

// chunkedBodyReader is an unexported alias for ChunkedBodyReader to satisfy both naming conventions.
type chunkedBodyReader = ChunkedBodyReader

var _ io.ReadCloser = (*ChunkedBodyReader)(nil)

// NewChunkedBodyReader creates a new ChunkedBodyReader.
func NewChunkedBodyReader(r *bufio.Reader, closer io.Closer, maxBodyBytes int64) *ChunkedBodyReader {
	return newChunkedBodyReader(r, closer, maxBodyBytes, nil)
}

// newChunkedBodyReader initializes a ChunkedBodyReader with optional request header reference for trailer merging.
func newChunkedBodyReader(r *bufio.Reader, closer io.Closer, maxBodyBytes int64, reqHeader Header) *ChunkedBodyReader {
	return &ChunkedBodyReader{
		r:            r,
		closer:       closer,
		state:        stateChunkSize,
		maxBodyBytes: maxBodyBytes,
		reqHeader:    reqHeader,
		trailers:     make(http.Header),
	}
}

// SetCloser configures or updates the underlying connection closer for fail-fast teardown.
func (cr *ChunkedBodyReader) SetCloser(c io.Closer) {
	cr.closer = c
}

// Trailers returns the trailer headers decoded after the final chunk.
func (cr *ChunkedBodyReader) Trailers() http.Header {
	if cr.trailers == nil {
		return make(http.Header)
	}
	return cr.trailers
}

// Read decodes chunked transfer bytes into p, maintaining strict RFC 9112 state transitions.
func (cr *ChunkedBodyReader) Read(p []byte) (int, error) {
	if cr.closed {
		return 0, io.EOF
	}
	if cr.err != nil {
		return 0, cr.err
	}
	if len(p) == 0 {
		return 0, nil
	}

	n := 0
	for n < len(p) {
		switch cr.state {
		case stateChunkSize:
			size, err := cr.parseChunkSizeLine()
			if err != nil {
				cr.err = err
				if cr.closer != nil {
					_ = cr.closer.Close()
				}
				if n > 0 {
					return n, nil
				}
				return 0, cr.err
			}

			if size == 0 {
				cr.state = stateTrailerSection
			} else {
				cr.chunkRemaining = size
				cr.state = stateChunkData
			}

		case stateChunkData:
			if cr.maxBodyBytes > 0 && cr.totalBodyRead >= cr.maxBodyBytes {
				cr.err = ErrBodyTooLarge
				if cr.closer != nil {
					_ = cr.closer.Close()
				}
				if n > 0 {
					return n, nil
				}
				return 0, ErrBodyTooLarge
			}

			toRead := len(p) - n
			if int64(toRead) > cr.chunkRemaining {
				toRead = int(cr.chunkRemaining)
			}

			readCount, readErr := cr.r.Read(p[n : n+toRead])
			if readCount > 0 {
				n += readCount
				cr.chunkRemaining -= int64(readCount)
				cr.totalBodyRead += int64(readCount)

				if cr.maxBodyBytes > 0 && cr.totalBodyRead > cr.maxBodyBytes {
					cr.err = ErrBodyTooLarge
					if cr.closer != nil {
						_ = cr.closer.Close()
					}
					return n, ErrBodyTooLarge
				}
			}

			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					cr.err = io.ErrUnexpectedEOF
				} else {
					cr.err = readErr
				}
				if cr.closer != nil {
					_ = cr.closer.Close()
				}
				if n > 0 {
					return n, nil
				}
				return 0, cr.err
			}

			if cr.chunkRemaining == 0 {
				cr.state = stateChunkCRLF
			}

		case stateChunkCRLF:
			var crlf [2]byte
			if _, err := io.ReadFull(cr.r, crlf[:]); err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					cr.err = fmt.Errorf("%w: unexpected EOF reading chunk CRLF delimiter", ErrBadRequest)
				} else {
					cr.err = err
				}
				if cr.closer != nil {
					_ = cr.closer.Close()
				}
				if n > 0 {
					return n, nil
				}
				return 0, cr.err
			}

			if crlf[0] != '\r' || crlf[1] != '\n' {
				cr.err = fmt.Errorf("%w: malformed chunk CRLF delimiter", ErrBadRequest)
				if cr.closer != nil {
					_ = cr.closer.Close()
				}
				if n > 0 {
					return n, nil
				}
				return 0, cr.err
			}

			cr.state = stateChunkSize

		case stateTrailerSection:
			if err := cr.parseTrailers(); err != nil {
				cr.err = err
				if cr.closer != nil {
					_ = cr.closer.Close()
				}
				if n > 0 {
					return n, nil
				}
				return 0, cr.err
			}
			cr.state = stateDone

		case stateDone:
			if n > 0 {
				return n, nil
			}
			return 0, io.EOF
		}
	}

	return n, nil
}

// parseChunkSizeLine reads and parses a strict RFC 9112 §7.1 chunk size line.
func (cr *ChunkedBodyReader) parseChunkSizeLine() (int64, error) {
	var lineBuf [512]byte
	idx := 0
	for {
		b, err := cr.r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0, io.ErrUnexpectedEOF
			}
			return 0, err
		}
		if idx < len(lineBuf) {
			lineBuf[idx] = b
			idx++
		} else {
			return 0, fmt.Errorf("%w: chunk size line exceeds maximum limit", ErrBadRequest)
		}
		if b == '\n' {
			break
		}
	}

	if idx < 2 || lineBuf[idx-2] != '\r' {
		return 0, fmt.Errorf("%w: bare LF in chunk size line", ErrBadRequest)
	}

	line := lineBuf[:idx-2]
	if len(line) == 0 {
		return 0, fmt.Errorf("%w: empty chunk size line", ErrBadRequest)
	}

	// Split hex size token and chunk extension at semicolon ';'
	semiIdx := -1
	for i := 0; i < len(line); i++ {
		if line[i] == ';' {
			semiIdx = i
			break
		}
	}

	var hexBytes []byte
	var extBytes []byte
	if semiIdx >= 0 {
		hexBytes = line[:semiIdx]
		extBytes = line[semiIdx:]
	} else {
		hexBytes = line
	}

	// Validate chunk-size token grammar: 1*HEXDIG
	if len(hexBytes) == 0 {
		return 0, fmt.Errorf("%w: empty chunk-size token", ErrBadRequest)
	}

	// Reject leading signs ('+' or '-')
	if hexBytes[0] == '+' || hexBytes[0] == '-' {
		return 0, fmt.Errorf("%w: signed hex chunk size not permitted (%q)", ErrBadRequest, hexBytes)
	}

	// Reject leading whitespace/tab
	if hexBytes[0] == ' ' || hexBytes[0] == '\t' {
		return 0, fmt.Errorf("%w: leading whitespace in chunk size", ErrBadRequest)
	}

	// If extension exists, BWS (trailing space/tab) before ';' is permitted per RFC 9112 §7.1
	// If no extension exists, trailing whitespace is forbidden
	trimmedHex := string(hexBytes)
	if semiIdx >= 0 {
		trimmedHex = strings.TrimRight(string(hexBytes), " \t")
	}

	if len(trimmedHex) == 0 {
		return 0, fmt.Errorf("%w: empty hex chunk size token", ErrBadRequest)
	}

	for i := 0; i < len(trimmedHex); i++ {
		c := trimmedHex[i]
		if !isHexDigit(c) {
			return 0, fmt.Errorf("%w: invalid hex character %q in chunk size", ErrBadRequest, c)
		}
	}

	size, err := strconv.ParseUint(trimmedHex, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid chunk size hex: %v", ErrBadRequest, err)
	}
	if size > math.MaxInt64 {
		return 0, fmt.Errorf("%w: chunk size exceeds max int64", ErrBadRequest)
	}

	// Validate chunk extension bounds and character sets
	if extBytes != nil {
		// Measured extension bytes (including ';' and any BWS)
		totalExtBytes := len(line) - len(trimmedHex)
		if totalExtBytes >= MaxChunkExtensionBytes {
			return 0, fmt.Errorf("%w: chunk extension length %d exceeds max limit (%d)", ErrBadRequest, totalExtBytes, MaxChunkExtensionBytes)
		}

		for i := 0; i < len(extBytes); i++ {
			c := extBytes[i]
			if (c < 0x20 && c != '\t') || c == 0x7F {
				return 0, fmt.Errorf("%w: control character 0x%02x in chunk extension", ErrBadRequest, c)
			}
		}
	}

	return int64(size), nil
}

// parseTrailers parses trailing headers following a terminal 0\r\n chunk.
func (cr *ChunkedBodyReader) parseTrailers() error {
	totalTrailerBytes := 0

	for {
		var lineBuf [1024]byte
		idx := 0
		for {
			b, err := cr.r.ReadByte()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return io.ErrUnexpectedEOF
				}
				return err
			}
			if idx < len(lineBuf) {
				lineBuf[idx] = b
				idx++
			} else {
				return fmt.Errorf("%w: trailer line exceeds maximum length", ErrHeaderTooLarge)
			}
			if b == '\n' {
				break
			}
		}

		if idx < 2 || lineBuf[idx-2] != '\r' {
			return fmt.Errorf("%w: bare LF in trailer header line", ErrBadRequest)
		}

		line := lineBuf[:idx-2]
		totalTrailerBytes += idx
		if totalTrailerBytes > MaxTrailerBytes {
			return fmt.Errorf("%w: total trailer headers size exceeds %d bytes", ErrHeaderTooLarge, MaxTrailerBytes)
		}

		if len(line) == 0 {
			// Blank line "\r\n" indicates end of trailers
			break
		}

		colonIdx := -1
		startSearch := 0
		if len(line) > 0 && line[0] == ':' {
			startSearch = 1
		}
		if idx := bytes.IndexByte(line[startSearch:], ':'); idx != -1 {
			colonIdx = startSearch + idx
		}

		if colonIdx == -1 {
			return fmt.Errorf("%w: malformed trailer line (missing colon)", ErrBadRequest)
		}

		k := strings.TrimSpace(string(line[:colonIdx]))
		if k == "" {
			return fmt.Errorf("%w: empty trailer header name", ErrBadRequest)
		}

		if isForbiddenTrailer(k) {
			return fmt.Errorf("%w: prohibited trailer header %q (RFC 9112 §7.1.2)", ErrBadRequest, k)
		}

		for i := 0; i < len(k); i++ {
			if !validHeaderTokenTable[k[i]] {
				return fmt.Errorf("%w: invalid character in trailer header name %q", ErrBadRequest, k)
			}
		}

		v := strings.TrimSpace(string(line[colonIdx+1:]))
		for i := 0; i < len(v); i++ {
			c := v[i]
			if (c < 0x20 && c != '\t') || c == 0x7F {
				return fmt.Errorf("%w: control character in trailer header %q value", ErrBadRequest, k)
			}
		}

		if cr.reqHeader != nil {
			cr.reqHeader.Add(k, v)
		}
		if cr.trailers == nil {
			cr.trailers = make(http.Header)
		}
		cr.trailers.Add(k, v)
	}

	return nil
}

// Close drains unconsumed body bytes up to MaxDrainBytes or closes the physical connection.
func (cr *ChunkedBodyReader) Close() error {
	if cr.closed {
		return nil
	}
	defer func() {
		cr.closed = true
	}()

	if cr.state == stateDone {
		return nil
	}

	if cr.err != nil {
		if cr.closer != nil {
			_ = cr.closer.Close()
		}
		return cr.err
	}

	// Attempt bounded socket drainage
	drainBuf := make([]byte, 4096)
	var drained int64
	for cr.state != stateDone && drained < MaxDrainBytes {
		toDrain := int64(len(drainBuf))
		if MaxDrainBytes-drained < toDrain {
			toDrain = MaxDrainBytes - drained
		}
		n, err := cr.Read(drainBuf[:toDrain])
		drained += int64(n)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			break
		}
	}

	if cr.state != stateDone {
		if cr.closer != nil {
			_ = cr.closer.Close()
		}
	}

	return nil
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isForbiddenTrailer(key string) bool {
	k := strings.TrimSpace(key)
	if strings.HasPrefix(k, ":") {
		return true
	}
	switch strings.ToLower(k) {
	case "transfer-encoding",
		"content-length",
		"connection",
		"host",
		"keep-alive",
		"te",
		"trailer",
		"trailers",
		"upgrade":
		return true
	default:
		return false
	}
}
