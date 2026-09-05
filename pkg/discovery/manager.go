package discovery

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"toron/pkg/config"
	"toron/pkg/proxy"
	"toron/pkg/router"
)

// Manager orchestrates OCI container discovery providers and updates Toron Router upstreams dynamically.
type Manager struct {
	mu           sync.RWMutex
	cfg          config.DiscoveryConfig
	router       *router.Router
	providers    []Provider
	activeRoutes map[string]*DiscoveredRoute // containerID -> route
	events       chan ContainerEvent
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	running      bool
}

// NewManager constructs a container discovery Manager instance.
func NewManager(cfg config.DiscoveryConfig, r *router.Router) *Manager {
	if cfg.DefaultWeight <= 0 {
		cfg.DefaultWeight = 1
	}

	m := &Manager{
		cfg:          cfg,
		router:       r,
		activeRoutes: make(map[string]*DiscoveredRoute),
		events:       make(chan ContainerEvent, 100),
	}

	// Register default Unix REST API provider if discovery is enabled or engine is set
	engine := cfg.Engine
	if engine == "" || engine == "auto" || engine == "docker" || engine == "podman" {
		p := NewUnixRESTProvider("Universal-UnixREST", cfg.SocketPath)
		m.providers = append(m.providers, p)
	}

	return m
}

// AddProvider registers an additional container discovery provider (e.g. CRI, static).
func (m *Manager) AddProvider(p Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers = append(m.providers, p)
}

// ActiveRoutes returns a thread-safe snapshot of currently active discovered container routes.
func (m *Manager) ActiveRoutes() []*DiscoveredRoute {
	m.mu.RLock()
	defer m.mu.RUnlock()

	routes := make([]*DiscoveredRoute, 0, len(m.activeRoutes))
	for _, r := range m.activeRoutes {
		routes = append(routes, r)
	}
	return routes
}

// Start begins background container polling and event streaming across registered providers.
func (m *Manager) Start(parentCtx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(parentCtx)
	m.cancel = cancel
	m.running = true
	m.mu.Unlock()

	// Initial scan of active containers across providers
	m.syncInitialContainers(ctx)

	// Start event consumer worker
	m.wg.Add(1)
	go m.consumeEventsWorker(ctx)

	// Start provider event watchers
	m.mu.RLock()
	providers := m.providers
	m.mu.RUnlock()

	for _, p := range providers {
		m.wg.Add(1)
		go func(prov Provider) {
			defer m.wg.Done()
			log.Printf("[DISCOVERY] Starting watcher for provider: %s", prov.Name())
			if err := prov.WatchEvents(ctx, m.events); err != nil && ctx.Err() == nil {
				log.Printf("[DISCOVERY] Watcher error for %s: %v (will retry on poll)", prov.Name(), err)
			}
		}(p)
	}

	// Start periodic sync ticker if poll interval is set
	if m.cfg.PollInterval > 0 {
		m.wg.Add(1)
		go m.periodicPollWorker(ctx)
	}

	log.Printf("[DISCOVERY] Container Auto-Discovery Engine running with %d provider(s)", len(providers))
	return nil
}

// Stop cleanly terminates all provider watchers and background workers.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()

	m.wg.Wait()
	log.Printf("[DISCOVERY] Container Auto-Discovery Engine stopped")
}

func (m *Manager) syncInitialContainers(ctx context.Context) {
	m.mu.RLock()
	providers := m.providers
	m.mu.RUnlock()

	activeIDs := make(map[string]bool)
	for _, p := range providers {
		containers, err := p.ListContainers(ctx)
		if err != nil {
			log.Printf("[DISCOVERY] Initial scan failed for provider %s: %v", p.Name(), err)
			continue
		}
		for _, c := range containers {
			if _, ok := ParseContainerLabels(c, m.cfg.DefaultWeight); ok {
				activeIDs[c.ID] = true
				m.handleContainerStart(c)
			}
		}
	}

	// Purge any previously active containers that are no longer running
	m.mu.Lock()
	var stoppedIDs []string
	for id := range m.activeRoutes {
		if !activeIDs[id] {
			stoppedIDs = append(stoppedIDs, id)
		}
	}
	m.mu.Unlock()

	for _, id := range stoppedIDs {
		m.handleContainerStop(id)
	}
}

func (m *Manager) consumeEventsWorker(ctx context.Context) {
	defer m.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-m.events:
			if !ok {
				return
			}
			switch evt.Type {
			case EventStart:
				if evt.Container != nil {
					m.handleContainerStart(*evt.Container)
				} else {
					// Trigger rescan if container payload is partial
					m.syncInitialContainers(ctx)
				}
			case EventStop, EventDie:
				m.handleContainerStop(evt.ContainerID)
			}
		}
	}
}

func (m *Manager) periodicPollWorker(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.syncInitialContainers(ctx)
		}
	}
}

func (m *Manager) handleContainerStart(c Container) {
	route, ok := ParseContainerLabels(c, m.cfg.DefaultWeight)
	if !ok {
		enableVal := strings.ToLower(strings.TrimSpace(c.Labels[LabelEnable]))
		if enableVal == "true" || enableVal == "1" || enableVal == "yes" {
			log.Printf("[DISCOVERY] WARNING: OCI container %s route rejected by prefix/security policy (Host: %q, Prefix: %q)",
				c.ID, c.Labels[LabelHost], c.Labels[LabelPrefix])
		}
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already registered
	if existing, found := m.activeRoutes[c.ID]; found && existing.TargetURL() == route.TargetURL() {
		return
	}

	m.activeRoutes[c.ID] = route
	log.Printf("[DISCOVERY] Registered OCI container route: %s -> %s (Host: %q, Prefix: %q)",
		route.ContainerName, route.TargetURL(), route.Host, route.Prefix)

	if m.router != nil {
		opts := proxy.ProxyOptions{
			Targets:           []string{route.TargetURL()},
			StripPrefix:       route.StripPrefix,
			RewriteRedirects:  route.RewriteRedirects,
			RewriteCookiePath: route.RewriteCookiePath,
		}
		_ = m.router.RoutePrefix("upstream", route.Host, route.Prefix, nil, "", opts)
	}
}

func (m *Manager) handleContainerStop(containerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	route, found := m.activeRoutes[containerID]
	if !found {
		return
	}

	delete(m.activeRoutes, containerID)
	log.Printf("[DISCOVERY] Deregistered OCI container route: %s (%s)", route.ContainerName, containerID)

	if m.router != nil {
		m.router.RemovePrefixRoute(route.Host, route.Prefix)
	}
}
