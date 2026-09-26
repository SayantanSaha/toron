package server

import (
	"bufio"
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

func NewPrefixConn(conn net.Conn, prefix []byte) net.Conn {
	return &prefixConn{Conn: conn, prefix: prefix}
}

type ActivityReader = activityReader

func NewActivityReader(r *bufio.Reader, conn net.Conn, tracker *connDeadlineTracker, timeout time.Duration, limit int64) *activityReader {
	return newActivityReader(r, conn, tracker, timeout, limit)
}
