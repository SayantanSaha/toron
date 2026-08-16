package sidecar

import (
	"fmt"
	"sync/atomic"
)

// WeightedSplitter selects upstream targets according to assigned integer weights.
type WeightedSplitter struct {
	targets     []SplitTarget
	totalWeight int
	counter     uint64
}

// NewWeightedSplitter constructs a weighted traffic splitter.
func NewWeightedSplitter(targets []SplitTarget) (*WeightedSplitter, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("sidecar: at least 1 target required for traffic splitting")
	}

	total := 0
	cleanTargets := make([]SplitTarget, 0, len(targets))
	for _, t := range targets {
		w := t.Weight
		if w <= 0 {
			w = 1
		}
		total += w
		cleanTargets = append(cleanTargets, SplitTarget{URL: t.URL, Weight: w})
	}

	return &WeightedSplitter{
		targets:     cleanTargets,
		totalWeight: total,
	}, nil
}

// Select returns the next target URL based on cumulative weight thresholds.
func (s *WeightedSplitter) Select() string {
	if len(s.targets) == 1 {
		return s.targets[0].URL
	}

	idx := atomic.AddUint64(&s.counter, 1)
	mod := int(idx % uint64(s.totalWeight))

	accum := 0
	for _, t := range s.targets {
		accum += t.Weight
		if mod < accum {
			return t.URL
		}
	}

	return s.targets[0].URL
}
