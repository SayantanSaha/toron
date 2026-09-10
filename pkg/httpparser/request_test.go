package httpparser

import (
	"net"
	"net/http"
	"testing"
)

type mockAddr struct {
	addr string
}

func (m *mockAddr) Network() string { return "tcp" }
func (m *mockAddr) String() string  { return m.addr }

type mockConn struct {
	net.Conn
	remoteAddr string
}

func (m *mockConn) RemoteAddr() net.Addr {
	if m.remoteAddr == "" {
		return nil
	}
	return &mockAddr{addr: m.remoteAddr}
}

func TestRequest_RemoteHostAndIP_Parsing(t *testing.T) {
	tests := []struct {
		id           string
		remoteAddr   string
		rawConnAddr  string
		useNilReq    bool
		expectedHost string
		expectedIP   string
		expectNilIP  bool
		description  string
	}{
		{
			id:           "1.1",
			remoteAddr:   "192.0.2.1:8080",
			expectedHost: "192.0.2.1",
			expectedIP:   "192.0.2.1",
			description:  "Standard IPv4 with port",
		},
		{
			id:           "1.2",
			remoteAddr:   "192.0.2.1",
			expectedHost: "192.0.2.1",
			expectedIP:   "192.0.2.1",
			description:  "Bare IPv4 without port",
		},
		{
			id:           "1.3",
			remoteAddr:   "[2001:db8::1]:8443",
			expectedHost: "2001:db8::1",
			expectedIP:   "2001:db8::1",
			description:  "Standard IPv6 with port and brackets",
		},
		{
			id:           "1.4",
			remoteAddr:   "2001:db8::1",
			expectedHost: "2001:db8::1",
			expectedIP:   "2001:db8::1",
			description:  "Bare IPv6 without brackets or port",
		},
		{
			id:           "1.5",
			remoteAddr:   "[2001:db8::1]",
			expectedHost: "2001:db8::1",
			expectedIP:   "2001:db8::1",
			description:  "Bracketed IPv6 without port (bracket stripping)",
		},
		{
			id:           "1.6",
			remoteAddr:   "  198.51.100.25:9000  ",
			expectedHost: "198.51.100.25",
			expectedIP:   "198.51.100.25",
			description:  "Surrounding whitespace handling",
		},
		{
			id:           "1.7",
			remoteAddr:   "",
			rawConnAddr:  "203.0.113.42:12345",
			expectedHost: "203.0.113.42",
			expectedIP:   "203.0.113.42",
			description:  "Fallback to RawConn.RemoteAddr() when RemoteAddr empty",
		},
		{
			id:           "1.8",
			remoteAddr:   "",
			rawConnAddr:  "[2001:db8:cafe::10]:5000",
			expectedHost: "2001:db8:cafe::10",
			expectedIP:   "2001:db8:cafe::10",
			description:  "Fallback to RawConn with IPv6",
		},
		{
			id:           "1.9",
			remoteAddr:   "",
			expectedHost: "",
			expectNilIP:  true,
			description:  "Completely empty / unpopulated request",
		},
		{
			id:           "1.10",
			remoteAddr:   "malformed:port:with:extra:colons",
			expectedHost: "malformed:port:with:extra:colons",
			expectNilIP:  true,
			description:  "Malformed address string (unparseable IP)",
		},
		{
			id:           "1.11",
			remoteAddr:   "invalid-host-name",
			expectedHost: "invalid-host-name",
			expectNilIP:  true,
			description:  "Non-IP string",
		},
		{
			id:           "1.12",
			useNilReq:    true,
			expectedHost: "",
			expectNilIP:  true,
			description:  "Nil receiver protection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.id+"_"+tt.description, func(t *testing.T) {
			var req *Request
			if !tt.useNilReq {
				req, _ = NewRequest("GET", "/test", "HTTP/1.1")
				req.RemoteAddr = tt.remoteAddr
				if tt.rawConnAddr != "" {
					req.RawConn = &mockConn{remoteAddr: tt.rawConnAddr}
				}
			}

			host := req.RemoteHost()
			if host != tt.expectedHost {
				t.Errorf("[%s] RemoteHost() = %q, want %q", tt.id, host, tt.expectedHost)
			}

			ip := req.RemoteIP()
			if tt.expectNilIP {
				if ip != nil {
					t.Errorf("[%s] RemoteIP() expected nil, got %v", tt.id, ip)
				}
			} else {
				expected := net.ParseIP(tt.expectedIP)
				if ip == nil || !ip.Equal(expected) {
					t.Errorf("[%s] RemoteIP() = %v, want %v", tt.id, ip, expected)
				}
			}
		})
	}
}

func TestRequest_NewRequestFromStd_RemoteAddr(t *testing.T) {
	stdReq, err := http.NewRequest("GET", "http://example.com/hello?foo=bar", nil)
	if err != nil {
		t.Fatalf("failed to create std request: %v", err)
	}
	stdReq.RemoteAddr = "192.0.2.10:44300"
	stdReq.Header.Set("X-Custom", "value")

	req := NewRequestFromStd(stdReq)
	if req == nil {
		t.Fatal("NewRequestFromStd returned nil")
	}

	if req.RemoteAddr != "192.0.2.10:44300" {
		t.Errorf("expected RemoteAddr %q, got %q", "192.0.2.10:44300", req.RemoteAddr)
	}
	if req.RemoteHost() != "192.0.2.10" {
		t.Errorf("expected RemoteHost %q, got %q", "192.0.2.10", req.RemoteHost())
	}
	if ip := req.RemoteIP(); ip == nil || ip.String() != "192.0.2.10" {
		t.Errorf("expected RemoteIP 192.0.2.10, got %v", ip)
	}
}
