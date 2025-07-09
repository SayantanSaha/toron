package listener

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SayantanSaha/toron/internal/config"
	"golang.org/x/crypto/acme/autocert"
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
		log.Printf("[Toron] TLS mode: %s", l.cfg.TLSMode)
		log.Printf("[Toron] Starting HTTPS listener on %s", l.cfg.Address)

		switch strings.ToLower(l.cfg.TLSMode) {
		case "autocert":
			return l.startAutocert()
		case "mtls":
			return l.startMTLS()
		case "manual":
			fallthrough
		default:
			return l.server.ListenAndServeTLS(l.cfg.CertFile, l.cfg.KeyFile)
		}
	}
	log.Printf("[Toron] Starting HTTP listener on %s", l.cfg.Address)
	return l.server.ListenAndServe()
}

func (l *HTTPListener) Stop(ctx context.Context) error {
	log.Println("[Toron] Shutting down HTTP server...")
	return l.server.Shutdown(ctx)
}

func (l *HTTPListener) startAutocert() error {
	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(l.cfg.AutocertDomains...),
		Cache:      autocert.DirCache("certs/autocert"),
	}

	l.server.TLSConfig = manager.TLSConfig()

	// Serve HTTPS using autocert's listener (handles TLS internally)
	return l.server.Serve(manager.Listener())
}
func (l *HTTPListener) startMTLS() error {
	caCert, err := os.ReadFile(l.cfg.MTLS.CACertFile)
	if err != nil {
		return err
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	clientAuth := tls.RequireAndVerifyClientCert
	switch strings.ToLower(l.cfg.MTLS.ClientAuthType) {
	case "require":
		clientAuth = tls.RequireAnyClientCert
	case "optional":
		clientAuth = tls.VerifyClientCertIfGiven
	}

	l.server.TLSConfig = &tls.Config{
		ClientCAs:  caPool,
		ClientAuth: clientAuth,
	}

	return l.server.ListenAndServeTLS(l.cfg.CertFile, l.cfg.KeyFile)
}
