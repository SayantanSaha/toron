package router

import "toron/pkg/httpparser"

// SecurityHeadersConfig defines HTTP security headers policies.
type SecurityHeadersConfig struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	HSTS               string `yaml:"hsts" json:"hsts"`
	ContentTypeOptions string `yaml:"content_type_options" json:"content_type_options"`
	FrameOptions       string `yaml:"frame_options" json:"frame_options"`
	ReferrerPolicy     string `yaml:"referrer_policy" json:"referrer_policy"`
	CSP                string `yaml:"csp" json:"csp"`
	PermissionsPolicy  string `yaml:"permissions_policy" json:"permissions_policy"`
}

// DefaultSecurityHeadersConfig returns OWASP recommended baseline security headers.
func DefaultSecurityHeadersConfig() SecurityHeadersConfig {
	return SecurityHeadersConfig{
		Enabled:            true,
		HSTS:               "max-age=31536000; includeSubDomains",
		ContentTypeOptions: "nosniff",
		FrameOptions:       "DENY",
		ReferrerPolicy:     "strict-origin-when-cross-origin",
		CSP:                "default-src 'self'",
		PermissionsPolicy:  "camera=(), microphone=(), geolocation=()",
	}
}

// NewSecurityHeadersMiddleware appends configured enterprise security headers to outgoing responses.
func NewSecurityHeadersMiddleware(cfg SecurityHeadersConfig) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			if cfg.HSTS != "" {
				res.Header.Set("Strict-Transport-Security", cfg.HSTS)
			}
			if cfg.ContentTypeOptions != "" {
				res.Header.Set("X-Content-Type-Options", cfg.ContentTypeOptions)
			}
			if cfg.FrameOptions != "" {
				res.Header.Set("X-Frame-Options", cfg.FrameOptions)
			}
			if cfg.ReferrerPolicy != "" {
				res.Header.Set("Referrer-Policy", cfg.ReferrerPolicy)
			}
			if cfg.CSP != "" {
				res.Header.Set("Content-Security-Policy", cfg.CSP)
			}
			if cfg.PermissionsPolicy != "" {
				res.Header.Set("Permissions-Policy", cfg.PermissionsPolicy)
			}

			next(req, res)
		}
	}
}
