package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/http2"

	"toron/pkg/httpparser"
	"toron/pkg/reactor"
	"toron/pkg/router"
)

const http2ClientPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

// ErrServerClosed is returned when operations are performed on a closed server.
var ErrServerClosed = reactor.ErrServerClosed

type prefixConn struct {
	net.Conn
	prefix []byte
}

func (c *prefixConn) Read(b []byte) (n int, err error) {
	if len(c.prefix) > 0 {
		n = copy(b, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(b)
}

// Server orchestrates the reactor, router, HTTP/1.1, and HTTP/2 request processing lifecycle.
type Server struct {
	config  Config
	router  *router.Router
	reactor *reactor.Reactor
}

// New creates a new Toron HTTP Server instance.
func New(cfg Config, r *router.Router) *Server {
	if r == nil {
		r = router.New()
	}

	srv := &Server{
		config: cfg,
		router: r,
	}

	reactorCfg := reactor.Config{
		Addr:           cfg.Addr,
		WorkerPoolSize: cfg.WorkerPoolSize,
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		IdleTimeout:    cfg.IdleTimeout,
		MaxBufferBytes: cfg.MaxHeaderBytes,
	}

	srv.reactor = reactor.New(reactorCfg, reactor.HandlerFunc(srv.handleConn))
	return srv
}

// Addr returns the net.Addr of the active server listener.
func (s *Server) Addr() net.Addr {
	return s.reactor.Addr()
}

// ListenAndServe binds listener and starts processing incoming HTTP requests.
func (s *Server) ListenAndServe() error {
	return s.reactor.ListenAndServe()
}

// ListenAndServeTLS binds TLS listener and starts processing incoming HTTPS requests.
func (s *Server) ListenAndServeTLS(certFile, keyFile string) error {
	s.config.TLSEnabled = true
	if certFile != "" {
		s.config.TLSCertFile = certFile
	}
	if keyFile != "" {
		s.config.TLSKeyFile = keyFile
	}
	tlsConfig, err := CreateTLSConfig(s.config)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return err
	}
	tlsListener := tls.NewListener(ln, tlsConfig)
	return s.reactor.Serve(tlsListener)
}

// Serve accepts connections from the given net.Listener.
func (s *Server) Serve(ln net.Listener) error {
	return s.reactor.Serve(ln)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.reactor.Shutdown(ctx)
}

// handleConn is the internal handler called by the reactor for each accepted TCP socket connection.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) error {
	opts := httpparser.ParserOptions{
		MaxHeaderBytes: s.config.MaxHeaderBytes,
		MaxBodyBytes:   s.config.MaxBodyBytes,
	}

	// TLS ALPN Protocol Detection (h2 over TLS)
	if tlsConn, ok := conn.(*tls.Conn); ok {
		if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
			h2Server := &http2.Server{
				MaxConcurrentStreams: s.config.HTTP2MaxConcurrentStreams,
				MaxReadFrameSize:     s.config.HTTP2MaxFrameSize,
			}
			h2Server.ServeConn(conn, &http2.ServeConnOpts{
				Handler: s.http2AdapterHandler(),
			})
			return nil
		}
	}

	if s.config.HTTP2Enabled {
		buf := make([]byte, len(http2ClientPreface))
		if s.config.ReadTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(s.config.ReadTimeout))
		}
		n, err := io.ReadFull(conn, buf)
		if err == nil && string(buf[:n]) == http2ClientPreface {
			pConn := &prefixConn{Conn: conn, prefix: buf[:n]}
			h2Server := &http2.Server{
				MaxConcurrentStreams: s.config.HTTP2MaxConcurrentStreams,
				MaxReadFrameSize:     s.config.HTTP2MaxFrameSize,
			}
			h2Server.ServeConn(pConn, &http2.ServeConnOpts{
				Handler: s.http2AdapterHandler(),
			})
			return nil
		}
		if n > 0 {
			conn = &prefixConn{Conn: conn, prefix: buf[:n]}
		}
	}

	firstRequest := true
	for {
		timeout := s.config.ReadTimeout
		if !firstRequest && s.config.IdleTimeout > 0 {
			timeout = s.config.IdleTimeout
		}
		if timeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(timeout))
		}

		req, err := httpparser.ParseRequest(conn, opts)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}

			// If idle timeout occurred on persistent connection, exit loop silently
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() && !firstRequest {
				return nil
			}

			// Send appropriate error response
			res := httpparser.NewResponse()
			res.Header.Set("Connection", "close")
			switch {
			case errors.Is(err, httpparser.ErrHeaderTooLarge):
				res.SetStatus(http.StatusRequestHeaderFieldsTooLarge)
				res.Header.Set("Content-Type", "application/json")
				_, _ = res.WriteString(`{"error":"431 Request Header Fields Too Large"}`)
			case errors.Is(err, httpparser.ErrBodyTooLarge):
				res.SetStatus(http.StatusRequestEntityTooLarge)
				res.Header.Set("Content-Type", "application/json")
				_, _ = res.WriteString(`{"error":"413 Payload Too Large"}`)
			default:
				res.SetStatus(http.StatusBadRequest)
				res.Header.Set("Content-Type", "application/json")
				_, _ = res.WriteString(`{"error":"400 Bad Request"}`)
			}

			if s.config.WriteTimeout > 0 {
				_ = conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
			}
			_ = res.Serialize(conn)
			return err
		}

		req.RawConn = conn
		firstRequest = false

		// Process request through router
		res := httpparser.NewResponse()

		// Set default Connection header
		connHeader := strings.ToLower(req.Header.Get("Connection"))
		if connHeader == "close" {
			res.Header.Set("Connection", "close")
		} else {
			res.Header.Set("Connection", "keep-alive")
		}

		s.router.ServeHTTP(req, res)

		if res.UpgradedConn != nil || res.StatusCode == http.StatusSwitchingProtocols {
			_ = conn.SetDeadline(time.Time{})
			if res.UpgradedConn != nil {
				_ = res.UpgradedConn.SetDeadline(time.Time{})
			}

			if err := res.Serialize(conn); err != nil {
				if res.UpgradedConn != nil {
					res.UpgradedConn.Close()
				}
				return fmt.Errorf("server: failed to write upgrade response: %w", err)
			}

			if res.UpgradedConn != nil {
				go func() {
					_, _ = io.Copy(res.UpgradedConn, conn)
					_ = res.UpgradedConn.Close()
				}()
				_, _ = io.Copy(conn, res.UpgradedConn)
				_ = conn.Close()
				return nil
			}
		}

		if s.config.WriteTimeout > 0 {
			_ = conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
		}

		if err := res.Serialize(conn); err != nil {
			return fmt.Errorf("server: failed to write response: %w", err)
		}

		if connHeader == "close" {
			return nil
		}
	}
}

func (s *Server) http2AdapterHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := &httpparser.Request{
			Method: r.Method,
			Path:   r.URL.Path,
			Proto:  r.Proto,
			Header: make(httpparser.Header),
		}
		for k, vv := range r.Header {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
		if r.Host != "" {
			req.Header.Set("Host", r.Host)
		}

		if r.Body != nil {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil {
				req.Body = bytes.NewReader(bodyBytes)
			}
		}

		res := httpparser.NewResponse()
		s.router.ServeHTTP(req, res)

		for k, vv := range res.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(res.StatusCode)
		if len(res.Body.Bytes()) > 0 {
			_, _ = w.Write(res.Body.Bytes())
		}
	})
}
