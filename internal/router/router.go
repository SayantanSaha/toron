package router

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/SayantanSaha/toron/internal/config"
)

type Router struct {
	routes map[string]string
	mux    *http.ServeMux
	mu     sync.RWMutex
}

func NewRouter(routes []config.Route) *Router {
	r := &Router{
		routes: make(map[string]string),
		mux:    http.NewServeMux(),
	}
	log.Println("[Toron] Initializing router with routes...", len(routes))
	if len(routes) > 0 {
		log.Println("[Toron] Found routes:", routes)
		for _, route := range routes {
			r.routes[route.Path] = route.Backend
			var handler http.Handler
			if route.MatchType == "prefix_match" {
				handler = r.dynamicHandler(route.Path, route.Backend, route.StripPrefix)
				r.mux.Handle(route.Path+"/", handler)
				r.mux.Handle(route.Path, handler) // also support base path
			} else {
				handler = r.dynamicHandler(route.Path, route.Backend, route.StripPrefix)
				r.mux.HandleFunc(route.Path, func(w http.ResponseWriter, req *http.Request) {
					if req.URL.Path == route.Path {
						handler.ServeHTTP(w, req)
					} else {
						http.NotFound(w, req)
					}
				})
			}
		}

		// fallback: if route exists but doesn't match
		r.mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
			http.Error(w, "route not found", http.StatusNotFound)
		})
	} else {
		// serve default Hello World page
		r.mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<html><body><h1>Hello, world!</h1></body></html>`))
		})
	}

	return r
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

func (r *Router) dynamicHandler(prefix, backend string, strip bool) http.HandlerFunc {
	// Check if backend is a directory (serve static files)
	if stat, err := os.Stat(backend); err == nil && stat.IsDir() {
		fs := http.StripPrefix(prefix, http.FileServer(http.Dir(backend)))
		log.Printf("[Toron] Serving static files from %s at %s", backend, prefix)
		return func(w http.ResponseWriter, req *http.Request) {
			// Log the request
			log.Printf("[Toron] Serving static file: %s %s", req.Method, req.URL.Path)
			// If strip is true, remove the prefix from the request URL path
			fs.ServeHTTP(w, req)
		}
	}

	// Check if backend is a valid URL (reverse proxy)
	target, err := url.Parse(backend)
	if err != nil {
		log.Printf("[Toron] Invalid backend URL: %s", backend)
		return func(w http.ResponseWriter, req *http.Request) {
			log.Printf("[Toron] Error parsing backend URL %s: %v", req.URL.Path, err)
			// Return 502 Bad Gateway if backend URL is invalid
			// This is a common practice to indicate that the server is unable to reach the backend
			// or the backend URL is malformed.
			// It helps clients understand that the request could not be fulfilled due to a backend issue
			// or misconfiguration.
			// This is not a security risk as it does not expose sensitive information.
			// It simply informs the client that the request could not be processed due to an invalid
			// backend configuration.
			http.Error(w, "invalid backend: "+backend, http.StatusBadGateway)
		}
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	log.Printf("[Toron] Routing %s to backend %s (stripPrefix=%v)", prefix, target.String(), strip)

	proxy.ModifyResponse = func(resp *http.Response) error {
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/json") && !strings.Contains(ct, "text/plain") {
			return nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		original := string(body)

		aliases := []string{
			target.String(),
			"//" + target.Host,
			"http://" + target.Host,
			"https://" + target.Host,
		}

		for _, alias := range aliases {
			re := regexp.MustCompile(regexp.QuoteMeta(alias))
			original = re.ReplaceAllString(original, prefix)
		}

		resp.Body = io.NopCloser(strings.NewReader(original))
		resp.ContentLength = int64(len(original))
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(original)))

		return nil
	}

	return func(w http.ResponseWriter, req *http.Request) {
		// Log the request
		log.Printf("[Toron] Received dynamic request: %s %s", req.Method, req.URL.Path)
		if strip {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
			log.Printf("[Toron] Stripped prefix: %s", req.URL.Path)
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		}
		proxy.ServeHTTP(w, req)
	}
}
