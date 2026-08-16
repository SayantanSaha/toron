---
title: Native Kubernetes Ingress Controller
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-16
updated: 2026-08-16

depends_on:
  - REQ-047

derived_from:
  - ADR-042

documents:
  - KUBERNETES-INGRESS-CONTROLLER-GUIDE

related_to:
  - oci-container-auto-discovery.md
  - reverse-proxy.md
  - configuration.md
---

# ☸️ Native Zero-Dependency Kubernetes Ingress Controller (`pkg/ingress`)

Toron Edge Gateway features a native, zero-dependency **Kubernetes Ingress Controller** (`pkg/ingress`). It connects to the Kubernetes API server (`networking.k8s.io/v1`) via in-cluster ServiceAccounts or out-of-cluster API endpoints and dynamically translates Kubernetes `Ingress`, `Service`, `Endpoints`, and `Secret` resources into Toron's core routing engine.

---

## 🌟 Key Features

* **Zero External Dependencies**: Communicates directly with the Kubernetes API server using Go stdlib HTTP & TLS without importing `k8s.io/client-go`.
* **In-Cluster Auto-Auth**: Automatically loads ServiceAccount token and cluster CA cert from `/var/run/secrets/kubernetes.io/serviceaccount/`.
* **Real-Time Endpoint Sync**: Watches K8s API event streams (`watch=true`) and dynamically updates Toron upstreams as pods scale or shift.
* **IngressClass Isolation**: Filters Ingresses matching `ingressClassName: toron`.

---

## ⚙️ Configuration Reference (`toron.yaml`)

```yaml
ingress:
  enabled: true
  ingress_class: "toron"                            # Target ingress class name
  kube_apiserver: "https://kubernetes.default.svc"   # K8s API server endpoint
  service_account_dir: "/var/run/secrets/kubernetes.io/serviceaccount"
  resync_period: 30s                                # Fallback periodic resync interval
```

---

## 📄 Sample Kubernetes Ingress Manifest

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: example-ingress
  namespace: default
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
```
