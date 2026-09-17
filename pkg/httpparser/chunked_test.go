package httpparser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// mockTrackingConn records whether Close() has been called on the underlying connection.
type mockTrackingConn struct {
	Closed bool
}

func (m *mockTrackingConn) Read(b []byte) (n int, err error)   { return 0, io.EOF }
func (m *mockTrackingConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (m *mockTrackingConn) Close() error                       { m.Closed = true; return nil }
func (m *mockTrackingConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *mockTrackingConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *mockTrackingConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockTrackingConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockTrackingConn) SetWriteDeadline(t time.Time) error { return nil }

func newMockConn() *mockTrackingConn {
	return &mockTrackingConn{}
}

// TC-133-01: Positive Multi-Chunk Streaming Reading & Terminal Zero Chunk
func TestChunkedBodyReader_PositiveMultiChunk(t *testing.T) {
	rawPayload := "4\r\nWiki\r\n5\r\npedia\r\ne\r\n in \r\n\r\naction\r\n0\r\n\r\n"
	expectedOutput := "Wikipedia in \r\n\r\naction"
	expectedLen := int64(len(expectedOutput)) // 23 bytes

	bufferSizes := []int{1, 2, 4, 7, 16, 64, 1024}

	for _, bufSize := range bufferSizes {
		t.Run(fmt.Sprintf("BufferSize_%d", bufSize), func(t *testing.T) {
			bufr := bufio.NewReader(strings.NewReader(rawPayload))
			reader := newChunkedBodyReader(bufr, nil, 1048576, nil)

			var out bytes.Buffer
			buf := make([]byte, bufSize)

			for {
				n, err := reader.Read(buf)
				if n > 0 {
					out.Write(buf[:n])
				}
				if err != nil {
					if !errors.Is(err, io.EOF) {
						t.Fatalf("expected io.EOF on completion, got: %v", err)
					}
					break
				}
			}

			if out.String() != expectedOutput {
				t.Fatalf("decoded payload mismatch: expected %q, got %q", expectedOutput, out.String())
			}

			if reader.totalBodyRead != expectedLen {
				t.Fatalf("totalBodyRead metric mismatch: expected %d, got %d", expectedLen, reader.totalBodyRead)
			}

			if reader.state != stateDone {
				t.Fatalf("expected stateDone, got state %d", reader.state)
			}
		})
	}
}

// TC-133-02: Zero-Length Terminal Chunk & Immediate EOF Handling
func TestChunkedBodyReader_EmptyPayload(t *testing.T) {
	rawPayload := "0\r\n\r\n"
	bufr := bufio.NewReader(strings.NewReader(rawPayload))
	reader := newChunkedBodyReader(bufr, nil, 1048576, nil)

	buf := make([]byte, 64)
	n, err := reader.Read(buf)
	if n != 0 {
		t.Fatalf("expected n == 0 for empty payload, got %d", n)
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF on initial read, got %v", err)
	}

	// Idempotence test across repeated reads
	for i := 0; i < 3; i++ {
		n2, err2 := reader.Read(buf)
		if n2 != 0 || !errors.Is(err2, io.EOF) {
			t.Fatalf("repeated read %d: expected (0, io.EOF), got (%d, %v)", i+1, n2, err2)
		}
	}

	if reader.totalBodyRead != 0 {
		t.Fatalf("expected totalBodyRead == 0, got %d", reader.totalBodyRead)
	}

	if reader.state != stateDone {
		t.Fatalf("expected stateDone, got state %d", reader.state)
	}
}

// TC-133-03: Strict RFC 9112 §7.1 Hex Size Wire Parsing (1*HEXDIG) & Non-Hex / Sign Rejection
func TestChunkedBodyReader_StrictHexValidation(t *testing.T) {
	testMatrix := []struct {
		name string
		raw  string
	}{
		{"PositiveSign", "+10\r\ndatadatadatadata\r\n0\r\n\r\n"},
		{"NegativeSign", "-5\r\nhello\r\n0\r\n\r\n"},
		{"LeadingSpace", " 10\r\ndatadatadatadata\r\n0\r\n\r\n"},
		{"LeadingTab", "\t10\r\ndatadatadatadata\r\n0\r\n\r\n"},
		{"EmbeddedSpace", "1 0\r\ndatadatadatadata\r\n0\r\n\r\n"},
		{"NonHex_1G", "1G\r\ndata\r\n0\r\n\r\n"},
		{"NonHex_ZZ", "ZZ\r\ndata\r\n0\r\n\r\n"},
		{"NonHex_123Q", "123Q\r\n0123\r\n0\r\n\r\n"},
		{"Prefix_0x10", "0x10\r\ndatadatadatadata\r\n0\r\n\r\n"},
		{"Prefix_0X1A", "0X1A\r\n0123456789ABCDEF0123456789\r\n0\r\n\r\n"},
		{"Overflow_17Digits", "10000000000000000\r\nfoo\r\n0\r\n\r\n"},
		{"Overflow_MaxUint64", "FFFFFFFFFFFFFFFF1\r\nfoo\r\n0\r\n\r\n"},
		{"EmptyLine", "\r\n"},
		{"SemicolonWithoutHex", ";ext=val\r\n"},
	}

	for _, tc := range testMatrix {
		t.Run(tc.name, func(t *testing.T) {
			mockConn := newMockConn()
			bufr := bufio.NewReader(strings.NewReader(tc.raw))
			reader := newChunkedBodyReader(bufr, mockConn, 1048576, nil)

			buf := make([]byte, 64)
			_, err := reader.Read(buf)
			if err == nil {
				t.Fatalf("expected error for malformed hex %q, but got nil", tc.raw)
			}
			if !errors.Is(err, ErrBadRequest) {
				t.Fatalf("expected ErrBadRequest, got: %v", err)
			}
			if !mockConn.Closed {
				t.Fatalf("expected mockConn to be closed upon ErrBadRequest teardown")
			}

			// Permanent error state check
			_, err2 := reader.Read(buf)
			if !errors.Is(err2, ErrBadRequest) {
				t.Fatalf("expected permanent ErrBadRequest state on subsequent read, got: %v", err2)
			}
		})
	}
}

// TC-133-04: Bounded Chunk Extension Clamping (<= 256B) & Control Character Rejection
func TestChunkedBodyReader_ChunkExtensionLimits(t *testing.T) {
	// Vector A: Valid extension <= 256B
	t.Run("VectorA_ValidExtension", func(t *testing.T) {
		raw := "5;name=val\r\nhello\r\n0\r\n\r\n"
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, nil, 1048576, nil)
		out, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if string(out) != "hello" {
			t.Fatalf("expected 'hello', got %q", string(out))
		}
	})

	// Vector B: Valid multi-param extension
	t.Run("VectorB_ValidMultiParam", func(t *testing.T) {
		raw := "5;a=1;b=2;c=foo\r\nhello\r\n0\r\n\r\n"
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, nil, 1048576, nil)
		out, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if string(out) != "hello" {
			t.Fatalf("expected 'hello', got %q", string(out))
		}
	})

	// Vector C: Max boundary 256B (total line 256 bytes)
	t.Run("VectorC_MaxBoundary256B", func(t *testing.T) {
		raw := "5;" + strings.Repeat("x", 254) + "\r\nhello\r\n0\r\n\r\n"
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, nil, 1048576, nil)
		out, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("expected success for 256B boundary extension, got: %v", err)
		}
		if string(out) != "hello" {
			t.Fatalf("expected 'hello', got %q", string(out))
		}
	})

	// Vector D: Oversized 257B
	t.Run("VectorD_Oversized257B", func(t *testing.T) {
		raw := "5;" + strings.Repeat("x", 255) + "\r\nhello\r\n0\r\n\r\n"
		mockConn := newMockConn()
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, mockConn, 1048576, nil)
		buf := make([]byte, 64)
		_, err := reader.Read(buf)
		if err == nil || !errors.Is(err, ErrBadRequest) {
			t.Fatalf("expected ErrBadRequest for oversized extension, got: %v", err)
		}
	})

	// Vector E: Extension flooding attack 16KB
	t.Run("VectorE_Flooding16KB", func(t *testing.T) {
		raw := "5;ext=" + strings.Repeat("A", 16384) + "\r\nhello\r\n0\r\n\r\n"
		mockConn := newMockConn()
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, mockConn, 1048576, nil)
		buf := make([]byte, 64)
		_, err := reader.Read(buf)
		if err == nil || !errors.Is(err, ErrBadRequest) {
			t.Fatalf("expected ErrBadRequest for 16KB extension flood, got: %v", err)
		}
	})

	// Control character vectors F, G, H
	ctrlVectors := []struct {
		name string
		raw  string
	}{
		{"VectorF_NullByte", "5;foo=\x00bar\r\nhello\r\n0\r\n\r\n"},
		{"VectorG_ESCByte", "5;foo=\x1bbar\r\nhello\r\n0\r\n\r\n"},
		{"VectorH_DELByte", "5;foo=\x7fbar\r\nhello\r\n0\r\n\r\n"},
	}

	for _, cv := range ctrlVectors {
		t.Run(cv.name, func(t *testing.T) {
			mockConn := newMockConn()
			bufr := bufio.NewReader(strings.NewReader(cv.raw))
			reader := newChunkedBodyReader(bufr, mockConn, 1048576, nil)
			buf := make([]byte, 64)
			_, err := reader.Read(buf)
			if err == nil || !errors.Is(err, ErrBadRequest) {
				t.Fatalf("expected ErrBadRequest for control character, got: %v", err)
			}
		})
	}

	// Vector I: Permitted BWS HTAB
	t.Run("VectorI_BWSHTAB", func(t *testing.T) {
		raw := "5;\tname=val\r\nhello\r\n0\r\n\r\n"
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, nil, 1048576, nil)
		out, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("expected success for HTAB in extension, got: %v", err)
		}
		if string(out) != "hello" {
			t.Fatalf("expected 'hello', got %q", string(out))
		}
	})
}

