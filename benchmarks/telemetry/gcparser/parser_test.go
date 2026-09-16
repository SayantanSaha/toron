package gcparser

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

// TC-130.5: GC Trace Parser Syntax Scanning across Go 1.20, Go 1.22, and Go 1.24 Format Variations.
func TestGCParser_SyntaxFormats(t *testing.T) {
	// Line 1: Go 1.20 format
	l1 := "gc 1 @0.012s 5%: 0.021+0.45+0.012 ms clock, 0.16+0.20/0.40/0.80+0.09 ms cpu, 4->4->2 MB, 5 MB goal, 4 P"
	evt1, ok1 := ParseLine(l1)
	if !ok1 {
		t.Fatalf("expected Go 1.20 line to parse, got false")
	}
	if evt1.CycleNum != 1 || evt1.TimestampSec != 0.012 || evt1.CPUPercent != 5.0 || evt1.Processors != 4 {
		t.Errorf("unexpected Go 1.20 parsed event: %+v", evt1)
	}
	if evt1.HeapStartMB != 4.0 || evt1.HeapSweepMB != 4.0 || evt1.HeapLiveMB != 2.0 || evt1.ReclaimedMB != 2.0 {
		t.Errorf("unexpected Go 1.20 heap metrics: %+v", evt1)
	}

	// Line 2: Go 1.22 format (with stacks and globals)
	l2 := "gc 42 @12.345s 2%: 0.045+1.23+0.015 ms clock, 0.36+0.45/1.10/2.30+0.12 ms cpu, 14->16->8 MB, 18 MB goal, 8 MB stacks, 0 MB globals, 8 P"
	evt2, ok2 := ParseLine(l2)
	if !ok2 {
		t.Fatalf("expected Go 1.22 line to parse, got false")
	}
	if evt2.CycleNum != 42 {
		t.Errorf("expected CycleNum 42, got %d", evt2.CycleNum)
	}
	if math.Abs(evt2.TimestampSec-12.345) > 1e-6 {
		t.Errorf("expected TimestampSec 12.345, got %f", evt2.TimestampSec)
	}
	if evt2.CPUPercent != 2.0 {
		t.Errorf("expected CPUPercent 2.0, got %f", evt2.CPUPercent)
	}
	if math.Abs(evt2.ClockSTW1Ms-0.045) > 1e-6 {
		t.Errorf("expected ClockSTW1Ms 0.045, got %f", evt2.ClockSTW1Ms)
	}
	if math.Abs(evt2.ClockMarkMs-1.23) > 1e-6 {
		t.Errorf("expected ClockMarkMs 1.23, got %f", evt2.ClockMarkMs)
	}
	if math.Abs(evt2.ClockSTW2Ms-0.015) > 1e-6 {
		t.Errorf("expected ClockSTW2Ms 0.015, got %f", evt2.ClockSTW2Ms)
	}
	if math.Abs(evt2.TotalSTWMs-0.060) > 1e-6 {
		t.Errorf("expected TotalSTWMs 0.060, got %f", evt2.TotalSTWMs)
	}
	if evt2.HeapStartMB != 14.0 || evt2.HeapSweepMB != 16.0 || evt2.HeapLiveMB != 8.0 || evt2.HeapGoalMB != 18.0 {
		t.Errorf("unexpected heap transitions: %+v", evt2)
	}
	if evt2.ReclaimedMB != 8.0 {
		t.Errorf("expected ReclaimedMB 8.0, got %f", evt2.ReclaimedMB)
	}
	if evt2.Processors != 8 {
		t.Errorf("expected Processors 8, got %d", evt2.Processors)
	}

	// Line 3: Go 1.24 format (fractional ms clock, 10 P)
	l3 := "gc 101 @60.102s 1%: 0.032+0.850+0.011 ms clock, 0.25+0.35/0.75/1.80+0.08 ms cpu, 12->14->6 MB, 16 MB goal, 6 MB stacks, 0 MB globals, 10 P"
	evt3, ok3 := ParseLine(l3)
	if !ok3 {
		t.Fatalf("expected Go 1.24 line to parse, got false")
	}
	if evt3.CycleNum != 101 || evt3.Processors != 10 || math.Abs(evt3.TotalSTWMs-0.043) > 1e-6 {
		t.Errorf("unexpected Go 1.24 parsed event: %+v", evt3)
	}
}

