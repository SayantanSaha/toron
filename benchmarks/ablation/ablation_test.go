package ablation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TC-115-01: Task Catalog & Data Model Completeness
func TestTaskCatalogCompleteness(t *testing.T) {
	tasks := GetTaskCohort()
	if len(tasks) != 10 {
		t.Fatalf("expected 10 tasks in cohort, got %d", len(tasks))
	}

	expectedIDs := map[string]bool{
		"TASK-061": true, "TASK-062": true, "TASK-063": true, "TASK-064": true, "TASK-065": true,
		"TASK-066": true, "TASK-067": true, "TASK-068": true, "TASK-069": true, "TASK-070": true,
	}

	seen := make(map[string]bool)
	for _, task := range tasks {
		if !expectedIDs[task.ID] {
			t.Errorf("unexpected task ID: %s", task.ID)
		}
		if seen[task.ID] {
			t.Errorf("duplicate task ID: %s", task.ID)
		}
		seen[task.ID] = true

		if len(task.Title) == 0 {
			t.Errorf("task %s has empty title", task.ID)
		}
		if len(task.Subsystem) == 0 {
			t.Errorf("task %s has empty subsystem", task.ID)
		}
		if len(task.TargetVulnerability) == 0 {
			t.Errorf("task %s has empty target vulnerability", task.ID)
		}
		if len(task.AcceptanceCriteria) < 2 {
			t.Errorf("task %s has fewer than 2 acceptance criteria (%d)", task.ID, len(task.AcceptanceCriteria))
		}
		if task.ConditionADocs.Req == "" || task.ConditionADocs.Task == "" || task.ConditionADocs.Adr == "" {
			t.Errorf("task %s has missing Condition A documentation links", task.ID)
		}
	}
}

// TC-115-02: Data Archival Integrity (Condition A & Condition B)
func TestDataArchivalIntegrity(t *testing.T) {
	dataDir := "data"
	tasks := GetTaskCohort()

	// 1. Verify Condition A metadata
	metaPath := filepath.Join(dataDir, "condition_a", "metadata.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("failed to read Condition A metadata file: %v", err)
	}

	var metaA ConditionAMetadata
	if err := json.Unmarshal(metaBytes, &metaA); err != nil {
		t.Fatalf("invalid Condition A metadata JSON: %v", err)
	}

	if metaA.CohortSize != 10 {
		t.Errorf("expected cohort_size 10 in Condition A metadata, got %d", metaA.CohortSize)
	}

	// 2. Verify Condition B artifacts for all 10 tasks
	for _, task := range tasks {
		if _, ok := metaA.Tasks[task.ID]; !ok {
			t.Errorf("missing task %s in Condition A metadata map", task.ID)
		}

		folderName := strings.ToLower(strings.ReplaceAll(task.ID, "-", "_"))
		taskDir := filepath.Join(dataDir, "condition_b", folderName)

		requiredFiles := []string{
			"prompt.txt",
			"response_raw.md",
			"patch.diff",
			"test_execution.log",
			"telemetry.json",
		}

		for _, rf := range requiredFiles {
			path := filepath.Join(taskDir, rf)
			info, err := os.Stat(path)
			if err != nil {
				t.Errorf("missing required file for %s: %s (%v)", task.ID, rf, err)
				continue
			}
			if info.Size() == 0 {
				t.Errorf("file is empty for %s: %s", task.ID, rf)
			}
		}

		// Verify telemetry JSON validity
		telemBytes, err := os.ReadFile(filepath.Join(taskDir, "telemetry.json"))
		if err == nil {
			var telem ConditionMetrics
			if err := json.Unmarshal(telemBytes, &telem); err != nil {
				t.Errorf("invalid telemetry JSON for %s: %v", task.ID, err)
			}
			if telem.TotalCriteria == 0 || telem.TotalTests == 0 {
				t.Errorf("unpopulated criteria or tests in telemetry for %s", task.ID)
			}
		}
	}
}

