package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/router"
	"toron/pkg/server"
	"toron/pkg/waf"
)

// setupInProcessToronServer boots an in-process Toron edge gateway for load testing.
func setupInProcessToronServer(t *testing.T) (*server.Server, string, func()) {
	t.Helper()

	r := router.New()

	// Configure WAF and router for active defense (REQ-116, REQ-117, TC-117 Section 3.2)
	wafEngine, err := waf.NewEngine(waf.DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create WAF engine: %v", err)
	}
	wafMw := waf.NewWAFMiddleware(wafEngine)
	r.Use(func(next router.HandlerFunc) router.HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			wafMw(waf.HandlerFunc(next))(req, res)
		}
	})

	// Mount static directory at /internal/dashboard
	staticDir := t.TempDir()
	r.Static("/internal/dashboard", staticDir)

	r.GET("/health", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"status":"ok","time":"` + time.Now().Format(time.RFC3339) + `"}`)
	})
	r.POST("/echo", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"status":"echo"}`)
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.WorkerPoolSize = 64
	cfg.HTTP2Enabled = true

	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen on %s: %v", cfg.Addr, err)
	}

	go func() {
		_ = srv.Serve(ln)
	}()

	addr := ln.Addr().String()

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
		_ = ln.Close()
	}

	return srv, addr, cleanup
}

func TestLoadGen_AdversarialInjection(t *testing.T) {
	_, addr, cleanup := setupInProcessToronServer(t)
	defer cleanup()

	targetURL := "http://" + addr + "/health"

	cfg := LoadGenConfig{
		TargetURL:   targetURL,
		Concurrency: 10,
		Duration:    2 * time.Second,
		TargetRate:  2000,
		AttackRatio: 0.15, // 15% adversarial injection
		Method:      "GET",
	}

	report, err := RunLoadGen(cfg)
	if err != nil {
		t.Fatalf("RunLoadGen failed: %v", err)
	}

	if report.TotalRequestsExecuted == 0 {
		t.Fatalf("expected requests to be executed, got 0")
	}

	if report.BenignStream.TotalRequests == 0 {
		t.Errorf("expected benign requests, got 0")
	}

	if report.AdversarialStream.TotalRequests == 0 {
		t.Errorf("expected adversarial probes, got 0")
	}

	// Assert zero bypasses on attack stream
	if report.AdversarialStream.FailedRequests > 0 {
		t.Errorf("expected 0 attack bypasses, got %d", report.AdversarialStream.FailedRequests)
	}

	// Assert 100% invariant enforcement rate
	if report.InvariantEnforcementRate < 100.0 {
		t.Errorf("expected 100%% invariant enforcement, got %.2f%%", report.InvariantEnforcementRate)
	}

	// Assert zero-starvation on benign stream
	if !report.ZeroStarvationVerified {
		t.Errorf("expected zero starvation verified (p99 <= 50ms), got p99 = %.2f ms",
			report.BenignStream.LatenciesMs.P99)
	}

	if report.OverallVerdict != "PASS" {
		t.Errorf("expected overall verdict PASS, got %s", report.OverallVerdict)
	}
}

func TestLoadGen_ReportGeneration(t *testing.T) {
	_, addr, cleanup := setupInProcessToronServer(t)
	defer cleanup()

	targetURL := "http://" + addr + "/health"
	tmpDir := t.TempDir()

	jsonPath := filepath.Join(tmpDir, "report.json")
	mdPath := filepath.Join(tmpDir, "report.md")

	cfg := LoadGenConfig{
		TargetURL:   targetURL,
		Concurrency: 5,
		Duration:    1 * time.Second,
		TargetRate:  1000,
		AttackRatio: 0.10,
		Method:      "GET",
		JSONPath:    jsonPath,
		MDPath:      mdPath,
	}

	report, err := RunLoadGen(cfg)
	if err != nil {
		t.Fatalf("RunLoadGen failed: %v", err)
	}

	// Write JSON
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		t.Fatalf("failed to write JSON: %v", err)
	}

	// Write Markdown
	if err := GenerateMarkdownReport(report, mdPath); err != nil {
		t.Fatalf("failed to generate Markdown: %v", err)
	}

	// Verify file sizes
	jsonInfo, err := os.Stat(jsonPath)
	if err != nil || jsonInfo.Size() == 0 {
		t.Errorf("expected non-empty JSON report, got err=%v", err)
	}

	mdInfo, err := os.Stat(mdPath)
	if err != nil || mdInfo.Size() == 0 {
		t.Errorf("expected non-empty MD report, got err=%v", err)
	}
}

