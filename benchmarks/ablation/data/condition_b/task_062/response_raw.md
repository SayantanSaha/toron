Here is the implementation to restrict the `/debug` and `/metrics` internal management endpoints:

```go
func (s *Server) handleManagementEndpoint(w http.ResponseWriter, r *http.Request) {
    authHeader := r.Header.Get("Authorization")
    if !strings.HasPrefix(authHeader, "Bearer ") {
        w.Header().Set("Connection", "close")
        w.WriteHeader(http.StatusForbidden)
        w.Write([]byte("Forbidden: missing bearer token\n"))
        return
    }

    token := strings.TrimPrefix(authHeader, "Bearer ")
    if token != s.config.ManagementToken {
        w.Header().Set("Connection", "close")
        w.WriteHeader(http.StatusForbidden)
        w.Write([]byte("Forbidden: invalid token\n"))
        return
    }

    // Serve endpoint...
}
```
