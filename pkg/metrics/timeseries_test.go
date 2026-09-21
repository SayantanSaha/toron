package metrics

import (
	"testing"
	"time"
)

func TestTimeSeriesCollector_SnapshotAndRingBuffer(t *testing.T) {
	reg := NewMetricsRegistry()
	collector := NewTimeSeriesCollector(10)

	// Record requests
	reg.RecordRequest("GET", "200", "/api/v1/orders", 0.015)
	reg.RecordRequest("POST", "201", "/api/v1/orders", 0.025)
	reg.RecordRequest("GET", "500", "/api/v1/orders", 0.120)

	// Take snapshot
	collector.Snapshot(reg)

	points := collector.GetPoints()
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}

	p := points[0]
	if p.RPS <= 0 {
		t.Errorf("expected RPS > 0, got %f", p.RPS)
	}
	if p.P50 <= 0 {
		t.Errorf("expected P50 > 0, got %f", p.P50)
	}

	// Test ring buffer wrapping with capacity 10
	for i := 0; i < 25; i++ {
		reg.RecordRequest("GET", "200", "/api/v1/orders", 0.010)
		collector.Snapshot(reg)
	}

	wrappedPoints := collector.GetPoints()
	if len(wrappedPoints) != 10 {
		t.Fatalf("expected capacity 10 points after wrapping, got %d", len(wrappedPoints))
	}

	seriesData := collector.GetSeriesData()
	if seriesData == nil {
		t.Fatalf("expected non-nil series data")
	}

	labels, ok := seriesData["labels"].([]string)
	if !ok || len(labels) != 10 {
		t.Errorf("expected 10 labels, got %d", len(labels))
	}
}

func TestTimeSeriesCollector_PeriodicSampling(t *testing.T) {
	reg := NewMetricsRegistry()
	collector := NewTimeSeriesCollector(5)
	collector.Start(reg, 10*time.Millisecond)
	defer collector.Stop()

	reg.RecordRequest("GET", "200", "/test", 0.005)
	time.Sleep(50 * time.Millisecond)

	pts := collector.GetPoints()
	if len(pts) == 0 {
		t.Errorf("expected points collected by periodic ticker, got 0")
	}
}
