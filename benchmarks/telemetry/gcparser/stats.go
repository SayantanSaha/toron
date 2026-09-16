package gcparser

import (
	"math"
	"sort"
	"time"
)

// ComputeStatistics aggregates an array of GCEvent records into GCTelemetry.
func ComputeStatistics(records []GCEvent, totalDuration time.Duration) *GCTelemetry {
	if len(records) == 0 {
		return &GCTelemetry{
			Enabled:     true,
			TotalCycles: 0,
		}
	}

	n := float64(len(records))
	var totalSTW, totalMark, totalReclaimed float64
	var peakLive, peakTrigger, sumLive, maxMark float64
	stws := make([]float64, len(records))

	var sumT, sumY, sumTY, sumT2 float64

	for i, r := range records {
		stws[i] = r.TotalSTWMs
		totalSTW += r.TotalSTWMs
		totalMark += r.ClockMarkMs
		totalReclaimed += r.ReclaimedMB
		sumLive += r.HeapLiveMB

		if r.HeapLiveMB > peakLive {
			peakLive = r.HeapLiveMB
		}
		if r.HeapStartMB > peakTrigger {
			peakTrigger = r.HeapStartMB
		}
		if r.ClockMarkMs > maxMark {
			maxMark = r.ClockMarkMs
		}

		// OLS Regression accumulators: (t, y) = (TimestampSec, HeapLiveMB)
		t := r.TimestampSec
		y := r.HeapLiveMB
		sumT += t
		sumY += y
		sumTY += t * y
		sumT2 += t * t
	}

	sort.Float64s(stws)

	// OLS Linear Regression Slope in MB/min:
	var slopeMBPerMin float64
	denom := n*sumT2 - sumT*sumT
	if len(records) > 1 && math.Abs(denom) > 1e-9 {
		slopePerSec := (n*sumTY - sumT*sumY) / denom
		slopeMBPerMin = slopePerSec * 60.0
	}

	effectiveDuration := totalDuration.Seconds()
	if effectiveDuration <= 0 && len(records) > 0 {
		effectiveDuration = records[len(records)-1].TimestampSec
	}
	var cyclesPerSec float64
	if effectiveDuration > 0 {
		cyclesPerSec = n / effectiveDuration
	}

	lastRecord := records[len(records)-1]

	return &GCTelemetry{
		Enabled:          true,
		TotalCycles:      int64(len(records)),
		GCCPUPercent:     lastRecord.CPUPercent,
		CyclesPerSecond:  cyclesPerSec,
		TotalReclaimedMB: totalReclaimed,
		PauseTimesMs: GCPauseStatistics{
			MinSTWMs:   stws[0],
			MeanSTWMs:  totalSTW / n,
			P50STWMs:   percentile(stws, 0.50),
			P95STWMs:   percentile(stws, 0.95),
			P99STWMs:   percentile(stws, 0.99),
			MaxSTWMs:   stws[len(stws)-1],
			TotalSTWMs: totalSTW,
			MeanMarkMs: totalMark / n,
			MaxMarkMs:  maxMark,
		},
		HeapMetricsMB: GCHeapStatistics{
			InitialLiveHeapMB:  records[0].HeapLiveMB,
			FinalLiveHeapMB:    lastRecord.HeapLiveMB,
			PeakLiveHeapMB:     peakLive,
			MeanLiveHeapMB:     sumLive / n,
			PeakTriggerHeapMB:  peakTrigger,
			HeapGrowthSlopeMBm: slopeMBPerMin,
		},
	}
}

// percentile computes the p-th percentile (0.0 <= p <= 1.0) of a sorted float64 slice.
// Rank(P) = ceil(p * N) - 1.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0.0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1.0 {
		return sorted[len(sorted)-1]
	}
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}
