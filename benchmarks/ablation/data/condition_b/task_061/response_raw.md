Here is the solution to implement the Transfer-Encoding protocol guard:

### 1. In `pkg/httpparser/parser.go`:
Add a custom error `ErrUnsupportedTransferEncoding` and check if `Transfer-Encoding` exists in headers:

```go
var ErrUnsupportedTransferEncoding = errors.New("unsupported transfer encoding")

// Inside ParseRequest:
if te := req.Header.Get("Transfer-Encoding"); te != "" {
    return nil, ErrUnsupportedTransferEncoding
}
```

### 2. In `pkg/server/server.go`:
Handle the error in `handleConn`:

```go
req, err := httpparser.ParseRequest(br)
if err != nil {
    if errors.Is(err, httpparser.ErrUnsupportedTransferEncoding) {
        conn.Write([]byte("HTTP/1.1 501 Not Implemented\r\nContent-Length: 0\r\n\r\n"))
        continue
    }
    return err
}
```
