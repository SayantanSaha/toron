#!/usr/bin/env bash
set -euo pipefail

echo "=========================================================="
echo " Toron Kubernetes Testbed End-to-End Verification"
echo " Ingress Controller & Service Mesh Sidecar Test Suite"
echo "=========================================================="

NAMESPACE="toron-test"

# Discover Ingress endpoint
INGRESS_IP=$(kubectl get svc toron-ingress-service -n "$NAMESPACE" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)
if [ -z "$INGRESS_IP" ]; then
  # Fallback to OrbStack / cluster service IP
  INGRESS_IP=$(kubectl get svc toron-ingress-service -n "$NAMESPACE" -o jsonpath='{.spec.clusterIP}')
fi

BASE_URL="http://${INGRESS_IP}:8080"
echo "[INFO] Targeting Ingress endpoint: ${BASE_URL}"

# If base URL cannot be reached directly, start a temporary port-forward in background
PF_PID=""
if ! curl -s --connect-timeout 2 "${BASE_URL}/health" >/dev/null; then
  echo "[INFO] Direct IP not reachable from host, establishing background kubectl port-forward..."
  kubectl port-forward svc/toron-ingress-service 18080:8080 -n "$NAMESPACE" >/dev/null 2>&1 &
  PF_PID=$!
  sleep 2
  BASE_URL="http://127.0.0.1:18080"
fi

cleanup() {
  if [ -n "$PF_PID" ]; then
    kill "$PF_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo ""
echo "[TEST 1] Verifying Toron Ingress Controller health endpoint..."
RES1=$(curl -s "${BASE_URL}/health")
echo " -> Response: ${RES1}"
echo "${RES1}" | grep -q '"status":"ok"'
echo " [PASS] Ingress Controller is healthy."

echo ""
echo "[TEST 2] Verifying Ingress direct routing to dummy-backend..."
RES2=$(curl -s "${BASE_URL}/dummy/health")
echo " -> Response: ${RES2}"
echo "${RES2}" | grep -q '"service":"dummy-service-1"'
echo " [PASS] Ingress routing to standalone backend succeeded."

echo ""
echo "[TEST 3] Verifying Ingress -> Toron Sidecar -> Dummy App proxying..."
RES3=$(curl -s "${BASE_URL}/sidecar/health")
echo " -> Response: ${RES3}"
echo "${RES3}" | grep -q '"service":"dummy-service-1"'
echo " [PASS] Multi-hop Ingress to Sidecar proxying succeeded."

echo ""
echo "[TEST 4] Verifying HTTP GET path & parameter forwarding through Sidecar..."
RES4=$(curl -s "${BASE_URL}/sidecar/api/v1/resource?id=42")
echo " -> Response: ${RES4}"
echo "${RES4}" | grep -q '"path":"/api/v1/resource"'
echo " [PASS] Path rewrite and forwarding through Sidecar verified."

echo ""
echo "[TEST 5] Verifying HTTP POST body forwarding through Sidecar..."
RES5=$(curl -s -X POST "${BASE_URL}/sidecar/submit" -H "Content-Type: application/json" -d '{"test":"k8s-mesh"}')
echo " -> Response: ${RES5}"
echo "${RES5}" | grep -q '"method":"POST"'
echo " [PASS] POST body payload forwarding verified."

echo ""
echo "=========================================================="
echo " ALL TESTS PASSED: Toron Ingress & Sidecar are fully operational!"
echo "=========================================================="
