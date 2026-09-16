package gcparser

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Standard Go runtime gctrace regular expression.
// Handles Go 1.20, 1.22, 1.24+ variations:
// gc 42 @12.345s 2%: 0.045+1.23+0.015 ms clock, 0.36+0.45/1.10/2.30+0.12 ms cpu, 14->16->8 MB, 18 MB goal, 8 MB stacks, 0 MB globals, 8 P
var gcRegex = regexp.MustCompile(`^gc\s+(\d+)\s+@([\d.]+)s\s+([\d.]+)%:\s+([\d.]+)\+([\d.]+)\+([\d.]+)\s+ms clock,\s+([^\s,]+)\s+ms cpu,\s+([\d.]+)->([\d.]+)->([\d.]+)\s+MB,\s+([\d.]+)\s+MB goal(?:,\s+[\d.]+\s+MB stacks)?(?:,\s+[\d.]+\s+MB globals)?(?:,\s+(\d+)\s+P)?`)

// ParseLine parses a single gctrace log line into a GCEvent.
// Returns (event, true) if valid GC line, or (nil, false) if non-GC line.
func ParseLine(line string) (*GCEvent, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "gc ") {
		return nil, false
	}

	// Try fast string scanning first for maximum performance (< 50ms per 10k lines)
	if evt, ok := fastParseLine(trimmed); ok {
		return evt, true
	}

	// Fallback to regex for any unusual formatting variations
	matches := gcRegex.FindStringSubmatch(trimmed)
	if len(matches) < 12 {
		return nil, false
	}

	cycleNum, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return nil, false
	}

	ts, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return nil, false
	}

	cpuPct, err := strconv.ParseFloat(matches[3], 64)
	if err != nil {
		return nil, false
	}

	stw1, err := strconv.ParseFloat(matches[4], 64)
	if err != nil {
		return nil, false
	}

	mark, err := strconv.ParseFloat(matches[5], 64)
	if err != nil {
		return nil, false
	}

	stw2, err := strconv.ParseFloat(matches[6], 64)
	if err != nil {
		return nil, false
	}

	hStart, err := strconv.ParseFloat(matches[8], 64)
	if err != nil {
		return nil, false
	}

	hSweep, err := strconv.ParseFloat(matches[9], 64)
	if err != nil {
		return nil, false
	}

	hLive, err := strconv.ParseFloat(matches[10], 64)
	if err != nil {
		return nil, false
	}

	hGoal, err := strconv.ParseFloat(matches[11], 64)
	if err != nil {
		return nil, false
	}

	procs := 0
	if len(matches) > 12 && matches[12] != "" {
		if p, err := strconv.Atoi(matches[12]); err == nil {
			procs = p
		}
	}

	totalSTW := stw1 + stw2
	reclaimed := hSweep - hLive

	return &GCEvent{
		CycleNum:     cycleNum,
		TimestampSec: ts,
		CPUPercent:   cpuPct,
		ClockSTW1Ms:  stw1,
		ClockMarkMs:  mark,
		ClockSTW2Ms:  stw2,
		TotalSTWMs:   totalSTW,
		HeapStartMB:  hStart,
		HeapSweepMB:  hSweep,
		HeapLiveMB:   hLive,
		HeapGoalMB:   hGoal,
		ReclaimedMB:  reclaimed,
		Processors:   procs,
	}, true
}