// mockAdversarialServer provides a programmable TCP mock server for status testing.
type mockAdversarialServer struct {
	mu           sync.Mutex
	responseMap  map[string]int
	fallbackCode int
	closeOnRecv  bool
}

func newMockAdversarialServer(t *testing.T, fallbackCode int) (*mockAdversarialServer, string, func()) {
	t.Helper()
	ms := &mockAdversarialServer{
		responseMap:  make(map[string]int),
		fallbackCode: fallbackCode,
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	closed := make(chan struct{})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-closed:
					return
				default:
					return
				}
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				n, err := c.Read(buf)
				if err != nil && n == 0 {
					return
				}
				data := string(buf[:n])

				ms.mu.Lock()
				closeNow := ms.closeOnRecv
				fallback := ms.fallbackCode
				respCode := fallback
				for key, code := range ms.responseMap {
					if strings.Contains(data, key) {
						respCode = code
						break
					}
				}
				ms.mu.Unlock()

				if closeNow {
					return
				}

				// If benign request (GET /health) and not overridden in responseMap
				if strings.HasPrefix(data, "GET /health HTTP") &&
					!strings.Contains(data, "canary") &&
					!strings.Contains(data, "evil") &&
					!strings.Contains(data, "Bad") &&
					!strings.Contains(data, "Injected") &&
					!strings.Contains(data, "Oversized") {
					resp := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nOK"
					_, _ = c.Write([]byte(resp))
					return
				}

				reason := http.StatusText(respCode)
				if reason == "" {
					reason = "Custom"
				}
				resp := fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", respCode, reason)
				_, _ = c.Write([]byte(resp))
			}(conn)
		}
	}()

	cleanup := func() {
		close(closed)
		_ = ln.Close()
	}

	return ms, ln.Addr().String(), cleanup
}

