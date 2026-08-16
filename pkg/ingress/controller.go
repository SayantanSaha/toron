package ingress

import (
	"context"
	"log"
	"sync"
	"time"

	"toron/pkg/config"
	"toron/pkg/discovery"
	"toron/pkg/proxy"
	"toron/pkg/router"
)

// Controller manages Kubernetes Ingress resources and synchronizes upstreams into Toron Router.
type Controller struct {
	mu           sync.RWMutex
	cfg          config.IngressConfig
	router       *router.Router
	client       *Client
	activeRoutes map[string]*discovery.DiscoveredRoute
	events       chan K8sWatchEvent
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	running      bool
}

// NewController constructs a native Kubernetes Ingress Controller.
func NewController(cfg config.IngressConfig, r *router.Router) (*Controller, error) {
	if cfg.IngressClass == "" {
		cfg.IngressClass = "toron"
	}

	cli, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &Controller{
		cfg:          cfg,
		router:       r,
		client:       cli,
		activeRoutes: make(map[string]*discovery.DiscoveredRoute),
		events:       make(chan K8sWatchEvent, 100),
	}, nil
}

// ActiveRoutes returns a thread-safe snapshot of active K8s Ingress routes.
func (c *Controller) ActiveRoutes() []*discovery.DiscoveredRoute {
	c.mu.RLock()
	defer c.mu.RUnlock()

	routes := make([]*discovery.DiscoveredRoute, 0, len(c.activeRoutes))
	for _, r := range c.activeRoutes {
		routes = append(routes, r)
	}
	return routes
}

// Start initiates initial Ingress sync and starts event watch workers.
func (c *Controller) Start(parentCtx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(parentCtx)
	c.cancel = cancel
	c.running = true
	c.mu.Unlock()

	// Initial sync
	c.syncIngresses(ctx)

	// Start watch event worker
	c.wg.Add(1)
	go c.watchWorker(ctx)

	// Start periodic resync ticker if configured
	if c.cfg.ResyncPeriod > 0 {
		c.wg.Add(1)
		go c.periodicResyncWorker(ctx)
	}

	log.Printf("[INGRESS] Kubernetes Ingress Controller started (Class: %q, APIServer: %q)",
		c.cfg.IngressClass, c.client.apiServer)
	return nil
}

// Stop cleanly terminates controller workers.
func (c *Controller) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()

	c.wg.Wait()
	log.Printf("[INGRESS] Kubernetes Ingress Controller stopped")
}

func (c *Controller) syncIngresses(ctx context.Context) {
	ingList, err := c.client.ListIngresses(ctx)
	if err != nil {
		log.Printf("[INGRESS] Failed to list ingresses from apiserver: %v", err)
		return
	}

	endpointsMap := make(map[string]*Endpoints)
	for _, ing := range ingList {
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, pathRule := range rule.HTTP.Paths {
				if pathRule.Backend.Service != nil {
					svcName := pathRule.Backend.Service.Name
					epKey := ing.Metadata.Namespace + "/" + svcName
					if _, exists := endpointsMap[epKey]; !exists {
						if ep, err := c.client.GetEndpoints(ctx, ing.Metadata.Namespace, svcName); err == nil {
							endpointsMap[epKey] = ep
						}
					}
				}
			}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Rebuild route map
	newRoutes := make(map[string]*discovery.DiscoveredRoute)

	for _, ing := range ingList {
		routes, ok := TranslateIngress(ing, c.cfg.IngressClass, endpointsMap)
		if !ok {
			continue
		}
		for _, r := range routes {
			newRoutes[r.ContainerID] = r
			if c.router != nil {
				opts := proxy.ProxyOptions{
					Targets: []string{r.TargetURL()},
				}
				_ = c.router.RoutePrefix("upstream", r.Host, r.Prefix, nil, "", opts)
			}
		}
	}

	c.activeRoutes = newRoutes
}

func (c *Controller) watchWorker(ctx context.Context) {
	defer c.wg.Done()
	if err := c.client.WatchIngresses(ctx, c.events); err != nil && ctx.Err() == nil {
		log.Printf("[INGRESS] Watch stream connection closed: %v (falling back to resync)", err)
	}
}

func (c *Controller) periodicResyncWorker(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(c.cfg.ResyncPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.syncIngresses(ctx)
		}
	}
}
