package listener

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/SayantanSaha/toron/internal/config"
)

type HTTPListener struct {
	server *http.Server
	cfg    config.ServerConfig
}

func (l *HTTPListener) Address() any {
	panic("unimplemented")
}

func NewHTTPListener(cfg config.ServerConfig, handler http.Handler) *HTTPListener {
	srv := &http.Server{
		Addr:         cfg.Address,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	return &HTTPListener{
		server: srv,
		cfg:    cfg,
	}
}

func (l *HTTPListener) Start() error {
	if l.cfg.UseTLS {
		log.Printf("Starting HTTPS listener on %s", l.cfg.Address)
		return l.server.ListenAndServeTLS(l.cfg.CertFile, l.cfg.KeyFile)
	}
	log.Printf("Starting HTTP listener on %s", l.cfg.Address)
	return l.server.ListenAndServe()
}

func (l *HTTPListener) Stop(ctx context.Context) error {
	log.Println("Shutting down HTTP server...")
	return l.server.Shutdown(ctx)
}
