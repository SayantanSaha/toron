package waf

import (
	"bytes"
	"strings"

	"toron/pkg/httpparser"
)

// HandlerFunc describes an HTTP request handler function.
type HandlerFunc func(req *httpparser.Request, res *httpparser.Response)

// MiddlewareFunc describes middleware wrapping a HandlerFunc.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// NewWAFMiddleware constructs router middleware using a WAFEngine.
func NewWAFMiddleware(engine *WAFEngine) MiddlewareFunc {
	protocolCfg := DefaultProtocolConfig()

	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			if engine == nil || !engine.Config().Enabled {
				next(req, res)
				return
			}

			// Check path exclusions (e.g. routes with dedicated route-level WAF policies)
			for _, p := range engine.Config().Excluded {
				if req.Path == p || strings.HasPrefix(req.Path, strings.TrimSuffix(p, "/")+"/") {
					next(req, res)
					return
				}
			}

			// 0. Fast-Path CIDR IP Access Control Check
			if acl := engine.IPAccessList(); acl != nil && acl.HasRules() {
				clientIP := ExtractClientIP(req)
				if clientIP != nil {
					allowed, reason := acl.CheckIP(clientIP)
					if !allowed {
						if res.Body == nil {
							res.Body = bytes.NewBuffer(nil)
						}
						res.Body.Reset()
						res.Header.Set("Content-Type", "application/json")
						res.SetStatus(403)
						_, _ = res.WriteString(`{"error":"Forbidden","message":"` + reason + `"}`)
						return
					}
				}
			}

			// 1. Protocol Integrity & Request Smuggling Guard
			if protoErr := ValidateProtocolIntegrity(req, protocolCfg); protoErr != nil {
				if res.Body == nil {
					res.Body = bytes.NewBuffer(nil)
				}
				res.Body.Reset()
				res.Header.Set("Content-Type", "application/json")

				if protoErr == ErrHeaderValueTooLong || protoErr == ErrQueryTooLong || protoErr == ErrParamTooLong {
					res.SetStatus(413)
					_, _ = res.WriteString(`{"error":"Payload Too Large","message":"` + protoErr.Error() + `"}`)
				} else {
					res.SetStatus(400)
					_, _ = res.WriteString(`{"error":"Bad Request","message":"` + protoErr.Error() + `"}`)
				}
				return
			}

			// 2. Layer 7 OWASP Threat Rules Inspection
			blocked, score, matched, _ := engine.InspectToron(req)

			if score > 0 {
				res.Header.Set("X-Toron-WAF-Anomaly-Score", intToString(score))
			}

			if blocked {
				res.SetStatus(403)
				res.Header.Set("Content-Type", "application/json")

				body := FormatBlockedResponse(score, matched)
				if res.Body == nil {
					res.Body = bytes.NewBuffer(nil)
				}
				res.Body.Reset()
				_, _ = res.WriteString(body)
				return
			}

			next(req, res)
		}
	}
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
