package proxy

import (
	"errors"
	"log"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// UDPOptions holds configuration options for UDPProxy.
type UDPOptions struct {
	MaxWorkers  int
	IdleTimeout time.Duration
}

// UDPOption defines a functional option for configuring a UDPProxy.
type UDPOption func(*UDPProxy)

// WithUDPMaxWorkers sets the maximum concurrent UDP worker goroutines / queue capacity.
func WithUDPMaxWorkers(n int) UDPOption {
	return func(p *UDPProxy) {
		if n > 0 {
			p.maxWorkers = n
		}
	}
}

// WithMaxWorkers is an alias for WithUDPMaxWorkers.
func WithMaxWorkers(n int) UDPOption {
	return WithUDPMaxWorkers(n)
}

// WithUDPIdleTimeout sets the inactivity timeout for cached client sessions.
func WithUDPIdleTimeout(d time.Duration) UDPOption {
	return func(p *UDPProxy) {
		if d > 0 {
			p.idleTimeout = d
		}
	}
}

// WithUDPOptions applies a UDPOptions configuration.
func WithUDPOptions(opts UDPOptions) UDPOption {
	return func(p *UDPProxy) {
		if opts.MaxWorkers > 0 {
			p.maxWorkers = opts.MaxWorkers
		}
		if opts.IdleTimeout > 0 {
			p.idleTimeout = opts.IdleTimeout
		}
	}
}

type udpPacketTask struct {
	clientAddrPort netip.AddrPort
	bufPtr         *[65535]byte
	n              int
}

type udpSession struct {
	clientAddrPort netip.AddrPort
	upstreamConn   *net.UDPConn
	targetAddr     string
	lastActivity   atomic.Int64
	closed         chan struct{}
	closeOnce      sync.Once
}

func (s *udpSession) touch() {
	s.lastActivity.Store(time.Now().UnixNano())
}

func (s *udpSession) isExpired(now time.Time, timeout time.Duration) bool {
	last := time.Unix(0, s.lastActivity.Load())
	return now.Sub(last) > timeout
}

func (s *udpSession) Close() {
	s.closeOnce.Do(func() {
		close(s.closed)
		if s.upstreamConn != nil {
			_ = s.upstreamConn.Close()
		}
	})
}

// UDPProxy implements Layer 4 UDP datagram socket proxying and packet forwarding.
type UDPProxy struct {
	targets     []string
	counter     uint64
	timeout     time.Duration
	maxWorkers  int
	idleTimeout time.Duration

	mu          sync.Mutex
	conn        *net.UDPConn
	closed      chan struct{}
	packetQueue chan udpPacketTask
	bufPool     sync.Pool

	sessionMu sync.RWMutex
	sessions  map[netip.AddrPort]*udpSession
}

// NewUDPProxy creates a new UDPProxy for forwarding UDP datagrams to target backends.
func NewUDPProxy(targets []string, timeout time.Duration, opts ...UDPOption) (*UDPProxy, error) {
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
	p := &UDPProxy{
		targets:     copied,
		timeout:     timeout,
		maxWorkers:  1024,
		idleTimeout: 60 * time.Second,
		closed:      make(chan struct{}),
		sessions:    make(map[netip.AddrPort]*udpSession),
		bufPool: sync.Pool{
			New: func() any {
				return new([65535]byte)
			},
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	p.packetQueue = make(chan udpPacketTask, p.maxWorkers)
	return p, nil
}

// ActiveSessions returns the number of currently active UDP sessions in the registry.
func (p *UDPProxy) ActiveSessions() int {
	p.sessionMu.RLock()
	defer p.sessionMu.RUnlock()
	return len(p.sessions)
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

	var workerWg sync.WaitGroup
	for i := 0; i < p.maxWorkers; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for task := range p.packetQueue {
				p.forwardPacket(conn, task.clientAddrPort, task.bufPtr[:task.n])
				p.bufPool.Put(task.bufPtr)
			}
		}()
	}

	go p.startSweeper()

	var closeOnce sync.Once
	defer func() {
		closeOnce.Do(func() {
			close(p.packetQueue)
		})
		workerWg.Wait()
	}()

	for {
		bufPtr := p.bufPool.Get().(*[65535]byte)
		n, clientAddrPort, err := conn.ReadFromUDPAddrPort(bufPtr[:])
		if err != nil {
			p.bufPool.Put(bufPtr)
			select {
			case <-p.closed:
				return nil
			default:
				return err
			}
		}

		task := udpPacketTask{
			clientAddrPort: clientAddrPort,
			bufPtr:         bufPtr,
			n:              n,
		}

		select {
		case p.packetQueue <- task:
		case <-p.closed:
			p.bufPool.Put(bufPtr)
			return nil
		default:
			p.bufPool.Put(bufPtr)
			log.Printf("[UDPProxy] Dropping datagram from %s: worker queue saturated (%d workers)", clientAddrPort, p.maxWorkers)
		}
	}
}

func (p *UDPProxy) forwardPacket(inboundConn *net.UDPConn, clientAddrPort netip.AddrPort, packet []byte) {
	sess, err := p.getOrCreateSession(inboundConn, clientAddrPort)
	if err != nil {
		log.Printf("[UDPProxy] Error resolving session for %s: %v", clientAddrPort, err)
		return
	}

	sess.touch()
	_, err = sess.upstreamConn.Write(packet)
	if err != nil {
		log.Printf("[UDPProxy] Error forwarding datagram to %s: %v", sess.targetAddr, err)
	}
}

func (p *UDPProxy) getOrCreateSession(inboundConn *net.UDPConn, clientAddrPort netip.AddrPort) (*udpSession, error) {
	p.sessionMu.RLock()
	sess, ok := p.sessions[clientAddrPort]
	p.sessionMu.RUnlock()
	if ok {
		return sess, nil
	}

	targetAddrStr := p.SelectTarget()
	targetUDPAddr, err := net.ResolveUDPAddr("udp", targetAddrStr)
	if err != nil {
		return nil, err
	}

	upstreamConn, err := net.DialUDP("udp", nil, targetUDPAddr)
	if err != nil {
		return nil, err
	}

	p.sessionMu.Lock()
	if existing, ok := p.sessions[clientAddrPort]; ok {
		p.sessionMu.Unlock()
		_ = upstreamConn.Close()
		return existing, nil
	}

	select {
	case <-p.closed:
		p.sessionMu.Unlock()
		_ = upstreamConn.Close()
		return nil, errors.New("udpproxy: closed")
	default:
	}

	sess = &udpSession{
		clientAddrPort: clientAddrPort,
		upstreamConn:   upstreamConn,
		targetAddr:     targetAddrStr,
		closed:         make(chan struct{}),
	}
	sess.touch()
	p.sessions[clientAddrPort] = sess
	p.sessionMu.Unlock()

	go p.readUpstreamResponses(inboundConn, sess)
	return sess, nil
}

func (p *UDPProxy) readUpstreamResponses(inboundConn *net.UDPConn, sess *udpSession) {
	for {
		respBufPtr := p.bufPool.Get().(*[65535]byte)
		n, err := sess.upstreamConn.Read(respBufPtr[:])
		if err != nil {
			p.bufPool.Put(respBufPtr)
			return
		}
		sess.touch()
		_, _ = inboundConn.WriteToUDPAddrPort(respBufPtr[:n], sess.clientAddrPort)
		p.bufPool.Put(respBufPtr)
	}
}

func (p *UDPProxy) startSweeper() {
	sweepInterval := p.idleTimeout / 2
	if sweepInterval <= 0 {
		sweepInterval = 10 * time.Millisecond
	}
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.closed:
			return
		case now := <-ticker.C:
			p.sweepExpiredSessions(now)
		}
	}
}

func (p *UDPProxy) sweepExpiredSessions(now time.Time) {
	var expired []*udpSession
	p.sessionMu.Lock()
	for ap, sess := range p.sessions {
		if sess.isExpired(now, p.idleTimeout) {
			expired = append(expired, sess)
			delete(p.sessions, ap)
			log.Printf("[UDPProxy] Idle session expired for client %s, closed upstream socket", sess.clientAddrPort)
		}
	}
	p.sessionMu.Unlock()

	for _, sess := range expired {
		sess.Close()
	}
}

// Close gracefully closes the UDP proxy socket listener and all session sockets.
func (p *UDPProxy) Close() error {
	p.mu.Lock()
	select {
	case <-p.closed:
		p.mu.Unlock()
		return nil
	default:
		close(p.closed)
	}

	var err error
	if p.conn != nil {
		err = p.conn.Close()
	}
	p.mu.Unlock()

	p.sessionMu.Lock()
	for _, sess := range p.sessions {
		sess.Close()
	}
	p.sessions = make(map[netip.AddrPort]*udpSession)
	p.sessionMu.Unlock()

	return err
}
