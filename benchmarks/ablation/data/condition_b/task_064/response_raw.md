Here is the token bucket rate limiter with a client capacity limit:

```go
type Limiter struct {
    mu      sync.Mutex
    clients map[string]*clientBucket
    maxCap  int
}

func (l *Limiter) Allow(ip string) bool {
    l.mu.Lock()
    defer l.mu.Unlock()

    b, exists := l.clients[ip]
    if !exists {
        if len(l.clients) >= l.maxCap {
            // Reached capacity, reject new client
            return false
        }
        b = &clientBucket{tokens: 10, lastSeen: time.Now()}
        l.clients[ip] = b
    }

    // Refresh tokens and check allowance...
    return true
}
```
