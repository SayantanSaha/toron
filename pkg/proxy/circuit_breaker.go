package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
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

// UpstreamTarget tracks the URL, health check configuration, and circuit breaker state for a backend server.
type UpstreamTarget struct {
	URL                 *url.URL
	HealthCheckPath     string
	HealthCheckInterval time.Duration
	ConsecutiveFailures int32
	MaxFailures         int32
	State               CircuitState
	LastStateChange     time.Time
	CooldownPeriod      time.Duration
	mu                  sync.RWMutex
	cancelProbe         context.CancelFunc
}

// NewUpstreamTarget constructs a target node with circuit breaker defaults.
func NewUpstreamTarget(targetURL *url.URL, healthPath string, maxFailures int, cooldown time.Duration) *UpstreamTarget {
	if maxFailures <= 0 {
		maxFailures = 3
	}
	if cooldown <= 0 {
		cooldown = 10 * time.Second
	}

	return &UpstreamTarget{
		URL:                 targetURL,
		HealthCheckPath:     healthPath,
		MaxFailures:         int32(maxFailures),
		State:               StateClosed,
		LastStateChange:     time.Now(),
		CooldownPeriod:      cooldown,
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
		t.State = StateOpen
		t.LastStateChange = time.Now()
	}
}

// StartActiveHealthCheck starts a background prober if HealthCheckPath is set.
func (t *UpstreamTarget) StartActiveHealthCheck(client *http.Client, interval time.Duration) {
	if t.HealthCheckPath == "" {
		return
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	t.HealthCheckInterval = interval

	ctx, cancel := context.WithCancel(context.Background())
	t.cancelProbe = cancel

	probeURL := *t.URL
	probeURL.Path = singleJoiningSlash(t.URL.Path, t.HealthCheckPath)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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
