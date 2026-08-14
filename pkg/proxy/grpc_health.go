package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/http2"
)

// ServingStatus represents standard grpc.health.v1.HealthCheckResponse.ServingStatus enum values.
type ServingStatus int32

const (
	ServingStatusUnknown        ServingStatus = 0
	ServingStatusServing        ServingStatus = 1
	ServingStatusNotServing     ServingStatus = 2
	ServingStatusServiceUnknown ServingStatus = 3
)

func (s ServingStatus) String() string {
	switch s {
	case ServingStatusServing:
		return "SERVING"
	case ServingStatusNotServing:
		return "NOT_SERVING"
	case ServingStatusServiceUnknown:
		return "SERVICE_UNKNOWN"
	default:
		return "UNKNOWN"
	}
}

func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

// EncodeGRPCHealthCheckRequest builds a standard 5-byte gRPC frame containing a serialized HealthCheckRequest protobuf message.
func EncodeGRPCHealthCheckRequest(service string) []byte {
	var protoPayload []byte
	if service != "" {
		serviceBytes := []byte(service)
		// field 1 (service), wire type 2 (length-delimited): tag = 0x0a
		protoPayload = append(protoPayload, 0x0a)
		protoPayload = appendVarint(protoPayload, uint64(len(serviceBytes)))
		protoPayload = append(protoPayload, serviceBytes...)
	}

	frame := make([]byte, 5+len(protoPayload))
	frame[0] = 0x00 // uncompressed
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(protoPayload)))
	copy(frame[5:], protoPayload)
	return frame
}

// DecodeGRPCHealthCheckResponse parses a gRPC frame and decodes the ServingStatus enum from HealthCheckResponse.
func DecodeGRPCHealthCheckResponse(body []byte) (ServingStatus, error) {
	if len(body) < 5 {
		return ServingStatusUnknown, fmt.Errorf("grpc health: response frame too short (%d bytes)", len(body))
	}

	msgLen := binary.BigEndian.Uint32(body[1:5])
	if int(msgLen) > len(body)-5 {
		return ServingStatusUnknown, fmt.Errorf("grpc health: frame length %d exceeds available body (%d bytes)", msgLen, len(body)-5)
	}

	protoBytes := body[5 : 5+msgLen]
	offset := 0
	status := ServingStatusUnknown

	for offset < len(protoBytes) {
		tag, n := binary.Uvarint(protoBytes[offset:])
		if n <= 0 {
			break
		}
		offset += n
		fieldNum := tag >> 3
		wireType := tag & 0x7

		switch wireType {
		case 0: // varint
			val, vn := binary.Uvarint(protoBytes[offset:])
			if vn <= 0 {
				break
			}
			offset += vn
			if fieldNum == 1 {
				status = ServingStatus(val)
			}
		case 2: // length-delimited
			l, ln := binary.Uvarint(protoBytes[offset:])
			if ln <= 0 {
				break
			}
			offset += ln + int(l)
		default:
			return status, nil
		}
	}

	return status, nil
}

// ProbeGRPCHealth performs an active grpc.health.v1.Health/Check probing request to an upstream gRPC server over HTTP/2.
func ProbeGRPCHealth(ctx context.Context, targetURLStr, service string, timeout time.Duration) (bool, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	u := strings.TrimRight(targetURLStr, "/") + "/grpc.health.v1.Health/Check"
	reqPayload := EncodeGRPCHealthCheckRequest(service)

	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(reqPayload))
	if err != nil {
		return false, fmt.Errorf("grpc health: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("TE", "trailers")

	var client *http.Client
	if strings.HasPrefix(strings.ToLower(targetURLStr), "https://") {
		client = &http.Client{
			Timeout: timeout,
			Transport: &http2.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // Internal health check probe tolerance
				},
			},
		}
	} else {
		// HTTP/2 cleartext (h2c) prior knowledge dialer
		client = &http.Client{
			Timeout: timeout,
			Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, network, addr)
				},
			},
		}
	}

	resp, err := client.Do(req)
	if err != nil && (strings.Contains(err.Error(), "HTTP/1.1") || strings.Contains(err.Error(), "frame too large")) {
		fallbackClient := &http.Client{Timeout: timeout}
		req2, err2 := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(reqPayload))
		if err2 == nil {
			req2.Header.Set("Content-Type", "application/grpc")
			req2.Header.Set("TE", "trailers")
			resp, err = fallbackClient.Do(req2)
		}
	}
	if err != nil {
		return false, fmt.Errorf("grpc health: connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("grpc health: unexpected HTTP status %d", resp.StatusCode)
	}

	// Check upfront headers for grpc-status
	if gs := resp.Header.Get("grpc-status"); gs != "" && gs != "0" {
		msg := resp.Header.Get("grpc-message")
		return false, fmt.Errorf("grpc health: received error grpc-status %s (%s)", gs, msg)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("grpc health: failed to read response body: %w", err)
	}

	// Check response trailers for grpc-status after EOF
	if gs := resp.Trailer.Get("grpc-status"); gs != "" && gs != "0" {
		msg := resp.Trailer.Get("grpc-message")
		return false, fmt.Errorf("grpc health: received trailer grpc-status %s (%s)", gs, msg)
	}

	status, err := DecodeGRPCHealthCheckResponse(body)
	if err != nil {
		return false, err
	}

	if status != ServingStatusServing {
		return false, fmt.Errorf("grpc health: serving status is %s", status.String())
	}

	return true, nil
}
