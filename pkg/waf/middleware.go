package waf

import (
	"bytes"
	"strings"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/metrics"
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
				pTrimmed := strings.TrimSpace(p)
				if pTrimmed == "" {
					continue
				}
				if pTrimmed == "/" {
					if req.Path == "/" {
						next(req, res)
						return
					}
					continue
				}
				if req.Path == pTrimmed || strings.HasPrefix(req.Path, strings.TrimSuffix(pTrimmed, "/")+"/") {
					next(req, res)
					return
				}
			}

			start := time.Now()
			defer func() {
				metrics.DefaultRegistry.RecordWAFInspectionDuration(time.Since(start).Seconds())
			}()

			var tp []string
			if engine != nil {
				tp = engine.Config().TrustedProxies
			}

			clientNetIP := ExtractClientIP(req, tp)
			clientIP := ""
			if clientNetIP != nil {
				clientIP = clientNetIP.String()
			}

			// 0. Fast-Path Auto-Ban Check
			if autoBan := engine.AutoBanManager(); autoBan != nil && autoBan.IsEnabled() && clientIP != "" {
				if banned, banEntry := autoBan.IsBanned(clientIP); banned {
					metrics.DefaultRegistry.RecordWAFBlocked("auto_ban", req.Path)
					if logger := engine.AuditLogger(); logger != nil {
						banType := "temporary"
						reason := ""
						if banEntry != nil {
							banType = string(banEntry.Type)
							reason = banEntry.Reason
						}
						logger.LogEvent(SecurityEvent{
							Event:          "auto_ban_drop",
							ClientIP:       clientIP,
							Method:         req.Method,
							Path:           req.Path,
							Category:       "auto_ban",
							AnomalyScore:   0,
							Action:         "blocked",
							Location:       "remote_addr",
							PayloadSnippet: "Client IP banned (" + banType + "): " + reason,
						})
					}

					if res.Body == nil {
						res.Body = bytes.NewBuffer(nil)
					}
					res.Body.Reset()
					res.Header.Set("Content-Type", "application/json")
					res.Header.Set("Connection", "close")
					res.SetStatus(403)
					banType := "temporary"
					if banEntry != nil {
						banType = string(banEntry.Type)
					}
					_, _ = res.WriteString(`{"error":"Forbidden","message":"Client IP address has been banned due to repeated security violations","ban_type":"` + banType + `"}`)
					return
				}
			}

			// 0b. Fast-Path CIDR IP Access Control Check
			if acl := engine.IPAccessList(); acl != nil && acl.HasRules() {
				allowed, reason := acl.CheckIP(clientNetIP)
				if !allowed {
					metrics.DefaultRegistry.RecordWAFBlocked("ip_acl", req.Path)
					if logger := engine.AuditLogger(); logger != nil {
						logger.LogEvent(SecurityEvent{
							Event:        "ip_acl_block",
							ClientIP:     clientIP,
							Method:       req.Method,
							Path:         req.Path,
							Category:     "ip_acl",
							AnomalyScore: 0,
							Action:       "blocked",
							Location:     "remote_addr",
						})
					}

					if autoBan := engine.AutoBanManager(); autoBan != nil && autoBan.IsEnabled() && clientIP != "" {
						autoBan.RecordViolation(clientIP, "ip_acl")
					}

					if res.Body == nil {
						res.Body = bytes.NewBuffer(nil)
					}
					res.Body.Reset()
					res.Header.Set("Content-Type", "application/json")
					res.Header.Set("Connection", "close")
					res.SetStatus(403)
					_, _ = res.WriteString(`{"error":"Forbidden","message":"` + reason + `"}`)
					return
				}
			}

			// 1. Protocol Integrity & Request Smuggling Guard
			if protoErr := ValidateProtocolIntegrity(req, protocolCfg); protoErr != nil {
				metrics.DefaultRegistry.RecordWAFBlocked("protocol", req.Path)
				if logger := engine.AuditLogger(); logger != nil {
					logger.LogEvent(SecurityEvent{
						Event:          "protocol_violation",
						ClientIP:       clientIP,
						Method:         req.Method,
						Path:           req.Path,
						Category:       "protocol",
						AnomalyScore:   0,
						Action:         "blocked",
						PayloadSnippet: protoErr.Error(),
					})
				}

				if autoBan := engine.AutoBanManager(); autoBan != nil && autoBan.IsEnabled() && clientIP != "" {
					autoBan.RecordViolation(clientIP, "protocol")
				}

				if res.Body == nil {
					res.Body = bytes.NewBuffer(nil)
				}
				res.Body.Reset()
				res.Header.Set("Content-Type", "application/json")
				res.Header.Set("Connection", "close")

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
				cat := "waf"
				for _, r := range matched {
					cat = string(r.Category)
					metrics.DefaultRegistry.RecordWAFBlocked(string(r.Category), req.Path)
					if logger := engine.AuditLogger(); logger != nil {
						snippet := ""
						if req.URL != nil && req.URL.RawQuery != "" {
							snippet = req.URL.RawQuery
						}
						logger.LogEvent(SecurityEvent{
							Event:          "waf_block",
							ClientIP:       clientIP,
							Method:         req.Method,
							Path:           req.Path,
							Category:       string(r.Category),
							RuleID:         r.ID,
							AnomalyScore:   score,
							Action:         "blocked",
							Location:       locationString(r.Locations),
							PayloadSnippet: snippet,
						})
					}
				}

				if autoBan := engine.AutoBanManager(); autoBan != nil && autoBan.IsEnabled() && clientIP != "" {
					autoBan.RecordViolation(clientIP, cat)
				}

				res.SetStatus(403)
				res.Header.Set("Content-Type", "application/json")
				res.Header.Set("Connection", "close")

				body := FormatBlockedResponse(score, matched)
				if res.Body == nil {
					res.Body = bytes.NewBuffer(nil)
				}
				res.Body.Reset()
				_, _ = res.WriteString(body)
				return
			}

			// Detection mode recording
			if score > 0 && !blocked {
				for _, r := range matched {
					metrics.DefaultRegistry.RecordWAFAnomaly(string(r.Category), "detection")
					if logger := engine.AuditLogger(); logger != nil {
						snippet := ""
						if req.URL != nil && req.URL.RawQuery != "" {
							snippet = req.URL.RawQuery
						}
						logger.LogEvent(SecurityEvent{
							Event:          "waf_detection",
							ClientIP:       clientIP,
							Method:         req.Method,
							Path:           req.Path,
							Category:       string(r.Category),
							RuleID:         r.ID,
							AnomalyScore:   score,
							Action:         "logged",
							Location:       locationString(r.Locations),
							PayloadSnippet: snippet,
						})
					}
				}
			}

			next(req, res)
		}
	}
}

func locationString(loc InspectLocation) string {
	var parts []string
	if loc&InspectURL != 0 {
		parts = append(parts, "url")
	}
	if loc&InspectQuery != 0 {
		parts = append(parts, "query")
	}
	if loc&InspectHeaders != 0 {
		parts = append(parts, "headers")
	}
	if loc&InspectBody != 0 {
		parts = append(parts, "body")
	}
	return strings.Join(parts, ",")
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
