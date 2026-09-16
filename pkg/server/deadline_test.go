package server

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type mockDeadlineConn struct {
	net.Conn
	mu                 sync.Mutex
	readDeadlines      []time.Time
	writeDeadlines     []time.Time
	readDeadlineCount  atomic.Uint64
	writeDeadlineCount atomic.Uint64
	closeWriteCalled   bool
	supportsCloseWrite bool
	supportsSyscall    bool
}

func newMockDeadlineConn(supportsCloseWrite, supportsSyscall bool) *mockDeadlineConn {
	return &mockDeadlineConn{
		supportsCloseWrite: supportsCloseWrite,
		supportsSyscall:    supportsSyscall,
	}
}

func (m *mockDeadlineConn) Read(b []byte) (n int, err error) {
	return 0, nil
}

func (m *mockDeadlineConn) Write(b []byte) (n int, err error) {
	return len(b), nil
}

func (m *mockDeadlineConn) Close() error {
	return nil
}

func (m *mockDeadlineConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func (m *mockDeadlineConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
}

func (m *mockDeadlineConn) SetDeadline(t time.Time) error {
	_ = m.SetReadDeadline(t)
	_ = m.SetWriteDeadline(t)
	return nil
}

func (m *mockDeadlineConn) SetReadDeadline(t time.Time) error {
	m.mu.Lock()
	m.readDeadlines = append(m.readDeadlines, t)
	m.mu.Unlock()
	m.readDeadlineCount.Add(1)
	return nil
}

func (m *mockDeadlineConn) SetWriteDeadline(t time.Time) error {
	m.mu.Lock()
	m.writeDeadlines = append(m.writeDeadlines, t)
	m.mu.Unlock()
	m.writeDeadlineCount.Add(1)
	return nil
}

func (m *mockDeadlineConn) CloseWrite() error {
	if !m.supportsCloseWrite {
		return errors.New("closewrite not supported")
	}
	m.mu.Lock()
	m.closeWriteCalled = true
	m.mu.Unlock()
	return nil
}

func (m *mockDeadlineConn) SyscallConn() (syscall.RawConn, error) {
	if !m.supportsSyscall {
		return nil, syscall.EINVAL
	}
	return &mockRawConn{}, nil
}

type mockRawConn struct{}

func (m *mockRawConn) Control(f func(fd uintptr)) error {
	f(0)
	return nil
}

func (m *mockRawConn) Read(f func(fd uintptr) (done bool)) error {
	f(0)
	return nil
}

func (m *mockRawConn) Write(f func(fd uintptr) (done bool)) error {
	f(0)
	return nil
}

// TC-126.2: Deadline Renewal on Window Expiration ($R \le \tau/2$ Threshold)
func TestDeadlineTracker_RenewalOnWindowExpiration(t *testing.T) {
	mockConn := newMockDeadlineConn(true, true)
	tracker := newConnDeadlineTracker(mockConn)

	t.Run("Subtest 2A (Initial Set & Amortization)", func(t *testing.T) {
		err := tracker.SetAmortizedReadDeadline(200 * time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mockConn.readDeadlineCount.Load() != 1 {
			t.Fatalf("expected 1 read deadline call, got %d", mockConn.readDeadlineCount.Load())
		}
		rCount, wCount := tracker.SyscallCounts()
		if rCount != 1 || wCount != 0 {
			t.Fatalf("expected syscall counts (1, 0), got (%d, %d)", rCount, wCount)
		}

		// Immediately invoke again (< timeout/2 remaining elapsed)
		err = tracker.SetAmortizedReadDeadline(200 * time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mockConn.readDeadlineCount.Load() != 1 {
			t.Fatalf("expected call to be amortized (count remains 1), got %d", mockConn.readDeadlineCount.Load())
		}
	})

	t.Run("Subtest 2B (Window Expiration Renewal at <= tau/2)", func(t *testing.T) {
		// Sleep past the half-window: 200ms timeout -> half window is 100ms
		time.Sleep(115 * time.Millisecond)

		err := tracker.SetAmortizedReadDeadline(200 * time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mockConn.readDeadlineCount.Load() != 2 {
			t.Fatalf("expected fresh SetReadDeadline call (count=2), got %d", mockConn.readDeadlineCount.Load())
		}
	})

	t.Run("Subtest 2C (Timeout Mutation Forces Immediate Renewal)", func(t *testing.T) {
		// Mutation from 200ms to 500ms should immediately trigger fresh syscall
		err := tracker.SetAmortizedReadDeadline(500 * time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mockConn.readDeadlineCount.Load() != 3 {
			t.Fatalf("expected immediate renewal on timeout mutation (count=3), got %d", mockConn.readDeadlineCount.Load())
		}
	})

	t.Run("Subtest 2D (Timeout <= 0 Clears Socket Deadline)", func(t *testing.T) {
		err := tracker.SetAmortizedReadDeadline(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		mockConn.mu.Lock()
		lastRead := mockConn.readDeadlines[len(mockConn.readDeadlines)-1]
		mockConn.mu.Unlock()
		if !lastRead.IsZero() {
			t.Fatalf("expected zero deadline to be passed to connection, got %v", lastRead)
		}
		if !tracker.lastReadDeadline.IsZero() {
			t.Fatalf("expected tracker lastReadDeadline to be zero")
		}

		// Second call with timeout 0 should be no-op (no redundant syscall)
		currCount := mockConn.readDeadlineCount.Load()
		err = tracker.SetAmortizedReadDeadline(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mockConn.readDeadlineCount.Load() != currCount {
			t.Fatalf("expected no redundant clear call, got %d vs %d", mockConn.readDeadlineCount.Load(), currCount)
		}
	})

	t.Run("Subtest 2E (Symmetric Write Deadline Validation)", func(t *testing.T) {
		// Initial Set & Amortization
		err := tracker.SetAmortizedWriteDeadline(200 * time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mockConn.writeDeadlineCount.Load() != 1 {
			t.Fatalf("expected 1 write deadline call, got %d", mockConn.writeDeadlineCount.Load())
		}

		err = tracker.SetAmortizedWriteDeadline(200 * time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mockConn.writeDeadlineCount.Load() != 1 {
			t.Fatalf("expected write call to be amortized, got %d", mockConn.writeDeadlineCount.Load())
		}

		// Window expiration renewal at <= tau/2
		time.Sleep(115 * time.Millisecond)
		err = tracker.SetAmortizedWriteDeadline(200 * time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mockConn.writeDeadlineCount.Load() != 2 {
			t.Fatalf("expected write renewal on window expiration, got %d", mockConn.writeDeadlineCount.Load())
		}

		// Timeout mutation
		err = tracker.SetAmortizedWriteDeadline(500 * time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mockConn.writeDeadlineCount.Load() != 3 {
			t.Fatalf("expected write renewal on timeout mutation, got %d", mockConn.writeDeadlineCount.Load())
		}

		// Timeout <= 0 clears socket deadline
		err = tracker.SetAmortizedWriteDeadline(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		mockConn.mu.Lock()
		lastWrite := mockConn.writeDeadlines[len(mockConn.writeDeadlines)-1]
		mockConn.mu.Unlock()
		if !lastWrite.IsZero() {
			t.Fatalf("expected zero write deadline, got %v", lastWrite)
		}
		if !tracker.lastWriteDeadline.IsZero() {
			t.Fatalf("expected tracker lastWriteDeadline to be zero")
		}

		currCount := mockConn.writeDeadlineCount.Load()
		err = tracker.SetAmortizedWriteDeadline(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mockConn.writeDeadlineCount.Load() != currCount {
			t.Fatalf("expected no redundant write clear call, got %d vs %d", mockConn.writeDeadlineCount.Load(), currCount)
		}
	})
}

func TestDeadlineTracker_ForcedOverridesAndResets(t *testing.T) {
	mockConn := newMockDeadlineConn(true, true)
	tracker := newConnDeadlineTracker(mockConn)

	// Test ForceSetReadDeadline
	targetRead := time.Now().Add(10 * time.Second)
	if err := tracker.ForceSetReadDeadline(targetRead); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mockConn.readDeadlineCount.Load() != 1 {
		t.Fatalf("expected 1 read deadline call, got %d", mockConn.readDeadlineCount.Load())
	}
	if !tracker.lastReadDeadline.Equal(targetRead) {
		t.Fatalf("expected lastReadDeadline to match target")
	}
	if tracker.lastReadTimeout != 0 {
		t.Fatalf("expected lastReadTimeout to be reset to 0 by force call")
	}

	// Test ResetReadAmortization
	tracker.ResetReadAmortization()
	if !tracker.lastReadDeadline.IsZero() || tracker.lastReadTimeout != 0 {
		t.Fatalf("expected reset read deadline to be zero")
	}

	// Test ForceSetWriteDeadline
	targetWrite := time.Now().Add(10 * time.Second)
	if err := tracker.ForceSetWriteDeadline(targetWrite); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mockConn.writeDeadlineCount.Load() != 1 {
		t.Fatalf("expected 1 write deadline call, got %d", mockConn.writeDeadlineCount.Load())
	}
	if !tracker.lastWriteDeadline.Equal(targetWrite) {
		t.Fatalf("expected lastWriteDeadline to match target")
	}
	if tracker.lastWriteTimeout != 0 {
		t.Fatalf("expected lastWriteTimeout to be reset to 0 by force call")
	}

	// Test ResetWriteAmortization
	tracker.ResetWriteAmortization()
	if !tracker.lastWriteDeadline.IsZero() || tracker.lastWriteTimeout != 0 {
		t.Fatalf("expected reset write deadline to be zero")
	}
}

func TestDeadlineTracker_InterfacesAndUnwrap(t *testing.T) {
	mockConn := newMockDeadlineConn(true, true)
	tracker := newConnDeadlineTracker(mockConn)

	// Unwrap
	if tracker.Unwrap() != mockConn {
		t.Fatalf("expected Unwrap() to return mockConn")
	}

	// CloseWrite supported
	if err := tracker.CloseWrite(); err != nil {
		t.Fatalf("expected CloseWrite to succeed, got %v", err)
	}
	if !mockConn.closeWriteCalled {
		t.Fatalf("expected underlying CloseWrite to be called")
	}

	// CloseWrite unsupported
	unsupportedConn := newMockDeadlineConn(false, false)
	unsupportedTracker := newConnDeadlineTracker(unsupportedConn)
	if err := unsupportedTracker.CloseWrite(); err == nil {
		t.Fatalf("expected error when CloseWrite is unsupported")
	}

	// SyscallConn supported
	rawConn, err := tracker.SyscallConn()
	if err != nil || rawConn == nil {
		t.Fatalf("expected SyscallConn to succeed, got %v", err)
	}

	// SyscallConn unsupported
	_, err = unsupportedTracker.SyscallConn()
	if err == nil {
		t.Fatalf("expected error when SyscallConn is unsupported")
	}

	// Standard net.Conn delegation
	d := time.Now().Add(time.Second)
	if err := tracker.SetReadDeadline(d); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := tracker.SetWriteDeadline(d); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := tracker.SetDeadline(d); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeadlineTracker_ConcurrentAccess(t *testing.T) {
	mockConn := newMockDeadlineConn(true, true)
	tracker := newConnDeadlineTracker(mockConn)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			timeout := time.Duration(100+id*10) * time.Millisecond
			_ = tracker.SetAmortizedReadDeadline(timeout)
			_ = tracker.SetAmortizedWriteDeadline(timeout)
			_ = tracker.ForceSetReadDeadline(time.Now().Add(timeout))
			_ = tracker.ForceSetWriteDeadline(time.Now().Add(timeout))
			tracker.ResetReadAmortization()
			tracker.ResetWriteAmortization()
			_, _ = tracker.SyscallCounts()
		}(i)
	}
	wg.Wait()
}
