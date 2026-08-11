package router

import (
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"toron/pkg/httpparser"
)

// HandlerFunc describes an HTTP request handler function in Toron.
type HandlerFunc func(req *httpparser.Request, res *httpparser.Response)

// MiddlewareFunc describes middleware wrapping a HandlerFunc.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

type prefixRoute struct {
	prefix  string
	handler HandlerFunc
}

// Router handles URL routing, method dispatching, static file serving, and middleware execution.
type Router struct {
	mu               sync.RWMutex
	routes           map[string]map[string]HandlerFunc // path -> method -> handler
	prefixRoutes     []prefixRoute
	middlewares      []MiddlewareFunc
	NotFound         HandlerFunc
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

// Handle registers a handler for a specific HTTP method and exact path.
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

// Static registers a URL prefix to serve static files from a local directory path.
func (r *Router) Static(prefix, dirPath string) {
	cleanPrefix := "/" + strings.Trim(prefix, "/")
	if cleanPrefix == "/" {
		cleanPrefix = ""
	}

	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		absDir = dirPath
	}

	staticHandler := func(req *httpparser.Request, res *httpparser.Response) {
		if req.Method != "GET" && req.Method != "HEAD" {
			r.MethodNotAllowed(req, res)
			return
		}

		relPath := req.Path
		if cleanPrefix != "" {
			relPath = strings.TrimPrefix(req.Path, cleanPrefix)
		}
		if relPath == "" || relPath == "/" {
			relPath = "/index.html"
		}

		// Security: Prevent path traversal
		cleanRel := filepath.Clean(filepath.FromSlash(relPath))
		targetPath := filepath.Join(absDir, cleanRel)

		relFromDir, err := filepath.Rel(absDir, targetPath)
		if err != nil || strings.HasPrefix(relFromDir, "..") || strings.HasPrefix(relFromDir, ".") && len(relFromDir) > 1 && relFromDir[1] == '.' {
			res.SetStatus(http.StatusForbidden)
			res.Header.Set("Content-Type", "application/json")
			_, _ = res.WriteString(`{"error":"403 Forbidden: Path Traversal Disallowed"}`)
			return
		}

		fileInfo, err := os.Stat(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				r.NotFound(req, res)
				return
			}
			res.SetStatus(http.StatusInternalServerError)
			return
		}

		if fileInfo.IsDir() {
			targetPath = filepath.Join(targetPath, "index.html")
			fileInfo, err = os.Stat(targetPath)
			if err != nil || fileInfo.IsDir() {
				r.NotFound(req, res)
				return
			}
		}

		data, err := os.ReadFile(targetPath)
		if err != nil {
			r.NotFound(req, res)
			return
		}

		// Detect Content-Type
		ext := filepath.Ext(targetPath)
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			switch ext {
			case ".html", ".htm":
				mimeType = "text/html; charset=utf-8"
			case ".css":
				mimeType = "text/css; charset=utf-8"
			case ".js":
				mimeType = "application/javascript; charset=utf-8"
			case ".json":
				mimeType = "application/json; charset=utf-8"
			case ".png":
				mimeType = "image/png"
			case ".jpg", ".jpeg":
				mimeType = "image/jpeg"
			case ".svg":
				mimeType = "image/svg+xml"
			default:
				mimeType = "application/octet-stream"
			}
		}

		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", mimeType)
		if req.Method != "HEAD" {
			_, _ = res.Write(data)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefixRoutes = append(r.prefixRoutes, prefixRoute{
		prefix:  cleanPrefix,
		handler: staticHandler,
	})
}

// ServeHTTP dispatches the request to registered handlers through the middleware chain.
func (r *Router) ServeHTTP(req *httpparser.Request, res *httpparser.Response) {
	r.mu.RLock()
	var targetHandler HandlerFunc

	methodsMap, pathExists := r.routes[req.Path]
	if pathExists {
		h, methodExists := methodsMap[req.Method]
		if methodExists {
			targetHandler = h
		} else {
			targetHandler = r.MethodNotAllowed
		}
	} else {
		// Check prefix routes (e.g. static file routes)
		for _, pr := range r.prefixRoutes {
			if pr.prefix == "" || strings.HasPrefix(req.Path, pr.prefix+"/") || req.Path == pr.prefix {
				targetHandler = pr.handler
				break
			}
		}
		if targetHandler == nil {
			targetHandler = r.NotFound
		}
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
