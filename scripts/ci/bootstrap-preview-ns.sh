#!/usr/bin/env bash
# Bootstrap secrets + cluster observe binding into a dasm-burner preview namespace.
# Pattern from mock-me/scripts/ci/bootstrap-preview-ns.sh
set -euo pipefail

OWNER_RAW="${1:?usage: bootstrap-preview-ns.sh <github-username>}"
OWNER="$(echo "${OWNER_RAW}" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-+/-/g' | cut -c1-20)"
NS="dasm-burner-dev-${OWNER}"
PROD_NS="${PROD_NS:-dasm-burner-system}"

if ! command -v oc >/dev/null 2>&1; then
  echo "ERROR: oc not found on this machine" >&2
  exit 1
fi

echo "Bootstrapping secrets + RBAC for ns=${NS} (from ${PROD_NS})"
oc get ns "${NS}" >/dev/null 2>&1 || oc create namespace "${NS}"

oc apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: openshift-gitops-argocd-application-controller
  namespace: ${NS}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: admin
subjects:
  - kind: ServiceAccount
    name: openshift-gitops-argocd-application-controller
    namespace: openshift-gitops
EOF

# Preview SA needs the same cluster observe rights as prod (nodes / OVN pods / events).
oc apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: dasm-burner-observe-${NS}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: dasm-burner-observe
subjects:
  - kind: ServiceAccount
    name: dasm-burner-sa
    namespace: ${NS}
EOF

copy_secret() {
  local name="$1"
  if ! oc -n "${PROD_NS}" get secret "${name}" >/dev/null 2>&1; then
    echo "WARN: prod secret ${PROD_NS}/${name} missing — skip" >&2
    return 0
  fi
  oc -n "${PROD_NS}" get secret "${name}" -o json \
    | python3 -c '
import json,sys
o=json.load(sys.stdin)
o["metadata"]={"name":o["metadata"]["name"],"namespace":"'"${NS}"'"}
for k in ("resourceVersion","uid","creationTimestamp","managedFields","ownerReferences"):
    o.get("metadata",{}).pop(k, None)
print(json.dumps(o))
' | oc apply -f -
}

copy_secret dasm-burner-oidc

if oc -n "${PROD_NS}" get configmap dasm-burner-oidc-ca >/dev/null 2>&1; then
  oc -n "${PROD_NS}" get configmap dasm-burner-oidc-ca -o json \
    | python3 -c '
import json,sys
o=json.load(sys.stdin)
o["metadata"]={"name":o["metadata"]["name"],"namespace":"'"${NS}"'"}
for k in ("resourceVersion","uid","creationTimestamp","managedFields","ownerReferences"):
    o.get("metadata",{}).pop(k, None)
print(json.dumps(o))
' | oc apply -f -
else
  echo "WARN: prod ConfigMap ${PROD_NS}/dasm-burner-oidc-ca missing" >&2
fi

echo "Bootstrap complete for ${NS}"
