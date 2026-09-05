package reactor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrServerClosed = errors.New("reactor: server closed")
)

// Handler handles events for an accepted TCP connection.
type Handler interface {
	HandleConn(ctx context.Context, conn net.Conn) error
}

// HandlerFunc type adapter.
type HandlerFunc func(ctx context.Context, conn net.Conn) error

func (f HandlerFunc) HandleConn(ctx context.Context, conn net.Conn) error {
	return f(ctx, conn)
}

// Config defines configuration parameters for the Event Reactor.
type Config struct {
	Addr           string
	WorkerPoolSize int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	MaxBufferBytes int
}

// DefaultConfig returns reasonable default configuration values.
func DefaultConfig() Config {
	return Config{
		Addr:           ":8080",
		WorkerPoolSize: 128,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxBufferBytes: 32 * 1024, // 32 KB per buffer
	}
}

// Reactor manages non-blocking/event-driven TCP socket connections and worker dispatch.
type Reactor struct {
	config     Config
	handler    Handler
	listener   net.Listener
	bufferPool *sync.Pool

	connsMu sync.Mutex
	conns   map[net.Conn]struct{}

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	isShutdown atomic.Bool
}

// New creates a new Event Reactor instance.
func New(cfg Config, handler Handler) *Reactor {
	if cfg.WorkerPoolSize <= 0 {
		cfg.WorkerPoolSize = 64
	}
	if cfg.MaxBufferBytes <= 0 {
		cfg.MaxBufferBytes = 32 * 1024
	}

	ctx, cancel := context.WithCancel(context.Background())

	bufSize := cfg.MaxBufferBytes
	pool := &sync.Pool{
		New: func() any {
			b := make([]byte, bufSize)
			return &b
		},
	}

	return &Reactor{
		config:     cfg,
		handler:    handler,
		bufferPool: pool,
		conns:      make(map[net.Conn]struct{}),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// GetBuffer acquires a byte buffer from the reactor's memory pool.
func (r *Reactor) GetBuffer() *[]byte {
	return r.bufferPool.Get().(*[]byte)
}

// PutBuffer returns a byte buffer to the reactor's memory pool.
func (r *Reactor) PutBuffer(b *[]byte) {
	if b != nil {
		r.bufferPool.Put(b)
	}
}

// ListenAndServe starts the TCP listener and worker event loops.
func (r *Reactor) ListenAndServe() error {
	ln, err := net.Listen("tcp", r.config.Addr)
	if err != nil {
		return fmt.Errorf("reactor: failed to bind address %s: %w", r.config.Addr, err)
	}
	return r.Serve(ln)
}

// Addr returns the listener address (useful for dynamic port binding in tests).
func (r *Reactor) Addr() net.Addr {
	if r.listener != nil {
		return r.listener.Addr()
	}
	return nil
}

// Serve accepts incoming TCP connections on the provided net.Listener.
func (r *Reactor) Serve(ln net.Listener) error {
	r.listener = ln

	defer func() {
		r.wg.Wait()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if r.isShutdown.Load() {
				return ErrServerClosed
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return fmt.Errorf("reactor: accept error: %w", err)
		}

		if r.isShutdown.Load() {
			_ = conn.Close()
			return ErrServerClosed
		}

		r.trackConn(conn, true)
		r.wg.Add(1)
		go func(c net.Conn) {
			defer r.wg.Done()
			defer r.trackConn(c, false)
			r.processConn(c)
		}(conn)
	}
}

func (r *Reactor) trackConn(c net.Conn, add bool) {
	r.connsMu.Lock()
	defer r.connsMu.Unlock()
	if add {
		if r.isShutdown.Load() {
			_ = c.Close()
		}
		r.conns[c] = struct{}{}
	} else {
		delete(r.conns, c)
	}
}

func (r *Reactor) processConn(conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()

	if r.config.ReadTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(r.config.ReadTimeout))
	}

	if err := r.handler.HandleConn(r.ctx, conn); err != nil && !errors.Is(err, io.EOF) {
		// Log or suppress unexpected connection handling errors
	}
}

// Shutdown gracefully stops the reactor listener and closes active workers.
func (r *Reactor) Shutdown(ctx context.Context) error {
	if !r.isShutdown.CompareAndSwap(false, true) {
		return nil // Already shutting down
	}

	r.cancel()

	if r.listener != nil {
		_ = r.listener.Close()
	}

	r.connsMu.Lock()
	for c := range r.conns {
		_ = c.Close()
	}
	r.connsMu.Unlock()

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
