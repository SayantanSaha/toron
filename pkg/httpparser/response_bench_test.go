package httpparser_test

import (
	"io"
	"net/http"
	"testing"

	"toron/pkg/httpparser"
)

func BenchmarkResponse_Serialize_Pooled(b *testing.B) {
	res := httpparser.NewResponse()
	res.SetStatus(http.StatusOK)
	res.Header.Set("Content-Type", "application/json")
	res.Header.Set("Server", "toron")
	res.Header.Set("X-Request-Id", "bench-req-12345")
	_, _ = res.WriteString(`{"status":"ok"}`)

	discard := io.Discard
	// Warm up pool
	_ = res.Serialize(discard)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = res.Serialize(discard)
	}
}