// TC-130.6: STW Pause Percentile Metrics Computation (Min, Mean, P50, P95, P99, Max, Total Pause, Mark Durations).
func TestGCParser_STWPausePercentiles(t *testing.T) {
	// Construct 100 GCEvent records with known STW pauses from 0.010 ms to 1.000 ms (step 0.010 ms)
	records := make([]GCEvent, 100)
	var expectedTotalSTW float64
	var expectedTotalMark float64

	for i := 0; i < 100; i++ {
		pauseVal := float64(i+1) * 0.010
		markVal := 0.50 + float64(i)*(1.50/99.0) // 0.50 to 2.00 ms
		expectedTotalSTW += pauseVal
		expectedTotalMark += markVal

		records[i] = GCEvent{
			CycleNum:     int64(i + 1),
			TimestampSec: float64(i + 1),
			CPUPercent:   1.5,
			ClockSTW1Ms:  pauseVal / 2.0,
			ClockMarkMs:  markVal,
			ClockSTW2Ms:  pauseVal / 2.0,
			TotalSTWMs:   pauseVal,
			HeapStartMB:  10.0,
			HeapSweepMB:  12.0,
			HeapLiveMB:   6.0,
			HeapGoalMB:   15.0,
			ReclaimedMB:  6.0,
		}
	}

	tele := ComputeStatistics(records, 100*time.Second)

	if tele.TotalCycles != 100 {
		t.Fatalf("expected 100 cycles, got %d", tele.TotalCycles)
	}

	if math.Abs(tele.PauseTimesMs.MinSTWMs-0.010) > 1e-6 {
		t.Errorf("expected MinSTWMs 0.010, got %f", tele.PauseTimesMs.MinSTWMs)
	}
	if math.Abs(tele.PauseTimesMs.MaxSTWMs-1.000) > 1e-6 {
		t.Errorf("expected MaxSTWMs 1.000, got %f", tele.PauseTimesMs.MaxSTWMs)
	}
	expectedMeanSTW := expectedTotalSTW / 100.0
	if math.Abs(tele.PauseTimesMs.MeanSTWMs-expectedMeanSTW) > 1e-6 {
		t.Errorf("expected MeanSTWMs %f, got %f", expectedMeanSTW, tele.PauseTimesMs.MeanSTWMs)
	}
	if math.Abs(tele.PauseTimesMs.TotalSTWMs-expectedTotalSTW) > 1e-4 {
		t.Errorf("expected TotalSTWMs %f, got %f", expectedTotalSTW, tele.PauseTimesMs.TotalSTWMs)
	}
	if math.Abs(tele.PauseTimesMs.P50STWMs-0.500) > 1e-6 {
		t.Errorf("expected P50STWMs 0.500, got %f", tele.PauseTimesMs.P50STWMs)
	}
	if math.Abs(tele.PauseTimesMs.P95STWMs-0.950) > 1e-6 {
		t.Errorf("expected P95STWMs 0.950, got %f", tele.PauseTimesMs.P95STWMs)
	}
	if math.Abs(tele.PauseTimesMs.P99STWMs-0.990) > 1e-6 {
		t.Errorf("expected P99STWMs 0.990, got %f", tele.PauseTimesMs.P99STWMs)
	}
	expectedMeanMark := expectedTotalMark / 100.0
	if math.Abs(tele.PauseTimesMs.MeanMarkMs-expectedMeanMark) > 1e-6 {
		t.Errorf("expected MeanMarkMs %f, got %f", expectedMeanMark, tele.PauseTimesMs.MeanMarkMs)
	}
	if math.Abs(tele.PauseTimesMs.MaxMarkMs-2.000) > 1e-6 {
		t.Errorf("expected MaxMarkMs 2.000, got %f", tele.PauseTimesMs.MaxMarkMs)
	}
}

