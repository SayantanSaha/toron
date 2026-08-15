package waf

import (
	"errors"
	"net/url"
	"strings"

	"toron/pkg/httpparser"
)

var (
	ErrSmugglingConflict  = errors.New("conflicting Content-Length and Transfer-Encoding headers")
	ErrControlCharFound   = errors.New("non-printable control character in request")
	ErrHeaderValueTooLong = errors.New("header value exceeds maximum allowed byte limit")
	ErrQueryTooLong       = errors.New("query string exceeds maximum allowed byte limit")
	ErrParamTooLong       = errors.New("query parameter exceeds maximum allowed byte limit")
)

// ProtocolIntegrityConfig configures protocol strictness and payload limits.
type ProtocolIntegrityConfig struct {
	RejectSmuggling bool  `json:"reject_smuggling" yaml:"reject_smuggling"`
	RejectControl   bool  `json:"reject_control" yaml:"reject_control"`
	MaxHeaderValue  int   `json:"max_header_value" yaml:"max_header_value"` // Default 4 KB
	MaxQuerySize    int   `json:"max_query_size" yaml:"max_query_size"`     // Default 4 KB
	MaxParamSize    int   `json:"max_param_size" yaml:"max_param_size"`     // Default 2 KB
}

// DefaultProtocolConfig returns sensible default limits.
func DefaultProtocolConfig() ProtocolIntegrityConfig {
	return ProtocolIntegrityConfig{
		RejectSmuggling: true,
		RejectControl:   true,
		MaxHeaderValue:  4096, // 4 KB
		MaxQuerySize:    4096, // 4 KB
		MaxParamSize:    2048, // 2 KB
	}
}

// ValidateProtocolIntegrity inspects a Toron request for protocol violations.
func ValidateProtocolIntegrity(req *httpparser.Request, cfg ProtocolIntegrityConfig) error {
	if req == nil {
		return nil
	}

	if cfg.MaxHeaderValue <= 0 {
		cfg.MaxHeaderValue = 4096
	}
	if cfg.MaxQuerySize <= 0 {
		cfg.MaxQuerySize = 4096
	}
	if cfg.MaxParamSize <= 0 {
		cfg.MaxParamSize = 2048
	}

	// 1. Smuggling Prevention: Check conflicting headers
	if cfg.RejectSmuggling {
		hasCL := req.Header.Get("Content-Length") != ""
		hasTE := req.Header.Get("Transfer-Encoding") != ""
		if hasCL && hasTE {
			return ErrSmugglingConflict
		}
		// Mismatched / Multiple Content-Length values check
		if clVals, exists := req.Header["content-length"]; exists && len(clVals) > 1 {
			for i := 1; i < len(clVals); i++ {
				if strings.TrimSpace(clVals[i]) != strings.TrimSpace(clVals[0]) {
					return ErrSmugglingConflict
				}
			}
		}
	}

	// 2. Control Character Guard (ASCII 0x00-0x1F except \t, and 0x7F DEL)
	if cfg.RejectControl {
		if hasControlChar(req.Path) || hasControlChar(req.RequestURI) {
			return ErrControlCharFound
		}
		for k, vals := range req.Header {
			if hasControlChar(k) {
				return ErrControlCharFound
			}
			for _, v := range vals {
				if hasControlChar(v) {
					return ErrControlCharFound
				}
			}
		}
	}

	// 3. Header Value Size Bounding
	for _, vals := range req.Header {
		for _, v := range vals {
			if len(v) > cfg.MaxHeaderValue {
				return ErrHeaderValueTooLong
			}
		}
	}

	// 4. Query String & Parameter Size Bounding
	rawQuery := ""
	if req.URL != nil {
		rawQuery = req.URL.RawQuery
	} else if req.RequestURI != "" {
		if u, err := url.ParseRequestURI(req.RequestURI); err == nil {
			rawQuery = u.RawQuery
		}
	}

	if rawQuery != "" {
		if len(rawQuery) > cfg.MaxQuerySize {
			return ErrQueryTooLong
		}
		if hasControlChar(rawQuery) && cfg.RejectControl {
			return ErrControlCharFound
		}

		params := strings.Split(rawQuery, "&")
		for _, p := range params {
			if len(p) > cfg.MaxParamSize {
				return ErrParamTooLong
			}
		}
	}

	return nil
}

func hasControlChar(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if (b < 0x20 && b != '\t') || b == 0x7F {
			return true
		}
	}
	return false
}
