package proxy

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"toron/pkg/httpparser"
)

func helperMakeTargets(count int) []*UpstreamTarget {
	targets := make([]*UpstreamTarget, count)
	for i := 0; i < count; i++ {
		u, _ := url.Parse(fmt.Sprintf("http://localhost:900%d", i+1))
		t := NewUpstreamTarget(u, "", 3, 10*time.Second)
		t.Weight = 1
		targets[i] = t
	}
	return targets
}

func TestWeightedRoundRobinBalancer(t *testing.T) {
	targets := helperMakeTargets(3)
	targets[0].Weight = 5
	targets[1].Weight = 1
	targets[2].Weight = 1

	lb, err := NewWeightedRoundRobinBalancer(targets)
	if err != nil {
		t.Fatalf("failed to create WeightedRoundRobinBalancer: %v", err)
	}

	req := &httpparser.Request{Path: "/"}
	counts := make(map[string]int)

	for i := 0; i < 7; i++ {
		selected, err := lb.Next(req)
		if err != nil {
			t.Fatalf("unexpected error on Next: %v", err)
		}
		counts[selected.URL.Host]++
	}

	if counts["localhost:9001"] != 5 || counts["localhost:9002"] != 1 || counts["localhost:9003"] != 1 {
		t.Errorf("expected 5:1:1 distribution, got %+v", counts)
	}
}

func TestWeightedRandomBalancer(t *testing.T) {
	targets := helperMakeTargets(2)
	targets[0].Weight = 10
	targets[1].Weight = 1

	lb, err := NewWeightedRandomBalancer(targets)
	if err != nil {
		t.Fatalf("failed to create WeightedRandomBalancer: %v", err)
	}

	req := &httpparser.Request{Path: "/"}
	for i := 0; i < 100; i++ {
		_, err := lb.Next(req)
		if err != nil {
			t.Fatalf("unexpected error on Next: %v", err)
		}
	}
}

func TestLeastConnBalancer(t *testing.T) {
	targets := helperMakeTargets(3)
	targets[0].IncActiveConns()
	targets[0].IncActiveConns() // 2 conns
	targets[1].IncActiveConns() // 1 conn
	// target 2 has 0 conns

	lb, err := NewLeastConnBalancer(targets)
	if err != nil {
		t.Fatalf("failed to create LeastConnBalancer: %v", err)
	}

	req := &httpparser.Request{Path: "/"}
	selected, err := lb.Next(req)
	if err != nil {
		t.Fatalf("unexpected error on Next: %v", err)
	}

	if selected.URL.Host != "localhost:9003" {
		t.Errorf("expected target with 0 conns (localhost:9003), got %s", selected.URL.Host)
	}
}

func TestWeightedLeastConnBalancer(t *testing.T) {
	targets := helperMakeTargets(2)
	targets[0].Weight = 2
	targets[0].IncActiveConns()
	targets[0].IncActiveConns()
	targets[0].IncActiveConns()
	targets[0].IncActiveConns() // 4 conns / weight 2 = ratio 2.0

	targets[1].Weight = 6
	targets[1].IncActiveConns()
	targets[1].IncActiveConns()
	targets[1].IncActiveConns()
	targets[1].IncActiveConns()
	targets[1].IncActiveConns() // 5 conns / weight 6 = ratio 0.83

	lb, err := NewWeightedLeastConnBalancer(targets)
	if err != nil {
		t.Fatalf("failed to create WeightedLeastConnBalancer: %v", err)
	}

	req := &httpparser.Request{Path: "/"}
	selected, err := lb.Next(req)
	if err != nil {
		t.Fatalf("unexpected error on Next: %v", err)
	}

	if selected.URL.Host != "localhost:9002" {
		t.Errorf("expected target with lower ratio (localhost:9002), got %s", selected.URL.Host)
	}
}

func TestLeastLatencyBalancer(t *testing.T) {
	targets := helperMakeTargets(2)
	targets[0].RecordLatency(50 * time.Millisecond) // ~50000 us
	targets[1].RecordLatency(5 * time.Millisecond)  // ~5000 us

	lb, err := NewLeastLatencyBalancer(targets)
	if err != nil {
		t.Fatalf("failed to create LeastLatencyBalancer: %v", err)
	}

	req := &httpparser.Request{Path: "/"}
	selected, err := lb.Next(req)
	if err != nil {
		t.Fatalf("unexpected error on Next: %v", err)
	}

	if selected.URL.Host != "localhost:9002" {
		t.Errorf("expected target with lowest latency (localhost:9002), got %s", selected.URL.Host)
	}
}
