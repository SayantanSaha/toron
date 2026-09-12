package waf

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"toron/pkg/httpparser"
)

func TestWAF_SQLInjectionDetection(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create WAF engine: %v", err)
	}

	tests := []struct {
		name        string
		path        string
		query       string
		headers     map[string]string
		body        string
		wantBlocked bool
	}{
		{
			name:        "Benign GET Request",
			path:        "/api/v1/users",
			query:       "id=123&name=alice",
			wantBlocked: false,
		},
		{
			name:        "SQLi UNION SELECT in Query",
			path:        "/api/v1/search",
			query:       "q=1'+UNION+SELECT+null,username,password+FROM+users--",
			wantBlocked: true,
		},
		{
			name:        "SQLi OR 1=1 in Query",
			path:        "/login",
			query:       "user=admin' OR 1=1--",
			wantBlocked: true,
		},
		{
			name:        "SQLi DROP TABLE in Body",
			path:        "/api/v1/execute",
			body:        `{"query": "DROP TABLE users;"}`,
			wantBlocked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqURL, _ := url.Parse("http://localhost" + tt.path + "?" + tt.query)
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = bytes.NewReader([]byte(tt.body))
			}
			req, _ := http.NewRequest("POST", reqURL.String(), bodyReader)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			blocked, score, matched, err := engine.Inspect(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if blocked != tt.wantBlocked {
				t.Errorf("blocked = %v, want %v (score: %d, matched: %v)", blocked, tt.wantBlocked, score, matched)
			}
		})
	}
}

func TestWAF_XSSDetection(t *testing.T) {
	engine, _ := NewEngine(DefaultConfig())

	reqURL, _ := url.Parse("http://localhost/search?q=<script>alert('xss')</script>")
	req, _ := http.NewRequest("GET", reqURL.String(), nil)

	blocked, score, matched, _ := engine.Inspect(req)
	if !blocked {
		t.Errorf("expected XSS payload to be blocked, score=%d, matched=%v", score, matched)
	}
}

func TestWAF_PathTraversalDetection(t *testing.T) {
	engine, _ := NewEngine(DefaultConfig())

	reqURL, _ := url.Parse("http://localhost/static/../../etc/passwd")
	req, _ := http.NewRequest("GET", reqURL.String(), nil)

	blocked, score, matched, _ := engine.Inspect(req)
	if !blocked {
		t.Errorf("expected Path Traversal payload to be blocked, score=%d, matched=%v", score, matched)
	}
}

func TestWAF_CommandInjectionDetection(t *testing.T) {
	engine, _ := NewEngine(DefaultConfig())

	reqURL, _ := url.Parse("http://localhost/ping?host=127.0.0.1;+/bin/sh")
	req, _ := http.NewRequest("GET", reqURL.String(), nil)

	blocked, score, matched, _ := engine.Inspect(req)
	if !blocked {
		t.Errorf("expected RCE payload to be blocked, score=%d, matched=%v", score, matched)
	}
}

func TestWAF_PathTraversal_CaseInsensitive(t *testing.T) {
	cfg := DefaultConfig()
	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	t.Run("TC-080-01: Uppercase Hex Dot-Dot-Slash in Header", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://localhost/api", nil)
		req.Header.Set("X-Path", "%2E%2E/secret.txt")

		blocked, score, matched, _ := engine.Inspect(req)
		if !blocked || score < 5 {
			t.Fatalf("expected header traversal to trigger, blocked=%v, score=%d", blocked, score)
		}
		found := false
		for _, m := range matched {
			if m.ID == "TRAVERSAL-001" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected TRAVERSAL-001 rule to match, got %v", matched)
		}
	})

	t.Run("TC-080-02: Mixed-Case Hex Dot-Dot-Slash in Body", func(t *testing.T) {
		body := `{"file":"%2e%2E%2Fetc/passwd"}`
		req, _ := http.NewRequest("POST", "http://localhost/api", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		blocked, score, matched, _ := engine.Inspect(req)
		if !blocked || score < 5 {
			t.Fatalf("expected body traversal to trigger, blocked=%v, score=%d", blocked, score)
		}
		found := false
		for _, m := range matched {
			if m.ID == "TRAVERSAL-001" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected TRAVERSAL-001 rule to match in body, got %v", matched)
		}
	})

	t.Run("TC-080-03: Encoded Backslash Sequence in Query", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://localhost/api?file=%2e%2e%5cwindows%5cwin.ini", nil)

		blocked, score, matched, _ := engine.Inspect(req)
		if !blocked || score < 5 {
			t.Fatalf("expected query traversal to trigger, blocked=%v, score=%d", blocked, score)
		}
		found := false
		for _, m := range matched {
			if m.ID == "TRAVERSAL-001" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected TRAVERSAL-001 rule to match in query, got %v", matched)
		}
	})
}

