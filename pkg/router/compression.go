package router

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"toron/pkg/httpparser"
)

// CompressionConfig defines options for the transparent response compression middleware.
type CompressionConfig struct {
	Enabled   bool     `yaml:"enabled" json:"enabled"`
	MinLength int      `yaml:"min_length" json:"min_length"`
	Level     int      `yaml:"level" json:"level"`
	Encodings []string `yaml:"encodings" json:"encodings"`
	Types     []string `yaml:"types" json:"types"`
}

// DefaultCompressionConfig returns recommended production defaults for compression.
func DefaultCompressionConfig() CompressionConfig {
	return CompressionConfig{
		Enabled:   true,
		MinLength: 512,
		Level:     gzip.DefaultCompression,
		Encodings: []string{"zstd", "br", "gzip", "deflate"},
		Types: []string{
			"text/",
			"application/json",
			"application/javascript",
			"application/xml",
			"application/xhtml+xml",
			"image/svg+xml",
		},
	}
}

type compressionManager struct {
	config     CompressionConfig
	zstdPool   sync.Pool
	brotliPool sync.Pool
	gzipPool   sync.Pool
	flatePool  sync.Pool
}

func newCompressionManager(cfg CompressionConfig) *compressionManager {
	if cfg.MinLength <= 0 {
		cfg.MinLength = 512
	}
	if len(cfg.Encodings) == 0 {
		cfg.Encodings = []string{"zstd", "br", "gzip", "deflate"}
	}
	if len(cfg.Types) == 0 {
		cfg.Types = DefaultCompressionConfig().Types
	}

	cm := &compressionManager{
		config: cfg,
	}

	cm.zstdPool = sync.Pool{
		New: func() any {
			level := zstd.SpeedDefault
			if cfg.Level == 1 {
				level = zstd.SpeedFastest
			} else if cfg.Level == 9 {
				level = zstd.SpeedBestCompression
			}
			enc, _ := zstd.NewWriter(io.Discard, zstd.WithEncoderLevel(level))
			return enc
		},
	}

	cm.brotliPool = sync.Pool{
		New: func() any {
			level := cfg.Level
			if level < brotli.BestSpeed || level > brotli.BestCompression {
				level = brotli.DefaultCompression
			}
			return brotli.NewWriterLevel(io.Discard, level)
		},
	}

	cm.gzipPool = sync.Pool{
		New: func() any {
			level := cfg.Level
			if level < gzip.HuffmanOnly || level > gzip.BestCompression {
				level = gzip.DefaultCompression
			}
			gw, _ := gzip.NewWriterLevel(io.Discard, level)
			return gw
		},
	}

	cm.flatePool = sync.Pool{
		New: func() any {
			level := cfg.Level
			if level < flate.HuffmanOnly || level > flate.BestCompression {
				level = flate.DefaultCompression
			}
			fw, _ := flate.NewWriter(io.Discard, level)
			return fw
		},
	}

	return cm
}

