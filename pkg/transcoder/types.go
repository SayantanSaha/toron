package transcoder

import (
	"net/http"
	"strconv"
	"strings"
)

// TranscodeRule defines a REST HTTP path to gRPC method mapping.
type TranscodeRule struct {
	HTTPMethod    string
	HTTPPath      string
	GRPCMethod    string
	UpstreamURL   string
	FieldMappings map[string]string
}

// MapGRPCStatusToHTTP converts a gRPC status code string or integer to an HTTP status code.
func MapGRPCStatusToHTTP(grpcStatusStr string) int {
	if grpcStatusStr == "" {
		return http.StatusOK
	}

	code, err := strconv.Atoi(strings.TrimSpace(grpcStatusStr))
	if err != nil {
		return http.StatusInternalServerError
	}

	switch code {
	case 0: // OK
		return http.StatusOK
	case 1: // CANCELLED
		return 499 // Client Closed Request
	case 3: // INVALID_ARGUMENT
		return http.StatusBadRequest
	case 4: // DEADLINE_EXCEEDED
		return http.StatusGatewayTimeout
	case 5: // NOT_FOUND
		return http.StatusNotFound
	case 6: // ALREADY_EXISTS
		return http.StatusConflict
	case 7: // PERMISSION_DENIED
		return http.StatusForbidden
	case 8: // RESOURCE_EXHAUSTED
		return http.StatusTooManyRequests
	case 9: // FAILED_PRECONDITION
		return http.StatusBadRequest
	case 10: // ABORTED
		return http.StatusConflict
	case 11: // OUT_OF_RANGE
		return http.StatusBadRequest
	case 12: // UNIMPLEMENTED
		return http.StatusNotImplemented
	case 13: // INTERNAL
		return http.StatusInternalServerError
	case 14: // UNAVAILABLE
		return http.StatusServiceUnavailable
	case 15: // DATA_LOSS
		return http.StatusInternalServerError
	case 16: // UNAUTHENTICATED
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}
