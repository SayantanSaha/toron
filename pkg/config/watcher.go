package config

import (
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// RouteWatcher monitors a routes.yaml configuration file for changes and triggers dynamic hot reloading.
type RouteWatcher struct {
	filePath  string
	onReload  func(routes []ProxyRouteConfig)
	watcher   *fsnotify.Watcher
	closed    chan struct{}
	mu        sync.Mutex
	debouncer *time.Timer
}

// NewRouteWatcher initializes a background file watcher for routes configuration file updates.
func NewRouteWatcher(filePath string, onReload func(routes []ProxyRouteConfig)) (*RouteWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	rw := &RouteWatcher{
		filePath: absPath,
		onReload: onReload,
		watcher:  watcher,
		closed:   make(chan struct{}),
	}

	// Watch file directory to catch write, create, and rename file edit operations
	dir := filepath.Dir(absPath)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, err
	}

	go rw.listen()
	return rw, nil
}

func (w *RouteWatcher) listen() {
	for {
		select {
		case <-w.closed:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if filepath.Clean(event.Name) == filepath.Clean(w.filePath) {
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					w.scheduleReload()
				}
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[RouteWatcher] File watcher error: %v", err)
		}
	}
}

func (w *RouteWatcher) scheduleReload() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.debouncer != nil {
		w.debouncer.Stop()
	}

	w.debouncer = time.AfterFunc(100*time.Millisecond, func() {
		w.triggerReload()
	})
}

func (w *RouteWatcher) triggerReload() {
	routes, err := LoadRoutesFromFile(w.filePath)
	if err != nil {
		log.Printf("[RouteWatcher] Hot reload failed to parse %s: %v. Retaining active routes.", w.filePath, err)
		return
	}

	log.Printf("[RouteWatcher] Hot reloading %d routing rules from %s", len(routes), w.filePath)
	if w.onReload != nil {
		w.onReload(routes)
	}
}

// Stop gracefully terminates the file watcher background worker.
func (w *RouteWatcher) Stop() error {
	w.mu.Lock()
	select {
	case <-w.closed:
		w.mu.Unlock()
		return nil
	default:
		close(w.closed)
	}
	if w.debouncer != nil {
		w.debouncer.Stop()
	}
	w.mu.Unlock()

	return w.watcher.Close()
}

// ConfigWatcher monitors a server config.yaml configuration file for changes and triggers dynamic hot reloading.
type ConfigWatcher struct {
	configPath string
	routesPath string
	onReload   func(cfg *AppConfig)
	watcher    *fsnotify.Watcher
	closed     chan struct{}
	mu         sync.Mutex
	debouncer  *time.Timer
}

// NewConfigWatcher initializes a background file watcher for server configuration file updates.
func NewConfigWatcher(configPath string, routesPath string, onReload func(cfg *AppConfig)) (*ConfigWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		absPath = configPath
	}

	cw := &ConfigWatcher{
		configPath: absPath,
		routesPath: routesPath,
		onReload:   onReload,
		watcher:    watcher,
		closed:     make(chan struct{}),
	}

	dir := filepath.Dir(absPath)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, err
	}

	go cw.listen()
	return cw, nil
}

func (w *ConfigWatcher) listen() {
	for {
		select {
		case <-w.closed:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if filepath.Clean(event.Name) == filepath.Clean(w.configPath) {
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					w.scheduleReload()
				}
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[ConfigWatcher] File watcher error: %v", err)
		}
	}
}

func (w *ConfigWatcher) scheduleReload() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.debouncer != nil {
		w.debouncer.Stop()
	}

	w.debouncer = time.AfterFunc(100*time.Millisecond, func() {
		w.triggerReload()
	})
}

func (w *ConfigWatcher) triggerReload() {
	appCfg, err := LoadFromFiles(w.configPath, w.routesPath)
	if err != nil {
		log.Printf("[ConfigWatcher] Hot reload failed to parse %s: %v. Retaining active configuration.", w.configPath, err)
		return
	}

	if valErr := ValidateConfig(appCfg); valErr != nil {
		log.Printf("[ConfigWatcher] Hot reload validation failed for %s: %v. Retaining active configuration.", w.configPath, valErr)
		return
	}

	log.Printf("[ConfigWatcher] Hot reloading configuration from %s", w.configPath)
	if w.onReload != nil {
		w.onReload(appCfg)
	}
}

// Stop gracefully terminates the file watcher background worker.
func (w *ConfigWatcher) Stop() error {
	w.mu.Lock()
	select {
	case <-w.closed:
		w.mu.Unlock()
		return nil
	default:
		close(w.closed)
	}
	if w.debouncer != nil {
		w.debouncer.Stop()
	}
	w.mu.Unlock()

	return w.watcher.Close()
}