func TestLoadGen_ClassificationLogic(t *testing.T) {
	// Test Tier 1 Active Defense: 400, 403, 413, 431, 501
	tier1Codes := []int{
		http.StatusBadRequest,
		http.StatusForbidden,
		http.StatusRequestEntityTooLarge,
		http.StatusRequestHeaderFieldsTooLarge,
		http.StatusNotImplemented,
	}

	for _, code := range tier1Codes {
		t.Run(fmt.Sprintf("Tier1_%d", code), func(t *testing.T) {
			_, addr, cleanup := newMockAdversarialServer(t, code)
			defer cleanup()

			cfg := LoadGenConfig{
				TargetURL:   "http://" + addr + "/health",
				Concurrency: 2,
				Duration:    200 * time.Millisecond,
				TargetRate:  200,
				AttackRatio: 1.0, // 100% attack stream
			}

			rep, err := RunLoadGen(cfg)
			if err != nil {
				t.Fatalf("RunLoadGen failed: %v", err)
			}

			if rep.AdversarialStream.TotalRequests == 0 {
				t.Fatalf("expected attack requests, got 0")
			}
			if rep.AdversarialStream.ActiveDefenseRequests != rep.AdversarialStream.TotalRequests {
				t.Errorf("expected %d active defense requests, got %d",
					rep.AdversarialStream.TotalRequests, rep.AdversarialStream.ActiveDefenseRequests)
			}
			if rep.AdversarialStream.RouteMissRequests != 0 {
				t.Errorf("expected 0 route miss requests, got %d", rep.AdversarialStream.RouteMissRequests)
			}
			if rep.AdversarialStream.BypassedRequests != 0 {
				t.Errorf("expected 0 bypassed requests, got %d", rep.AdversarialStream.BypassedRequests)
			}
			if rep.AdversarialStream.UnhandledRequests != 0 {
				t.Errorf("expected 0 unhandled requests, got %d", rep.AdversarialStream.UnhandledRequests)
			}
			if rep.ActiveDefenseRatePct != 100.0 {
				t.Errorf("expected 100.0%% active defense rate, got %.2f%%", rep.ActiveDefenseRatePct)
			}
			if rep.RouteMissRatePct != 0.0 {
				t.Errorf("expected 0.0%% route miss rate, got %.2f%%", rep.RouteMissRatePct)
			}
			if rep.InvariantEnforcementRate != 100.0 {
				t.Errorf("expected 100.0%% invariant rate, got %.2f%%", rep.InvariantEnforcementRate)
			}
		})
	}

	// Test Tier 1 Socket Reset (transport-level EOF synthesized to 400)
	t.Run("Tier1_SocketReset", func(t *testing.T) {
		ms, addr, cleanup := newMockAdversarialServer(t, 400)
		defer cleanup()
		ms.mu.Lock()
		ms.closeOnRecv = true
		ms.mu.Unlock()

		cfg := LoadGenConfig{
			TargetURL:   "http://" + addr + "/health",
			Concurrency: 2,
			Duration:    200 * time.Millisecond,
			TargetRate:  200,
			AttackRatio: 1.0,
		}

		rep, err := RunLoadGen(cfg)
		if err != nil {
			t.Fatalf("RunLoadGen failed: %v", err)
		}

		if rep.AdversarialStream.TotalRequests == 0 {
			t.Fatalf("expected attack requests, got 0")
		}
		if rep.AdversarialStream.ActiveDefenseRequests != rep.AdversarialStream.TotalRequests {
			t.Errorf("expected all requests to be active defense on socket reset, got %d/%d",
				rep.AdversarialStream.ActiveDefenseRequests, rep.AdversarialStream.TotalRequests)
		}
		if rep.ActiveDefenseRatePct != 100.0 {
			t.Errorf("expected 100.0%% active defense rate, got %.2f%%", rep.ActiveDefenseRatePct)
		}
	})

	// Test Tier 2 Route Miss: 404
	t.Run("Tier2_404_RouteMiss", func(t *testing.T) {
		_, addr, cleanup := newMockAdversarialServer(t, http.StatusNotFound)
		defer cleanup()

		cfg := LoadGenConfig{
			TargetURL:   "http://" + addr + "/health",
			Concurrency: 2,
			Duration:    200 * time.Millisecond,
			TargetRate:  200,
			AttackRatio: 1.0,
		}

		rep, err := RunLoadGen(cfg)
		if err != nil {
			t.Fatalf("RunLoadGen failed: %v", err)
		}

		if rep.AdversarialStream.TotalRequests == 0 {
			t.Fatalf("expected attack requests, got 0")
		}
		if rep.AdversarialStream.RouteMissRequests != rep.AdversarialStream.TotalRequests {
			t.Errorf("expected %d route miss requests, got %d",
				rep.AdversarialStream.TotalRequests, rep.AdversarialStream.RouteMissRequests)
		}
		if rep.AdversarialStream.ActiveDefenseRequests != 0 {
			t.Errorf("expected 0 active defense requests, got %d", rep.AdversarialStream.ActiveDefenseRequests)
		}
		if rep.ActiveDefenseRatePct != 0.0 {
			t.Errorf("expected 0.0%% active defense rate, got %.2f%%", rep.ActiveDefenseRatePct)
		}
		if rep.RouteMissRatePct != 100.0 {
			t.Errorf("expected 100.0%% route miss rate, got %.2f%%", rep.RouteMissRatePct)
		}
		if rep.OverallVerdict != "FAIL" {
			t.Errorf("expected overall verdict FAIL, got %s", rep.OverallVerdict)
		}
	})

	// Test Tier 3 Attack Bypass: 200
	t.Run("Tier3_200_Bypass", func(t *testing.T) {
		_, addr, cleanup := newMockAdversarialServer(t, http.StatusOK)
		defer cleanup()

		cfg := LoadGenConfig{
			TargetURL:   "http://" + addr + "/health",
			Concurrency: 2,
			Duration:    200 * time.Millisecond,
			TargetRate:  200,
			AttackRatio: 1.0,
		}

		rep, err := RunLoadGen(cfg)
		if err != nil {
			t.Fatalf("RunLoadGen failed: %v", err)
		}

		if rep.AdversarialStream.BypassedRequests != rep.AdversarialStream.TotalRequests {
			t.Errorf("expected %d bypassed requests, got %d",
				rep.AdversarialStream.TotalRequests, rep.AdversarialStream.BypassedRequests)
		}
		if rep.AdversarialStream.ActiveDefenseRequests != 0 {
			t.Errorf("expected 0 active defense requests, got %d", rep.AdversarialStream.ActiveDefenseRequests)
		}
		if rep.ActiveDefenseRatePct != 0.0 {
			t.Errorf("expected 0.0%% active defense rate, got %.2f%%", rep.ActiveDefenseRatePct)
		}
		if rep.OverallVerdict != "FAIL" {
			t.Errorf("expected overall verdict FAIL, got %s", rep.OverallVerdict)
		}
	})

	// Test Tier 4 Unhandled Anomaly: 500, 502, 999
	for _, code := range []int{http.StatusInternalServerError, http.StatusBadGateway, 999} {
		t.Run(fmt.Sprintf("Tier4_%d_Unhandled", code), func(t *testing.T) {
			_, addr, cleanup := newMockAdversarialServer(t, code)
			defer cleanup()

			cfg := LoadGenConfig{
				TargetURL:   "http://" + addr + "/health",
				Concurrency: 2,
				Duration:    200 * time.Millisecond,
				TargetRate:  200,
				AttackRatio: 1.0,
			}

			rep, err := RunLoadGen(cfg)
			if err != nil {
				t.Fatalf("RunLoadGen failed: %v", err)
			}

			if rep.AdversarialStream.UnhandledRequests != rep.AdversarialStream.TotalRequests {
				t.Errorf("expected %d unhandled requests, got %d",
					rep.AdversarialStream.TotalRequests, rep.AdversarialStream.UnhandledRequests)
			}
			if rep.AdversarialStream.ActiveDefenseRequests != 0 {
				t.Errorf("expected 0 active defense requests, got %d", rep.AdversarialStream.ActiveDefenseRequests)
			}
			if rep.ActiveDefenseRatePct != 0.0 {
				t.Errorf("expected 0.0%% active defense rate, got %.2f%%", rep.ActiveDefenseRatePct)
			}
			if rep.OverallVerdict != "FAIL" {
				t.Errorf("expected overall verdict FAIL, got %s", rep.OverallVerdict)
			}
		})
	}
}

