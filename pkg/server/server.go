package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/reactor"
	"toron/pkg/router"
)

// ErrServerClosed is returned when operations are performed on a closed server.
var ErrServerClosed = reactor.ErrServerClosed

// Server orchestrates the reactor, router, and HTTP request processing lifecycle.
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

	for {
		if s.config.ReadTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(s.config.ReadTimeout))
		}

		req, err := httpparser.ParseRequest(conn, opts)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}

			// Send appropriate error response
			res := httpparser.NewResponse()
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

		// Process request through router
		res := httpparser.NewResponse()
		s.router.ServeHTTP(req, res)

		if s.config.WriteTimeout > 0 {
			_ = conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
		}

		if err := res.Serialize(conn); err != nil {
			return fmt.Errorf("server: failed to write response: %w", err)
		}

		if req.Header.Get("Connection") == "close" {
			return nil
		}
		return nil
	}
}
