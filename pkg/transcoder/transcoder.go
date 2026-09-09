package transcoder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"toron/pkg/config"
	"toron/pkg/httpparser"
	"toron/pkg/router"
)

// Engine manages REST-to-gRPC transcoding routes and HTTP/2 proxy forwarding.
type Engine struct {
	mu           sync.RWMutex
	cfg          config.TranscoderConfig
	maxBodyBytes int64
	router       *router.Router
	rules        []TranscodeRule
	httpClient   *http.Client
}

// NewEngine constructs a REST-to-gRPC Transcoder Engine instance.
func NewEngine(cfg config.TranscoderConfig, r *router.Router) (*Engine, error) {
	var rules []TranscodeRule
	for _, route := range cfg.Routes {
		rules = append(rules, TranscodeRule{
			HTTPMethod:    strings.ToUpper(strings.TrimSpace(route.HTTPMethod)),
			HTTPPath:      route.HTTPPath,
			GRPCMethod:    route.GRPCMethod,
			UpstreamURL:   route.UpstreamURL,
			FieldMappings: route.FieldMappings,
		})
	}

	httpClient := &http.Client{
		Timeout: 15 * time.Second,
	}

	maxBody := cfg.GetMaxBodyBytes()
	if maxBody <= 0 {
		maxBody = 4 * 1024 * 1024
	}

	e := &Engine{
		cfg:          cfg,
		maxBodyBytes: maxBody,
		router:       r,
		rules:        rules,
		httpClient:   httpClient,
	}

	e.registerRoutes()
	return e, nil
}

func (e *Engine) registerRoutes() {
	if e.router == nil {
		return
	}

	for _, rule := range e.rules {
		rRule := rule
		handler := func(req *httpparser.Request, res *httpparser.Response) {
			e.HandleTranscode(req, res, rRule)
		}

		if strings.Contains(rRule.HTTPPath, "/:") {
			cleanPrefix := rRule.HTTPPath
			if idx := strings.Index(cleanPrefix, "/:"); idx != -1 {
				cleanPrefix = cleanPrefix[:idx]
			}
			if cleanPrefix == "" {
				cleanPrefix = "/"
			}

			log.Printf("[TRANSCODER] Registered REST-to-gRPC parameterized route: %s %s (prefix: %s) -> %s %s",
				rRule.HTTPMethod, rRule.HTTPPath, cleanPrefix, rRule.UpstreamURL, rRule.GRPCMethod)

			matcher := func(p string) bool {
				return MatchPathPattern(rRule.HTTPPath, p)
			}

			e.router.HandlePrefixWithMatcher(rRule.HTTPMethod, "", cleanPrefix, nil, matcher, handler)
		} else {
			log.Printf("[TRANSCODER] Registered REST-to-gRPC exact route: %s %s -> %s %s",
				rRule.HTTPMethod, rRule.HTTPPath, rRule.UpstreamURL, rRule.GRPCMethod)

			e.router.Handle(rRule.HTTPMethod, rRule.HTTPPath, handler)
		}
	}
}

// transcoderHopByHopHeaders defines standard connection-specific headers that MUST NOT
// be forwarded to upstream HTTP/2 gRPC services per RFC 7230 §6.1, RFC 7540 §8.1.2.2, and RFC 9113 §8.2.2.
var transcoderHopByHopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"trailers":            true,
	"transfer-encoding":   true,
	"upgrade":             true,
	"proxy-connection":    true,
	"host":                true,
}