func TestLoadGen_StatusClassification_FourTiers(t *testing.T) {
	TestLoadGen_ClassificationLogic(t)
}

func TestLoadGen_RouteMissFailsVerdict(t *testing.T) {
	ms, addr, cleanup := newMockAdversarialServer(t, http.StatusBadRequest)
	defer cleanup()
	ms.mu.Lock()
	ms.responseMap["canary_traversal.txt"] = http.StatusNotFound
	ms.mu.Unlock()

	cfg := LoadGenConfig{
		TargetURL:   "http://" + addr + "/health",
		Concurrency: 5,
		Duration:    1 * time.Second,
		TargetRate:  1000,
		AttackRatio: 0.25, // 25% attack probes
		Method:      "GET",
	}

	report, err := RunLoadGen(cfg)
	if err != nil {
		t.Fatalf("RunLoadGen failed: %v", err)
	}

	if report.AdversarialStream.RouteMissRequests == 0 {
		t.Errorf("expected RouteMissRequests > 0 from canary_traversal.txt probes, got 0")
	}

	if report.ActiveDefenseRatePct >= 100.0 {
		t.Errorf("expected ActiveDefenseRatePct < 100.0%% due to 404 route misses, got %.2f%%", report.ActiveDefenseRatePct)
	}

	if report.RouteMissRatePct <= 0.0 {
		t.Errorf("expected RouteMissRatePct > 0.0%%, got %.2f%%", report.RouteMissRatePct)
	}

	if report.OverallVerdict != "FAIL" {
		t.Errorf("expected OverallVerdict == 'FAIL' when route miss occurs, got %s", report.OverallVerdict)
	}

	// Verify ADV-06 specific summary
	for _, v := range report.AttackVectors {
		if v.ID == "ADV-06" {
			if v.ProbesSent > 0 && v.RouteMiss != v.ProbesSent {
				t.Errorf("expected ADV-06 RouteMiss (%d) to equal ProbesSent (%d)", v.RouteMiss, v.ProbesSent)
			}
			if v.Rejected != 0 {
				t.Errorf("expected ADV-06 Rejected == 0, got %d", v.Rejected)
			}
			if v.ActiveDefenseRatePct != 0.0 {
				t.Errorf("expected ADV-06 ActiveDefenseRatePct == 0.0, got %.2f", v.ActiveDefenseRatePct)
			}
			break
		}
	}
}

