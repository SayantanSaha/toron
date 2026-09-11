package discovery

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"toron/pkg/config"
	"toron/pkg/proxy"
	"toron/pkg/router"
)

// CompositeRouteKey partitions discovered routes multi-dimensionally by Host, Prefix, Method, and CanonicalHeaders.
type CompositeRouteKey struct {
	Host             string
	Prefix           string
	Method           string
	CanonicalHeaders string
}

// CanonicalHeaders formats a headers map into an alphabetically sorted query-like string (k1=v1&k2=v2).
// Ensures deterministic composite key formatting regardless of Go map iteration order.
func CanonicalHeaders(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(headers[k])
	}
	return sb.String()
}

func makeCompositeRouteKey(r *DiscoveredRoute) CompositeRouteKey {
	cleanHost := strings.ToLower(strings.TrimSpace(r.Host))
	cleanPrefix := "/" + strings.Trim(r.Prefix, "/")
	if cleanPrefix == "/" {
		cleanPrefix = ""
	}
	cleanMethod := strings.ToUpper(strings.TrimSpace(r.Method))
	return CompositeRouteKey{
		Host:             cleanHost,
		Prefix:           cleanPrefix,
		Method:           cleanMethod,
		CanonicalHeaders: CanonicalHeaders(r.Headers),
	}
}

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

func (m *Manager) buildDesiredSpecsLocked() []router.PrefixRouteSpec {
	if len(m.activeRoutes) == 0 {
		return []router.PrefixRouteSpec{}
	}

	type groupData struct {
		key     CompositeRouteKey
		headers map[string]string
		routes  []*DiscoveredRoute
	}

	groups := make(map[CompositeRouteKey]*groupData)
	for _, route := range m.activeRoutes {
		key := makeCompositeRouteKey(route)
		g, exists := groups[key]
		if !exists {
			g = &groupData{
				key:     key,
				headers: route.Headers,
			}
			groups[key] = g
		}
		g.routes = append(g.routes, route)
	}

	specs := make([]router.PrefixRouteSpec, 0, len(groups))
	for _, g := range groups {
		// Collect and deduplicate targets
		targetsSeen := make(map[string]struct{})
		for _, r := range g.routes {
			targetsSeen[r.TargetURL()] = struct{}{}
		}
		targets := make([]string, 0, len(targetsSeen))
		for t := range targetsSeen {
			targets = append(targets, t)
		}
		sort.Strings(targets)

		// Aggregate proxy options
		var (
			stripPrefix         *bool
			rewriteRedirects    *bool
			rewriteCookiePath   *bool
			healthCheckPath     string
			healthCheckInterval time.Duration
		)

		for _, r := range g.routes {
			if stripPrefix == nil && r.StripPrefix != nil {
				stripPrefix = r.StripPrefix
			}
			if rewriteRedirects == nil && r.RewriteRedirects != nil {
				rewriteRedirects = r.RewriteRedirects
			}
			if rewriteCookiePath == nil && r.RewriteCookiePath != nil {
				rewriteCookiePath = r.RewriteCookiePath
			}
			if healthCheckPath == "" && r.HealthCheckPath != "" {
				healthCheckPath = r.HealthCheckPath
			}
			if healthCheckInterval == 0 && r.HealthCheckInterval > 0 {
				healthCheckInterval = r.HealthCheckInterval
			}
		}

		opts := proxy.ProxyOptions{
			Targets:             targets,
			Algorithm:           proxy.AlgorithmRoundRobin,
			HealthCheckPath:     healthCheckPath,
			HealthCheckInterval: healthCheckInterval,
			StripPrefix:         stripPrefix,
			RewriteRedirects:    rewriteRedirects,
			RewriteCookiePath:   rewriteCookiePath,
		}

		var groupHeaders map[string]string
		if len(g.headers) > 0 {
			groupHeaders = make(map[string]string, len(g.headers))
			for k, v := range g.headers {
				groupHeaders[k] = v
			}
		}

		specs = append(specs, router.PrefixRouteSpec{
			TargetType: router.RouteTypeUpstream,
			Host:       g.key.Host,
			Prefix:     g.key.Prefix,
			Method:     g.key.Method,
			Headers:    groupHeaders,
			Opts:       opts,
		})
	}

	router.SortPrefixRouteSpecs(specs)
	return specs
}

func (m *Manager) syncInitialContainers(ctx context.Context) {
	m.mu.RLock()
	providers := m.providers
	m.mu.RUnlock()

	newActive := make(map[string]*DiscoveredRoute)
	for _, p := range providers {
		containers, err := p.ListContainers(ctx)
		if err != nil {
			log.Printf("[DISCOVERY] Initial scan failed for provider %s: %v", p.Name(), err)
			continue
		}
		for _, c := range containers {
			if route, ok := ParseContainerLabels(c, m.cfg.DefaultWeight); ok {
				newActive[c.ID] = route
			}
		}
	}

	m.mu.Lock()
	m.activeRoutes = newActive
	desiredSpecs := m.buildDesiredSpecsLocked()
	m.mu.Unlock()

	if m.router != nil {
		_ = m.router.ReplacePrefixRoutesBySource("oci-discovery", desiredSpecs)
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
	m.activeRoutes[c.ID] = route
	desiredSpecs := m.buildDesiredSpecsLocked()
	m.mu.Unlock()

	log.Printf("[DISCOVERY] Registered OCI container route: %s -> %s (Host: %q, Prefix: %q)",
		route.ContainerName, route.TargetURL(), route.Host, route.Prefix)

	if m.router != nil {
		_ = m.router.ReplacePrefixRoutesBySource("oci-discovery", desiredSpecs)
	}
}

func (m *Manager) handleContainerStop(containerID string) {
	m.mu.Lock()
	route, found := m.activeRoutes[containerID]
	if !found {
		m.mu.Unlock()
		return
	}

	delete(m.activeRoutes, containerID)
	desiredSpecs := m.buildDesiredSpecsLocked()
	m.mu.Unlock()

	log.Printf("[DISCOVERY] Deregistered OCI container route: %s (%s)", route.ContainerName, containerID)

	if m.router != nil {
		_ = m.router.ReplacePrefixRoutesBySource("oci-discovery", desiredSpecs)
	}
}
