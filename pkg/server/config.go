package server

import "time"

// Config defines the configuration for the Toron web server.
type Config struct {
	Addr           string
	WorkerPoolSize int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	MaxHeaderBytes int
	MaxBodyBytes   int64
}

// DefaultConfig provides recommended production defaults.
func DefaultConfig() Config {
	return Config{
		Addr:           ":8080",
		WorkerPoolSize: 128,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 8 * 1024,        // 8 KB
		MaxBodyBytes:   4 * 1024 * 1024, // 4 MB
	}
}