// HandleTranscode processes a REST request, translates parameters into a gRPC wire frame, forwards to gRPC backend, and formats JSON response.
func (e *Engine) HandleTranscode(req *httpparser.Request, res *httpparser.Response, rule TranscodeRule) {
	maxBody := e.maxBodyBytes
	if maxBody <= 0 {
		maxBody = 4 * 1024 * 1024
	}

	payloadMap := make(map[string]interface{})

	// 1. Parse JSON body if present
	if req.Body != nil {
		// Tier 1: Fast-fail on declared Content-Length
		contentLength := req.ContentLength
		if contentLength <= 0 && req.Header != nil {
			if clStr := req.Header.Get("Content-Length"); clStr != "" {
				if clVal, parseErr := strconv.ParseInt(strings.TrimSpace(clStr), 10, 64); parseErr == nil {
					contentLength = clVal
				}
			}
		}

		if contentLength > maxBody && contentLength > 0 {
			res.SetStatus(http.StatusRequestEntityTooLarge)
			res.Header.Set("Content-Type", "application/json")
			_, _ = res.WriteString(fmt.Sprintf(`{"error":"Payload Too Large: request Content-Length %d exceeds limit of %d bytes"}`, contentLength, maxBody))
			if closer, ok := req.Body.(io.Closer); ok {
				_ = closer.Close()
			}
			return
		}

		// Tier 2: Bounded stream read via LimitReader
		bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, maxBody+1))
		if err != nil {
			res.SetStatus(http.StatusBadRequest)
			res.Header.Set("Content-Type", "application/json")
			_, _ = res.WriteString(fmt.Sprintf(`{"error":"Bad Request: failed to read request body: %v"}`, err))
			return
		}

		if int64(len(bodyBytes)) > maxBody {
			res.SetStatus(http.StatusRequestEntityTooLarge)
			res.Header.Set("Content-Type", "application/json")
			_, _ = res.WriteString(fmt.Sprintf(`{"error":"Payload Too Large: request body exceeds limit of %d bytes"}`, maxBody))
			if closer, ok := req.Body.(io.Closer); ok {
				_ = closer.Close()
			}
			return
		}

		if len(bodyBytes) > 0 {
			var bodyMap map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &bodyMap); err == nil {
				for k, v := range bodyMap {
					payloadMap[k] = v
				}
			}
		}
	}

	// 2. Parse Query Parameters
	if req.QueryParams != nil {
		for k, vv := range req.QueryParams {
			if len(vv) > 0 {
				payloadMap[k] = vv[0]
			}
		}
	}

	// 3. Extract Path Parameters (e.g. /v1/users/:id)
	pathParams := extractPathParams(rule.HTTPPath, req.Path)
	for k, v := range pathParams {
		payloadMap[k] = v
	}

	// Apply optional custom field mappings
	for jsonKey, targetKey := range rule.FieldMappings {
		if val, exists := payloadMap[jsonKey]; exists {
			payloadMap[targetKey] = val
			delete(payloadMap, jsonKey)
		}
	}

	jsonBytes, err := json.Marshal(payloadMap)
	if err != nil {
		res.SetStatus(http.StatusBadRequest)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(fmt.Sprintf(`{"error":"Failed to encode JSON request: %v"}`, err))
		return
	}

	// Construct gRPC 5-byte wire frame
	grpcPayload := EncodeGRPCFrame(jsonBytes)

	grpcTargetURL := strings.TrimSuffix(rule.UpstreamURL, "/") + "/" + strings.TrimPrefix(rule.GRPCMethod, "/")

	grpcReq, err := http.NewRequestWithContext(context.Background(), "POST", grpcTargetURL, bytes.NewReader(grpcPayload))
	if err != nil {
		res.SetStatus(http.StatusInternalServerError)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(fmt.Sprintf(`{"error":"Failed to create gRPC request: %v"}`, err))
		return
	}

	// Build custom hop-by-hop tokens from Connection header (RFC 7230 §6.1 / RFC 9110 §7.6.1)
	customHopByHop := make(map[string]bool)
	if req.Header != nil {
		if connHdr := req.Header.Get("Connection"); connHdr != "" {
			for _, tok := range strings.Split(connHdr, ",") {
				tok = strings.ToLower(strings.TrimSpace(tok))
				if tok != "" {
					customHopByHop[tok] = true
				}
			}
		}
	}

	// Forward client headers, stripping RFC 7230 / RFC 7540 hop-by-hop and content-* headers
	if req.Header != nil {
		for k, vv := range req.Header {
			lowerKey := strings.ToLower(k)
			if transcoderHopByHopHeaders[lowerKey] || customHopByHop[lowerKey] || strings.HasPrefix(lowerKey, "content-") {
				continue
			}
			for _, v := range vv {
				grpcReq.Header.Add(k, v)
			}
		}
	}

	// Canonical gRPC wire headers (RFC 7540 §8.1.2.2 strictly requires TE: trailers only)
	grpcReq.Header.Set("Content-Type", "application/grpc")
	grpcReq.Header.Set("TE", "trailers")

	resp, err := e.httpClient.Do(grpcReq)
	if err != nil {
		res.SetStatus(http.StatusBadGateway)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(fmt.Sprintf(`{"error":"gRPC upstream communication failed: %v"}`, err))
		return
	}
	defer resp.Body.Close()

	// Extract gRPC status from trailers or headers
	grpcStatus := resp.Header.Get("grpc-status")
	if grpcStatus == "" {
		grpcStatus = resp.Trailer.Get("grpc-status")
	}
	if grpcStatus == "" {
		grpcStatus = "0" // Default OK if omitted
	}

	httpStatus := MapGRPCStatusToHTTP(grpcStatus)
	res.SetStatus(httpStatus)
	res.Header.Set("Content-Type", "application/json")
	res.Header.Set("X-gRPC-Status", grpcStatus)

	if httpStatus != http.StatusOK {
		grpcMsg := resp.Header.Get("grpc-message")
		if grpcMsg == "" {
			grpcMsg = resp.Trailer.Get("grpc-message")
		}
		_, _ = res.WriteString(fmt.Sprintf(`{"error":"gRPC error status %s","grpc_message":%q}`, grpcStatus, grpcMsg))
		return
	}

	// Decode returning gRPC wire frame payload
	outBytes, err := DecodeGRPCFrame(resp.Body)
	if err != nil {
		if errors.Is(err, ErrFrameTooLarge) {
			res.SetStatus(http.StatusBadGateway)
			res.Header.Set("Content-Type", "application/json")
			_, _ = res.WriteString(fmt.Sprintf(`{"error":"502 Bad Gateway","message":%q}`, err.Error()))
			return
		}
		// Fallback raw output
		_, _ = res.WriteString(`{}`)
		return
	}
	if len(outBytes) == 0 {
		_, _ = res.WriteString(`{}`)
		return
	}

	// Output clean JSON
	var jsonOut map[string]interface{}
	if err := json.Unmarshal(outBytes, &jsonOut); err == nil {
		prettyJSON, _ := json.Marshal(jsonOut)
		_, _ = res.Write(prettyJSON)
	} else {
		_, _ = res.Write(outBytes)
	}
}

func extractPathParams(pattern, path string) map[string]string {
	params := make(map[string]string)
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	if len(patternParts) != len(pathParts) {
		return params
	}

	for i, part := range patternParts {
		if strings.HasPrefix(part, ":") {
			paramName := strings.TrimPrefix(part, ":")
			val, _ := url.PathUnescape(pathParts[i])
			params[paramName] = val
		}
	}
	return params
}

// MatchPathPattern checks if an incoming URL path matches a parameterized route pattern (e.g. /v1/users/:id).
// Literal segments must match verbatim, and wildcard parameter segments (prefixed with ':') match any non-empty segment.
func MatchPathPattern(pattern, path string) bool {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	if len(patternParts) != len(pathParts) {
		return false
	}

	for i, part := range patternParts {
		if strings.HasPrefix(part, ":") {
			if pathParts[i] == "" {
				return false
			}
			continue
		}
		if part != pathParts[i] {
			return false
		}
	}
	return true
}
