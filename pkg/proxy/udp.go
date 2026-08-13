package proxy

import (
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// UDPProxy implements Layer 4 UDP datagram socket proxying and packet forwarding.
type UDPProxy struct {
	targets []string
	counter uint64
	timeout time.Duration

	mu     sync.Mutex
	conn   *net.UDPConn
	closed chan struct{}
}

// NewUDPProxy creates a new UDPProxy for forwarding UDP datagrams to target backends.
func NewUDPProxy(targets []string, timeout time.Duration) (*UDPProxy, error) {
	if len(targets) == 0 {
		return nil, errors.New("udpproxy: no target backends provided")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	copied := make([]string, len(targets))
	for i, t := range targets {
		addr := strings.TrimSpace(t)
		addr = strings.TrimPrefix(addr, "http://")
		addr = strings.TrimPrefix(addr, "https://")
		addr = strings.TrimPrefix(addr, "udp://")
		copied[i] = addr
	}
	return &UDPProxy{
		targets: copied,
		timeout: timeout,
		closed:  make(chan struct{}),
	}, nil
}

// SelectTarget picks the next UDP target backend address using round-robin selection.
func (p *UDPProxy) SelectTarget() string {
	if len(p.targets) == 1 {
		return p.targets[0]
	}
	idx := atomic.AddUint64(&p.counter, 1) - 1
	return p.targets[idx%uint64(len(p.targets))]
}

// Serve binds to incoming UDP datagram packets and relays datagrams to target backends.
func (p *UDPProxy) Serve(conn *net.UDPConn) error {
	p.mu.Lock()
	p.conn = conn
	p.mu.Unlock()

	buf := make([]byte, 65535)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-p.closed:
				return nil
			default:
				return err
			}
		}

		packet := make([]byte, n)
		copy(packet, buf[:n])
		go p.handleDatagram(conn, clientAddr, packet)
	}
}

func (p *UDPProxy) handleDatagram(conn *net.UDPConn, clientAddr *net.UDPAddr, packet []byte) {
	targetAddrStr := p.SelectTarget()
	targetUDPAddr, err := net.ResolveUDPAddr("udp", targetAddrStr)
	if err != nil {
		log.Printf("[UDPProxy] Error resolving target UDP address %s: %v", targetAddrStr, err)
		return
	}

	upstreamConn, err := net.DialUDP("udp", nil, targetUDPAddr)
	if err != nil {
		log.Printf("[UDPProxy] Error dialing target UDP %s: %v", targetAddrStr, err)
		return
	}
	defer upstreamConn.Close()

	_ = upstreamConn.SetDeadline(time.Now().Add(p.timeout))
	_, err = upstreamConn.Write(packet)
	if err != nil {
		return
	}

	respBuf := make([]byte, 65535)
	respN, err := upstreamConn.Read(respBuf)
	if err != nil {
		return
	}

	_, _ = conn.WriteToUDP(respBuf[:respN], clientAddr)
}

// Close gracefully closes the UDP proxy socket listener.
func (p *UDPProxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	select {
	case <-p.closed:
		return nil
	default:
		close(p.closed)
	}

	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}
