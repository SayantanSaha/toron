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

func TestMetricsRegistry_WAFTelemetry(t *testing.T) {
	reg := metrics.NewMetricsRegistry()

	reg.RecordWAFBlocked("sqli", "/api/users")
	reg.RecordWAFBlocked("xss", "/api/comments")
	reg.RecordWAFAnomaly("traversal", "detection")
	reg.RecordWAFInspectionDuration(0.00042)

	out := reg.ExportPrometheus()

	if !strings.Contains(out, "toron_waf_blocked_requests_total{category=\"sqli\",route=\"/api/users\"} 1") {
		t.Errorf("expected sqli blocked metric in output, got:\n%s", out)
	}

	if !strings.Contains(out, "toron_waf_anomalies_detected_total{category=\"traversal\",mode=\"detection\"} 1") {
		t.Errorf("expected anomaly metric in output, got:\n%s", out)
	}

	if !strings.Contains(out, "toron_waf_inspection_duration_seconds_count 1") {
		t.Errorf("expected waf inspection duration histogram in output, got:\n%s", out)
	}

	summary := reg.GetSummaryJSON()
	wafMap, ok := summary["waf"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected waf section in summary JSON")
	}
	if wafMap["blocked_total"] != uint64(2) {
		t.Errorf("expected blocked_total 2, got %v", wafMap["blocked_total"])
	}
	if wafMap["anomalies_total"] != uint64(1) {
		t.Errorf("expected anomalies_total 1, got %v", wafMap["anomalies_total"])
	}
}