func TestLoadGen_BypassFailsVerdict(t *testing.T) {
	_, addr, cleanup := newMockAdversarialServer(t, http.StatusOK)
	defer cleanup()

	cfg := LoadGenConfig{
		TargetURL:   "http://" + addr + "/health",
		Concurrency: 4,
		Duration:    500 * time.Millisecond,
		TargetRate:  500,
		AttackRatio: 0.20,
		Method:      "GET",
	}

	report, err := RunLoadGen(cfg)
	if err != nil {
		t.Fatalf("RunLoadGen failed: %v", err)
	}

	if report.AdversarialStream.BypassedRequests == 0 {
		t.Errorf("expected BypassedRequests > 0, got 0")
	}

	if report.OverallVerdict != "FAIL" {
		t.Errorf("expected overall verdict FAIL on bypass, got %s", report.OverallVerdict)
	}
}

func TestLoadGen_AttackBypassFailsVerdict(t *testing.T) {
	TestLoadGen_BypassFailsVerdict(t)
}

func TestLoadGen_UnhandledAnomalyFailsVerdict(t *testing.T) {
	_, addr, cleanup := newMockAdversarialServer(t, http.StatusInternalServerError)
	defer cleanup()

	cfg := LoadGenConfig{
		TargetURL:   "http://" + addr + "/health",
		Concurrency: 4,
		Duration:    500 * time.Millisecond,
		TargetRate:  500,
		AttackRatio: 0.20,
		Method:      "GET",
	}

	report, err := RunLoadGen(cfg)
	if err != nil {
		t.Fatalf("RunLoadGen failed: %v", err)
	}

	if report.AdversarialStream.UnhandledRequests == 0 {
		t.Errorf("expected UnhandledRequests > 0, got 0")
	}

	if report.OverallVerdict != "FAIL" {
		t.Errorf("expected overall verdict FAIL on 500 error, got %s", report.OverallVerdict)
	}
}

func TestLoadGen_AST_NoCatchAllElse(t *testing.T) {
	loadgenPath := "loadgen.go"
	if _, err := os.Stat(loadgenPath); os.IsNotExist(err) {
		loadgenPath = filepath.Join("..", "..", "benchmarks", "wrk2", "loadgen.go")
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, loadgenPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse loadgen.go: %v", err)
	}

	var foundSwitch bool
	var foundCaseBadRequest bool
	var foundCaseNotFound bool
	var foundCaseOK bool
	var foundDefaultUnhandled bool
	var illegalElseCall bool

	ast.Inspect(node, func(n ast.Node) bool {
		// Verify no if-else branch invokes attackRejected.Add
		if ifStmt, ok := n.(*ast.IfStmt); ok {
			if ifStmt.Else != nil {
				ast.Inspect(ifStmt.Else, func(elseNode ast.Node) bool {
					if call, ok := elseNode.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							if sel.Sel.Name == "Add" {
								if id, ok := sel.X.(*ast.Ident); ok && id.Name == "attackRejected" {
									illegalElseCall = true
								}
							}
						}
					}
					return true
				})
			}
		}

		// Locate the switch statement on `code` in RunLoadGen
		if sw, ok := n.(*ast.SwitchStmt); ok {
			if ident, ok := sw.Tag.(*ast.Ident); ok && ident.Name == "code" {
				foundSwitch = true
				for _, stmt := range sw.Body.List {
					cc, ok := stmt.(*ast.CaseClause)
					if !ok {
						continue
					}
					if cc.List == nil {
						// Default case -> must increment attackUnhandled
						for _, bodyStmt := range cc.Body {
							if exprStmt, ok := bodyStmt.(*ast.ExprStmt); ok {
								if call, ok := exprStmt.X.(*ast.CallExpr); ok {
									if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
										if sel.Sel.Name == "Add" {
											if id, ok := sel.X.(*ast.Ident); ok && id.Name == "attackUnhandled" {
												foundDefaultUnhandled = true
											}
										}
									}
								}
							}
						}
					} else {
						for _, expr := range cc.List {
							if sel, ok := expr.(*ast.SelectorExpr); ok {
								switch sel.Sel.Name {
								case "StatusBadRequest":
									foundCaseBadRequest = true
								case "StatusNotFound":
									foundCaseNotFound = true
								case "StatusOK":
									foundCaseOK = true
								}
							}
						}
					}
				}
			}
		}
		return true
	})

	if illegalElseCall {
		t.Errorf("CRITICAL: detected illegal call to attackRejected.Add within an else branch")
	}
	if !foundSwitch {
		t.Errorf("expected switch code construct, none found")
	}
	if !foundCaseBadRequest {
		t.Errorf("expected switch case with StatusBadRequest")
	}
	if !foundCaseNotFound {
		t.Errorf("expected switch case with StatusNotFound (Tier 2 Route Miss)")
	}
	if !foundCaseOK {
		t.Errorf("expected switch case with StatusOK (Tier 3 Attack Bypass)")
	}
	if !foundDefaultUnhandled {
		t.Errorf("expected default case incrementing attackUnhandled (Tier 4)")
	}
}