// TC-130.7: Ordinary Least Squares (OLS) Linear Regression Heap Growth Slope Computation (MB/min).
func TestGCParser_OLSHeapGrowthSlope(t *testing.T) {
	// Case A: Constant Bounded Baseline (Zero Growth)
	recordsA := make([]GCEvent, 60)
	for i := 0; i < 60; i++ {
		recordsA[i] = GCEvent{
			CycleNum:     int64(i + 1),
			TimestampSec: float64(i + 1),
			HeapLiveMB:   8.0,
		}
	}
	teleA := ComputeStatistics(recordsA, 60*time.Second)
	if math.Abs(teleA.HeapMetricsMB.HeapGrowthSlopeMBm) > 1e-9 {
		t.Errorf("Case A: expected slope 0.0 MB/min, got %f", teleA.HeapMetricsMB.HeapGrowthSlopeMBm)
	}

	// Case B: Known Linear Growth (0.60 MB/min)
	// H_live(t) = 8.0 + 0.01 * t (grows by 0.60 MB over 60s -> slope = 0.01 MB/s * 60 = 0.60 MB/min)
	recordsB := make([]GCEvent, 60)
	for i := 0; i < 60; i++ {
		tSec := float64(i + 1)
		recordsB[i] = GCEvent{
			CycleNum:     int64(i + 1),
			TimestampSec: tSec,
			HeapLiveMB:   8.0 + 0.01*tSec,
		}
	}
	teleB := ComputeStatistics(recordsB, 60*time.Second)
	if math.Abs(teleB.HeapMetricsMB.HeapGrowthSlopeMBm-0.60) > 1e-6 {
		t.Errorf("Case B: expected slope 0.60 MB/min, got %f", teleB.HeapMetricsMB.HeapGrowthSlopeMBm)
	}

	// Case C: Cyclic Sawtooth with Flat Trend: H_live(t) = 8.0 + sin(t)
	recordsC := make([]GCEvent, 300)
	for i := 0; i < 300; i++ {
		tSec := float64(i + 1)
		recordsC[i] = GCEvent{
			CycleNum:     int64(i + 1),
			TimestampSec: tSec,
			HeapLiveMB:   8.0 + math.Sin(tSec),
		}
	}
	teleC := ComputeStatistics(recordsC, 300*time.Second)
	if math.Abs(teleC.HeapMetricsMB.HeapGrowthSlopeMBm) > 0.05 {
		t.Errorf("Case C: expected bounded slope < 0.05 MB/min, got %f", teleC.HeapMetricsMB.HeapGrowthSlopeMBm)
	}

	// Case D: Single Data Point (N=1)
	recordsD := []GCEvent{
		{CycleNum: 1, TimestampSec: 1.0, HeapLiveMB: 8.0},
	}
	teleD := ComputeStatistics(recordsD, 5*time.Second)
	if teleD.HeapMetricsMB.HeapGrowthSlopeMBm != 0.0 {
		t.Errorf("Case D: expected slope 0.0 for N=1, got %f", teleD.HeapMetricsMB.HeapGrowthSlopeMBm)
	}
}

// TC-130.8: Zero-Cycle GC Trace Handling (N=0 Cycles Graceful Fallback without Panics or NaN).
func TestGCParser_ZeroCycles(t *testing.T) {
	// Test empty reader
	tele1, err1 := ParseReader(strings.NewReader(""), 5*time.Second)
	if err1 != nil {
		t.Fatalf("expected nil error on empty log, got: %v", err1)
	}
	if !tele1.Enabled || tele1.TotalCycles != 0 || tele1.CyclesPerSecond != 0.0 || tele1.TotalReclaimedMB != 0.0 {
		t.Errorf("unexpected zero-cycle telemetry: %+v", tele1)
	}
	if tele1.PauseTimesMs.P50STWMs != 0.0 || tele1.PauseTimesMs.MaxSTWMs != 0.0 {
		t.Errorf("expected zero pauses, got: %+v", tele1.PauseTimesMs)
	}
	if tele1.HeapMetricsMB.HeapGrowthSlopeMBm != 0.0 {
		t.Errorf("expected zero slope, got: %f", tele1.HeapMetricsMB.HeapGrowthSlopeMBm)
	}

	// Verify JSON serialization produces valid JSON without NaN or Inf
	data, err := json.Marshal(tele1)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if strings.Contains(string(data), "NaN") || strings.Contains(string(data), "null") && strings.Contains(string(data), `"total_cycles":0`) {
		t.Errorf("invalid json serialized: %s", string(data))
	}

	// Test reader with only non-GC lines
	nonGCLog := "2026/09/16 10:00:00 [INFO] Toron server listening on :8080\nReady to listen\ngoroutine 1 [running]:\n"
	tele2, err2 := ParseReader(strings.NewReader(nonGCLog), 10*time.Second)
	if err2 != nil {
		t.Fatalf("expected nil error on non-GC log, got: %v", err2)
	}
	if tele2.TotalCycles != 0 {
		t.Errorf("expected 0 cycles, got %d", tele2.TotalCycles)
	}
}

