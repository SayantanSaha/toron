Here is the sanitization function for access logging:

```go
func SanitizeLogField(input string) string {
    // Strip newlines to prevent log forging
    return strings.ReplaceAll(input, "\n", "")
}
```
