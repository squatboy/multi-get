#!/usr/bin/env bash

set -euo pipefail

if [[ "${E2E_ALLOW_CLUSTER_MUTATION:-}" != "1" ]]; then
	cat >&2 <<'EOF'
E2E_ALLOW_CLUSTER_MUTATION=1 is required; refusing to mutate the cluster.
EOF
	exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
kubectl_bin="$(command -v kubectl)"
go_bin="${GO:-go}"

"$go_bin" build -o "$repo_root/bin/kubectl-multi-get" ./cmd/kubectl-multi-get
export PATH="$repo_root/bin:$PATH"

context_name="$(kubectl config current-context)"
api_server="$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')"
printf 'context: %s\napi-server: %s\n' "$context_name" "$api_server"

run_id="$(date +%s)-$$"
prefix="multi-get-e2e-${run_id}"
label_selector="multi-get.e2e=true"
run_label="multi-get.run=${run_id}"
namespace_a="${prefix}-dev-a"
namespace_b="${prefix}-dev-b"
namespace_other="${prefix}-other"
widget_crd="${prefix}.e2e.multi-get.example"

cleanup_status=0
cleanup() {
	cleanup_status=$?
	trap - EXIT INT TERM
	set +e

	for namespace in "$namespace_a" "$namespace_b" "$namespace_other"; do
		if [[ "$namespace" == "${prefix}-"* ]] && kubectl get namespace "$namespace" -o jsonpath='{.metadata.labels.multi-get\.e2e}' 2>/dev/null | grep -qx true; then
			kubectl delete namespace "$namespace" --ignore-not-found --wait=false >/dev/null 2>&1
		fi
	done
	if [[ "$widget_crd" == "${prefix}."* ]] && kubectl get crd "$widget_crd" -o jsonpath='{.metadata.labels.multi-get\.e2e}' 2>/dev/null | grep -qx true; then
		kubectl delete crd "$widget_crd" --ignore-not-found >/dev/null 2>&1
	fi
	exit "$cleanup_status"
}
trap cleanup EXIT INT TERM

for namespace in "$namespace_a" "$namespace_b" "$namespace_other"; do
	if kubectl get namespace "$namespace" >/dev/null 2>&1; then
		echo "fixture namespace already exists: $namespace" >&2
		exit 1
	fi
	done
if kubectl get crd "$widget_crd" >/dev/null 2>&1; then
	echo "fixture CRD already exists: $widget_crd" >&2
	exit 1
fi

for namespace in "$namespace_a" "$namespace_b" "$namespace_other"; do
	kubectl create namespace "$namespace" >/dev/null
	kubectl label namespace "$namespace" "$label_selector" "$run_label" >/dev/null
done

kubectl apply -f - >/dev/null <<EOF
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: ${widget_crd}
  labels:
    multi-get.e2e: "true"
    multi-get.run: "${run_id}"
spec:
  group: e2e.multi-get.example
  names:
    plural: widgets
    singular: widget
    kind: Widget
    shortNames:
      - wid
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
EOF
kubectl wait --for=condition=Established "crd/${widget_crd}" --timeout=60s >/dev/null

for namespace in "$namespace_a" "$namespace_b"; do
	kubectl apply -n "$namespace" -f - >/dev/null <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${prefix}-config
  labels:
    app: multi-get-e2e
    multi-get.e2e: "true"
    multi-get.run: "${run_id}"
data:
  value: "${namespace}"
---
apiVersion: v1
kind: Secret
metadata:
  name: ${prefix}-secret
  labels:
    app: multi-get-e2e
    multi-get.e2e: "true"
    multi-get.run: "${run_id}"
type: Opaque
stringData:
  value: "fixture"
---
apiVersion: v1
kind: Service
metadata:
  name: ${prefix}-service
  labels:
    app: multi-get-e2e
    multi-get.e2e: "true"
    multi-get.run: "${run_id}"
spec:
  selector:
    app: multi-get-e2e
  ports:
    - port: 80
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${prefix}-deployment
  labels:
    app: multi-get-e2e
    multi-get.e2e: "true"
    multi-get.run: "${run_id}"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: multi-get-e2e
  template:
    metadata:
      labels:
        app: multi-get-e2e
        multi-get.e2e: "true"
        multi-get.run: "${run_id}"
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.9
          ports:
            - containerPort: 8080
---
apiVersion: e2e.multi-get.example/v1
kind: Widget
metadata:
  name: ${prefix}-widget
  labels:
    app: multi-get-e2e
    multi-get.e2e: "true"
    multi-get.run: "${run_id}"
spec: {}
EOF
done

kubectl multi-get get pods -n "${namespace_a},${namespace_b}" >/dev/null
kubectl multi-get get pods -n "${namespace_b},${namespace_a}" >/dev/null
kubectl multi-get get pods --find-ns "${prefix}-dev" >/dev/null
kubectl multi-get get pods -A >/dev/null
kubectl multi-get get pods,services -n "${namespace_a},${namespace_b}" >/dev/null
kubectl multi-get get pods -n "${namespace_a},${namespace_b}" -l app=multi-get-e2e >/dev/null
kubectl multi-get get pods -n "${namespace_a},${namespace_b}" --field-selector "metadata.namespace=${namespace_a}" >/dev/null
kubectl multi-get get pods -n "${namespace_a},${namespace_b}" -o wide >/dev/null

json_output="$(kubectl multi-get get pods -n "${namespace_a},${namespace_b}" -o json)"
grep -q '"kind": "List"' <<<"$json_output"
grep -q "${namespace_a}" <<<"$json_output"
kubectl multi-get get pods -n "${namespace_a},${namespace_b}" -o yaml >/dev/null
kubectl multi-get get pods -n "${namespace_a},${namespace_b}" -o name >/dev/null
kubectl multi-get get widgets -n "${namespace_a},${namespace_b}" >/dev/null

if kubectl multi-get get pods --find-ns "${prefix}-missing" >/dev/null 2>&1; then
	echo "expected no-match to fail" >&2
	exit 1
fi
if kubectl multi-get get pods,services "${prefix}-pod" -n "$namespace_a" >/dev/null 2>&1; then
	echo "expected multi-resource named lookup to fail" >&2
	exit 1
fi
if kubectl multi-get get pods,services -o json -n "$namespace_a" >/dev/null 2>&1; then
	echo "expected multi-resource structured output to fail" >&2
	exit 1
fi
if kubectl multi-get get nodes -A >/dev/null 2>&1; then
	echo "expected cluster-scoped lookup to fail" >&2
	exit 1
fi

echo "local E2E passed"