func TestLoadGen_MarkdownTable6Format(t *testing.T) {
	tmpDir := t.TempDir()
	mdPath := filepath.Join(tmpDir, "table6_test.md")

	rep := &SaturationStressReport{
		Timestamp:             time.Now().UTC().Format(time.RFC3339),
		TargetURL:             "http://127.0.0.1:8080/health",
		TargetHostPort:        "127.0.0.1:8080",
		Concurrency:           50,
		DurationSeconds:       10.0,
		TargetRateRPS:         5000,
		AttackRatio:           0.10,
		TotalRequestsExecuted: 24310,
		TotalActualRPS:        2431.0,
		ActiveDefenseRatePct:  95.0,
		RouteMissRatePct:      5.0,
		OverallVerdict:        "FAIL",
		BenignStream: StreamMetrics{
			TotalRequests:   21879,
			SuccessRequests: 21879,
			FailedRequests:  0,
		},
		AdversarialStream: StreamMetrics{
			TotalRequests:         2431,
			SuccessRequests:       2310,
			FailedRequests:        121,
			ActiveDefenseRequests: 2310,
			RouteMissRequests:     121,
			BypassedRequests:      0,
			UnhandledRequests:     0,
		},
		AttackVectors: []AttackVectorSummary{
			{
				ID:                   "ADV-01",
				Name:                 "CL.TE Conflicting Framing Smuggle",
				Category:             "HTTP Request Smuggling (CWE-444)",
				ProbesSent:           300,
				Rejected:             300,
				RouteMiss:            0,
				Bypassed:             0,
				Unhandled:            0,
				ActiveDefenseRatePct: 100.0,
				RouteMissRatePct:     0.0,
			},
			{
				ID:                   "ADV-06",
				Name:                 "Path Traversal Directory Escape",
				Category:             "Path Traversal Defense (CWE-22)",
				ProbesSent:           350,
				Rejected:             300,
				RouteMiss:            50,
				Bypassed:             0,
				Unhandled:            0,
				ActiveDefenseRatePct: 85.7,
				RouteMissRatePct:     14.3,
			},
		},
	}

	if err := GenerateMarkdownReport(rep, mdPath); err != nil {
		t.Fatalf("GenerateMarkdownReport failed: %v", err)
	}

	contentBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("failed to read generated markdown: %v", err)
	}
	content := string(contentBytes)

	// Verify Section 1 Executive Summary item 3 text
	expectedItem3 := "with **0 route misses ($404$)**, **0 unhandled anomalies**, and **`0` security bypasses**"
	if !strings.Contains(content, expectedItem3) {
		t.Errorf("Section 1 item 3 text missing expected phrase %q", expectedItem3)
	}

	// Verify Section 2 evaluation verdict row
	if !strings.Contains(content, "2310 Active Defense / 121 Route Miss / 0 Bypass (**95.0% Active Defense**)") {
		t.Errorf("Section 2 evaluation verdict row format mismatch in markdown:\n%s", content)
	}

	// Verify Section 4 Table 6 Header
	expectedSection4 := "## 4. Concurrent Adversarial Invariant Breakdown (Table 6 Alignment)"
	if !strings.Contains(content, expectedSection4) {
		t.Errorf("missing Section 4 Table 6 header: %q", expectedSection4)
	}

	expectedCols := "| Vector ID | Attack Name | Category | Probes Sent | Active Defense (4xx/501) | Route Miss (404) | Bypass Count (200) | Unhandled | Active Defense Rate |"
	if !strings.Contains(content, expectedCols) {
		t.Errorf("missing 9-column Table 6 header: %q", expectedCols)
	}

	// Verify ADV-06 row
	expectedRow := "| `ADV-06` | Path Traversal Directory Escape | Path Traversal Defense (CWE-22) | 350 | 300 | 50 | 0 | 0 | **85.7%** |"
	if !strings.Contains(content, expectedRow) {
		t.Errorf("missing or mismatched ADV-06 row: %q", expectedRow)
	}
}