func fastParseLine(line string) (*GCEvent, bool) {
	// Must start with "gc "
	rem := line[3:]
	// cycle number
	sp := strings.IndexByte(rem, ' ')
	if sp <= 0 {
		return nil, false
	}
	cycleNum, err := strconv.ParseInt(rem[:sp], 10, 64)
	if err != nil {
		return nil, false
	}
	rem = strings.TrimSpace(rem[sp+1:])

	// @<ts>s
	if !strings.HasPrefix(rem, "@") {
		return nil, false
	}
	rem = rem[1:]
	sIdx := strings.IndexByte(rem, 's')
	if sIdx <= 0 {
		return nil, false
	}
	ts, err := strconv.ParseFloat(rem[:sIdx], 64)
	if err != nil {
		return nil, false
	}
	rem = strings.TrimSpace(rem[sIdx+1:])

	// <cpu>%:
	pctIdx := strings.Index(rem, "%:")
	if pctIdx <= 0 {
		return nil, false
	}
	cpuPct, err := strconv.ParseFloat(rem[:pctIdx], 64)
	if err != nil {
		return nil, false
	}
	rem = strings.TrimSpace(rem[pctIdx+2:])

	// <stw1>+<mark>+<stw2> ms clock,
	clockTag := " ms clock,"
	clockIdx := strings.Index(rem, clockTag)
	if clockIdx <= 0 {
		return nil, false
	}
	clockStr := rem[:clockIdx]
	rem = strings.TrimSpace(rem[clockIdx+len(clockTag):])

	p1 := strings.IndexByte(clockStr, '+')
	if p1 <= 0 {
		return nil, false
	}
	stw1, err := strconv.ParseFloat(clockStr[:p1], 64)
	if err != nil {
		return nil, false
	}
	clockRest := clockStr[p1+1:]
	p2 := strings.IndexByte(clockRest, '+')
	if p2 <= 0 {
		return nil, false
	}
	mark, err := strconv.ParseFloat(clockRest[:p2], 64)
	if err != nil {
		return nil, false
	}
	stw2, err := strconv.ParseFloat(clockRest[p2+1:], 64)
	if err != nil {
		return nil, false
	}

	// <cpu> ms cpu,
	cpuTag := " ms cpu,"
	cpuIdx := strings.Index(rem, cpuTag)
	if cpuIdx <= 0 {
		return nil, false
	}
	rem = strings.TrimSpace(rem[cpuIdx+len(cpuTag):])

	// <hStart>-><hSweep>-><hLive> MB,
	mbTag := " MB,"
	mbIdx := strings.Index(rem, mbTag)
	if mbIdx <= 0 {
		return nil, false
	}
	heapStr := rem[:mbIdx]
	rem = strings.TrimSpace(rem[mbIdx+len(mbTag):])

	a1 := strings.Index(heapStr, "->")
	if a1 <= 0 {
		return nil, false
	}
	hStart, err := strconv.ParseFloat(heapStr[:a1], 64)
	if err != nil {
		return nil, false
	}
	heapRest := heapStr[a1+2:]
	a2 := strings.Index(heapRest, "->")
	if a2 <= 0 {
		return nil, false
	}
	hSweep, err := strconv.ParseFloat(heapRest[:a2], 64)
	if err != nil {
		return nil, false
	}
	hLive, err := strconv.ParseFloat(heapRest[a2+2:], 64)
	if err != nil {
		return nil, false
	}

	// <goal> MB goal
	goalTag := " MB goal"
	goalIdx := strings.Index(rem, goalTag)
	if goalIdx <= 0 {
		return nil, false
	}
	hGoal, err := strconv.ParseFloat(rem[:goalIdx], 64)
	if err != nil {
		return nil, false
	}
	rem = strings.TrimSpace(rem[goalIdx+len(goalTag):])

	// Optional tokens: , 8 MB stacks, 0 MB globals, 8 P
	procs := 0
	if strings.HasPrefix(rem, ",") {
		parts := strings.Split(rem[1:], ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasSuffix(part, " P") {
				pStr := strings.TrimSuffix(part, " P")
				if p, err := strconv.Atoi(strings.TrimSpace(pStr)); err == nil {
					procs = p
				}
			}
		}
	}

	totalSTW := stw1 + stw2
	reclaimed := hSweep - hLive

	return &GCEvent{
		CycleNum:     cycleNum,
		TimestampSec: ts,
		CPUPercent:   cpuPct,
		ClockSTW1Ms:  stw1,
		ClockMarkMs:  mark,
		ClockSTW2Ms:  stw2,
		TotalSTWMs:   totalSTW,
		HeapStartMB:  hStart,
		HeapSweepMB:  hSweep,
		HeapLiveMB:   hLive,
		HeapGoalMB:   hGoal,
		ReclaimedMB:  reclaimed,
		Processors:   procs,
	}, true
}

// ParseReader scans an io.Reader containing Go stderr logs and computes GC telemetry.
func ParseReader(r io.Reader, totalDuration time.Duration) (*GCTelemetry, error) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var records []GCEvent
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "gc ") {
			continue
		}
		if evt, ok := ParseLine(trimmed); ok {
			records = append(records, *evt)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return ComputeStatistics(records, totalDuration), nil
}

// ParseReaderSeconds provides a convenience wrapper accepting duration in seconds.
func ParseReaderSeconds(r io.Reader, durationSec float64) (*GCTelemetry, error) {
	return ParseReader(r, time.Duration(durationSec*float64(time.Second)))
}

// ParseFile opens a file path and executes ParseReader.
func ParseFile(path string, totalDuration time.Duration) (*GCTelemetry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseReader(f, totalDuration)
}

// ParseFileSeconds provides a convenience wrapper accepting duration in seconds.
func ParseFileSeconds(path string, durationSec float64) (*GCTelemetry, error) {
	return ParseFile(path, time.Duration(durationSec*float64(time.Second)))
}
