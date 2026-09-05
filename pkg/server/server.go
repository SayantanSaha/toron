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
	"sync"
	"time"

	"golang.org/x/net/http2"

	"github.com/quic-go/quic-go/http3"

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
	mu       sync.RWMutex
	config   Config
	router   *router.Router
	reactor  *reactor.Reactor
	h3Server *http3.Server
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

// SNIRegistry returns the server's SNIRegistry instance, if configured.
func (s *Server) SNIRegistry() *SNIRegistry {
	return s.config.SNIRegistry
}

// SetSNIRegistry configures or updates the server's SNIRegistry.
func (s *Server) SetSNIRegistry(r *SNIRegistry) {
	s.config.SNIRegistry = r
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	h3 := s.h3Server
	s.mu.Unlock()

	if h3 != nil {
		_ = h3.Close()
	}
	if s.reactor != nil {
		return s.reactor.Shutdown(ctx)
	}
	return nil
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
			case errors.Is(err, httpparser.ErrUnsupportedTransferEncoding):
				res.SetStatus(http.StatusNotImplemented)
				res.Header.Set("Content-Type", "application/json")
				_, _ = res.WriteString(`{"error":"501 Not Implemented: Unsupported Transfer-Encoding"}`)
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

		if s.config.HTTPRedirectEnabled {
			if s.router.ShouldRedirectHTTP(req) {
				s.serveHTTPRedirect(req, res)
			} else {
				s.router.ServeHTTP(req, res)
			}
		} else {
			s.router.ServeHTTP(req, res)
		}

		if s.config.HTTP3Enabled && s.config.HTTP3AltSvcHeader {
			if res.Header.Get("Alt-Svc") == "" {
				h3Port := s.config.HTTP3Port
				if h3Port <= 0 {
					h3Port = 8443
				}
				res.Header.Set("Alt-Svc", fmt.Sprintf(`h3=":%d"; ma=2592000`, h3Port))
			}
		}

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
		req := httpparser.NewRequestFromStd(r)

		if r.Method != "CONNECT" && r.Body != nil {
			defer r.Body.Close()
			maxBodyBytes := s.config.MaxBodyBytes
			if maxBodyBytes <= 0 {
				maxBodyBytes = 4 * 1024 * 1024
			}
			lr := io.LimitReader(r.Body, maxBodyBytes+1)
			bodyBytes, err := io.ReadAll(lr)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"400 Bad Request"}`))
				return
			}
			if int64(len(bodyBytes)) > maxBodyBytes {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				_, _ = w.Write([]byte(`{"error":"413 Payload Too Large"}`))
				return
			}
			req.Body = bytes.NewReader(bodyBytes)
			req.ContentLength = int64(len(bodyBytes))
		}

		res := httpparser.NewResponse()
		s.router.ServeHTTP(req, res)

		if s.config.HTTP3Enabled && s.config.HTTP3AltSvcHeader {
			if res.Header.Get("Alt-Svc") == "" {
				h3Port := s.config.HTTP3Port
				if h3Port <= 0 {
					h3Port = 8443
				}
				res.Header.Set("Alt-Svc", fmt.Sprintf(`h3=":%d"; ma=2592000`, h3Port))
			}
		}

		for k, vv := range res.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}

		// RFC 8441: HTTP/2 WebSockets use 200 OK for successful extended CONNECT upgrade
		statusCode := res.StatusCode
		if statusCode == http.StatusSwitchingProtocols {
			statusCode = http.StatusOK
		}

		w.WriteHeader(statusCode)

		if res.UpgradedConn != nil {
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			go func() {
				_, _ = io.Copy(res.UpgradedConn, r.Body)
				_ = res.UpgradedConn.Close()
			}()
			_, _ = io.Copy(w, res.UpgradedConn)
			_ = res.UpgradedConn.Close()
			return
		}

		if len(res.Body.Bytes()) > 0 {
			_, _ = w.Write(res.Body.Bytes())
		}
	})
}

// HTTP2AdapterHandler returns an http.Handler that adapts Go's net/http requests to Toron's router.
func (s *Server) HTTP2AdapterHandler() http.Handler {
	return s.http2AdapterHandler()
}

// H3Server returns the active HTTP/3 server instance, if any.
func (s *Server) H3Server() *http3.Server {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.h3Server
}

// SetH3Server sets the active HTTP/3 server instance (used for testing or custom engine injection).
func (s *Server) SetH3Server(h *http3.Server) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.h3Server = h
}

// ListenAndServeH3 starts an HTTP/3 server over QUIC (UDP).
func (s *Server) ListenAndServeH3(certFile, keyFile string) error {
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

	port := s.config.HTTP3Port
	if port <= 0 {
		port = 8443
	}

	s.mu.Lock()
	s.h3Server = &http3.Server{
		Addr:      fmt.Sprintf(":%d", port),
		Handler:   s.http2AdapterHandler(),
		TLSConfig: tlsConfig,
	}
	h3 := s.h3Server
	s.mu.Unlock()

	err = h3.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return ErrServerClosed
	}
	return err
}

// serveHTTPRedirect handles cleartext HTTP-to-HTTPS redirection, preserving the request URI and query parameters.
func (s *Server) serveHTTPRedirect(req *httpparser.Request, res *httpparser.Response) {
	rawHost := req.Header.Get("Host")
	if rawHost == "" {
		res.SetStatus(http.StatusBadRequest)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"error":"400 Bad Request: Missing Host Header"}`)
		return
	}

	// Reject CRLF, control characters, backslashes, whitespace to prevent Open Redirect (CWE-601) and HTTP Response Splitting (CWE-113)
	if strings.ContainsAny(rawHost, "\r\n\t /\\") {
		res.SetStatus(http.StatusBadRequest)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"error":"400 Bad Request: Malformed Host Header"}`)
		return
	}

	// Strip port from Host header if present (e.g., example.com:80 -> example.com)
	host := rawHost
	if h, _, err := net.SplitHostPort(rawHost); err == nil {
		host = h
	}

	// Validate Host against recognized domains to prevent Open Redirect (ADR-065 / CWE-601)
	if !s.isRecognizedHost(host) {
		if s.config.HTTPRedirectDefaultHost != "" {
			host = s.config.HTTPRedirectDefaultHost
		} else {
			res.SetStatus(http.StatusBadRequest)
			res.Header.Set("Content-Type", "application/json")
			_, _ = res.WriteString(`{"error":"400 Bad Request: Unrecognized Host Header"}`)
			return
		}
	}

	targetPort := s.config.HTTPSPort
	if targetPort <= 0 {
		targetPort = 443
	}

	uri := req.RequestURI
	if uri == "" {
		uri = req.Path
		if uri == "" {
			uri = "/"
		}
	}
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}

	var redirectURL string
	if targetPort == 443 {
		redirectURL = "https://" + host + uri
	} else {
		redirectURL = fmt.Sprintf("https://%s:%d%s", host, targetPort, uri)
	}

	res.SetStatus(http.StatusMovedPermanently)
	res.Header.Set("Location", redirectURL)
	res.Header.Set("Content-Length", "0")
}

// isRecognizedHost verifies whether the host header matches an allowed or configured domain (ADR-065 / CWE-601).
func (s *Server) isRecognizedHost(host string) bool {
	if host == "" {
		return false
	}
	h := strings.ToLower(strings.TrimSpace(host))

	// 1. Explicitly configured AllowedHosts
	for _, allowed := range s.config.HTTPRedirectAllowedHosts {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if allowed == "*" || allowed == h {
			return true
		}
		if strings.HasPrefix(allowed, "*.") {
			suffix := allowed[1:]
			if strings.HasSuffix(h, suffix) || h == allowed[2:] {
				return true
			}
		}
	}

	// 2. DefaultHost
	if s.config.HTTPRedirectDefaultHost != "" && strings.EqualFold(h, s.config.HTTPRedirectDefaultHost) {
		return true
	}

	// 3. SNIRegistry
	if s.config.SNIRegistry != nil && s.config.SNIRegistry.HasHost(h) {
		return true
	}

	// 4. Router configured route hosts
	if s.router != nil && s.router.HasHost(h) {
		return true
	}

	// 5. Server Addr host
	if s.config.Addr != "" {
		addrHost, _, err := net.SplitHostPort(s.config.Addr)
		if err == nil && addrHost != "" && strings.EqualFold(h, addrHost) {
			return true
		}
	}

	// 5. Localhost and loopback addresses
	if h == "localhost" || h == "127.0.0.1" || h == "::1" {
		return true
	}

	return false
}
