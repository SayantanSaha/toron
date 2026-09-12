package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
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

// TestMultiHop_LiveModeInitialization implements TC-120-01.
// It verifies that live mode testbed environment properly initializes non-nil backend descriptors,
// populates required metadata, safely handles unstarted backend server close/teardown,
// and executes RunAllScenarios without nil pointer panics.
func TestMultiHop_LiveModeInitialization(t *testing.T) {
	edgeAddr := "127.0.0.1:8080"
	env := SetupLiveTestbed(edgeAddr)
	if env == nil {
		t.Fatalf("expected non-nil TestbedEnvironment")
	}

	if !env.IsLive {
		t.Errorf("expected IsLive to be true")
	}
	if env.EdgeAddr != edgeAddr {
		t.Errorf("expected EdgeAddr to be %s, got %s", edgeAddr, env.EdgeAddr)
	}
	if env.EdgeServer != nil {
		t.Errorf("expected EdgeServer to be nil in live mode")
	}

	// Step 2 & 3: Assert descriptors non-nil and metadata valid
	if env.NodeBackend == nil {
		t.Fatalf("expected non-nil NodeBackend descriptor")
	}
	if env.NodeBackend.Runtime != "Node.js 20 LTS" || env.NodeBackend.ParserEngine != "llhttp (C-based)" || env.NodeBackend.Prefix != "node" {
		t.Errorf("unexpected NodeBackend metadata: %+v", env.NodeBackend)
	}

	if env.PyBackend == nil {
		t.Fatalf("expected non-nil PyBackend descriptor")
	}
	if env.PyBackend.Runtime != "Python 3.11" || env.PyBackend.ParserEngine != "uvicorn / h11" || env.PyBackend.Prefix != "python" {
		t.Errorf("unexpected PyBackend metadata: %+v", env.PyBackend)
	}

	if env.GoBackend == nil {
		t.Fatalf("expected non-nil GoBackend descriptor")
	}
	if env.GoBackend.Runtime != "Go 1.24" || env.GoBackend.ParserEngine != "net/http" || env.GoBackend.Prefix != "go" {
		t.Errorf("unexpected GoBackend metadata: %+v", env.GoBackend)
	}

	// Step 4: Assert sb.Server == nil
	if env.NodeBackend.Server != nil || env.PyBackend.Server != nil || env.GoBackend.Server != nil {
		t.Errorf("expected sb.Server to be nil for unstarted live descriptors")
	}

	// Step 5: Safe Close and Teardown on unstarted descriptors (idempotent / no panic)
	env.NodeBackend.Close()
	env.NodeBackend.Close()
	env.PyBackend.Close()
	env.GoBackend.Close()
	env.Teardown()

	// Step 6: Verify RunAllScenarios iterates across descriptors without nil pointer panic
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on loopback: %v", err)
	}
	mockAddr := ln.Addr().String()
	_ = ln.Close() // closed immediately so probes fail fast without hang

	mockEnv := SetupLiveTestbed(mockAddr)
	defer mockEnv.Teardown()

	// Confirm that RunAllScenarios completes without any nil pointer dereference panic
	rep := RunAllScenarios(mockEnv)
	if rep.TotalScenarios != 30 {
		t.Errorf("expected 30 scenarios evaluated, got %d", rep.TotalScenarios)
	}
	if len(rep.Backends) != 3 {
		t.Errorf("expected 3 backend summaries, got %d", len(rep.Backends))
	}
	if rep.ExecutionMode != "Live Multi-Container (Docker Compose)" {
		t.Errorf("expected live execution mode, got %s", rep.ExecutionMode)
	}
}

// TestMultiHop_PseudoHeaderEchoVerification implements TC-120-04.
// It verifies that VECTOR-08 inspects the echoed JSON response from /echo over the wire,
// asserting zero colon-prefixed headers leak upstream, and verifies parser negative / boundary handling.
func TestMultiHop_PseudoHeaderEchoVerification(t *testing.T) {
	// 1. Positive validation against running standalone testbed
	env, err := SetupStandaloneTestbed()
	if err != nil {
		t.Fatalf("failed to setup testbed: %v", err)
	}
	defer env.Teardown()

	v8 := VectorDefinition{
		ID:             "VECTOR-08",
		Name:           "Pseudo-Header Upstream Isolation",
		Category:       "HTTP/2 to HTTP/1.1 Isolation",
		Protocol:       "HTTP/2",
		Description:    "HTTP/2 request with pseudo-headers asserting zero upstream wire leakage",
		ExpectedStatus: []int{200},
		ExpectClose:    false,
		IsAttack:       false,
	}

	res := ExecuteScenario(env, env.NodeBackend, v8)
	if !res.Stage1Passed || res.Stage1EdgeStatus != 200 {
		t.Fatalf("VECTOR-08 positive test failed: status=%d, passed=%v, reason=%s",
			res.Stage1EdgeStatus, res.Stage1Passed, res.FailureReason)
	}

	// 2. Direct unit test of parseAndValidateEchoHeaders: clean response
	cleanPayload := []byte(`{
		"status": "echo",
		"runtime": "Node.js 20 LTS",
		"sequence": 1,
		"headers": {
			"host": "localhost:8080",
			"content-type": "application/json",
			"x-forwarded-proto": "http",
			"user-agent": "Go-http-client/2.0"
		}
	}`)
	if err := parseAndValidateEchoHeaders(cleanPayload); err != nil {
		t.Errorf("expected clean payload to pass, got error: %v", err)
	}

	// 3. Negative test: Leaked pseudo-header
	leakedPayload := []byte(`{
		"status": "echo",
		"headers": {
			"host": "localhost:8080",
			":protocol": "websocket",
			"x-custom": "ok"
		}
	}`)
	err = parseAndValidateEchoHeaders(leakedPayload)
	if err == nil {
		t.Fatalf("expected error on leaked pseudo-header, got nil")
	}
	expectedErr := "pseudo-header leaked to upstream: :protocol"
	if err.Error() != expectedErr {
		t.Errorf("expected error %q, got %q", expectedErr, err.Error())
	}

	// 4. Negative test: Custom leaked pseudo-header
	customLeakedPayload := []byte(`{
		"status": "echo",
		"headers": {
			":custom-pseudo": "invisible"
		}
	}`)
	err = parseAndValidateEchoHeaders(customLeakedPayload)
	if err == nil {
		t.Fatalf("expected error on :custom-pseudo leak, got nil")
	}
	if !strings.Contains(err.Error(), ":custom-pseudo") {
		t.Errorf("expected error mentioning :custom-pseudo, got %v", err)
	}

	// 5. Boundary condition: Malformed JSON handled safely without panic
	malformedJSON := []byte(`{"headers": {not-valid-json`)
	if err := parseAndValidateEchoHeaders(malformedJSON); err == nil {
		t.Errorf("expected error on malformed JSON, got nil")
	}

	// 6. Boundary condition: Empty JSON body
	if err := parseAndValidateEchoHeaders([]byte{}); err == nil {
		t.Errorf("expected error on empty body, got nil")
	}
}

