package gcparser

// GCTelemetry aggregates all parsed Go runtime garbage collection metrics.
type GCTelemetry struct {
	Enabled          bool              `json:"enabled"`
	TotalCycles      int64             `json:"total_cycles"`
	GCCPUPercent     float64           `json:"gc_cpu_percent"`
	CyclesPerSecond  float64           `json:"cycles_per_second"`
	TotalReclaimedMB float64           `json:"total_reclaimed_mb"`
	PauseTimesMs     GCPauseStatistics `json:"pause_times_ms"`
	HeapMetricsMB    GCHeapStatistics  `json:"heap_metrics_mb"`
}

// GCPauseStatistics captures Stop-The-World (STW) and mark phase durations in milliseconds.
type GCPauseStatistics struct {
	MinSTWMs   float64 `json:"min_stw_ms"`
	MeanSTWMs  float64 `json:"mean_stw_ms"`
	P50STWMs   float64 `json:"p50_stw_ms"`
	P95STWMs   float64 `json:"p95_stw_ms"`
	P99STWMs   float64 `json:"p99_stw_ms"`
	MaxSTWMs   float64 `json:"max_stw_ms"`
	TotalSTWMs float64 `json:"total_stw_ms"`
	MeanMarkMs float64 `json:"mean_mark_ms"`
	MaxMarkMs  float64 `json:"max_mark_ms"`
}

// GCHeapStatistics captures live heap baselines and long-term memory growth in megabytes.
type GCHeapStatistics struct {
	InitialLiveHeapMB  float64 `json:"initial_live_heap_mb"`
	FinalLiveHeapMB    float64 `json:"final_live_heap_mb"`
	PeakLiveHeapMB     float64 `json:"peak_live_heap_mb"`
	MeanLiveHeapMB     float64 `json:"mean_live_heap_mb"`
	PeakTriggerHeapMB  float64 `json:"peak_trigger_heap_mb"`
	HeapGrowthSlopeMBm float64 `json:"heap_growth_slope_mb_per_min"`
}

// GCEvent records parsed fields for an individual GC cycle line.
type GCEvent struct {
	CycleNum     int64   `json:"cycle_num"`
	TimestampSec float64 `json:"timestamp_sec"`
	CPUPercent   float64 `json:"cpu_percent"`
	ClockSTW1Ms  float64 `json:"clock_stw1_ms"`
	ClockMarkMs  float64 `json:"clock_mark_ms"`
	ClockSTW2Ms  float64 `json:"clock_stw2_ms"`
	TotalSTWMs   float64 `json:"total_stw_ms"`
	HeapStartMB  float64 `json:"heap_start_mb"`
	HeapSweepMB  float64 `json:"heap_sweep_mb"`
	HeapLiveMB   float64 `json:"heap_live_mb"`
	HeapGoalMB   float64 `json:"heap_goal_mb"`
	ReclaimedMB  float64 `json:"reclaimed_mb"`
	Processors   int     `json:"processors,omitempty"`
}

// GCCycleRecord is an alias for GCEvent for architectural alignment with ADR-130 and TASK-153.
type GCCycleRecord = GCEvent