// TC-116-01: Layer 7 WAF Raw Wire URI Dot-Dot Traversal Inspection (REQ-116 / ADR-116 / TASK-139.1)
func TestWAF_InspectToron_RawWireURI_Traversal(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create WAF engine: %v", err)
	}

	testCases := []struct {
		name        string
		requestURI  string
		path        string
		wantBlocked bool
		wantRuleID  string
	}{
		{
			name:        "Raw Dot-Dot Path Traversal Sequence",
			requestURI:  "/internal/dashboard/../../canary_traversal.txt",
			path:        "/canary_traversal.txt",
			wantBlocked: true,
			wantRuleID:  "TRAVERSAL-001",
		},
		{
			name:        "Uppercase Percent-Encoded Traversal (%2E%2E)",
			requestURI:  "/internal/dashboard/%2E%2E/%2E%2E/canary_traversal.txt",
			path:        "/canary_traversal.txt",
			wantBlocked: true,
			wantRuleID:  "TRAVERSAL-001",
		},
		{
			name:        "Lowercase Percent-Encoded Traversal with Query and Fragment",
			requestURI:  "/internal/dashboard/%2e%2e/%2e%2e/canary_traversal.txt?query=test#section1",
			path:        "/canary_traversal.txt",
			wantBlocked: true,
			wantRuleID:  "TRAVERSAL-001",
		},
		{
			name:        "Encoded Slashes Traversal",
			requestURI:  "/assets/..%2f..%2fetc/passwd",
			path:        "/etc/passwd",
			wantBlocked: true,
			wantRuleID:  "TRAVERSAL-001",
		},
		{
			name:        "Double Percent-Encoded Traversal (%252e%252e)",
			requestURI:  "/internal/dashboard/%252e%252e/%252e%252e/canary_traversal.txt",
			path:        "/internal/dashboard/%2e%2e/%2e%2e/canary_traversal.txt",
			wantBlocked: true,
			wantRuleID:  "TRAVERSAL-001",
		},
		{
			name:        "Legitimate Request Without Traversal",
			requestURI:  "/internal/dashboard/assets/app.js",
			path:        "/internal/dashboard/assets/app.js",
			wantBlocked: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &httpparser.Request{
				Method:     "GET",
				RequestURI: tc.requestURI,
				Path:       tc.path,
				Header:     make(httpparser.Header),
			}

			blocked, score, matched, err := engine.InspectToron(req)
			if err != nil {
				t.Fatalf("unexpected InspectToron error: %v", err)
			}
			if blocked != tc.wantBlocked {
				t.Errorf("blocked mismatch: got %v, want %v (score=%d, matched=%v)", blocked, tc.wantBlocked, score, matched)
			}
			if tc.wantBlocked {
				if score < 5 {
					t.Errorf("expected threat score >= 5, got %d", score)
				}
				found := false
				for _, r := range matched {
					if r.ID == tc.wantRuleID {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected rule %s to match, got %v", tc.wantRuleID, matched)
				}
				blockedResp := FormatBlockedResponse(score, matched)
				if !strings.Contains(blockedResp, `"error":"Forbidden"`) || !strings.Contains(blockedResp, tc.wantRuleID) {
					t.Errorf("expected FormatBlockedResponse to include Forbidden and rule ID, got: %s", blockedResp)
				}
			}
		})
	}
}

// TC-116-02: WAF InspectToron Fallback when RequestURI is Empty (REQ-116 / ADR-116 / TASK-139.1)
func TestWAF_InspectToron_Fallback_EmptyRequestURI(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create WAF engine: %v", err)
	}

	testCases := []struct {
		name        string
		path        string
		wantBlocked bool
		wantRuleID  string
	}{
		{
			name:        "Fallback Dot-Dot Traversal in Path",
			path:        "/internal/../secret",
			wantBlocked: true,
			wantRuleID:  "TRAVERSAL-001",
		},
		{
			name:        "Fallback Encoded Traversal in Path",
			path:        "/%2e%2e/secret",
			wantBlocked: true,
			wantRuleID:  "TRAVERSAL-001",
		},
		{
			name:        "Fallback Benign Path",
			path:        "/api/v1/health",
			wantBlocked: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &httpparser.Request{
				Method:     "GET",
				RequestURI: "",
				Path:       tc.path,
				Header:     make(httpparser.Header),
			}

			blocked, score, matched, err := engine.InspectToron(req)
			if err != nil {
				t.Fatalf("unexpected InspectToron error: %v", err)
			}
			if blocked != tc.wantBlocked {
				t.Errorf("blocked mismatch: got %v, want %v (score=%d, matched=%v)", blocked, tc.wantBlocked, score, matched)
			}
			if tc.wantBlocked {
				if score < 5 {
					t.Errorf("expected threat score >= 5, got %d", score)
				}
				found := false
				for _, r := range matched {
					if r.ID == tc.wantRuleID {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected rule %s to match, got %v", tc.wantRuleID, matched)
				}
			}
		})
	}
}
