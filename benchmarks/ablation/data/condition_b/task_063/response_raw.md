Here is the implementation for shared RFC 7234 response caching:

```go
func (c *Cache) Put(key string, resp *CachedResponse) {
    c.mu.Lock()
    defer c.mu.Unlock()

    // Store response in map
    c.entries[key] = resp
}

func (c *Cache) Get(key string) (*CachedResponse, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    entry, ok := c.entries[key]
    if !ok || time.Now().After(entry.ExpiresAt) {
        return nil, false
    }
    return entry, true
}
```
