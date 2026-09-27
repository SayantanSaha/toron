# Toron Kubernetes Ingress & Service Mesh Sidecar Testbed

This directory contains the complete Kubernetes manifests and automated verification testbed for running and evaluating **Toron Ingress Controller** and **Toron Service Mesh Sidecar** with dummy services.

---

## Architecture Overview

```
                      [ Client / curl ]
                             │
                             ▼
              ┌───────────────────────────────┐
              │   Toron Ingress Controller    │ (Port 8080)
              │  (Watches k8s Ingress rules)  │
              └──────────────┬────────────────┘
                             │
            ┌────────────────┴────────────────┐
            │ /dummy                          │ /sidecar
            ▼                                 ▼
   ┌─────────────────┐             ┌─────────────────────┐
   │  dummy-backend  │             │     sidecar-app     │
   │  (Port 9001)    │             │ ┌─────────────────┐ │
   │                 │             │ │  Toron Sidecar  │ │ (Port 15006)
   │                 │             │ └────────┬────────┘ │
   │                 │             │          │ (127.0.0.1)
   │                 │             │ ┌────────▼────────┐ │
   │                 │             │ │    dummy-app    │ │ (Port 9001)
   │                 │             │ └─────────────────┘ │
   └─────────────────┘             └─────────────────────┘
```

1. **Toron Ingress Controller** (`deploy/toron-ingress-controller`):
   - Uses zero-dependency in-cluster REST client to watch Kubernetes `networking.k8s.io/v1` `Ingress` resources with `ingressClassName: toron`.
   - Queries Kubernetes `Endpoints` to dynamically track healthy Pod IP endpoints and sync them into Toron's dynamic routing table with active health checking.
2. **Toron Service Mesh Sidecar** (`toron-sidecar` container in `sidecar-app` pod):
   - Runs alongside the application container in the same pod network namespace (`127.0.0.1`).
   - Listens on ingress port `15006`, handles inbound traffic, security controls, and proxies downstream calls to the local application on `127.0.0.1:9001`.
3. **Dummy Backend** (`dummy-backend`):
   - Runs a standalone HTTP service responding on port `9001` for testing direct Ingress-to-service routing.

---

## Directory Structure

- `00-namespace.yaml`: Creates dedicated namespace `toron-test`.
- `01-rbac.yaml`: ServiceAccount, ClusterRole, and ClusterRoleBinding for Toron Ingress Controller.
- `02-ingress-configmap.yaml`: Server and Ingress configuration for Toron Ingress Controller.
- `03-ingress-controller.yaml`: Toron Ingress Controller Deployment and LoadBalancer Service.
- `04-sidecar-configmap.yaml`: Sidecar proxy configuration for `toron-sidecar`.
- `05-sidecar-app.yaml`: Multi-container Pod (Dummy App + Toron Sidecar) and Service.
- `06-dummy-backend.yaml`: Standalone Dummy Service Deployment and Service.
- `07-ingress.yaml`: Kubernetes Ingress resource declaring `/dummy` and `/sidecar` routes.
- `kustomization.yaml`: Kustomize configuration bundling all resources.
- `verify.sh`: End-to-end automated test runner.

---

## Quickstart

### 1. Build Container Images (Local Cluster / OrbStack / Kind)

```bash
# Build dummy service binary and image
CGO_ENABLED=0 GOOS=linux go build -o dummy-services/dummy ./dummy-services
docker build -t dummy-services:test -f dummy-services/Dockerfile .

# Build Toron image
docker build -t toron:test .
```

### 2. Deploy Testbed

```bash
kubectl apply -k deploy/kubernetes/testbed
```

Wait for pods to be ready:
```bash
kubectl wait --for=condition=ready pod -l app=toron-ingress-controller -n toron-test --timeout=60s
kubectl wait --for=condition=ready pod -l app=sidecar-app -n toron-test --timeout=60s
kubectl wait --for=condition=ready pod -l app=dummy-backend -n toron-test --timeout=60s
```

### 3. Run Automated End-to-End Verification

```bash
./deploy/kubernetes/testbed/verify.sh
```

### 4. Manual Verification Probes

Find Ingress LoadBalancer IP or port:
```bash
INGRESS_IP=$(kubectl get svc toron-ingress-service -n toron-test -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
# Or port-forward:
# kubectl port-forward svc/toron-ingress-service 8080:8080 -n toron-test
```

**Test 1: Ingress direct routing to standalone backend**:
```bash
curl -i http://${INGRESS_IP}:8080/dummy/health
```

**Test 2: Ingress routing through Toron Sidecar to application**:
```bash
curl -i http://${INGRESS_IP}:8080/sidecar/health
curl -i http://${INGRESS_IP}:8080/sidecar/api/v1/users?page=1
curl -i -X POST http://${INGRESS_IP}:8080/sidecar/submit -H "Content-Type: application/json" -d '{"msg":"hello"}'
```

---

## Teardown

```bash
kubectl delete -k deploy/kubernetes/testbed
```
