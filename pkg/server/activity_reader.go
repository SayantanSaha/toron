package server

import (
	"bufio"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"toron/pkg/httpparser"
)

var (
	// ErrAntiDripRateTooLow is returned when request body progress falls below minimum threshold across a timeout window.
	ErrAntiDripRateTooLow = errors.New("activityReader: read rate below minimum threshold (anti-drip clamp)")
)

// activityReader wraps an underlying connection and buffered reader to provide
// activity-refreshed socket read deadlines and anti-drip progress rate clamping.
type activityReader struct {
	r            *bufio.Reader
	conn         net.Conn
	tracker      *connDeadlineTracker
	timeout      time.Duration
	remaining    int64
	totalRead    int64
	minRateBytes int64
	windowStart  time.Time
	windowBytes  int64
	closed       bool
	mu           sync.Mutex
}

// newActivityReader creates an activityReader with the given timeout and body ceiling limit.
func newActivityReader(r *bufio.Reader, conn net.Conn, tracker *connDeadlineTracker, timeout time.Duration, limit int64) *activityReader {
	if tracker == nil && conn != nil {
		tracker = toDeadlineTracker(conn)
	}
	if r == nil && conn != nil {
		r = bufio.NewReader(conn)
	}
	minRate := int64(1024) // 1 KB per window default
	return &activityReader{
		r:            r,
		conn:         conn,
		tracker:      tracker,
		timeout:      timeout,
		remaining:    limit,
		minRateBytes: minRate,
		windowStart:  time.Now(),
	}
}

// SetMinRateBytes sets the minimum rate threshold in bytes per deadline window.
func (a *activityReader) SetMinRateBytes(rate int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.minRateBytes = rate
}

// Read reads from the underlying reader, refreshing socket read deadlines upon progress
// and enforcing cumulative byte ceilings and anti-drip rate clamping.
func (a *activityReader) Read(p []byte) (int, error) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return 0, io.EOF
	}

	if a.remaining < 0 {
		a.mu.Unlock()
		return 0, httpparser.ErrBodyTooLarge
	}
	a.mu.Unlock()

	n, err := a.r.Read(p)

	a.mu.Lock()
	defer a.mu.Unlock()

	if err != nil {
		if !errors.Is(err, io.EOF) && a.conn != nil {
			_ = a.conn.Close()
		}
		return n, err
	}

	if n > 0 {
		a.totalRead += int64(n)
		a.windowBytes += int64(n)

		// Cumulative ceiling check
		if a.remaining > 0 {
			a.remaining -= int64(n)
			if a.remaining < 0 {
				if a.conn != nil {
					_ = a.conn.Close()
				}
				return n, httpparser.ErrBodyTooLarge
			}
		}

		// Anti-drip progress rate clamping: check if deadline window expired with insufficient bytes
		if a.timeout > 0 && a.minRateBytes > 0 && time.Since(a.windowStart) >= a.timeout {
			if a.windowBytes < a.minRateBytes {
				if a.conn != nil {
					_ = a.conn.Close()
				}
				return n, ErrAntiDripRateTooLow
			}
		}

		effectiveMinRate := a.minRateBytes
		if a.remaining > 0 && a.remaining < effectiveMinRate {
			effectiveMinRate = a.remaining
		}

		// Refresh read deadline when minimum progress in window is satisfied
		if a.minRateBytes <= 0 || a.windowBytes >= effectiveMinRate {
			now := time.Now()
			if a.timeout > 0 {
				if a.tracker != nil {
					_ = a.tracker.SetAmortizedReadDeadline(a.timeout)
				} else if a.conn != nil {
					_ = a.conn.SetReadDeadline(now.Add(a.timeout))
				}
			}
			a.windowStart = now
			a.windowBytes = 0
		}
	}

	return n, nil
}

// Close marks the reader as closed without closing the underlying net.Conn.
func (a *activityReader) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	return nil
}

// TotalRead returns the cumulative count of bytes read through this reader.
func (a *activityReader) TotalRead() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.totalRead
}
