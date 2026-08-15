package waf

import (
	"bytes"
	"strconv"
	"strings"

	"toron/pkg/httpparser"
	"toron/pkg/router"
)

// NewWAFMiddleware constructs router middleware using a WAFEngine.
func NewWAFMiddleware(engine *WAFEngine) router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			if engine == nil || !engine.Config().Enabled {
				next(req, res)
				return
			}

			blocked, score, matched, _ := engine.InspectToron(req)

			if score > 0 {
				res.Header.Set("X-Toron-WAF-Anomaly-Score", strconv.Itoa(score))
			}

			if blocked {
				res.SetStatus(403)
				res.Header.Set("Content-Type", "application/json")

				var ruleIDs []string
				for _, r := range matched {
					ruleIDs = append(ruleIDs, r.ID)
				}
				body := FormatBlockedResponse(score, matched)
				if res.Body == nil {
					res.Body = bytes.NewBuffer(nil)
				}
				res.Body.Reset()
				_, _ = res.WriteString(body)
				_ = strings.Join(ruleIDs, ",")
				return
			}

			next(req, res)
		}
	}
}
