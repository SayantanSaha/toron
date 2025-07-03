package utils

import (
	"log"
	"net"
	"net/http"
	"strings"
)

// NewHTTPSRedirectHandler returns a handler that redirects HTTP traffic to HTTPS,
// preserving the original request URI and optionally including a custom HTTPS port.
func NewHTTPSRedirectHandler(httpsPort string) http.HandlerFunc {
	// Normalize port format
	log.Printf("Setting up HTTPS redirect handler for port: %s", httpsPort)
	portOnly := strings.TrimPrefix(httpsPort, ":")
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received redirect request: %s %s", r.Method, r.URL.String())
		host := r.Host

		// Remove port from incoming host if present
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		target := "https://" + host
		if portOnly != "443" && portOnly != "" {
			target += ":" + portOnly
		}
		target += r.URL.RequestURI()
		log.Printf("Redirecting %s to %s", r.URL.String(), target)
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}
}
