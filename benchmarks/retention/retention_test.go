package retention

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"
)

func TestFormatTimestamp(t *testing.T) {
	fixedTime := time.Date(2026, 9, 12, 12, 30, 45, 0, time.UTC)
	formatted := FormatTimestamp(fixedTime)
	expected := "2026-09-12_12-30-45"
	if formatted != expected {
		t.Fatalf("expected %s, got %s", expected, formatted)
	}

	matched, err := regexp.MatchString(`^\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}$`, formatted)
	if err != nil || !matched {
		t.Fatalf("formatted timestamp %s failed regex validation", formatted)
	}
}

func TestCreateHistoryDirectory(t *testing.T) {
	tempBase := t.TempDir()
	ts := "2026-09-12_10-00-00"

	// 1. Initial creation
	dir1, name1, err := CreateHistoryDirectory(tempBase, ts, "")
	if err != nil {
		t.Fatalf("CreateHistoryDirectory failed: %v", err)
	}
	if name1 != ts {
		t.Errorf("expected name %s, got %s", ts, name1)
	}
	if info, err := os.Stat(dir1); err != nil || !info.IsDir() {
		t.Fatalf("expected %s to be an existing directory", dir1)
	}

	// 2. Collision detection and suffix appending
	dir2, name2, err := CreateHistoryDirectory(tempBase, ts, "")
	if err != nil {
		t.Fatalf("second CreateHistoryDirectory failed: %v", err)
	}
	expectedName2 := fmt.Sprintf("%s_1", ts)
	if name2 != expectedName2 {
		t.Errorf("expected collision name %s, got %s", expectedName2, name2)
	}
	if dir2 == dir1 {
		t.Errorf("expected dir2 to differ from dir1")
	}

	// 3. Custom session name suffix
	dir3, name3, err := CreateHistoryDirectory(tempBase, ts, "custom_run")
	if err != nil {
		t.Fatalf("custom session name failed: %v", err)
	}
	expectedName3 := fmt.Sprintf("%s_custom_run", ts)
	if name3 != expectedName3 {
		t.Errorf("expected custom name %s, got %s", expectedName3, name3)
	}
	if _, err := os.Stat(dir3); err != nil {
		t.Fatalf("expected %s to exist", dir3)
	}
}

