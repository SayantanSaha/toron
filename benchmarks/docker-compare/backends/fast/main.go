package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
)

var requestCounter uint64

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9104"
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		seq := atomic.AddUint64(&requestCounter, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-Runtime", "go-fast-echo")
		w.Header().Set("X-Backend-Seq", fmt.Sprintf("%d", seq))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"ok","runtime":"fast-echo","engine":"go-fast","requests_handled":%d}`, seq)))
	})

	mux.HandleFunc("/canary", func(w http.ResponseWriter, r *http.Request) {
		seq := atomic.AddUint64(&requestCounter, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-Runtime", "go-fast-echo")
		w.Header().Set("X-Backend-Seq", fmt.Sprintf("%d", seq))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"canary_ok","runtime":"fast-echo","sequence":%d}`, seq)))
	})

	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		seq := atomic.AddUint64(&requestCounter, 1)
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		headers := make(map[string][]string, len(r.Header))
		for k, v := range r.Header {
			headers[k] = v
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-Runtime", "go-fast-echo")
		w.Header().Set("X-Backend-Seq", fmt.Sprintf("%d", seq))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":      "echo",
			"runtime":     "fast-echo",
			"sequence":    seq,
			"body_length": len(body),
			"headers":     headers,
			"body":        string(body),
		})
	})

	// Fallback handler for all other paths
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		seq := atomic.AddUint64(&requestCounter, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-Runtime", "go-fast-echo")
		w.Header().Set("X-Backend-Seq", fmt.Sprintf("%d", seq))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"ok","path":%q,"sequence":%d}`, r.URL.Path, seq)))
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	fmt.Printf("Fast Go Origin listening on :%s\n", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "Fast origin error: %v\n", err)
		os.Exit(1)
	}
}
