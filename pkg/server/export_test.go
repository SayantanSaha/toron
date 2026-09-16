package server

import (
	"net"
	"time"
)

// Export for whitebox testing in package server_test
type ConnDeadlineTracker = connDeadlineTracker

func NewConnDeadlineTracker(conn net.Conn) *connDeadlineTracker {
	return newConnDeadlineTracker(conn)
}

func RelayStreams(conn1, conn2 net.Conn, idleTimeout time.Duration) {
	relayStreams(conn1, conn2, idleTimeout)
}