func TestCopyFileAndReplicateArtifacts(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	histDir := filepath.Join(tempDir, "history", "run1")
	canonDir := filepath.Join(tempDir, "canonical")

	_ = os.MkdirAll(srcDir, 0755)
	_ = os.MkdirAll(histDir, 0755)
	_ = os.MkdirAll(canonDir, 0755)

	file1 := filepath.Join(srcDir, "report.json")
	file2 := filepath.Join(srcDir, "summary.md")
	content1 := []byte(`{"status": "ok", "latency": 123.45}`)
	content2 := []byte(`# Benchmark Summary Report`)

	if err := os.WriteFile(file1, content1, 0644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, content2, 0644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	replicated, err := ReplicateArtifacts([]string{file1, file2, "non_existent.txt"}, histDir, canonDir)
	if err != nil {
		t.Fatalf("ReplicateArtifacts failed: %v", err)
	}

	if len(replicated) != 2 {
		t.Fatalf("expected 2 replicated files, got %d", len(replicated))
	}

	// Verify both historical and canonical files match source
	for _, fname := range []string{"report.json", "summary.md"} {
		hData, err := os.ReadFile(filepath.Join(histDir, fname))
		if err != nil {
			t.Fatalf("failed to read historical %s: %v", fname, err)
		}
		cData, err := os.ReadFile(filepath.Join(canonDir, fname))
		if err != nil {
			t.Fatalf("failed to read canonical %s: %v", fname, err)
		}
		if string(hData) != string(cData) {
			t.Fatalf("content mismatch between historical and canonical for %s", fname)
		}
	}
}

func TestUpdateManifestAndSessionMeta(t *testing.T) {
	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "manifest.json")
	histDir := filepath.Join(tempDir, "run_2026-09-12_11-00-00")
	_ = os.MkdirAll(histDir, 0755)

	run1 := RunRecord{
		RunID:           "run_2026-09-12_11-00-00",
		Timestamp:       "2026-09-12T11:00:00Z",
		Directory:       "history/2026-09-12_11-00-00",
		Suite:           "fuzzer",
		Status:          "success",
		DurationSeconds: 12.34,
		Artifacts:       []string{"differential_fuzz_report.json"},
	}

	// First update (creates manifest)
	if err := UpdateManifest(manifestPath, run1); err != nil {
		t.Fatalf("initial UpdateManifest failed: %v", err)
	}

	// Read and verify
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to parse manifest: %v", err)
	}

	if m.SchemaVersion != ManifestSchemaVersion {
		t.Errorf("expected schema %s, got %s", ManifestSchemaVersion, m.SchemaVersion)
	}
	if m.TotalRuns != 1 || len(m.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", m.TotalRuns)
	}
	if m.Runs[0].RunID != run1.RunID {
		t.Errorf("expected run ID %s, got %s", run1.RunID, m.Runs[0].RunID)
	}

	// Second update (appends to manifest)
	run2 := RunRecord{
		RunID:           "run_2026-09-12_11-30-00",
		Timestamp:       "2026-09-12T11:30:00Z",
		Directory:       "history/2026-09-12_11-30-00",
		Suite:           "all",
		Status:          "success",
		DurationSeconds: 45.67,
		Artifacts:       []string{"report.json", "microbenchmarks.raw.txt"},
	}

	if err := UpdateManifest(manifestPath, run2); err != nil {
		t.Fatalf("second UpdateManifest failed: %v", err)
	}

	data2, _ := os.ReadFile(manifestPath)
	var m2 Manifest
	_ = json.Unmarshal(data2, &m2)

	if m2.TotalRuns != 2 || len(m2.Runs) != 2 {
		t.Fatalf("expected 2 runs in manifest, got %d", m2.TotalRuns)
	}
	if m2.Runs[1].RunID != run2.RunID {
		t.Errorf("expected run2 ID %s, got %s", run2.RunID, m2.Runs[1].RunID)
	}

	// Verify session_meta.json
	meta := SessionMeta{
		RunID:           run1.RunID,
		Timestamp:       run1.Timestamp,
		Suite:           run1.Suite,
		Status:          run1.Status,
		DurationSeconds: run1.DurationSeconds,
		Artifacts:       run1.Artifacts,
	}
	if err := WriteSessionMeta(histDir, meta); err != nil {
		t.Fatalf("WriteSessionMeta failed: %v", err)
	}

	metaData, err := os.ReadFile(filepath.Join(histDir, "session_meta.json"))
	if err != nil {
		t.Fatalf("failed to read session_meta.json: %v", err)
	}
	var parsedMeta SessionMeta
	if err := json.Unmarshal(metaData, &parsedMeta); err != nil {
		t.Fatalf("failed to parse session_meta.json: %v", err)
	}
	if parsedMeta.RunID != run1.RunID {
		t.Errorf("expected session meta run ID %s, got %s", run1.RunID, parsedMeta.RunID)
	}
}

func TestConcurrentManifestUpdates(t *testing.T) {
	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "manifest.json")

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			run := RunRecord{
				RunID:           fmt.Sprintf("worker_run_%d", idx),
				Timestamp:       time.Now().Format(time.RFC3339),
				Suite:           "test",
				Status:          "success",
				DurationSeconds: float64(idx),
			}
			if err := UpdateManifest(manifestPath, run); err != nil {
				t.Errorf("concurrent UpdateManifest worker %d failed: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest after concurrent writes: %v", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("corrupted JSON after concurrent writes: %v", err)
	}

	if m.TotalRuns != workers || len(m.Runs) != workers {
		t.Fatalf("expected %d runs after concurrent writes, got %d", workers, m.TotalRuns)
	}
}
