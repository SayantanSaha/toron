package metrics

import (
	"bytes"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

var defaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}

// MetricsRegistry collects counters, gauges, and latency histograms for Prometheus exposition format.
type MetricsRegistry struct {
	mu                   sync.RWMutex
	requestCounters      map[string]uint64
	requestHistograms    map[string]*Histogram
	circuitBreakerTrips  map[string]uint64
	activeQUICStreams    int64
	activeTCPConnections int64
}

// Histogram tracks value distribution across configured buckets.
type Histogram struct {
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

func newHistogram(buckets []float64) *Histogram {
	b := make([]float64, len(buckets))
	copy(b, buckets)
	sort.Float64s(b)
	return &Histogram{
		buckets: b,
		counts:  make([]uint64, len(b)),
	}
}

// Observe records a single floating-point duration value.
func (h *Histogram) Observe(val float64) {
	h.sum += val
	h.count++
	for i, b := range h.buckets {
		if val <= b {
			h.counts[i]++
		}
	}
}

// DefaultRegistry is the global default metrics collector instance.
var DefaultRegistry = NewMetricsRegistry()

// NewMetricsRegistry creates a new MetricsRegistry instance.
func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		requestCounters:     make(map[string]uint64),
		requestHistograms:   make(map[string]*Histogram),
		circuitBreakerTrips: make(map[string]uint64),
	}
}

// RecordRequest records an HTTP request method, status code, route prefix, and processing duration.
func (r *MetricsRegistry) RecordRequest(method, status, route string, durationSec float64) {
	if route == "" {
		route = "/"
	}
	key := fmt.Sprintf(`method=%q,status=%q,route=%q`, method, status, route)
	r.mu.Lock()
	r.requestCounters[key]++

	histKey := fmt.Sprintf(`method=%q,route=%q`, method, route)
	h, exists := r.requestHistograms[histKey]
	if !exists {
		h = newHistogram(defaultBuckets)
		r.requestHistograms[histKey] = h
	}
	h.Observe(durationSec)
	r.mu.Unlock()
}

// RecordCircuitBreakerTrip increments the circuit breaker trip counter for a backend target.
func (r *MetricsRegistry) RecordCircuitBreakerTrip(target string) {
	r.mu.Lock()
	r.circuitBreakerTrips[target]++
	r.mu.Unlock()
}

// IncActiveQUICStreams increments active HTTP/3 QUIC stream gauge.
func (r *MetricsRegistry) IncActiveQUICStreams() {
	atomic.AddInt64(&r.activeQUICStreams, 1)
}

// DecActiveQUICStreams decrements active HTTP/3 QUIC stream gauge.
func (r *MetricsRegistry) DecActiveQUICStreams() {
	atomic.AddInt64(&r.activeQUICStreams, -1)
}

// IncActiveTCPConns increments active TCP client connection gauge.
func (r *MetricsRegistry) IncActiveTCPConns() {
	atomic.AddInt64(&r.activeTCPConnections, 1)
}

// DecActiveTCPConns decrements active TCP client connection gauge.
func (r *MetricsRegistry) DecActiveTCPConns() {
	atomic.AddInt64(&r.activeTCPConnections, -1)
}

// ExportPrometheus formats all collected metrics into standard Prometheus exposition format (text/plain; version=0.0.4).
func (r *MetricsRegistry) ExportPrometheus() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var buf bytes.Buffer

	buf.WriteString("# HELP toron_http_requests_total Total number of HTTP requests processed.\n")
	buf.WriteString("# TYPE toron_http_requests_total counter\n")
	for key, val := range r.requestCounters {
		fmt.Fprintf(&buf, "toron_http_requests_total{%s} %d\n", key, val)
	}

	buf.WriteString("\n# HELP toron_http_request_duration_seconds HTTP request duration histogram in seconds.\n")
	buf.WriteString("# TYPE toron_http_request_duration_seconds histogram\n")
	for histKey, h := range r.requestHistograms {
		var cumulative uint64
		for i, b := range h.buckets {
			cumulative += h.counts[i]
			fmt.Fprintf(&buf, "toron_http_request_duration_seconds_bucket{%s,le=\"%g\"} %d\n", histKey, b, cumulative)
		}
		fmt.Fprintf(&buf, "toron_http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", histKey, h.count)
		fmt.Fprintf(&buf, "toron_http_request_duration_seconds_sum{%s} %g\n", histKey, h.sum)
		fmt.Fprintf(&buf, "toron_http_request_duration_seconds_count{%s} %d\n", histKey, h.count)
	}

	buf.WriteString("\n# HELP toron_active_quic_streams Active HTTP/3 QUIC streams count.\n")
	buf.WriteString("# TYPE toron_active_quic_streams gauge\n")
	fmt.Fprintf(&buf, "toron_active_quic_streams %d\n", atomic.LoadInt64(&r.activeQUICStreams))

	buf.WriteString("\n# HELP toron_tcp_connections_active Active client TCP connections.\n")
	buf.WriteString("# TYPE toron_tcp_connections_active gauge\n")
	fmt.Fprintf(&buf, "toron_tcp_connections_active %d\n", atomic.LoadInt64(&r.activeTCPConnections))

	buf.WriteString("\n# HELP toron_circuit_breaker_trips_total Total number of circuit breaker trips per target backend.\n")
	buf.WriteString("# TYPE toron_circuit_breaker_trips_total counter\n")
	for target, count := range r.circuitBreakerTrips {
		fmt.Fprintf(&buf, "toron_circuit_breaker_trips_total{target=%q} %d\n", target, count)
	}

	return buf.String()
}