func TestLoadGen_ReportGeneration_Table6(t *testing.T) {
	TestLoadGen_MarkdownTable6Format(t)
}

func TestLoadGen_TelemetryParity_JSON_CSV(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "report.json")
	csvPath := filepath.Join(tmpDir, "report.csv")

	rep := &SaturationStressReport{
		Timestamp:              time.Now().UTC().Format(time.RFC3339),
		TargetURL:              "http://127.0.0.1:8080/health",
		TargetHostPort:         "127.0.0.1:8080",
		Concurrency:            20,
		DurationSeconds:        5.0,
		TargetRateRPS:          2000,
		AttackRatio:            0.10,
		TotalRequestsExecuted:  10000,
		TotalActualRPS:         2000.0,
		ActiveDefenseRatePct:   92.5,
		RouteMissRatePct:       7.5,
		OverallVerdict:         "FAIL",
		ZeroStarvationVerified: true,
		BenignStream: StreamMetrics{
			ActualRPS:       1800.0,
			TotalRequests:   9000,
			SuccessRequests: 9000,
			FailedRequests:  0,
			LatenciesMs: LatencyPercentiles{
				P50: 1.25,
				P99: 4.80,
			},
		},
		AdversarialStream: StreamMetrics{
			ActualRPS:             200.0,
			TotalRequests:         1000,
			SuccessRequests:       925,
			FailedRequests:        75,
			ActiveDefenseRequests: 925,
			RouteMissRequests:     75,
			BypassedRequests:      0,
			UnhandledRequests:     0,
		},
	}

	// 1. JSON Parity
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	var parsedRep SaturationStressReport
	if err := json.Unmarshal(data, &parsedRep); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsedRep.AdversarialStream.ActiveDefenseRequests != rep.AdversarialStream.ActiveDefenseRequests {
		t.Errorf("JSON ActiveDefenseRequests mismatch: %d vs %d",
			parsedRep.AdversarialStream.ActiveDefenseRequests, rep.AdversarialStream.ActiveDefenseRequests)
	}
	if parsedRep.AdversarialStream.RouteMissRequests != rep.AdversarialStream.RouteMissRequests {
		t.Errorf("JSON RouteMissRequests mismatch: %d vs %d",
			parsedRep.AdversarialStream.RouteMissRequests, rep.AdversarialStream.RouteMissRequests)
	}
	if parsedRep.ActiveDefenseRatePct != rep.ActiveDefenseRatePct {
		t.Errorf("JSON ActiveDefenseRatePct mismatch: %.2f vs %.2f",
			parsedRep.ActiveDefenseRatePct, rep.ActiveDefenseRatePct)
	}

	// 2. CSV Parity
	f, err := os.Create(csvPath)
	if err != nil {
		t.Fatalf("os.Create CSV failed: %v", err)
	}
	writer := csv.NewWriter(f)
	_ = writer.Write([]string{
		"Concurrency", "TargetRPS", "TotalActualRPS", "BenignRPS", "BenignP50_ms", "BenignP99_ms",
		"AttackRPS", "AttackRejected", "AttackRouteMiss", "AttackBypassed", "AttackUnhandled",
		"ActiveDefenseRatePct", "ZeroStarvation",
	})
	_ = writer.Write([]string{
		strconv.Itoa(rep.Concurrency),
		strconv.Itoa(rep.TargetRateRPS),
		fmt.Sprintf("%.2f", rep.TotalActualRPS),
		fmt.Sprintf("%.2f", rep.BenignStream.ActualRPS),
		fmt.Sprintf("%.2f", rep.BenignStream.LatenciesMs.P50),
		fmt.Sprintf("%.2f", rep.BenignStream.LatenciesMs.P99),
		fmt.Sprintf("%.2f", rep.AdversarialStream.ActualRPS),
		strconv.FormatInt(rep.AdversarialStream.ActiveDefenseRequests, 10),
		strconv.FormatInt(rep.AdversarialStream.RouteMissRequests, 10),
		strconv.FormatInt(rep.AdversarialStream.BypassedRequests, 10),
		strconv.FormatInt(rep.AdversarialStream.UnhandledRequests, 10),
		fmt.Sprintf("%.1f", rep.ActiveDefenseRatePct),
		strconv.FormatBool(rep.ZeroStarvationVerified),
	})
	writer.Flush()
	f.Close()

	csvFile, err := os.Open(csvPath)
	if err != nil {
		t.Fatalf("os.Open CSV failed: %v", err)
	}
	defer csvFile.Close()

	csvReader := csv.NewReader(csvFile)
	records, err := csvReader.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll CSV failed: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 CSV rows, got %d", len(records))
	}

	header := records[0]
	expectedHeader := []string{
		"Concurrency", "TargetRPS", "TotalActualRPS", "BenignRPS", "BenignP50_ms", "BenignP99_ms",
		"AttackRPS", "AttackRejected", "AttackRouteMiss", "AttackBypassed", "AttackUnhandled",
		"ActiveDefenseRatePct", "ZeroStarvation",
	}
	if len(header) != len(expectedHeader) {
		t.Fatalf("CSV header length %d != expected %d", len(header), len(expectedHeader))
	}
	for i, h := range expectedHeader {
		if header[i] != h {
			t.Errorf("CSV header column %d: expected %s, got %s", i, h, header[i])
		}
	}

	row := records[1]
	if row[7] != "925" { // AttackRejected
		t.Errorf("CSV AttackRejected: expected 925, got %s", row[7])
	}
	if row[8] != "75" { // AttackRouteMiss
		t.Errorf("CSV AttackRouteMiss: expected 75, got %s", row[8])
	}
	if row[11] != "92.5" { // ActiveDefenseRatePct
		t.Errorf("CSV ActiveDefenseRatePct: expected 92.5, got %s", row[11])
	}
}

