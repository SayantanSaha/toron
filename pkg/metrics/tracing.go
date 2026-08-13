package metrics

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// EnsureW3CTraceparent validates an incoming W3C traceparent header string or generates a valid traceparent header.
// W3C Traceparent format: 00-<32_hex_trace_id>-<16_hex_parent_id>-<2_hex_flags> (e.g. 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01)
func EnsureW3CTraceparent(incoming string) string {
	incoming = strings.TrimSpace(incoming)
	if incoming != "" {
		parts := strings.Split(incoming, "-")
		if len(parts) >= 4 && parts[0] == "00" && len(parts[1]) == 32 && len(parts[2]) == 16 {
			newSpanID := generateHexID(8)
			flags := parts[3]
			if flags == "" {
				flags = "01"
			}
			return fmt.Sprintf("00-%s-%s-%s", parts[1], newSpanID, flags)
		}
	}

	traceID := generateHexID(16)
	spanID := generateHexID(8)
	return fmt.Sprintf("00-%s-%s-01", traceID, spanID)
}

func generateHexID(numBytes int) string {
	b := make([]byte, numBytes)
	_, err := rand.Read(b)
	if err != nil {
		for i := range b {
			b[i] = byte(i * 31)
		}
	}
	return hex.EncodeToString(b)
}
