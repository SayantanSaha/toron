Here is the HTTP to HTTPS redirect handler:

```go
func (s *Server) handleHTTPSRedirect(w http.ResponseWriter, r *http.Request) {
    host := r.Host
    if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
        host = fwdHost
    }

    targetURL := "https://" + host + r.URL.RequestURI()
    http.Redirect(w, r, targetURL, http.StatusMovedPermanently)
}
```
