package proxy

import (
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TCPOptions holds configuration options for TCPProxy.
type TCPOptions struct {
	MaxConnections int
	IdleTimeout    time.Duration
}

// TCPOption defines a functional option for configuring a TCPProxy.
type TCPOption func(*TCPProxy)

// WithTCPMaxConnections sets the maximum concurrent client connections.
func WithTCPMaxConnections(n int) TCPOption {
	return func(p *TCPProxy) {
		if n > 0 {
			p.maxConnections = n
		}
	}
}

// WithMaxConnections is an alias for WithTCPMaxConnections.
func WithMaxConnections(n int) TCPOption {
	return WithTCPMaxConnections(n)
}

// WithTCPIdleTimeout sets the inactivity timeout for connections.
func WithTCPIdleTimeout(d time.Duration) TCPOption {
	return func(p *TCPProxy) {
		if d > 0 {
			p.idleTimeout = d
		}
	}
}

// WithIdleTimeout is an alias for WithTCPIdleTimeout.
func WithIdleTimeout(d time.Duration) TCPOption {
	return WithTCPIdleTimeout(d)
}

// WithTCPOptions applies a TCPOptions configuration.
func WithTCPOptions(opts TCPOptions) TCPOption {
	return func(p *TCPProxy) {
		if opts.MaxConnections > 0 {
			p.maxConnections = opts.MaxConnections
		}
		if opts.IdleTimeout > 0 {
			p.idleTimeout = opts.IdleTimeout
		}
	}
}

// TCPProxy implements Layer 4 TCP transport socket proxying and stream forwarding.
type TCPProxy struct {
	targets        []string
	counter        uint64
	timeout        time.Duration
	maxConnections int
	idleTimeout    time.Duration
	activeConns    int64

	mu            sync.Mutex
	listener      net.Listener
	closed        chan struct{}
	connsMu       sync.Mutex
	activeConnSet map[net.Conn]struct{}
}

// NewTCPProxy creates a new TCPProxy for forwarding TCP streams to target backends.
func NewTCPProxy(targets []string, timeout time.Duration, opts ...TCPOption) (*TCPProxy, error) {
	if len(targets) == 0 {
		return nil, errors.New("tcpproxy: no target backends provided")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	copied := make([]string, len(targets))
	for i, t := range targets {
		addr := strings.TrimSpace(t)
		addr = strings.TrimPrefix(addr, "http://")
		addr = strings.TrimPrefix(addr, "https://")
		addr = strings.TrimPrefix(addr, "tcp://")
		copied[i] = addr
	}
	p := &TCPProxy{
		targets:        copied,
		timeout:        timeout,
		maxConnections: 10000,
		idleTimeout:    60 * time.Second,
		closed:         make(chan struct{}),
		activeConnSet:  make(map[net.Conn]struct{}),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

// ActiveConnections returns the number of currently active client connections.
func (p *TCPProxy) ActiveConnections() int64 {
	return atomic.LoadInt64(&p.activeConns)
}

// SelectTarget picks the next TCP target address using round-robin selection.
func (p *TCPProxy) SelectTarget() string {
	if len(p.targets) == 1 {
		return p.targets[0]
	}
	idx := atomic.AddUint64(&p.counter, 1) - 1
	return p.targets[idx%uint64(len(p.targets))]
}

// Serve accepts incoming client connections from listener and relays streams to target backends.
func (p *TCPProxy) Serve(l net.Listener) error {
	p.mu.Lock()
	p.listener = l
	p.mu.Unlock()

	for {
		clientConn, err := l.Accept()
		if err != nil {
			select {
			case <-p.closed:
				return nil
			default:
				return err
			}
		}

		if atomic.LoadInt64(&p.activeConns) >= int64(p.maxConnections) {
			log.Printf("[TCPProxy] Maximum connection limit reached (%d), rejecting connection from %s", p.maxConnections, clientConn.RemoteAddr())
			_ = clientConn.Close()
			continue
		}

		atomic.AddInt64(&p.activeConns, 1)
		go p.handleConn(clientConn)
	}
}

func (p *TCPProxy) registerConn(c net.Conn) bool {
	p.connsMu.Lock()
	defer p.connsMu.Unlock()
	select {
	case <-p.closed:
		_ = c.Close()
		return false
	default:
	}
	p.activeConnSet[c] = struct{}{}
	return true
}

func (p *TCPProxy) unregisterConn(c net.Conn) {
	p.connsMu.Lock()
	delete(p.activeConnSet, c)
	p.connsMu.Unlock()
}

func (p *TCPProxy) handleConn(clientConn net.Conn) {
	defer func() {
		_ = clientConn.Close()
		atomic.AddInt64(&p.activeConns, -1)
	}()

	if !p.registerConn(clientConn) {
		return
	}
	defer p.unregisterConn(clientConn)

	targetAddr := p.SelectTarget()
	upstreamConn, err := net.DialTimeout("tcp", targetAddr, p.timeout)
	if err != nil {
		log.Printf("[TCPProxy] Error dialing target %s: %v", targetAddr, err)
		return
	}
	defer upstreamConn.Close()

	if !p.registerConn(upstreamConn) {
		return
	}
	defer p.unregisterConn(upstreamConn)

	p.relay(clientConn, upstreamConn)
}

func (p *TCPProxy) relay(clientConn, upstreamConn net.Conn) {
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	var closeOnce sync.Once
	closeBoth := func(reason string) {
		closeOnce.Do(func() {
			if reason == "idle" {
				log.Printf("[TCPProxy] Idle timeout (%s) reached for connection %s <-> %s, terminating stream",
					p.idleTimeout, clientConn.RemoteAddr(), upstreamConn.RemoteAddr())
			}
			_ = clientConn.Close()
			_ = upstreamConn.Close()
		})
	}

	copyDirection := func(dst, src net.Conn) {
		buf := make([]byte, 32*1024)
		for {
			lastNano := lastActivity.Load()
			elapsed := time.Since(time.Unix(0, lastNano))
			if elapsed >= p.idleTimeout {
				closeBoth("idle")
				return
			}
			remaining := p.idleTimeout - elapsed
			_ = src.SetReadDeadline(time.Now().Add(remaining))

			n, readErr := src.Read(buf)
			if n > 0 {
				lastActivity.Store(time.Now().UnixNano())
				_ = dst.SetWriteDeadline(time.Now().Add(p.idleTimeout))
				_, writeErr := dst.Write(buf[:n])
				if writeErr != nil {
					closeBoth("write_error")
					return
				}
			}
			if readErr != nil {
				if readErr == io.EOF {
					if tc, ok := dst.(interface{ CloseWrite() error }); ok {
						_ = tc.CloseWrite()
					}
					return
				}
				if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
					select {
					case <-p.closed:
						closeBoth("closed")
						return
					default:
					}
					last := lastActivity.Load()
					if time.Since(time.Unix(0, last)) >= p.idleTimeout {
						closeBoth("idle")
						return
					}
					continue
				}
				closeBoth("read_error")
				return
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		copyDirection(upstreamConn, clientConn)
	}()
	go func() {
		defer wg.Done()
		copyDirection(clientConn, upstreamConn)
	}()
	wg.Wait()
}

// Close gracefully closes the proxy listener and terminates all active connections.
func (p *TCPProxy) Close() error {
	p.mu.Lock()
	select {
	case <-p.closed:
		p.mu.Unlock()
		return nil
	default:
		close(p.closed)
	}

	var lErr error
	if p.listener != nil {
		lErr = p.listener.Close()
	}
	p.mu.Unlock()

	p.connsMu.Lock()
	for conn := range p.activeConnSet {
		_ = conn.Close()
	}
	p.activeConnSet = make(map[net.Conn]struct{})
	p.connsMu.Unlock()

	return lErr
}
