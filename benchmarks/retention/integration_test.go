package retention_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"toron/benchmarks/retention"
)

func TestEndToEndCLIIntegration(t *testing.T) {
	tempBase := t.TempDir()
	historyBase := filepath.Join(tempBase, "history")
	canonicalDir := filepath.Join(tempBase, "canonical")

	_ = os.MkdirAll(historyBase, 0755)
	_ = os.MkdirAll(canonicalDir, 0755)

	// Create sample benchmark artifacts
	artifact1 := filepath.Join(tempBase, "report1.json")
	artifact2 := filepath.Join(tempBase, "report1.md")
	_ = os.WriteFile(artifact1, []byte(`{"p99": 2378.0, "status": "pass"}`), 0644)
	_ = os.WriteFile(artifact2, []byte("# Report 1 Summary"), 0644)

	// Step 1: Execute init action via Go CLI
	cmdInit := exec.Command("go", "run", "./cmd/main.go",
		"-action", "init",
		"-history-base", historyBase,
		"-canonical-dir", canonicalDir,
		"-suite", "test_suite",
	)
	outInit, err := cmdInit.CombinedOutput()
	if err != nil {
		t.Fatalf("cmdInit failed: %v, output: %s", err, string(outInit))
	}

	outLines := strings.Split(strings.TrimSpace(string(outInit)), "\n")
	var sessionDir string
	for _, l := range outLines {
		if strings.HasPrefix(l, "SESSION_DIR=") {
			sessionDir = strings.TrimPrefix(l, "SESSION_DIR=")
		}
	}
	if sessionDir == "" {
		t.Fatalf("failed to parse SESSION_DIR from output: %s", string(outInit))
	}

	// Step 2: Execute archive action
	cmdArchive := exec.Command("go", "run", "./cmd/main.go",
		"-action", "archive",
		"-history-base", historyBase,
		"-canonical-dir", canonicalDir,
		"-session-dir", sessionDir,
		"-suite", "differential_fuzzer",
		"-status", "success",
		"-duration", "5.2",
		"-artifacts", strings.Join([]string{artifact1, artifact2}, ","),
		"-params-json", `{"trials": 1000, "warmup": 50}`,
		"-command", "run_fuzzer.sh -k 1000",
	)
	outArchive, err := cmdArchive.CombinedOutput()
	if err != nil {
		t.Fatalf("cmdArchive failed: %v, output: %s", err, string(outArchive))
	}

	// Verify files exist in both history sessionDir and canonicalDir
	for _, f := range []string{"report1.json", "report1.md"} {
		hPath := filepath.Join(sessionDir, f)
		cPath := filepath.Join(canonicalDir, f)

		hData, err := os.ReadFile(hPath)
		if err != nil {
			t.Fatalf("expected historical file %s: %v", hPath, err)
		}
		cData, err := os.ReadFile(cPath)
		if err != nil {
			t.Fatalf("expected canonical file %s: %v", cPath, err)
		}
		if string(hData) != string(cData) {
			t.Fatalf("data mismatch for %s", f)
		}
	}

	// Verify session_meta.json
	metaPath := filepath.Join(sessionDir, "session_meta.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("expected session_meta.json in %s: %v", sessionDir, err)
	}
	var meta retention.SessionMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("failed to parse session_meta.json: %v", err)
	}
	if meta.Suite != "differential_fuzzer" || meta.Status != "success" {
		t.Errorf("unexpected meta content: %+v", meta)
	}

	// Step 3: Run a second archive to test append behavior
	artifact3 := filepath.Join(tempBase, "ablation.json")
	_ = os.WriteFile(artifact3, []byte(`{"ablation": "ok"}`), 0644)

	cmdArchive2 := exec.Command("go", "run", "./cmd/main.go",
		"-action", "archive",
		"-history-base", historyBase,
		"-canonical-dir", canonicalDir,
		"-suite", "ablation",
		"-status", "success",
		"-duration", "10.0",
		"-artifacts", artifact3,
	)
	outArchive2, err := cmdArchive2.CombinedOutput()
	if err != nil {
		t.Fatalf("cmdArchive2 failed: %v, output: %s", err, string(outArchive2))
	}

	// Verify manifest.json
	manifestPath := filepath.Join(historyBase, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest.json: %v", err)
	}

	var m retention.Manifest
	if err := json.Unmarshal(manifestData, &m); err != nil {
		t.Fatalf("failed to parse manifest.json: %v", err)
	}

	if m.TotalRuns != 2 || len(m.Runs) != 2 {
		t.Fatalf("expected 2 runs in manifest, got %d", m.TotalRuns)
	}

	if m.Runs[0].Suite != "differential_fuzzer" {
		t.Errorf("expected first run suite 'differential_fuzzer', got %s", m.Runs[0].Suite)
	}
	if m.Runs[1].Suite != "ablation" {
		t.Errorf("expected second run suite 'ablation', got %s", m.Runs[1].Suite)
	}
}