// TestMultiHop_LiveModeSimulation implements TC-120-06.
// It executes the entire 30-scenario benchmark suite in simulated live mode (IsLive: true),
// communicating strictly over physical TCP/h2c sockets without direct in-memory adapter calls,
// verifying complete evaluation parity and report generation.
func TestMultiHop_LiveModeSimulation(t *testing.T) {
	// Setup in-process listening servers (Toron Edge + 3 backends on loopback TCP)
	inproc, err := SetupStandaloneTestbed()
	if err != nil {
		t.Fatalf("failed to setup base testbed: %v", err)
	}
	defer inproc.Teardown()

	// Construct simulated live environment targeting inproc.EdgeAddr with IsLive: true
	// and EdgeServer explicitly nil to prove zero in-process handler invocation.
	liveEnv := SetupLiveTestbed(inproc.EdgeAddr)

	report := RunAllScenarios(liveEnv)

	if report.TotalScenarios != 30 {
		t.Fatalf("expected 30 scenarios, got %d", report.TotalScenarios)
	}
	if report.PassedScenarios != 30 {
		t.Errorf("expected 30/30 passed scenarios, got %d passed, %d failed",
			report.PassedScenarios, report.FailedScenarios)
		for _, r := range report.Results {
			if !r.OverallPassed {
				t.Errorf("failed vector %s/%s: stage1_passed=%v, canary_passed=%v, reason=%s",
					r.BackendPrefix, r.VectorID, r.Stage1Passed, r.Stage2CanaryPassed, r.FailureReason)
			}
		}
	}
	if report.FailedScenarios != 0 {
		t.Errorf("expected 0 failed scenarios, got %d", report.FailedScenarios)
	}
	if report.OverallVerdict != "PASS" {
		t.Errorf("expected PASS verdict, got %s", report.OverallVerdict)
	}
	if report.ExecutionMode != "Live Multi-Container (Docker Compose)" {
		t.Errorf("expected Live Multi-Container (Docker Compose), got %s", report.ExecutionMode)
	}
	if report.DesynchronizationCount != 0 {
		t.Errorf("expected 0 desync, got %d", report.DesynchronizationCount)
	}
	if report.PoolPoisoningCount != 0 {
		t.Errorf("expected 0 pool poisoning, got %d", report.PoolPoisoningCount)
	}

	// Verify all 3 backends
	if len(report.Backends) != 3 {
		t.Fatalf("expected 3 backends, got %d", len(report.Backends))
	}
	for _, b := range report.Backends {
		if b.PassedVectors != 10 {
			t.Errorf("backend %s: expected 10/10 passed, got %d", b.Runtime, b.PassedVectors)
		}
	}

	// Verify report generation in live mode
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "multihop_live_report.json")
	mdPath := filepath.Join(tmpDir, "multihop_live_report.md")

	if err := GenerateReports(report, jsonPath, mdPath); err != nil {
		t.Fatalf("failed to generate live reports: %v", err)
	}

	jsonBytes, err := os.ReadFile(jsonPath)
	if err != nil || len(jsonBytes) == 0 {
		t.Fatalf("live JSON report was not generated or is empty")
	}

	var parsedReport MultiHopReport
	if err := json.Unmarshal(jsonBytes, &parsedReport); err != nil {
		t.Fatalf("failed to parse generated JSON report: %v", err)
	}
	if parsedReport.TotalScenarios != 30 || parsedReport.PassedScenarios != 30 {
		t.Errorf("parsed report scenario count mismatch")
	}
	if len(parsedReport.Results) != 30 {
		t.Errorf("expected 30 result items in JSON, got %d", len(parsedReport.Results))
	}
}
