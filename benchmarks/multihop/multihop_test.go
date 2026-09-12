package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMultiHop_HeterogeneousEvaluation executes the complete BMK-03 evaluation suite.
func TestMultiHop_HeterogeneousEvaluation(t *testing.T) {
	env, err := SetupStandaloneTestbed()
	if err != nil {
		t.Fatalf("failed to initialize standalone testbed: %v", err)
	}
	defer env.Teardown()

	report := RunAllScenarios(env)

	if report.TotalScenarios != 30 {
		t.Fatalf("expected 30 scenarios (10 vectors x 3 backends), got %d", report.TotalScenarios)
	}

	if report.PassedScenarios != 30 {
		t.Errorf("expected 30/30 passed scenarios, got %d passed and %d failed",
			report.PassedScenarios, report.FailedScenarios)
	}

	if report.DesynchronizationCount != 0 {
		t.Errorf("expected 0 desynchronization events, got %d", report.DesynchronizationCount)
	}

	if report.PoolPoisoningCount != 0 {
		t.Errorf("expected 0 pool poisoning events, got %d", report.PoolPoisoningCount)
	}

	if report.OverallVerdict != "PASS" {
		t.Errorf("expected overall verdict PASS, got %s", report.OverallVerdict)
	}

	// Verify each backend has 10/10 passed vectors
	for _, b := range report.Backends {
		if b.PassedVectors != 10 {
			t.Errorf("backend %s (%s): expected 10/10 passed, got %d", b.Runtime, b.Parser, b.PassedVectors)
		}
		if b.DesyncCount != 0 {
			t.Errorf("backend %s: expected 0 desync, got %d", b.Runtime, b.DesyncCount)
		}
	}

	// Generate reports to verify report generation
	jsonPath := filepath.Join("..", "results", "multihop_report.json")
	mdPath := filepath.Join("..", "results", "multihop_report.md")

	if err := GenerateReports(report, jsonPath, mdPath); err != nil {
		t.Fatalf("failed to generate reports: %v", err)
	}

	// Verify report files exist and have content
	jsonInfo, err := os.Stat(jsonPath)
	if err != nil || jsonInfo.Size() == 0 {
		t.Fatalf("multihop_report.json was not generated or is empty")
	}

	mdInfo, err := os.Stat(mdPath)
	if err != nil || mdInfo.Size() == 0 {
		t.Fatalf("multihop_report.md was not generated or is empty")
	}
}

// Individual subtest specifications matching TC-113-01 through TC-113-10
func TestMultiHop_VectorsIndividual(t *testing.T) {
	env, err := SetupStandaloneTestbed()
	if err != nil {
		t.Fatalf("failed to initialize testbed: %v", err)
	}
	defer env.Teardown()

	vectors := GetStandardVectors()
	backends := []*SimulatedBackend{env.NodeBackend, env.PyBackend, env.GoBackend}

	for _, b := range backends {
		for _, v := range vectors {
			testName := b.Prefix + "/" + v.ID
			t.Run(testName, func(t *testing.T) {
				res := ExecuteScenario(env, b, v)
				if !res.OverallPassed {
					t.Fatalf("scenario %s failed: stage1_passed=%v, canary_passed=%v, desync=%v, reason=%s",
						testName, res.Stage1Passed, res.Stage2CanaryPassed, res.Desynchronization, res.FailureReason)
				}
				if res.Desynchronization {
					t.Fatalf("scenario %s resulted in desynchronization", testName)
				}
				if !res.PoolIntegrity {
					t.Fatalf("scenario %s corrupted connection pool", testName)
				}
			})
		}
	}
}
