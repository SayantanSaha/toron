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

// TCPProxy implements Layer 4 TCP transport socket proxying and stream forwarding.
type TCPProxy struct {
	targets []string
	counter uint64
	timeout time.Duration

	mu       sync.Mutex
	listener net.Listener
	closed   chan struct{}
}

// NewTCPProxy creates a new TCPProxy for forwarding TCP streams to target backends.
func NewTCPProxy(targets []string, timeout time.Duration) (*TCPProxy, error) {
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
	return &TCPProxy{
		targets: copied,
		timeout: timeout,
		closed:  make(chan struct{}),
	}, nil
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

		go p.handleConn(clientConn)
	}
}

func (p *TCPProxy) handleConn(clientConn net.Conn) {
	defer clientConn.Close()

	targetAddr := p.SelectTarget()
	upstreamConn, err := net.DialTimeout("tcp", targetAddr, p.timeout)
	if err != nil {
		log.Printf("[TCPProxy] Error dialing target %s: %v", targetAddr, err)
		return
	}
	defer upstreamConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstreamConn, clientConn)
		if tcpConn, ok := upstreamConn.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, upstreamConn)
		if tcpConn, ok := clientConn.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite()
		}
	}()

	wg.Wait()
}

// Close gracefully closes the proxy listener.
func (p *TCPProxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	select {
	case <-p.closed:
		return nil
	default:
		close(p.closed)
	}

	if p.listener != nil {
		return p.listener.Close()
	}
	return nil
}
