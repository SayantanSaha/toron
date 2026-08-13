package metrics_test

import (
	"strings"
	"testing"

	"toron/pkg/metrics"
)

func TestMetricsRegistry_ExportPrometheus(t *testing.T) {
	reg := metrics.NewMetricsRegistry()

	reg.RecordRequest("GET", "200", "/api/v1", 0.015)
	reg.RecordRequest("POST", "500", "/api/v1", 0.250)
	reg.RecordCircuitBreakerTrip("http://localhost:9001")
	reg.IncActiveQUICStreams()
	reg.IncActiveTCPConns()

	out := reg.ExportPrometheus()

	if !strings.Contains(out, "toron_http_requests_total{method=\"GET\",status=\"200\",route=\"/api/v1\"} 1") {
		t.Errorf("expected toron_http_requests_total counter in output, got:\n%s", out)
	}

	if !strings.Contains(out, "toron_circuit_breaker_trips_total{target=\"http://localhost:9001\"} 1") {
		t.Errorf("expected circuit breaker trip metric in output, got:\n%s", out)
	}

	if !strings.Contains(out, "toron_active_quic_streams 1") {
		t.Errorf("expected toron_active_quic_streams 1 in output, got:\n%s", out)
	}

	if !strings.Contains(out, "toron_tcp_connections_active 1") {
		t.Errorf("expected toron_tcp_connections_active 1 in output, got:\n%s", out)
	}
}
