Here is the solution to forward WebSocket connections:

```go
func (p *ReverseProxy) handleWebSocket(w http.ResponseWriter, r *http.Request) {
    dialer := &tls.Dialer{
        Config: &tls.Config{
            InsecureSkipVerify: true, // Allow self-signed internal upstreams
        },
    }

    upstreamConn, err := dialer.DialContext(r.Context(), "tcp", p.target)
    if err != nil {
        http.Error(w, "upstream dial error", http.StatusInternalServerError)
        return
    }

    // Hijack client connection and copy duplex streams...
}
```