// TC-115-03: Metric Calculation & Scoring Rigor
func TestMetricCalculationRigor(t *testing.T) {
	eval := NewEvaluator("data")
	report, err := eval.Evaluate()
	if err != nil {
		t.Fatalf("evaluation failed: %v", err)
	}

	for _, comp := range report.TaskCohort {
		// Condition A invariants
		if comp.ConditionA.DriftRatePercent != 0.0 {
			t.Errorf("expected Condition A drift rate 0.0%% for %s, got %.2f%%", comp.TaskID, comp.ConditionA.DriftRatePercent)
		}
		if comp.ConditionA.TotalDefects != 0 {
			t.Errorf("expected Condition A defects 0 for %s, got %d", comp.TaskID, comp.ConditionA.TotalDefects)
		}
		if comp.ConditionA.TestPassRatePercent != 100.0 {
			t.Errorf("expected Condition A pass rate 100.0%% for %s, got %.2f%%", comp.TaskID, comp.ConditionA.TestPassRatePercent)
		}
		if comp.ConditionA.ReviewerArrestRatePercent != 100.0 {
			t.Errorf("expected Condition A arrest rate 100.0%% for %s, got %.2f%%", comp.TaskID, comp.ConditionA.ReviewerArrestRatePercent)
		}

		// Condition B comparative bounds
		if comp.ConditionB.DriftRatePercent <= 0.0 {
			t.Errorf("expected Condition B to exhibit non-zero drift for %s", comp.TaskID)
		}
		if comp.ConditionB.TotalDefects <= 0 {
			t.Errorf("expected Condition B to inject at least 1 defect for %s", comp.TaskID)
		}
		if comp.ConditionB.TestPassRatePercent >= 100.0 {
			t.Errorf("expected Condition B pass rate < 100.0%% for %s", comp.TaskID)
		}
		if comp.TokenExpansionRatio <= 1.0 {
			t.Errorf("expected token expansion ratio > 1.0 for %s, got %.2f", comp.TaskID, comp.TokenExpansionRatio)
		}
	}
}

// TC-115-04: Aggregate Statistics & Confidence Bounds
func TestAggregateStatistics(t *testing.T) {
	eval := NewEvaluator("data")
	report, err := eval.Evaluate()
	if err != nil {
		t.Fatalf("evaluation failed: %v", err)
	}

	agg := report.Aggregate
	if agg.ConditionAMeanDrift != 0.0 {
		t.Errorf("expected Condition A mean drift 0.0, got %.2f", agg.ConditionAMeanDrift)
	}
	if agg.ConditionBMeanDrift < 50.0 {
		t.Errorf("expected Condition B mean drift >= 50.0%%, got %.2f%%", agg.ConditionBMeanDrift)
	}
	if agg.ConditionAMeanDefects != 0.0 {
		t.Errorf("expected Condition A mean defects 0.0, got %.2f", agg.ConditionAMeanDefects)
	}
	if agg.ConditionBMeanDefects < 1.0 {
		t.Errorf("expected Condition B mean defects >= 1.0, got %.2f", agg.ConditionBMeanDefects)
	}
	if agg.ConditionAMeanPassRate != 100.0 {
		t.Errorf("expected Condition A pass rate 100.0, got %.2f", agg.ConditionAMeanPassRate)
	}
	if agg.ConditionBMeanPassRate > 70.0 {
		t.Errorf("expected Condition B pass rate < 70.0, got %.2f", agg.ConditionBMeanPassRate)
	}
	if agg.OverallCostRatio < 2.0 || agg.OverallCostRatio > 5.0 {
		t.Errorf("expected cost ratio between 2.0x and 5.0x, got %.2fx", agg.OverallCostRatio)
	}
	if agg.ConditionAMeanArrestRate != 100.0 {
		t.Errorf("expected Condition A mean arrest rate 100.0, got %.2f", agg.ConditionAMeanArrestRate)
	}
}

// TC-115-05 & TC-115-06: Report Serialization
func TestReportGeneration(t *testing.T) {
	eval := NewEvaluator("data")
	report, err := eval.Evaluate()
	if err != nil {
		t.Fatalf("evaluation failed: %v", err)
	}

	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "report.json")
	mdPath := filepath.Join(tmpDir, "report.md")

	if err := GenerateJSONReport(report, jsonPath); err != nil {
		t.Fatalf("failed to generate JSON report: %v", err)
	}
	if err := GenerateMarkdownReport(report, mdPath); err != nil {
		t.Fatalf("failed to generate Markdown report: %v", err)
	}

	// Verify JSON content
	jsonBytes, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("failed to read generated JSON report: %v", err)
	}
	var reloaded AblationReport
	if err := json.Unmarshal(jsonBytes, &reloaded); err != nil {
		t.Fatalf("reloaded JSON failed to unmarshal: %v", err)
	}
	if len(reloaded.TaskCohort) != 10 {
		t.Errorf("reloaded report has %d tasks, expected 10", len(reloaded.TaskCohort))
	}

	// Verify Markdown content
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("failed to read generated Markdown report: %v", err)
	}
	mdStr := string(mdBytes)
	requiredSubstrings := []string{
		"# 10-Task Controlled Ablation Experiment Report (BMK-05)",
		"## 1. Executive Summary",
		"## 2. Aggregate Statistical Summary",
		"## 3. Task-by-Task Comparative Performance Matrix",
		"## 4. Qualitative Failure Mode Analysis (Condition B)",
		"## 5. Methodological Conclusions & Academic Verification",
		"AER-001",
		"MSR-001",
		"PDR-001",
		"TASK-061",
		"TASK-070",
	}
	for _, sub := range requiredSubstrings {
		if !strings.Contains(mdStr, sub) {
			t.Errorf("generated Markdown missing required section: %s", sub)
		}
	}
}
