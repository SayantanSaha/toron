package waf

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
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
