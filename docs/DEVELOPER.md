# Developer workflow — dasm-burner

Same dual-env model as [mock-me DEVELOPER.md](https://github.com/dasmlab/mock-me/blob/main/docs/DEVELOPER.md) and interview-me / dasmlab-home.

| Track | Git branch | live-cicd path | Argo app | Namespace | Hostname |
|-------|------------|----------------|----------|-----------|----------|
| **Prod** | `main` only | `clusters/2026-prod-1/dasm-burner/live/` | `dasm-burner` | `dasm-burner-system` | `dasm-burner.apps.2026-prod-1.ocp.dasmlab.org` |
| **Preview** | any other (incl. `development`) | `clusters/2026-prod-1/dasm-burner/previews/{owner}.yaml` | `dasm-burner-previews` | `dasm-burner-dev-{owner}` | `dev-{owner}-dasm-burner.apps.2026-prod-1.ocp.dasmlab.org` |

`owner` = sanitized GitHub actor (lowercase, non-alnum → `-`, max 20).

There is **no** `clusters/.../dasm-burner/development/` overlay. Pushing `development` must **not** write `live/` (that was a dasm-burner bug before this workflow split).

## CI scripts (referenced from siblings)

| Script | Role | Sibling source |
|--------|------|----------------|
| `scripts/ci/deploy-preview.sh` | Render preview envelope → live-cicd `previews/{owner}.yaml` | mock-me |
| `scripts/ci/bootstrap-preview-ns.sh` | Copy OIDC secret/CA + ClusterRoleBinding for observe | mock-me |
| `scripts/ci/ensure-preview-cert.sh` | HAProxy CERT on `10.20.1.10` | mock-me / dasmlab-home |
| `scripts/ci/ensure-prod-cert.sh` | Prod FQDN CERT | same |
| `scripts/ci/ensure-keycloak-preview-uris.sh` | Explicit Keycloak redirect URIs (no wildcards) | dasmlab-home |

## Local preview publish

```bash
export VERSION_TAG=v2026.08.13-$(git rev-parse --short HEAD)
export PREVIEW_ACTOR=lmcdasm   # or your GitHub username
# build+push image first, then:
bash scripts/ci/deploy-preview.sh
```

## OVN-Kube Diagnoser (v0.2 / development)

`development` is the major-bump branch for the first-class OVN diagnoser (`internal/ovndiag`). Prod (`main`) stays on workload apply + kube-burner reports until the diagnoser is merge-ready.

See [docs/OVN-DIAGNOSER.md](OVN-DIAGNOSER.md).