// NewCompressionMiddleware creates a middleware that automatically compresses HTTP responses
// based on the client's Accept-Encoding header using zstd, brotli, gzip, or deflate.
func NewCompressionMiddleware(cfg CompressionConfig) MiddlewareFunc {
	cm := newCompressionManager(cfg)

	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			next(req, res)

			if !cm.config.Enabled {
				return
			}

			// WebSocket upgrades or raw hijacked connections must not be compressed
			if res.StatusCode == http.StatusSwitchingProtocols || res.UpgradedConn != nil {
				return
			}

			// 204 No Content and 304 Not Modified have no body
			if res.StatusCode == http.StatusNoContent || res.StatusCode == http.StatusNotModified {
				return
			}

			// Do not compress responses below the minimum length threshold
			if res.Body.Len() < cm.config.MinLength {
				return
			}

			// If already compressed, pass through
			if res.Header.Get("Content-Encoding") != "" {
				return
			}

			// Check MIME type compressibility
			contentType := res.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "text/plain"
			}
			if !isMIMECompressible(contentType, cm.config.Types) {
				return
			}

			// Negotiate encoding
			acceptEncoding := req.Header.Get("Accept-Encoding")
			selected := selectCompressionEncoding(acceptEncoding, cm.config.Encodings)
			if selected == "" {
				return
			}

			// Always add/append Vary: Accept-Encoding
			vary := res.Header.Get("Vary")
			if vary == "" {
				res.Header.Set("Vary", "Accept-Encoding")
			} else if !strings.Contains(strings.ToLower(vary), "accept-encoding") {
				res.Header.Set("Vary", vary+", Accept-Encoding")
			}

			rawBytes := res.Body.Bytes()
			var compressedBuf bytes.Buffer

			switch selected {
			case "zstd":
				zw := cm.zstdPool.Get().(*zstd.Encoder)
				zw.Reset(&compressedBuf)
				if _, err := zw.Write(rawBytes); err != nil {
					cm.zstdPool.Put(zw)
					return
				}
				if err := zw.Close(); err != nil {
					cm.zstdPool.Put(zw)
					return
				}
				cm.zstdPool.Put(zw)
				res.Header.Set("Content-Encoding", "zstd")

			case "br":
				bw := cm.brotliPool.Get().(*brotli.Writer)
				bw.Reset(&compressedBuf)
				if _, err := bw.Write(rawBytes); err != nil {
					cm.brotliPool.Put(bw)
					return
				}
				if err := bw.Close(); err != nil {
					cm.brotliPool.Put(bw)
					return
				}
				cm.brotliPool.Put(bw)
				res.Header.Set("Content-Encoding", "br")

			case "gzip":
				gw := cm.gzipPool.Get().(*gzip.Writer)
				gw.Reset(&compressedBuf)
				if _, err := gw.Write(rawBytes); err != nil {
					cm.gzipPool.Put(gw)
					return
				}
				if err := gw.Close(); err != nil {
					cm.gzipPool.Put(gw)
					return
				}
				cm.gzipPool.Put(gw)
				res.Header.Set("Content-Encoding", "gzip")

			case "deflate":
				fw := cm.flatePool.Get().(*flate.Writer)
				fw.Reset(&compressedBuf)
				if _, err := fw.Write(rawBytes); err != nil {
					cm.flatePool.Put(fw)
					return
				}
				if err := fw.Close(); err != nil {
					cm.flatePool.Put(fw)
					return
				}
				cm.flatePool.Put(fw)
				res.Header.Set("Content-Encoding", "deflate")

			default:
				return
			}

			res.Body.Reset()
			_, _ = res.Body.Write(compressedBuf.Bytes())
			res.Header.Set("Content-Length", strconv.Itoa(res.Body.Len()))
		}
	}
}

func getEncodingServerRank(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "zstd":
		return 4
	case "br":
		return 3
	case "gzip":
		return 2
	case "deflate":
		return 1
	default:
		return 0
	}
}

func isSupportedEncoding(name string, supported []string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, s := range supported {
		if strings.ToLower(strings.TrimSpace(s)) == n {
			return true
		}
	}
	return false
}

func selectCompressionEncoding(acceptEncoding string, supported []string) string {
	if strings.TrimSpace(acceptEncoding) == "" {
		return ""
	}

	clauses := strings.Split(acceptEncoding, ",")
	type clientOption struct {
		name string
		q    float64
		rank int
	}

	options := make([]clientOption, 0, len(clauses))

	for _, clause := range clauses {
		parts := strings.Split(strings.TrimSpace(clause), ";")
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		if name == "" {
			continue
		}

		q := 1.0
		for _, param := range parts[1:] {
			param = strings.TrimSpace(param)
			if strings.HasPrefix(param, "q=") {
				if parsedQ, err := strconv.ParseFloat(strings.TrimPrefix(param, "q="), 64); err == nil {
					q = parsedQ
				}
			}
		}

		if q <= 0 {
			continue
		}

		if name == "*" {
			// Expand wildcard to supported encodings
			for _, s := range []string{"zstd", "br", "gzip", "deflate"} {
				if isSupportedEncoding(s, supported) {
					options = append(options, clientOption{
						name: s,
						q:    q,
						rank: getEncodingServerRank(s),
					})
				}
			}
		} else if isSupportedEncoding(name, supported) {
			options = append(options, clientOption{
				name: name,
				q:    q,
				rank: getEncodingServerRank(name),
			})
		}
	}

	if len(options) == 0 {
		return ""
	}

	// Pick highest quality factor; if equal, highest server rank
	best := options[0]
	for _, opt := range options[1:] {
		if opt.q > best.q {
			best = opt
		} else if opt.q == best.q && opt.rank > best.rank {
			best = opt
		}
	}

	return best.name
}

func isMIMECompressible(contentType string, allowedTypes []string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	for _, pattern := range allowedTypes {
		p := strings.ToLower(strings.TrimSpace(pattern))
		if strings.HasPrefix(ct, p) || strings.Contains(ct, p) {
			return true
		}
	}
	return false
}
