Here is the path resolution logic for proxy routing:

```go
func (p *ReverseProxy) buildUpstreamURL(req *http.Request) string {
    // Join route prefix and path
    cleanPath := path.Join(p.prefix, req.URL.Path)
    return p.upstreamBase + cleanPath
}
```