// TC-130.9: Non-GC Line Ignorance & Parser High-Throughput Robustness.
func TestGCParser_NonGCLineIgnorance(t *testing.T) {
	var sb strings.Builder
	validCycles := 50
	for i := 1; i <= 1000; i++ {
		if i%20 == 0 {
			cycle := i / 20
			sb.WriteString(fmt.Sprintf("gc %d @%d.000s 1%%: 0.020+0.50+0.010 ms clock, 0.10+0.20/0.40/0.80+0.05 ms cpu, 4->4->2 MB, 5 MB goal, 4 P\n", cycle, cycle))
		} else if i%3 == 0 {
			sb.WriteString("2026/09/16 10:00:00 [INFO] Request handled in 0.2ms\n")
		} else if i%5 == 0 {
			sb.WriteString("goroutine 1 [running]:\n")
		} else {
			sb.WriteString("gc partial corrupt line with no matches\n")
		}
	}

	tele, err := ParseReader(strings.NewReader(sb.String()), 60*time.Second)
	if err != nil {
		t.Fatalf("ParseReader failed: %v", err)
	}
	if tele.TotalCycles != int64(validCycles) {
		t.Fatalf("expected %d cycles, got %d", validCycles, tele.TotalCycles)
	}
}

// Benchmark 10,000 trace lines scanned in < 50ms.
func BenchmarkParseReader(b *testing.B) {
	var sb strings.Builder
	for i := 1; i <= 10000; i++ {
		if i%5 == 0 {
			sb.WriteString("2026/09/16 [INFO] Toron HTTP request processing\n")
		} else {
			sb.WriteString(fmt.Sprintf("gc %d @%d.500s 2%%: 0.045+1.23+0.015 ms clock, 0.36+0.45/1.10/2.30+0.12 ms cpu, 14->16->8 MB, 18 MB goal, 8 MB stacks, 0 MB globals, 8 P\n", i, i))
		}
	}
	content := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseReader(strings.NewReader(content), 300*time.Second)
		if err != nil {
			b.Fatalf("ParseReader benchmark failed: %v", err)
		}
	}
}

// TC-130.20: Concurrency & Thread Safety Validation under race detector.
func TestGCParser_ConcurrentSafety(t *testing.T) {
	logContent := `
gc 1 @0.500s 1%: 0.020+0.50+0.010 ms clock, 0.10+0.20/0.40/0.80+0.05 ms cpu, 4->4->2 MB, 5 MB goal, 4 P
gc 2 @1.000s 1%: 0.025+0.55+0.015 ms clock, 0.12+0.22/0.45/0.85+0.06 ms cpu, 5->5->2 MB, 5 MB goal, 4 P
gc 3 @1.500s 2%: 0.030+0.60+0.020 ms clock, 0.15+0.25/0.50/0.90+0.07 ms cpu, 6->6->3 MB, 6 MB goal, 4 P
`
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tele, err := ParseReader(strings.NewReader(logContent), 5*time.Second)
			if err != nil {
				t.Errorf("concurrent ParseReader failed: %v", err)
				return
			}
			if tele.TotalCycles != 3 {
				t.Errorf("expected 3 cycles, got %d", tele.TotalCycles)
			}
		}()
	}
	wg.Wait()
}