// TC-133-05: Strict Chunk Delimiter & CRLF Boundary Enforcement (Bare \n Rejection)
func TestChunkedBodyReader_CRLFDelimiters(t *testing.T) {
	// Vectors 1, 2, 3: Malformed delimiters after chunk data
	delimiterTests := []struct {
		name string
		raw  string
	}{
		{"BareLF", "5\r\nhello\n0\r\n\r\n"},
		{"BareCR", "5\r\nhello\r0\r\n\r\n"},
		{"GarbageXX", "5\r\nhelloXX0\r\n\r\n"},
	}

	for _, dt := range delimiterTests {
		t.Run(dt.name, func(t *testing.T) {
			bufr := bufio.NewReader(strings.NewReader(dt.raw))
			reader := newChunkedBodyReader(bufr, nil, 1048576, nil)

			// Read exactly 5 bytes of data ("hello")
			buf := make([]byte, 5)
			n, err := reader.Read(buf)
			if n != 5 || err != nil {
				t.Fatalf("expected to read 5 bytes of data, got %d bytes, err: %v", n, err)
			}
			if string(buf) != "hello" {
				t.Fatalf("expected 'hello', got %q", string(buf))
			}

			// Subsequent Read triggers stateChunkCRLF and must reject malformed delimiter
			_, err = reader.Read(buf)
			if err == nil || !errors.Is(err, ErrBadRequest) {
				t.Fatalf("expected ErrBadRequest for malformed delimiter, got: %v", err)
			}
		})
	}

	// Vectors 4, 5: Truncated streams
	truncatedTests := []struct {
		name string
		raw  string
	}{
		{"PrematureEOF_MidChunk", "5\r\nhel"},
		{"PrematureEOF_DuringDelimiter", "5\r\nhello\r"},
	}

	for _, tt := range truncatedTests {
		t.Run(tt.name, func(t *testing.T) {
			bufr := bufio.NewReader(strings.NewReader(tt.raw))
			reader := newChunkedBodyReader(bufr, nil, 1048576, nil)
			_, err := io.ReadAll(reader)
			if err == nil {
				t.Fatalf("expected error for truncated chunk stream %q, got nil", tt.raw)
			}
		})
	}

	// Vector 6: Valid CRLF
	t.Run("ValidCRLF", func(t *testing.T) {
		raw := "5\r\nhello\r\n0\r\n\r\n"
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, nil, 1048576, nil)
		out, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("expected clean read, got: %v", err)
		}
		if string(out) != "hello" {
			t.Fatalf("expected 'hello', got %q", string(out))
		}
	})
}

