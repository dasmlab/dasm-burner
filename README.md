# dasm-burner

OpenShift network-density control plane around [kube-burner](https://kube-burner.github.io/kube-burner/). Generates a controlled topology of namespaces, routes, services, and slim Deployments so you can measure API / OVN / node pressure.

**WARNING: NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT.** Lab / test only.

## Commands

```
dasm-burner plan --config config/smoke.yaml
dasm-burner generate --config config/smoke.yaml --out ./run
dasm-burner apply --config config/smoke.yaml --dry-run
dasm-burner apply --config config/smoke.yaml --i-understand-this-loads-the-control-plane --skip-baseline --measure
dasm-burner render-kube-burner --config config/smoke.yaml --out ./run
dasm-burner measure --config config/smoke.yaml --duration 2m
dasm-burner status --config config/smoke.yaml
dasm-burner report --config config/smoke.yaml --out ./run
dasm-burner serve --addr :8080 --config config/smoke.yaml --run-dir ./run
dasm-burner cleanup --config config/smoke.yaml --yes --wait
```

Real applies require `--i-understand-this-loads-the-control-plane`. More than 10 namespaces also require `--allow-large`. Abort gates run after every batch: not-Ready nodes, OOMKilled pods, managed pod failure %, and OVN pods not Ready. A 30s grace period applies before abort. The 2,500-namespace default is still a lab weapon — use `--allow-large` only when you mean it.

`deployment.batchSize` and `deployment.apiConcurrency` are independent: a batch of 50 namespaces can be written with only 20 concurrent API calls. Modes: `sequential`, `batch`, `rate`.

Default intended mix (`config/route-service-density.yaml`):

| Object | Count |
|---|---|
| Namespaces | 2,500 |
| Routes | 5,000 |
| Services | 5,000 |
| Deployments | 5,000 |
| Pods (replicas) | 15,000 |

Each namespace is two 1:1 route→service pairs, three replicas behind each Deployment. Names look like `kb-7f3a-ns-00001-x4k9`. Same `naming.seed` reproduces the same names.

Apply copies no pull secret by default (`ghcr.io/dasmlab/dasm-burner-web` is public).
Optionally set `application.imagePullSecret` / `imagePullSecretFrom` if you point at a private image.

Phase 2 still owns apply. Phase 3 renders kube-burner YAML and shells out to the pinned **kube-burner v2.8.1** binary (`make kube-burner` → `./bin/kube-burner`):

- `apply --measure` starts `kube-burner measure` (podLatency / serviceLatency) against labels `dasm-burner.dasmlab.org/run=<id>`, then `index` + `check-alerts` against OpenShift thanos-querier.
- Collected JSON lands in `<out>/kube-burner/collected`. Alerts warn; they do not fail apply.

How we use kube-burner’s config/go-templates vs what we still own: [docs/KUBE-BURNER.md](docs/KUBE-BURNER.md). UI SSO: [docs/KEYCLOAK_SETUP.md](docs/KEYCLOAK_SETUP.md). Density protocol: [docs/WAVE-ISOLATION.md](docs/WAVE-ISOLATION.md). Control-plane pins: [docs/OCP-SOURCE-MAP.md](docs/OCP-SOURCE-MAP.md). Hypotheses we can re-run from the UI: [docs/INVESTIGATIONS.md](docs/INVESTIGATIONS.md).

## Phases

1. **Topology** — config, naming, YAML generator, slim webserver.
2. **Batching** — client-go apply, sequential/batch/rate, readiness, convergence.
3. **kube-burner** — measure/index/alerts against the topology dasm-burner already applied; Prometheus via thanos-querier.
4. **OVN report + product** — `dasm-burner report` snapshots nodes/OVN/OOM/events and merges kube-burner collected metrics. Abort gates run after each apply batch. `dasm-burner serve` hosts the HAWT Quasar UI (observational — it does not apply load). Product image `ghcr.io/dasmlab/dasm-burner` ships via CX into `lmcdasm/dasmlab-live-cicd` → Argo CD `dasm-burner-system` on 2026-prod-1.

Ship path matches mock-me / interview-me: `ghcr.io/dasmlab/dasm-burner:vYYYY.MM.DD-<sha>` committed into `lmcdasm/dasmlab-live-cicd`.

Edge TLS for `dasm-burner.apps.2026-prod-1.ocp.dasmlab.org` is Let's Encrypt on HAProxy `10.20.1.10` (`new_haproxy/runme.sh`), not the OpenShift router cert. `scripts/ci/ensure-prod-cert.sh` adds `CERTn=FQDN` if missing and runs `./runme.sh`. The CX workflow does this after image smoke.
