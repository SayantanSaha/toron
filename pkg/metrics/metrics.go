package metrics

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

var defaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}
var defaultWAFBuckets = []float64{0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1}

// MetricsRegistry collects counters, gauges, and latency histograms for Prometheus exposition format.
type MetricsRegistry struct {
	mu                   sync.RWMutex
	requestCounters      map[string]uint64
	requestHistograms    map[string]*Histogram
	circuitBreakerTrips  map[string]uint64
	activeQUICStreams    int64
	activeTCPConnections int64
	wafBlockedCounters   map[string]uint64
	wafAnomalyCounters   map[string]uint64
	wafHistogram         *Histogram
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
		wafBlockedCounters:  make(map[string]uint64),
		wafAnomalyCounters:  make(map[string]uint64),
		wafHistogram:        newHistogram(defaultWAFBuckets),
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

// RecordWAFBlocked increments the counter of blocked requests labeled by category and route.
func (r *MetricsRegistry) RecordWAFBlocked(category, route string) {
	if category == "" {
		category = "unknown"
	}
	if route == "" {
		route = "/"
	}
	key := fmt.Sprintf(`category=%q,route=%q`, category, route)
	r.mu.Lock()
	r.wafBlockedCounters[key]++
	r.mu.Unlock()
}

// RecordWAFAnomaly increments the counter of detected anomalies labeled by category and mode.
func (r *MetricsRegistry) RecordWAFAnomaly(category, mode string) {
	if category == "" {
		category = "unknown"
	}
	if mode == "" {
		mode = "detection"
	}
	key := fmt.Sprintf(`category=%q,mode=%q`, category, mode)
	r.mu.Lock()
	r.wafAnomalyCounters[key]++
	r.mu.Unlock()
}

// RecordWAFInspectionDuration records WAF inspection processing latency in seconds.
func (r *MetricsRegistry) RecordWAFInspectionDuration(durationSec float64) {
	r.mu.Lock()
	if r.wafHistogram == nil {
		r.wafHistogram = newHistogram(defaultWAFBuckets)
	}
	r.wafHistogram.Observe(durationSec)
	r.mu.Unlock()
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

	if len(r.wafBlockedCounters) > 0 {
		buf.WriteString("\n# HELP toron_waf_blocked_requests_total Total number of malicious requests blocked by WAF.\n")
		buf.WriteString("# TYPE toron_waf_blocked_requests_total counter\n")
		for key, count := range r.wafBlockedCounters {
			fmt.Fprintf(&buf, "toron_waf_blocked_requests_total{%s} %d\n", key, count)
		}
	}

	if len(r.wafAnomalyCounters) > 0 {
		buf.WriteString("\n# HELP toron_waf_anomalies_detected_total Total number of threat anomalies detected by WAF.\n")
		buf.WriteString("# TYPE toron_waf_anomalies_detected_total counter\n")
		for key, count := range r.wafAnomalyCounters {
			fmt.Fprintf(&buf, "toron_waf_anomalies_detected_total{%s} %d\n", key, count)
		}
	}

	if r.wafHistogram != nil && r.wafHistogram.count > 0 {
		buf.WriteString("\n# HELP toron_waf_inspection_duration_seconds WAF inspection processing duration histogram in seconds.\n")
		buf.WriteString("# TYPE toron_waf_inspection_duration_seconds histogram\n")
		var cumulative uint64
		for i, b := range r.wafHistogram.buckets {
			cumulative += r.wafHistogram.counts[i]
			fmt.Fprintf(&buf, "toron_waf_inspection_duration_seconds_bucket{le=\"%g\"} %d\n", b, cumulative)
		}
		fmt.Fprintf(&buf, "toron_waf_inspection_duration_seconds_bucket{le=\"+Inf\"} %d\n", r.wafHistogram.count)
		fmt.Fprintf(&buf, "toron_waf_inspection_duration_seconds_sum %g\n", r.wafHistogram.sum)
		fmt.Fprintf(&buf, "toron_waf_inspection_duration_seconds_count %d\n", r.wafHistogram.count)
	}

	return buf.String()
}

// GetSummaryJSON aggregates collected telemetry metrics into a map for JSON serialization.
func (r *MetricsRegistry) GetSummaryJSON() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var totalReqs uint64
	byStatus := make(map[string]uint64)
	byMethod := make(map[string]uint64)
	byRoute := make(map[string]uint64)

	for key, count := range r.requestCounters {
		totalReqs += count
		method := extractTagValue(key, "method")
		status := extractTagValue(key, "status")
		route := extractTagValue(key, "route")

		if method != "" {
			byMethod[method] += count
		}
		if status != "" {
			byStatus[status] += count
		}
		if route != "" {
			byRoute[route] += count
		}
	}

	cbTrips := make(map[string]uint64)
	var totalTrips uint64
	for target, count := range r.circuitBreakerTrips {
		cbTrips[target] = count
		totalTrips += count
	}

	var totalWAFBlocked uint64
	wafBlockedByCat := make(map[string]uint64)
	for key, count := range r.wafBlockedCounters {
		totalWAFBlocked += count
		cat := extractTagValue(key, "category")
		if cat != "" {
			wafBlockedByCat[cat] += count
		}
	}

	var totalWAFAnomalies uint64
	for _, count := range r.wafAnomalyCounters {
		totalWAFAnomalies += count
	}

	return map[string]interface{}{
		"total_requests":         totalReqs,
		"active_quic_streams":    atomic.LoadInt64(&r.activeQUICStreams),
		"active_tcp_connections": atomic.LoadInt64(&r.activeTCPConnections),
		"circuit_breaker_trips": map[string]interface{}{
			"total":   totalTrips,
			"targets": cbTrips,
		},
		"waf": map[string]interface{}{
			"blocked_total":      totalWAFBlocked,
			"blocked_by_category": wafBlockedByCat,
			"anomalies_total":    totalWAFAnomalies,
		},
		"requests_by_status": byStatus,
		"requests_by_method": byMethod,
		"requests_by_route":  byRoute,
	}
}

func extractTagValue(s, tag string) string {
	prefix := tag + "=\""
	idx := strings.Index(s, prefix)
	if idx == -1 {
		return ""
	}
	start := idx + len(prefix)
	end := strings.Index(s[start:], "\"")
	if end == -1 {
		return ""
	}
	return s[start : start+end]
}
