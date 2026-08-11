package httpparser_test

import (
	"bytes"
	"io"
	"testing"

	"toron/pkg/httpparser"
)

func BenchmarkParseRequest_GET(b *testing.B) {
	rawReq := []byte("GET /api/v1/resource?query=test HTTP/1.1\r\nHost: localhost:8080\r\nUser-Agent: toron-bench\r\nAccept: application/json\r\n\r\n")
	opts := httpparser.DefaultParserOptions()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(rawReq)
		_, err := httpparser.ParseRequest(r, opts)
		if err != nil {
			b.Fatalf("parse error: %v", err)
		}
	}
}

func BenchmarkParseRequest_POST(b *testing.B) {
	rawReq := []byte("POST /api/v1/submit HTTP/1.1\r\nHost: localhost:8080\r\nContent-Type: application/json\r\nContent-Length: 27\r\n\r\n{\"key\":\"value\",\"status\":\"ok\"}")
	opts := httpparser.DefaultParserOptions()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(rawReq)
		_, err := httpparser.ParseRequest(r, opts)
		if err != nil {
			b.Fatalf("parse error: %v", err)
		}
	}
}

func BenchmarkResponse_Serialize(b *testing.B) {
	res := httpparser.NewResponse()
	res.SetStatus(200)
	res.Header.Set("Content-Type", "application/json")
	res.Header.Set("X-Server", "Toron")
	_, _ = res.WriteString(`{"status":"ok","code":200}`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		res.Serialize(io.Discard)
	}
}