func TestLoadGen_RaceFreeConcurrency(t *testing.T) {
	// Mixed mock server handling concurrent traffic
	ms, addr, cleanup := newMockAdversarialServer(t, http.StatusBadRequest)
	defer cleanup()

	ms.mu.Lock()
	ms.responseMap["canary_traversal.txt"] = http.StatusNotFound
	ms.responseMap["POST /echo"] = http.StatusOK
	ms.mu.Unlock()

	cfg := LoadGenConfig{
		TargetURL:   "http://" + addr + "/health",
		Concurrency: 20,
		Duration:    1 * time.Second,
		TargetRate:  3000,
		AttackRatio: 0.25,
		Method:      "GET",
	}

	report, err := RunLoadGen(cfg)
	if err != nil {
		t.Fatalf("RunLoadGen failed: %v", err)
	}

	// Verify partition invariant:
	// attackTotal == attackRejected + attackRouteMiss + attackBypassed + attackUnhandled
	act := report.AdversarialStream.ActiveDefenseRequests
	rm := report.AdversarialStream.RouteMissRequests
	byp := report.AdversarialStream.BypassedRequests
	unh := report.AdversarialStream.UnhandledRequests
	tot := report.AdversarialStream.TotalRequests

	if act+rm+byp+unh != tot {
		t.Errorf("Partition invariant violated: %d + %d + %d + %d != %d", act, rm, byp, unh, tot)
	}

	// Verify vector sums
	var sumSent, sumRej, sumRM, sumByp, sumUnh int64
	for _, v := range report.AttackVectors {
		sumSent += v.ProbesSent
		sumRej += v.Rejected
		sumRM += v.RouteMiss
		sumByp += v.Bypassed
		sumUnh += v.Unhandled
	}

	if sumSent != tot {
		t.Errorf("Sum of vector ProbesSent (%d) != total attack requests (%d)", sumSent, tot)
	}
	if sumRej != act {
		t.Errorf("Sum of vector Rejected (%d) != ActiveDefenseRequests (%d)", sumRej, act)
	}
	if sumRM != rm {
		t.Errorf("Sum of vector RouteMiss (%d) != RouteMissRequests (%d)", sumRM, rm)
	}
	if sumByp != byp {
		t.Errorf("Sum of vector Bypassed (%d) != BypassedRequests (%d)", sumByp, byp)
	}
	if sumUnh != unh {
		t.Errorf("Sum of vector Unhandled (%d) != UnhandledRequests (%d)", sumUnh, unh)
	}

	// Total executed requests check
	if report.BenignStream.TotalRequests+report.AdversarialStream.TotalRequests != report.TotalRequestsExecuted {
		t.Errorf("Benign + Attack != TotalRequestsExecuted")
	}
}
