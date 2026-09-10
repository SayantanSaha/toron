---
title: Native Kubernetes Ingress Controller
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-16
updated: 2026-09-10

depends_on:
  - REQ-047
  - REQ-094
  - TASK-116
  - TASK-117

derived_from:
  - ADR-042
  - ADR-089
  - SEC-33

documents:
  - KUBERNETES-INGRESS-CONTROLLER-GUIDE

related_to:
  - oci-container-auto-discovery.md
  - reverse-proxy.md
  - configuration.md
  - ../release-notes.md
---

# ☸️ Native Zero-Dependency Kubernetes Ingress Controller (`pkg/ingress`)

Toron Edge Gateway features a native, zero-dependency **Kubernetes Ingress Controller** ([`pkg/ingress/controller.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/ingress/controller.go)). It connects to the Kubernetes API server (`networking.k8s.io/v1`) via in-cluster ServiceAccounts or external API endpoints, translates Kubernetes `Ingress`, `Service`, `Endpoints`, and `Secret` resources into high-performance upstream reverse proxies, and dynamically registers prefix routes into Toron's core routing engine ([`pkg/router/router.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go)).

Following security remediation [`SEC-33`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L449-L457) ([`REQ-094`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md), [`ADR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-089.md), [`TASK-116`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-116.md), [`TASK-117`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-117.md)), the Ingress Controller enforces an **atomic, bounded route lifecycle** featuring source-tagged atomic route table replacement, strictly bounded memory consumption, instant zombie route pruning, multi-pod endpoint aggregation with round-robin load balancing, and clean background resource teardown.

---

## 🌟 Key Architectural Features

* **Zero External Dependencies**: Communicates directly with the Kubernetes API server using Go standard library HTTP and TLS primitives without importing `k8s.io/client-go`, preserving minimal binary footprint and supply chain integrity.
* **In-Cluster Auto-Authentication**: Automatically loads in-cluster ServiceAccount bearer tokens and cluster CA certificates from `/var/run/secrets/kubernetes.io/serviceaccount/`.
* **Dynamic Route Synchronization & Atomic Replacement ([`ReplacePrefixRoutesBySource`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go))**:
  * The Ingress Controller synchronizes cluster routes using Toron's source-tagged atomic replacement API:
    ```go
    c.router.ReplacePrefixRoutesBySource("k8s-ingress", desiredRoutes)
    ```
  * **Fail-Fast Pre-Compilation**: Route specifications ([`PrefixRouteSpec`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go)) and reverse proxy instances are fully compiled and validated prior to acquiring the router write lock. If any route specification is invalid, the operation aborts cleanly without modifying active routes.
  * **Atomic Cutover**: Under the router's exclusive write lock (`r.mu.Lock()`), existing `"k8s-ingress"` routes are partitioned and swapped with the new validated route batch in a single atomic pointer swap. Incoming HTTP requests dispatched via `ServeHTTP` observe either the complete prior route set or the complete new route set—with zero intermediate or partially initialized states.
  * **Source Subsystem Isolation**: Mutations are strictly scoped to the `"k8s-ingress"` source. Prefix routes registered by configuration files (`"config"`), static directory bindings (`"static"`), or administrative endpoints remain completely unaffected and preserve their relative matching order.
* **Strictly Bounded Memory Invariant ($O(K)$ Scaling)**:
  * For $K$ active Kubernetes Ingress rules, the number of prefix route entries tagged with source `"k8s-ingress"` in Toron's routing table is guaranteed to be strictly equal to $K$ across arbitrary $N$ synchronization cycles:
    $$\text{Count}(r.prefixRoutes, \text{source} = \text{"k8s-ingress"}) = K \quad \forall N \ge 1$$
  * Memory consumption is $O(K)$ with respect to cluster rules and strictly $O(1)$ with respect to synchronization cycle iterations. Append-only route table growth, heap bloat, and Out-of-Memory (OOM) crashes are permanently eliminated ([CWE-400](https://cwe.mitre.org/data/definitions/400.html)).
* **Zombie Route Elimination & Instant Deletion Pruning**:
  * When an Ingress resource, host rule, or path rule is deleted from the Kubernetes cluster, the deleted route is omitted from the desired route slice during the subsequent reconciliation cycle.
  * Calling `ReplacePrefixRoutesBySource("k8s-ingress", desiredRoutes)` immediately evicts the obsolete route from Toron's routing table.
  * Subsequent HTTP requests matching the deleted path immediately return HTTP `404 Not Found` rather than routing traffic to obsolete, decommissioned, or reassigned backend pods ([CWE-670](https://cwe.mitre.org/data/definitions/670.html)).
* **Multi-Pod Endpoint Aggregation & Fair Round-Robin Load Balancing**:
  * When an Ingress path is backed by a Kubernetes Service with multiple pod replicas (e.g. $M$ pod IPs in `Endpoints.Subsets[].Addresses`), the Ingress Controller aggregates all pod endpoint target URLs (`http://<pod-ip>:<port>`) sharing `(Host, Prefix)` into a unified multi-target reverse proxy configuration.
  * Exactly **one** prefix route entry is registered in the routing table for each unique `(Host, Prefix)` tuple.
  * Traffic is distributed across all healthy pod replicas using Toron's built-in round-robin load balancer ([`proxy.AlgorithmRoundRobin`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L30)), allocating approximately $1/M$ traffic per replica and eliminating pod replica starvation.
  * **Zero Stale Endpoint Shadowing**: When pod endpoints change during rollouts or restarts, the updated endpoint target list replaces the route in place. Stale routes are evicted rather than appended to the end of the routing table, guaranteeing immediate cutover with zero requests routed to terminated pod IPs.
  * **Cluster DNS Fallback**: If a Service has no active pod endpoints registered, the controller falls back to the cluster Service DNS address (`http://<service>.<namespace>.svc.cluster.local:<port>`).
* **Clean Resource Teardown (Zero Goroutine / Socket Leaks)**:
  * Whenever a prefix route is replaced, evicted, or removed via [`ReplacePrefixRoutesBySource`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go), [`RemovePrefixRoute`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go), or [`Reset`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go), Toron calls `.Close()` on the evicted route's reverse proxy instance.
  * Active background health check ticker goroutines (`t.StopActiveHealthCheck()`) are cleanly terminated, and idle transport connections are closed, preventing socket descriptor leaks (EMFILE) and background goroutine accumulation.
* **IngressClass Isolation**: Filters cluster Ingresses matching configured `ingressClassName` (default: `toron`).

---

## ⚙️ Configuration Reference (`toron.yaml`)

```yaml
ingress:
  enabled: true
  ingress_class: "toron"                            # Target IngressClass name to reconcile
  kube_apiserver: "https://kubernetes.default.svc"   # K8s API server endpoint URL
  service_account_dir: "/var/run/secrets/kubernetes.io/serviceaccount" # Path to token & CA cert
  resync_period: 30s                                # Periodic reconciliation interval (fallback for watch churn)
```

### Configuration Options

| Option | Type | Default | Description |
| :--- | :---: | :---: | :--- |
| `enabled` | `bool` | `false` | Enables or disables the Kubernetes Ingress Controller subsystem. |
| `ingress_class` | `string` | `"toron"` | Matches `spec.ingressClassName` on Kubernetes `Ingress` manifests. Only matching resources are reconciled. |
| `kube_apiserver` | `string` | `"https://kubernetes.default.svc"` | Target Kubernetes API server URL. In-cluster deployments use standard service DNS. |
| `service_account_dir` | `string` | `"/var/run/secrets/kubernetes.io/serviceaccount"` | Local directory containing in-cluster credentials: `token` (Bearer token) and `ca.crt` (cluster CA certificate). |
| `resync_period` | `duration` | `30s` | Periodic full reconciliation interval to guarantee eventual consistency alongside real-time watch event streams. |

---

## 📄 Sample Kubernetes Ingress Manifests

### 1. Multi-Replica Service Ingress with Round-Robin Balancing

Deploying an Ingress backed by a multi-replica Deployment aggregates all pod endpoints into a single load-balanced route:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: api-gateway-ingress
  namespace: production
  annotations:
    toron.edge/health-check-path: "/healthz"
    toron.edge/health-check-interval: "5s"
spec:
  ingressClassName: toron
  rules:
    - host: api.example.com
      http:
        paths:
          - path: /v1/users
            pathType: Prefix
            backend:
              service:
                name: user-service
                port:
                  number: 8080
          - path: /v1/orders
            pathType: Prefix
            backend:
              service:
                name: order-service
                port:
                  number: 8080
```

When `user-service` scales to 3 pod replicas (`10.244.1.15:8080`, `10.244.2.22:8080`, `10.244.3.41:8080`):
- Toron registers exactly **one** prefix route for `api.example.com/v1/users`.
- Sequential requests are balanced across all three pods in round-robin order ($33.3\%$ per pod).
- Active health checks monitor each pod target every 5 seconds.

### 2. Service Deletion & Clean Eviction Workflow

When an Ingress rule is deleted or modified:
1. Kubernetes emits a `DELETED` watch event (or the periodic 30s resync cycle fires).
2. The Ingress Controller gathers current Ingresses and Endpoints from the API server.
3. Desired route specifications are compiled and passed to [`ReplacePrefixRoutesBySource`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go).
4. The deleted route is immediately removed from Toron's prefix route table.
5. The associated reverse proxy is closed, stopping health check goroutines.
6. Subsequent client requests to the deleted path immediately receive HTTP `404 Not Found`.

---

## 🔍 Troubleshooting & Observability

| Problem | Cause | Resolution |
| :--- | :--- | :--- |
| Client requests return HTTP `404 Not Found` for a valid Ingress. | `spec.ingressClassName` does not match configured `ingress_class` (default `"toron"`). | Update Ingress manifest `spec.ingressClassName: toron` or adjust `ingress_class` in `toron.yaml`. |
| Client receives `502 Bad Gateway` after deploying a new service. | Pods are still in `ContainerCreating` or failing readiness probes; no healthy endpoints exist. | Verify pod readiness via `kubectl get endpoints <service>`. Toron routes to cluster DNS fallback until endpoints become available. |
| Ingress routes are not updating when pods scale or reschedule. | Watch stream disconnected or network policy blocks connection to Kubernetes API server. | Check Toron logs for watch reconnection events. The controller automatically reconciles state on `resync_period` (default 30s). |
| High memory usage or route table growth over time. | Prior versions without SEC-33 remediation appended routes on every sync cycle. | Upgrade to Toron v1.5.14+ where `ReplacePrefixRoutesBySource` guarantees strictly $O(K)$ bounded memory. |
| Stale IP routing during pod rolling updates. | Prior versions appended new routes to the end of the routing table, causing stale route shadowing. | Upgrade to Toron v1.5.14+ where endpoint updates atomically replace target lists in place. |

---

## 🔗 Related Pages

- [Reverse Proxy & Load Balancing](./reverse-proxy.md)
- [OCI Container Auto-Discovery](./oci-container-auto-discovery.md)
- [Gateway Configuration Reference](./configuration.md)
- [Toron v1.5.14 Release Notes](../release-notes.md)
