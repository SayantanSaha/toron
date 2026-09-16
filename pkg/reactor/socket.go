package reactor

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// ExtractTCPConn iteratively and recursively inspects and unwraps conn to locate
// the underlying *net.TCPConn. It transparently traverses *tls.Conn,
// interface{ Unwrap() net.Conn }, and interface{ NetConn() net.Conn }.
// A bounded depth limit guards against circular wrappers.
// Returns nil if the underlying transport is not a TCP socket or conn is nil.
func ExtractTCPConn(conn net.Conn) *net.TCPConn {
	current := conn
	const maxDepth = 10
	depth := 0

	for current != nil && depth < maxDepth {
		depth++
		if tcpConn, ok := current.(*net.TCPConn); ok {
			return tcpConn
		}
		if tc, ok := current.(*tls.Conn); ok {
			current = tc.NetConn()
			continue
		}
		if tc, ok := current.(interface{ NetConn() net.Conn }); ok {
			current = tc.NetConn()
			continue
		}
		if uw, ok := current.(interface{ Unwrap() net.Conn }); ok {
			current = uw.Unwrap()
			continue
		}
		break
	}
	return nil
}

// ConfigureTCPSocket inspects conn, extracts the underlying *net.TCPConn,
// and applies mission-critical transport options:
// 1. TCP_NODELAY (SetNoDelay(true)) to eliminate Nagle buffering latency.
// 2. TCP Keep-Alive (SetKeepAlive(true)) to detect half-open sockets.
// 3. Keep-Alive Probe Period (SetKeepAlivePeriod(60 * time.Second)) to bound dead connection detection.
// If conn is nil or not a TCP socket, ConfigureTCPSocket returns nil (no-op).
func ConfigureTCPSocket(conn net.Conn) error {
	tcpConn := ExtractTCPConn(conn)
	if tcpConn == nil {
		// In-memory pipe, Unix domain socket, or non-TCP connection: no-op
		return nil
	}

	// 1. Disable Nagle's algorithm: emit small frames immediately without delayed ACK stalls
	if err := tcpConn.SetNoDelay(true); err != nil {
		return fmt.Errorf("failed to set TCP_NODELAY: %w", err)
	}

	// 2. Enable TCP keep-alive probes for detecting half-open sockets during quiet intervals
	if err := tcpConn.SetKeepAlive(true); err != nil {
		return fmt.Errorf("failed to enable TCP keepalive: %w", err)
	}

	// 3. Set keep-alive probe period to 60 seconds
	if err := tcpConn.SetKeepAlivePeriod(60 * time.Second); err != nil {
		return fmt.Errorf("failed to set TCP keepalive period: %w", err)
	}

	return nil
}
