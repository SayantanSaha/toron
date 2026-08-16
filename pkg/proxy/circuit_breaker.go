package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"toron/pkg/metrics"
)

// CircuitState represents the health and availability state of an upstream target.
type CircuitState int32

const (
	StateClosed   CircuitState = iota // Healthy - normal routing
	StateOpen                         // Tripped - offline / unhealthy
	StateHalfOpen                     // Cooldown elapsed - probing recovery
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "Closed (Healthy)"
	case StateOpen:
		return "Open (Tripped)"
	case StateHalfOpen:
		return "HalfOpen (Probing)"
	default:
		return "Unknown"
	}
}

var (
	ErrNoHealthyUpstreamAvailable = errors.New("proxy: all upstream targets are unhealthy or circuit open")
)

// UpstreamTarget tracks the URL, health check configuration, circuit breaker state, load balancing weight, active connections, and latency metrics.
type UpstreamTarget struct {
	URL                 *url.URL
	HealthCheckType     string
	HealthCheckPath     string
	HealthCheckService  string
	HealthCheckInterval time.Duration
	ConsecutiveFailures int32
	MaxFailures         int32
	State               CircuitState
	LastStateChange     time.Time
	CooldownPeriod      time.Duration
	Weight              int
	EffectiveWeight     int
	CurrentWeight       int
	ActiveConns         int64
	AvgLatencyUS        int64
	mu                  sync.RWMutex
	cancelProbe         context.CancelFunc
}

func (t *UpstreamTarget) IncActiveConns() int64 {
	return atomic.AddInt64(&t.ActiveConns, 1)
}

func (t *UpstreamTarget) DecActiveConns() int64 {
	return atomic.AddInt64(&t.ActiveConns, -1)
}

func (t *UpstreamTarget) GetActiveConns() int64 {
	return atomic.LoadInt64(&t.ActiveConns)
}

func (t *UpstreamTarget) RecordLatency(d time.Duration) {
	us := d.Microseconds()
	if us <= 0 {
		us = 1
	}
	old := atomic.LoadInt64(&t.AvgLatencyUS)
	if old == 0 {
		atomic.StoreInt64(&t.AvgLatencyUS, us)
	} else {
		newVal := int64(float64(old)*0.8 + float64(us)*0.2)
		atomic.StoreInt64(&t.AvgLatencyUS, newVal)
	}
}

func (t *UpstreamTarget) GetAvgLatencyUS() int64 {
	return atomic.LoadInt64(&t.AvgLatencyUS)
}

// NewUpstreamTarget constructs a target node with circuit breaker defaults.
func NewUpstreamTarget(targetURL *url.URL, healthPath string, maxFailures int, cooldown time.Duration) *UpstreamTarget {
	return NewUpstreamTargetWithHealth(targetURL, "http", healthPath, "", maxFailures, cooldown)
}

// NewUpstreamTargetWithHealth constructs a target node supporting HTTP and gRPC health checks.
func NewUpstreamTargetWithHealth(targetURL *url.URL, healthType, healthPath, healthService string, maxFailures int, cooldown time.Duration) *UpstreamTarget {
	if maxFailures <= 0 {
		maxFailures = 3
	}
	if cooldown <= 0 {
		cooldown = 10 * time.Second
	}
	if healthType == "" {
		healthType = "http"
	}

	return &UpstreamTarget{
		URL:                targetURL,
		HealthCheckType:    strings.ToLower(strings.TrimSpace(healthType)),
		HealthCheckPath:    healthPath,
		HealthCheckService: healthService,
		MaxFailures:        int32(maxFailures),
		State:              StateClosed,
		LastStateChange:    time.Now(),
		CooldownPeriod:     cooldown,
		Weight:             1,
		EffectiveWeight:    1,
	}
}

// GetState returns current state, auto-transitioning from Open to HalfOpen if CooldownPeriod elapsed.
func (t *UpstreamTarget) GetState() CircuitState {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.State == StateOpen && time.Since(t.LastStateChange) >= t.CooldownPeriod {
		t.State = StateHalfOpen
		t.LastStateChange = time.Now()
	}
	return t.State
}

// RecordSuccess resets failure counters and restores state to StateClosed.
func (t *UpstreamTarget) RecordSuccess() {
	t.mu.Lock()
	defer t.mu.Unlock()

	atomic.StoreInt32(&t.ConsecutiveFailures, 0)
	t.State = StateClosed
	t.LastStateChange = time.Now()
}

// RecordFailure increments failure counters and trips state to StateOpen if threshold exceeded.
func (t *UpstreamTarget) RecordFailure() {
	t.mu.Lock()
	defer t.mu.Unlock()

	fails := atomic.AddInt32(&t.ConsecutiveFailures, 1)
	if fails >= t.MaxFailures {
		if t.State != StateOpen {
			metrics.DefaultRegistry.RecordCircuitBreakerTrip(t.URL.String())
		}
		t.State = StateOpen
		t.LastStateChange = time.Now()
	}
}

// StartActiveHealthCheck starts a background prober if HealthCheckPath or gRPC probing is set.
func (t *UpstreamTarget) StartActiveHealthCheck(client *http.Client, interval time.Duration) {
	if t.HealthCheckPath == "" && t.HealthCheckType != "grpc" {
		return
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	t.HealthCheckInterval = interval

	ctx, cancel := context.WithCancel(context.Background())
	t.cancelProbe = cancel

	probeURL := *t.URL
	if t.HealthCheckPath != "" {
		probeURL.Path = singleJoiningSlash(t.URL.Path, t.HealthCheckPath)
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if t.HealthCheckType == "grpc" {
					ok, err := ProbeGRPCHealth(ctx, t.URL.String(), t.HealthCheckService, interval)
					if ok && err == nil {
						t.RecordSuccess()
					} else {
						t.RecordFailure()
					}
				} else {
					req, err := http.NewRequestWithContext(ctx, "GET", probeURL.String(), nil)
					if err != nil {
						t.RecordFailure()
						continue
					}

					resp, err := client.Do(req)
					if err != nil {
						t.RecordFailure()
						continue
					}
					_ = resp.Body.Close()

					if resp.StatusCode >= 200 && resp.StatusCode < 300 {
						t.RecordSuccess()
					} else {
						t.RecordFailure()
					}
				}
			}
		}
	}()
}

// StopActiveHealthCheck stops the active background health checker goroutine.
func (t *UpstreamTarget) StopActiveHealthCheck() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancelProbe != nil {
		t.cancelProbe()
		t.cancelProbe = nil
	}
}
