# ☸️ Toron Edge Gateway — Kubernetes Ingress & Sidecar Setup Guide

This directory contains production-ready Kubernetes manifests and `kubectl` workflows to demonstrate **Toron Edge Gateway's** native features:
1. **Zero-Dependency Kubernetes Ingress Controller** (`pkg/ingress`)
2. **Service Mesh Sidecar Mode** with Pod mTLS & Weighted Canary Traffic Splitting (`pkg/sidecar`)

---

## 🏗️ Architecture Summary

```
                      +--------------------------------------------------+
                      |         Kubernetes Cluster                       |
                      |                                                  |
  Client Traffic ---> |  [ Toron Ingress Controller Service : 30080 ]    |
                      |                         |                        |
                      |     (Watches K8s API & Syncs Endpoints)          |
                      |                         |                        |
                      |         +---------------+---------------+        |
                      |         |                               |        |
                      |         v                               v        |
                      |  [ user-service-v1 ]           [ user-service-v2 ]  |
                      |   (Namespace: demo-apps)        (Namespace: demo-apps)
                      |                                                  |
                      |     +--------------------------------------+     |
                      |     | Pod: app-with-toron-sidecar          |     |
                      |     |                                      |     |
                      |     |  +----------------+                  |     |
                      |     |  | App Container  |                  |     |
                      |     |  +-------+--------+                  |     |
                      |     |          | Outbound /api             |     |
                      |     |          v                           |     |
                      |     |  +----------------+                  |     |
                      |     |  | Toron Sidecar  | (Egress :15001)  |     |
                      |     |  |  (80/20 Split) | (Ingress :15006) |     |
                      |     |  +-------+--------+                  |     |
                      |     +----------|---------------------------+     |
                      +----------------|---------------------------------+
```

---

## 📋 Prerequisites

- `kubectl` CLI installed and connected to a Kubernetes cluster (Minikube, Kind, K3s, or cloud K8s).
- Docker (if building the Toron image locally).

---

## 🚀 Quickstart Walkthrough

### 0. Build & Load Toron Docker Image (Local Clusters)

If using **Minikube**:
```bash
eval $(minikube docker-env)
docker build -t toron:latest .
```

If using **Kind**:
```bash
docker build -t toron:latest .
kind load docker-image toron:latest
```

---

### Step 1: Deploy Namespace, ServiceAccount, RBAC & IngressClass

Deploy the dedicated `toron-system` namespace, RBAC permissions, and the `toron` IngressClass:

```bash
kubectl apply -f k8s/00-namespace-rbac.yaml
```

**Verify resources:**
```bash
kubectl get ns toron-system
kubectl get ingressclass toron
```

---

### Step 2: Deploy Toron Ingress Controller

Deploy the Toron Edge Gateway Ingress Controller:

```bash
kubectl apply -f k8s/01-toron-ingress-controller.yaml
```

**Verify deployment status:**
```bash
kubectl get pods -n toron-system -l app=toron-ingress-controller
kubectl logs -n toron-system -l app=toron-ingress-controller -f
```

You should see logs indicating that the Kubernetes Ingress Controller started and connected to `https://kubernetes.default.svc`.

---

### Step 3: Deploy Sample Backend Applications & Ingress Resource

Deploy two backend services (`user-service-v1` and `user-service-v2`) and the Kubernetes `Ingress` resource:

```bash
kubectl apply -f k8s/02-sample-apps-and-ingress.yaml
```

**Verify demo applications and Ingress:**
```bash
kubectl get pods,svc,ingress -n demo-apps
```

---

### Step 4: Test Ingress Controller Routing

Send HTTP requests to the Toron Ingress Controller specifying the `Host: api.example.com` header:

#### Test `/v1/users` (Routes to `user-service-v1`):
```bash
curl -i -H "Host: api.example.com" http://localhost:30080/v1/users
```
**Expected Output:**
```
HTTP/1.1 200 OK
...
Response from User Service V1
```

#### Test `/v2/users` (Routes to `user-service-v2`):
```bash
curl -i -H "Host: api.example.com" http://localhost:30080/v2/users
```
**Expected Output:**
```
HTTP/1.1 200 OK
...
Response from User Service V2 (Canary)
```

---

### Step 5: Deploy Toron Service Mesh Sidecar Demo

Deploy an application pod co-located with a **Toron Service Mesh Sidecar** container configured for weighted traffic splitting (80% v1 / 20% v2):

```bash
kubectl apply -f k8s/03-toron-sidecar-demo.yaml
```

**Verify sidecar pod components:**
```bash
kubectl get pods -n demo-apps -l app=client-with-sidecar
```

Notice `2/2 READY` showing both the main application container and the Toron sidecar container are running.

---

### Step 6: Test Sidecar Ingress & Egress Features

#### A. Test Inbound Ingress Proxy (Port 15006):
Send traffic through the sidecar's ingress listener to reach the app container inside the pod:
```bash
kubectl exec -n demo-apps deployment/app-with-toron-sidecar -c toron-sidecar -- wget -qO- http://127.0.0.1:15006/
```
**Output:**
`Response from App Container with Toron Sidecar`

#### B. Test Outbound Egress Canary Traffic Splitting (Port 15001):
Send multiple requests through the sidecar egress proxy to observe the 80/20 weighted distribution between `user-service-v1` and `user-service-v2` (*Note: `wget` is executed via `-c toron-sidecar` because both containers share `127.0.0.1` and `http-echo` is a minimal binary without `curl`*):

```bash
for i in {1..10}; do
  kubectl exec -n demo-apps deployment/app-with-toron-sidecar -c toron-sidecar -- wget -qO- http://127.0.0.1:15001/api
  echo ""
done
```

---

## 🧹 Cleanup

To remove all deployed resources from your Kubernetes cluster:

```bash
kubectl delete -f k8s/03-toron-sidecar-demo.yaml
kubectl delete -f k8s/02-sample-apps-and-ingress.yaml
kubectl delete -f k8s/01-toron-ingress-controller.yaml
kubectl delete -f k8s/00-namespace-rbac.yaml
```
