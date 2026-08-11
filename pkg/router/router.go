package router

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"toron/pkg/httpparser"
)

// HandlerFunc describes an HTTP request handler function in Toron.
type HandlerFunc func(req *httpparser.Request, res *httpparser.Response)

// MiddlewareFunc describes middleware wrapping a HandlerFunc.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// Router handles URL routing, method dispatching, and middleware execution.
type Router struct {
	mu          sync.RWMutex
	routes      map[string]map[string]HandlerFunc // path -> method -> handler
	middlewares []MiddlewareFunc
	NotFound    HandlerFunc
	MethodNotAllowed HandlerFunc
}

// New creates a new Router instance with default 404/405 handlers.
func New() *Router {
	r := &Router{
		routes: make(map[string]map[string]HandlerFunc),
	}

	r.NotFound = func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusNotFound)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"error":"404 Not Found"}`)
	}

	r.MethodNotAllowed = func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusMethodNotAllowed)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"error":"405 Method Not Allowed"}`)
	}

	return r
}

// Use adds global middlewares to the router chain.
func (r *Router) Use(mw ...MiddlewareFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middlewares = append(r.middlewares, mw...)
}

// Handle registers a handler for a specific HTTP method and path pattern.
func (r *Router) Handle(method, path string, handler HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.routes[path]; !exists {
		r.routes[path] = make(map[string]HandlerFunc)
	}
	r.routes[path][method] = handler
}

// GET convenience helper.
func (r *Router) GET(path string, handler HandlerFunc) {
	r.Handle("GET", path, handler)
}

// POST convenience helper.
func (r *Router) POST(path string, handler HandlerFunc) {
	r.Handle("POST", path, handler)
}

// ServeHTTP dispatches the request to registered handlers through the middleware chain.
func (r *Router) ServeHTTP(req *httpparser.Request, res *httpparser.Response) {
	r.mu.RLock()
	methodsMap, pathExists := r.routes[req.Path]
	var targetHandler HandlerFunc
	if pathExists {
		h, methodExists := methodsMap[req.Method]
		if methodExists {
			targetHandler = h
		} else {
			targetHandler = r.MethodNotAllowed
		}
	} else {
		targetHandler = r.NotFound
	}
	middlewares := append([]MiddlewareFunc(nil), r.middlewares...)
	r.mu.RUnlock()

	// Chain middlewares in reverse order
	finalChain := targetHandler
	for i := len(middlewares) - 1; i >= 0; i-- {
		finalChain = middlewares[i](finalChain)
	}

	finalChain(req, res)
}

// LoggerMiddleware logs incoming requests and processing duration.
func LoggerMiddleware() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			start := time.Now()
			next(req, res)
			log.Printf("[TORON] %s %s -> %d (%v)", req.Method, req.Path, res.StatusCode, time.Since(start))
		}
	}
}

// RecoveryMiddleware captures panics and converts them to HTTP 500 Internal Server Error.
func RecoveryMiddleware() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[RECOVERY] Panic recovered: %v", r)
					res.SetStatus(http.StatusInternalServerError)
					res.Header.Set("Content-Type", "application/json")
					res.Body.Reset()
					_, _ = res.WriteString(fmt.Sprintf(`{"error":"500 Internal Server Error"}`))
				}
			}()
			next(req, res)
		}
	}
}