// TC-133-06: Cumulative Payload Body Bounding (opts.MaxBodyBytes Clamping & HTTP 413)
func TestChunkedBodyReader_MaxBodyBytesClamping(t *testing.T) {
	// Delivering 3 chunks of 50 bytes each (total 150 bytes), limit = 100 bytes
	chunk1 := "32\r\n" + strings.Repeat("A", 50) + "\r\n"
	chunk2 := "32\r\n" + strings.Repeat("B", 50) + "\r\n"
	chunk3 := "32\r\n" + strings.Repeat("C", 50) + "\r\n"
	terminal := "0\r\n\r\n"
	rawStream := chunk1 + chunk2 + chunk3 + terminal

	mockConn := newMockConn()
	bufr := bufio.NewReader(strings.NewReader(rawStream))
	reader := newChunkedBodyReader(bufr, mockConn, 100, nil)

	buf := make([]byte, 50)

	// Chunk 1: 50 bytes cumulative (<= 100)
	n1, err1 := reader.Read(buf)
	if n1 != 50 || err1 != nil {
		t.Fatalf("read 1: expected (50, nil), got (%d, %v)", n1, err1)
	}

	// Chunk 2: 100 bytes cumulative (== exact limit 100)
	n2, err2 := reader.Read(buf)
	if n2 != 50 || err2 != nil {
		t.Fatalf("read 2: expected (50, nil), got (%d, %v)", n2, err2)
	}

	// Chunk 3: 150 bytes cumulative (> limit 100) -> MUST abort immediately with ErrBodyTooLarge
	_, err3 := reader.Read(buf)
	if err3 == nil || !errors.Is(err3, ErrBodyTooLarge) {
		t.Fatalf("read 3: expected ErrBodyTooLarge, got: %v", err3)
	}

	if !mockConn.Closed {
		t.Fatalf("expected physical socket teardown upon ErrBodyTooLarge violation")
	}
}

