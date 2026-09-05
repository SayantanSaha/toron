package server

import "time"

// Config defines the configuration for the Toron web server.
type Config struct {
	Addr                      string
	WorkerPoolSize            int
	ReadTimeout               time.Duration
	WriteTimeout              time.Duration
	IdleTimeout               time.Duration
	MaxHeaderBytes            int
	MaxBodyBytes              int64
	HTTP2Enabled              bool
	HTTP2MaxConcurrentStreams uint32
	HTTP2MaxFrameSize         uint32
	TLSEnabled                bool
	TLSCertFile               string
	TLSKeyFile                string
	TLSAutoDevCert            bool
	SNIRegistry               *SNIRegistry
	HTTP3Enabled              bool
	HTTP3Port                 int
	HTTP3AltSvcHeader         bool
	HTTPRedirectEnabled       bool
	HTTPRedirectPort          int
	HTTPRedirectAllowedHosts  []string
	HTTPRedirectDefaultHost   string
	HTTPSPort                 int
}

// DefaultConfig provides recommended production defaults.
func DefaultConfig() Config {
	return Config{
		Addr:                      ":8080",
		WorkerPoolSize:            128,
		ReadTimeout:               5 * time.Second,
		WriteTimeout:              5 * time.Second,
		IdleTimeout:               30 * time.Second,
		MaxHeaderBytes:            8 * 1024,        // 8 KB
		MaxBodyBytes:              4 * 1024 * 1024, // 4 MB
		HTTP2Enabled:              true,
		HTTP2MaxConcurrentStreams: 250,
		HTTP2MaxFrameSize:         16384,
		TLSEnabled:                false,
		TLSAutoDevCert:            true,
		HTTP3Enabled:              true,
		HTTP3Port:                 8443,
		HTTP3AltSvcHeader:         true,
		HTTPRedirectEnabled:       false,
		HTTPRedirectPort:          80,
		HTTPSPort:                 443,
	}
}
