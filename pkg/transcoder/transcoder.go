package transcoder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"toron/pkg/config"
	"toron/pkg/httpparser"
	"toron/pkg/proxy"
	"toron/pkg/router"
)

// Engine manages REST-to-gRPC transcoding routes and HTTP/2 proxy forwarding.
type Engine struct {
	mu         sync.RWMutex
	cfg        config.TranscoderConfig
	router     *router.Router
	rules      []TranscodeRule
	httpClient *http.Client
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

	e := &Engine{
		cfg:        cfg,
		router:     r,
		rules:      rules,
		httpClient: httpClient,
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

		cleanPrefix := rRule.HTTPPath
		if idx := strings.Index(cleanPrefix, "/:"); idx != -1 {
			cleanPrefix = cleanPrefix[:idx]
		}

		if cleanPrefix == "" {
			cleanPrefix = "/"
		}

		log.Printf("[TRANSCODER] Registered REST-to-gRPC route: %s %s -> %s %s",
			rRule.HTTPMethod, rRule.HTTPPath, rRule.UpstreamURL, rRule.GRPCMethod)

		_ = e.router.RoutePrefix("upstream", "", cleanPrefix, nil, "", proxy.ProxyOptions{})
		e.router.Handle(rRule.HTTPMethod, cleanPrefix, handler)
	}
}

// HandleTranscode processes a REST request, translates parameters into a gRPC wire frame, forwards to gRPC backend, and formats JSON response.
func (e *Engine) HandleTranscode(req *httpparser.Request, res *httpparser.Response, rule TranscodeRule) {
	payloadMap := make(map[string]interface{})

	// 1. Parse JSON body if present
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil && len(bodyBytes) > 0 {
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

	grpcReq.Header.Set("Content-Type", "application/grpc")
	grpcReq.Header.Set("TE", "trailers")

	// Forward client headers
	if req.Header != nil {
		for k, vv := range req.Header {
			if !strings.HasPrefix(strings.ToLower(k), "content-") {
				for _, v := range vv {
					grpcReq.Header.Add(k, v)
				}
			}
		}
	}

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
	if err != nil || len(outBytes) == 0 {
		// Fallback raw output
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