// TC-133-07: RFC 9112 §7.1.2 Trailer Validation (<= 4KB) & Forbidden Header Filtering
func TestChunkedBodyReader_TrailersValidation(t *testing.T) {
	// 1. Valid Trailer Stream
	t.Run("ValidTrailers", func(t *testing.T) {
		raw := "4\r\ntest\r\n0\r\nExpires: Wed, 21 Oct 2026 07:28:00 GMT\r\nX-Checksum: sha256-9a0b\r\n\r\n"
		header := make(Header)
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, nil, 1048576, header)

		out, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if string(out) != "test" {
			t.Fatalf("expected body 'test', got %q", string(out))
		}

		if header.Get("Expires") != "Wed, 21 Oct 2026 07:28:00 GMT" {
			t.Fatalf("expected Expires trailer in header, got %q", header.Get("Expires"))
		}
		if header.Get("X-Checksum") != "sha256-9a0b" {
			t.Fatalf("expected X-Checksum trailer in header, got %q", header.Get("X-Checksum"))
		}
		if reader.Trailers().Get("Expires") != "Wed, 21 Oct 2026 07:28:00 GMT" {
			t.Fatalf("expected Expires in reader.Trailers(), got %q", reader.Trailers().Get("Expires"))
		}
	})

	// 2. Prohibited Trailer Vectors
	prohibitedTests := []struct {
		name   string
		header string
	}{
		{"Transfer-Encoding", "Transfer-Encoding: chunked"},
		{"Content-Length", "Content-Length: 42"},
		{"Connection", "Connection: close"},
		{"Host", "Host: attacker.com"},
		{"Keep-Alive", "Keep-Alive: timeout=10"},
		{"TE", "TE: trailers"},
		{"Trailer", "Trailer: X-Foo"},
		{"Upgrade", "Upgrade: websocket"},
		{"PseudoHeader", ":method: POST"},
	}

	for _, pt := range prohibitedTests {
		t.Run("Prohibited_"+pt.name, func(t *testing.T) {
			raw := "0\r\n" + pt.header + "\r\n\r\n"
			mockConn := newMockConn()
			bufr := bufio.NewReader(strings.NewReader(raw))
			reader := newChunkedBodyReader(bufr, mockConn, 1048576, nil)

			buf := make([]byte, 64)
			_, err := reader.Read(buf)
			if err == nil || !errors.Is(err, ErrBadRequest) {
				t.Fatalf("expected ErrBadRequest for forbidden trailer %q, got: %v", pt.header, err)
			}
			if !strings.Contains(err.Error(), "prohibited trailer header") {
				t.Fatalf("expected error message to mention prohibited trailer header, got: %v", err)
			}
		})
	}

	// 3. Oversized Trailer Stream (> 4096B)
	t.Run("OversizedTrailersExceeding4KB", func(t *testing.T) {
		raw := "0\r\nX-Large: " + strings.Repeat("A", 4100) + "\r\n\r\n"
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, nil, 1048576, nil)

		buf := make([]byte, 64)
		_, err := reader.Read(buf)
		if err == nil || !errors.Is(err, ErrHeaderTooLarge) {
			t.Fatalf("expected ErrHeaderTooLarge for trailers exceeding 4KB, got: %v", err)
		}
	})
}

