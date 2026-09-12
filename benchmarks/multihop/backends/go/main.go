package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
)

var requestCount uint64

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9103"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		seq := atomic.AddUint64(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-Runtime", "go-nethttp")
		w.Header().Set("X-Backend-Seq", fmt.Sprintf("%d", seq))
		json.NewEncoder(w).Encode(map[string]any{
			"status":           "ok",
			"runtime":          "go",
			"engine":           "net/http",
			"requests_handled": seq,
		})
	})

	mux.HandleFunc("/canary", func(w http.ResponseWriter, r *http.Request) {
		seq := atomic.AddUint64(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-Runtime", "go-nethttp")
		w.Header().Set("X-Backend-Seq", fmt.Sprintf("%d", seq))
		json.NewEncoder(w).Encode(map[string]any{
			"status":   "canary_ok",
			"runtime":  "go",
			"engine":   "net/http",
			"sequence": seq,
		})
	})

	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		seq := atomic.AddUint64(&requestCount, 1)
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		headers := make(map[string][]string)
		for k, v := range r.Header {
			headers[k] = v
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-Runtime", "go-nethttp")
		w.Header().Set("X-Backend-Seq", fmt.Sprintf("%d", seq))
		json.NewEncoder(w).Encode(map[string]any{
			"status":      "echo",
			"runtime":     "go",
			"engine":      "net/http",
			"sequence":    seq,
			"body_length": len(body),
			"headers":     headers,
			"body":        string(body),
		})
	})

	fmt.Printf("Go origin (net/http) listening on :%s\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
		os.Exit(1)
	}
}
