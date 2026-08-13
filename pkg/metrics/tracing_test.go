package metrics_test

import (
	"strings"
	"testing"

	"toron/pkg/metrics"
)

func TestEnsureW3CTraceparent_Generation(t *testing.T) {
	tp := metrics.EnsureW3CTraceparent("")
	parts := strings.Split(tp, "-")
	if len(parts) != 4 || parts[0] != "00" {
		t.Fatalf("invalid generated traceparent format: %q", tp)
	}
	if len(parts[1]) != 32 || len(parts[2]) != 16 {
		t.Errorf("expected 32-hex trace ID and 16-hex span ID, got traceID %q, spanID %q", parts[1], parts[2])
	}
}

func TestEnsureW3CTraceparent_Propagation(t *testing.T) {
	incoming := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	out := metrics.EnsureW3CTraceparent(incoming)

	parts := strings.Split(out, "-")
	if len(parts) != 4 || parts[0] != "00" {
		t.Fatalf("invalid propagated traceparent format: %q", out)
	}

	// Trace ID must be preserved
	if parts[1] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("expected preserved trace ID 4bf92f3577b34da6a3ce929d0e0e4736, got %q", parts[1])
	}

	// New Span ID must be generated
	if parts[2] == "00f067aa0ba902b7" || len(parts[2]) != 16 {
		t.Errorf("expected new 16-hex span ID, got %q", parts[2])
	}
}
