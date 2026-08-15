package waf

import (
	"bytes"

	"toron/pkg/httpparser"
	"toron/pkg/router"
)

// NewWAFMiddleware constructs router middleware using a WAFEngine.
func NewWAFMiddleware(engine *WAFEngine) router.MiddlewareFunc {
	protocolCfg := DefaultProtocolConfig()

	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			if engine == nil || !engine.Config().Enabled {
				next(req, res)
				return
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
