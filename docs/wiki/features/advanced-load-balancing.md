# Advanced Load Balancing Engine

Toron Edge Gateway features a zero-dependency, high-throughput load balancing engine supporting 8 strategies in `pkg/proxy`:

## Supported Load Balancing Strategies

| Strategy Keyword | Description | Use Case |
| :--- | :--- | :--- |
| `round_robin` | Sequential circular target selection | General-purpose stateless microservices |
| `weighted_round_robin` | Smooth weighted distribution (Nginx algorithm) | Heterogeneous server capacities |
| `random` / `weighted_random` | Probability distribution target selection | Stateless microservice clusters |
| `least_conn` | Routes to target with fewest active in-flight requests | Long-lived WebSocket or database proxying |
| `weighted_least_conn` | Routes to target with lowest `ActiveConns / Weight` | Long-lived requests across varied node specs |
| `least_latency` | Exponential moving average (EMA) response time selection | Latency-sensitive APIs and real-time backend clusters |
| `sticky_cookie` | Cookie-based session affinity | Stateful Web applications |
| `ip_hash` | Client IP hash persistence | IP-based session persistence |

## Configuration Example (`routes.yaml`)

```yaml
routes:
  - type: "upstream"
    prefix: "/services/weighted"
    algorithm: "weighted_round_robin"
    targets:
      - "http://localhost:9005"
      - "http://localhost:9006"

  - type: "upstream"
    prefix: "/services/least-conn"
    algorithm: "least_conn"
    targets:
      - "http://localhost:9007"
      - "http://localhost:9008"

  - type: "upstream"
    prefix: "/services/least-latency"
    algorithm: "least_latency"
    targets:
      - "http://localhost:9009"
      - "http://localhost:9010"
```
