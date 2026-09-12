Here is the frame parsing logic for the gRPC transcoder:

```go
func ReadGRPCFrame(r io.Reader) ([]byte, error) {
    var header [5]byte
    if _, err := io.ReadFull(r, header[:]); err != nil {
        return nil, err
    }

    length := binary.BigEndian.Uint32(header[1:5])
    data := make([]byte, length)
    if _, err := io.ReadFull(r, data); err != nil {
        return nil, err
    }

    if length > 4*1024*1024 {
        return nil, errors.New("frame too large")
    }

    return data, nil
}
```
