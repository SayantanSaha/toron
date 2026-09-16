package server

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// connDeadlineTracker wraps a net.Conn to amortize redundant SetReadDeadline
// and SetWriteDeadline system calls during rapid request processing.
type connDeadlineTracker struct {
	net.Conn
	lastReadDeadline  time.Time
	lastWriteDeadline time.Time
	lastReadTimeout   time.Duration
	lastWriteTimeout  time.Duration
	mu                sync.Mutex
	syscallCountRead  atomic.Uint64 // telemetry and test verification hook
	syscallCountWrite atomic.Uint64 // telemetry and test verification hook
}

// newConnDeadlineTracker instantiates a new tracker wrapping the given socket connection.
func newConnDeadlineTracker(conn net.Conn) *connDeadlineTracker {
	return &connDeadlineTracker{Conn: conn}
}

// Unwrap returns the underlying net.Conn.
func (t *connDeadlineTracker) Unwrap() net.Conn {
	return t.Conn
}

// CloseWrite forwards half-close calls if supported by the underlying connection.
func (t *connDeadlineTracker) CloseWrite() error {
	if cw, ok := t.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return fmt.Errorf("CloseWrite not supported")
}

// SyscallConn forwards raw socket control access if supported by the underlying connection.
func (t *connDeadlineTracker) SyscallConn() (syscall.RawConn, error) {
	if sc, ok := t.Conn.(syscall.Conn); ok {
		return sc.SyscallConn()
	}
	return nil, syscall.EINVAL
}

// SetAmortizedReadDeadline sets the read deadline on the underlying connection only if
// the remaining deadline window is less than or equal to timeout/2, or if the timeout duration has changed.
func (t *connDeadlineTracker) SetAmortizedReadDeadline(timeout time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if timeout <= 0 {
		if !t.lastReadDeadline.IsZero() {
			t.lastReadDeadline = time.Time{}
			t.lastReadTimeout = 0
			return t.Conn.SetReadDeadline(time.Time{})
		}
		return nil
	}

	now := time.Now()
	if !t.lastReadDeadline.IsZero() && timeout == t.lastReadTimeout {
		remaining := t.lastReadDeadline.Sub(now)
		if remaining > timeout/2 {
			// Amortized: existing deadline in kernel is valid and sufficiently far in future
			return nil
		}
	}

	deadline := now.Add(timeout)
	err := t.Conn.SetReadDeadline(deadline)
	if err == nil {
		t.lastReadDeadline = deadline
		t.lastReadTimeout = timeout
		t.syscallCountRead.Add(1)
	}
	return err
}

// SetAmortizedWriteDeadline sets the write deadline on the underlying connection only if
// the remaining deadline window is less than or equal to timeout/2, or if the timeout duration has changed.
func (t *connDeadlineTracker) SetAmortizedWriteDeadline(timeout time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if timeout <= 0 {
		if !t.lastWriteDeadline.IsZero() {
			t.lastWriteDeadline = time.Time{}
			t.lastWriteTimeout = 0
			return t.Conn.SetWriteDeadline(time.Time{})
		}
		return nil
	}

	now := time.Now()
	if !t.lastWriteDeadline.IsZero() && timeout == t.lastWriteTimeout {
		remaining := t.lastWriteDeadline.Sub(now)
		if remaining > timeout/2 {
			// Amortized: existing deadline in kernel is valid
			return nil
		}
	}

	deadline := now.Add(timeout)
	err := t.Conn.SetWriteDeadline(deadline)
	if err == nil {
		t.lastWriteDeadline = deadline
		t.lastWriteTimeout = timeout
		t.syscallCountWrite.Add(1)
	}
	return err
}

// ForceSetReadDeadline bypasses amortization and applies the given absolute deadline immediately.
func (t *connDeadlineTracker) ForceSetReadDeadline(deadline time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastReadDeadline = deadline
	t.lastReadTimeout = 0
	t.syscallCountRead.Add(1)
	return t.Conn.SetReadDeadline(deadline)
}

// ForceSetWriteDeadline bypasses amortization and applies the given absolute deadline immediately.
func (t *connDeadlineTracker) ForceSetWriteDeadline(deadline time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastWriteDeadline = deadline
	t.lastWriteTimeout = 0
	t.syscallCountWrite.Add(1)
	return t.Conn.SetWriteDeadline(deadline)
}

// ResetReadAmortization invalidates cached read deadline state.
func (t *connDeadlineTracker) ResetReadAmortization() {
	t.mu.Lock()
	t.lastReadDeadline = time.Time{}
	t.lastReadTimeout = 0
	t.mu.Unlock()
}

// ResetWriteAmortization invalidates cached write deadline state.
func (t *connDeadlineTracker) ResetWriteAmortization() {
	t.mu.Lock()
	t.lastWriteDeadline = time.Time{}
	t.lastWriteTimeout = 0
	t.mu.Unlock()
}

// SyscallCounts returns the number of read and write deadline system calls made through the tracker.
func (t *connDeadlineTracker) SyscallCounts() (read, write uint64) {
	return t.syscallCountRead.Load(), t.syscallCountWrite.Load()
}

// SetReadDeadline implements net.Conn by delegating to ForceSetReadDeadline.
func (t *connDeadlineTracker) SetReadDeadline(deadline time.Time) error {
	return t.ForceSetReadDeadline(deadline)
}

// SetWriteDeadline implements net.Conn by delegating to ForceSetWriteDeadline.
func (t *connDeadlineTracker) SetWriteDeadline(deadline time.Time) error {
	return t.ForceSetWriteDeadline(deadline)
}

// SetDeadline implements net.Conn by setting both read and write deadlines.
func (t *connDeadlineTracker) SetDeadline(deadline time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastReadDeadline = deadline
	t.lastReadTimeout = 0
	t.lastWriteDeadline = deadline
	t.lastWriteTimeout = 0
	t.syscallCountRead.Add(1)
	t.syscallCountWrite.Add(1)
	return t.Conn.SetDeadline(deadline)
}

// toDeadlineTracker returns the connection as a *connDeadlineTracker, wrapping it if necessary.
func toDeadlineTracker(c net.Conn) *connDeadlineTracker {
	if c == nil {
		return nil
	}
	if t, ok := c.(*connDeadlineTracker); ok {
		return t
	}
	return newConnDeadlineTracker(c)
}