// TC-133-08: Socket Drainage & Unconsumed Body Cleanup on Close() (64KB Threshold & Forceful Socket Teardown)
func TestChunkedBodyReader_DrainageAndClose(t *testing.T) {
	// Scenario A: Safe Drainage <= 64KB (16KB unread payload in valid chunks)
	t.Run("ScenarioA_SafeDrainage16KB", func(t *testing.T) {
		chunkData := strings.Repeat("D", 16384) // 16 KB
		raw := fmt.Sprintf("%x\r\n%s\r\n0\r\n\r\n", len(chunkData), chunkData)

		mockConn := newMockConn()
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, mockConn, 1048576, nil)

		// Read only 1 KB
		buf := make([]byte, 1024)
		n, err := reader.Read(buf)
		if n != 1024 || err != nil {
			t.Fatalf("expected to read 1024 bytes, got (%d, %v)", n, err)
		}

		// Close reader before reading the remaining 15KB
		if err := reader.Close(); err != nil {
			t.Fatalf("unexpected Close() error: %v", err)
		}

		if mockConn.Closed {
			t.Fatalf("expected mockConn to remain OPEN after safe 16KB drainage, but was closed")
		}
		if reader.state != stateDone {
			t.Fatalf("expected reader state to be stateDone after drainage, got %d", reader.state)
		}
	})

	// Scenario B: Exceeds Drainage Limit > 64KB (128KB payload)
	t.Run("ScenarioB_ExceedsDrainageLimit128KB", func(t *testing.T) {
		chunkData := strings.Repeat("E", 128*1024) // 128 KB
		raw := fmt.Sprintf("%x\r\n%s\r\n0\r\n\r\n", len(chunkData), chunkData)

		mockConn := newMockConn()
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, mockConn, 1048576, nil)

		// Read only 1 KB
		buf := make([]byte, 1024)
		n, err := reader.Read(buf)
		if n != 1024 || err != nil {
			t.Fatalf("expected to read 1024 bytes, got (%d, %v)", n, err)
		}

		// Close reader: cannot be drained within 64KB, MUST forcefully sever connection
		if err := reader.Close(); err != nil {
			t.Fatalf("unexpected Close() error: %v", err)
		}

		if !mockConn.Closed {
			t.Fatalf("expected mockConn to be CLOSED when unread payload exceeds 64KB drainage limit")
		}
	})

	// Scenario C: Error State Teardown
	t.Run("ScenarioC_ErrorStateTeardown", func(t *testing.T) {
		raw := "ZZ\r\n"
		mockConn := newMockConn()
		bufr := bufio.NewReader(strings.NewReader(raw))
		reader := newChunkedBodyReader(bufr, mockConn, 1048576, nil)

		buf := make([]byte, 64)
		_, err := reader.Read(buf)
		if err == nil || !errors.Is(err, ErrBadRequest) {
			t.Fatalf("expected ErrBadRequest, got: %v", err)
		}

		_ = reader.Close()
		if !mockConn.Closed {
			t.Fatalf("expected mockConn to be CLOSED after framing error")
		}
	})

	t.Run("ExportedConstructorCoverage", func(t *testing.T) {
		raw := "5\r\nhello\r\n0\r\n\r\n"
		bufr := bufio.NewReader(strings.NewReader(raw))
		r := NewChunkedBodyReader(bufr, nil, 1048576)
		if r == nil {
			t.Fatal("expected non-nil ChunkedBodyReader")
		}
		data, err := io.ReadAll(r)
		if err != nil || string(data) != "hello" {
			t.Fatalf("unexpected read: %s, %v", string(data), err)
		}
		if err := r.Close(); err != nil {
			t.Fatalf("unexpected Close error: %v", err)
		}
	})
}
