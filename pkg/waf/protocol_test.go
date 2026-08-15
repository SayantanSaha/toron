package waf

import (
	"net/url"
	"strings"
	"testing"

	"toron/pkg/httpparser"
)

func TestWAF_RequestSmugglingDetection(t *testing.T) {
	cfg := DefaultProtocolConfig()

	// Conflicting Content-Length and Transfer-Encoding headers
	hdr := make(httpparser.Header)
	hdr.Set("Content-Length", "42")
	hdr.Set("Transfer-Encoding", "chunked")

	req := &httpparser.Request{
		Method: "POST",
		Path:   "/api/submit",
		Header: hdr,
	}

	err := ValidateProtocolIntegrity(req, cfg)
	if err != ErrSmugglingConflict {
		t.Errorf("got error %v, want ErrSmugglingConflict", err)
	}
}

func TestWAF_ControlCharacterDetection(t *testing.T) {
	cfg := DefaultProtocolConfig()

	hdr1 := make(httpparser.Header)

	hdr2 := make(httpparser.Header)
	hdr2.Set("X-Custom", "val\x1b")

	hdr3 := make(httpparser.Header)
	hdr3.Set("User-Agent", "Toron/1.0")

	tests := []struct {
		name    string
		path    string
		header  httpparser.Header
		wantErr bool
	}{
		{
			name:    "Control char in path (0x07 BEL)",
			path:    "/api/\x07user",
			header:  hdr1,
			wantErr: true,
		},
		{
			name:    "Control char in header (0x1B ESC)",
			path:    "/api/user",
			header:  hdr2,
			wantErr: true,
		},
		{
			name:    "Clean request",
			path:    "/api/user",
			header:  hdr3,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &httpparser.Request{
				Method: "GET",
				Path:   tt.path,
				Header: tt.header,
			}
			err := ValidateProtocolIntegrity(r, cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("got error %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWAF_HeaderAndQuerySizeBounding(t *testing.T) {
	cfg := DefaultProtocolConfig()
	cfg.MaxHeaderValue = 50
	cfg.MaxParamSize = 30

	// 1. Oversized header value
	hdr := make(httpparser.Header)
	hdr.Set("X-Long-Header", strings.Repeat("A", 100))
	reqHeader := &httpparser.Request{
		Method: "GET",
		Path:   "/test",
		Header: hdr,
	}
	if err := ValidateProtocolIntegrity(reqHeader, cfg); err != ErrHeaderValueTooLong {
		t.Errorf("got error %v, want ErrHeaderValueTooLong", err)
	}

	// 2. Oversized query parameter
	u, _ := url.Parse("http://localhost/test?param=" + strings.Repeat("B", 50))
	reqQuery := &httpparser.Request{
		Method: "GET",
		Path:   u.Path,
		URL:    u,
		Header: make(httpparser.Header),
	}
	if err := ValidateProtocolIntegrity(reqQuery, cfg); err != ErrParamTooLong {
		t.Errorf("got error %v, want ErrParamTooLong", err)
	}
}
