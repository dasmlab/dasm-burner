# kube-burner usage in dasm-burner

dasm-burner is a **control plane around** [kube-burner](https://kube-burner.github.io/kube-burner/), not a fork and not a second load-generator. Upstream docs:

- Product: [kube-burner](https://kube-burner.github.io/kube-burner/)
- Config (`global`, jobs, go-templates): [Configuration reference](https://kube-burner.github.io/kube-burner/latest/reference/configuration/#global)
- Source: [github.com/kube-burner/kube-burner](https://github.com/kube-burner/kube-burner)

Pinned release: **v2.8.1**. Install with `make kube-burner` → `./bin/kube-burner`. Override with `KUBE_BURNER=/path/to/kube-burner`. The product image ships the same binary at `/usr/local/bin/kube-burner` (`KUBE_BURNER` set in the Containerfile).

---

## Observability & Alerting touches (update when changing this area)

We configure kube-burner as a **local indexer** per run (not Elastic/OpenSearch; TSDB deferred).

| Touch | Detail |
|-------|--------|
| Commands | `measure`, `index`, `check-alerts` (subprocess binary v2.8.1) |
| Flags | `--uuid` (=runId), `--job-name`, `--selector`, `--user-metadata`, `--metrics-directory`, `--tarball-name`, `--indexer-type=local` |
| Rendered files | `measure.yml`, `init.yml`, `metrics.yml`, `alerts.yml`, `metrics-endpoint.yml`, `user-metadata.yml` |
| Indexer | `type: local`, `createTarball: true`, `tarballName: kube-burner-metrics.tgz` under `kube-burner/collected/` |
| Profiles | OVN per-node CPU/mem, API/etcd, instant `nodesReady` / `ovnPodsReady` with `captureStart` |
| Alerts | warning-only (API 5xx, latency, etcd fsync, OVN CPU/mem) — do not fail apply |
| Job summary | Parsed from collected `*jobSummary*.json`; user-metadata merged by kube-burner |
| UI Execute | Real (non–dry-run) runs DiscoverPrometheus → measure → index → check-alerts before snapshot freeze |
| Snapshot | `reports/<id>/` includes `snapshot.json`, `metrics/*.json`, tarball copy |
| Report UI | Open/Close, OVN pods table with **restart Δ vs open**, jobSummary, metric cards, alerts |
| Control-plane pod | Live+preview: requests `256Mi`, limits **`2Gi`** (was 1Gi — OOMKilled opening large Report snapshots) |
| ObjectPressure | Basetype `OpenShiftObjectPressure`: render `init.yml` + `objectTemplates/` via `internal/burner/pressure.go`; live apply is **`kube-burner init`** (density stays client-go) |
| Unused | elastic/opensearch, tsdb, `kube-burner import`, churn (except ObjectPressure `init`) |

Cleanup (product, not kube-burner): async background delete with wait scaled `15m + 5s*NS` (cap 45m), **not** tied to HTTP/`r.Context()` (avoids OCP route ~30s cancel).

---

## Decision: subprocess binary, not a Go import

kube-burner **is** a Go module (`github.com/kube-burner/kube-burner/v2`). We still **exec the published binary** from `internal/burner/exec.go`.

| Reason | Detail |
|--------|--------|
| Toolchain | Upstream `go.mod` requires **Go 1.25**. This repo is **Go 1.24.4** with `GOTOOLCHAIN=local`. |
| k8s stack | kube-burner pulls client-go **v0.34+ / v0.35**. We pin **k8s.io/client-go v0.31.3** for OpenShift 4.18. A library import would fight that pin and tidy toward Go 1.26. |
| Public API | `measure`, `index`, and `check-alerts` live in `cmd/`. `pkg/` is not a small, versioned library. |
| Blast radius | Importing the module also pulls kubevirt, KEDA, Prometheus client stacks we do not need in the product image. |

`dasm-burner apply --measure` therefore shells out:

```
kube-burner measure -c measure.yml --selector=dasm-burner.dasmlab.org/run=<id> ...
kube-burner index -c measure.yml --metrics-endpoint metrics-endpoint.yml ...
kube-burner check-alerts --metrics-endpoint metrics-endpoint.yml ...
```

We **do** consume kube-burner’s **configuration schema** (YAML + go-template object files). We do **not** invent a parallel job language for those files. `dasm-burner render-kube-burner` writes them.

---

## What we render vs what we invented

The canvas / `OpenShiftNetworkDensity` document is a **compact generator** for kube-burner fields. One Namespace box with a count becomes `jobIterations`; objects inside it become `objects[].replicas` and Deployment `spec.replicas`.

| Canvas / dasm-burner field | kube-burner field | Notes |
|----------------------------|-------------------|--------|
| `topology.namespaces.count` | `jobs[].jobIterations` + `namespacedIterations: true` | One NS per iteration — not 2,500 boxes |
| `topology.routes.perNamespace` | `objects[]` replica count on `objectTemplates/route.yml` | `{{.Replica}}` |
| `topology.services.perNamespace` | same on `service.yml` | `{{.Replica}}` |
| `topology.workloads.replicasPerService` | Deployment `spec.replicas` inside the template | Not kube-burner job replicas |
| `relationships.routeToService: oneToOne` | Route `spec.to.name` = matching Service via `{{.Replica}}` | Same replica index |
| `deployment.apiConcurrency` | `jobs[].qps` / `burst` | Rendered into `init.yml` |
| `monitoring.podLatency` | `global.measurements: [{name: podLatency}]` | Used by `measure` |
| `monitoring.serviceLatency` | `{name: serviceLatency, svcTimeout: 60s}` | Used by `measure` |
| Prometheus / alerts | `metrics.yml`, `alerts.yml`, `metrics-endpoint.yml` | Used by `index` / `check-alerts` |
| Labels `dasm-burner.dasmlab.org/run` | `--selector` / `--uuid` | Shared with our client-go apply |

Go-templates we emit (kube-burner’s documented `{{.Iteration}}` / `{{.Replica}}`, not a new interpolator):

```yaml
metadata:
  name: kb-<run>-deploy-{{.Iteration}}-{{.Replica}}
```

`--user-data` is unused today; env-based templating is available upstream if we need extra vars later.

---

## Per phase: kube-burner used vs dasm-burner-only

### Phase 1 — Topology

| kube-burner | dasm-burner |
|-------------|-------------|
| Object templates *would* name resources with `{{.Iteration}}` / `{{.Replica}}` | We generate a fully named graph (`kb-{runID}-{kind}-{seq:05d}-{sfx}`) so apply and measure share labels |
| Jobs / objects schema | Compact `OpenShiftNetworkDensity` (Route→Service→Pod) or `OpenShiftObjectPressure` (object palette) YAML + canvas. Both **project into** kube-burner YAML |

We do not draw N namespace boxes. The canvas is the starting template (2 NS × 2 pairs × 3 pods) with instance counts.

### Phase 2 — Apply / batching

| kube-burner | dasm-burner |
|-------------|-------------|
| `kube-burner init -c init.yml` can Create at QPS with waiters, churn, hooks, `deletionStrategy` | **Density (`OpenShiftNetworkDensity`):** we do **not** call `init` for apply. client-go owns sequential / batch / rate, readiness, and cleanup. **ObjectPressure (`OpenShiftObjectPressure`):** live apply **does** run `kube-burner init` from rendered `init.yml` + `objectTemplates/` (stock palette + custom GVK; missing optional CRDs skipped with warn) |
| `global.gc`, `waitWhenFinished` | Density: rendered for hand-runs. ObjectPressure: consumed by live `init` |

Why density apply is ours: OpenShift Routes, SCC (`runAsNonRoot`, drop ALL, no `runAsUser: 65532` on workload NS), pull-secret copy, `--allow-large` / `--i-understand-this-loads-the-control-plane`, and abort gates between batches. Those are product rules, not missing kube-burner features.

`render-kube-burner` / Topology preview still writes a complete `init.yml` for both basetypes.

### Phase 3 — Measure / index / alerts

This is where we **use kube-burner to its fullest** for the job it is best at:

| Command | What we use |
|---------|-------------|
| `kube-burner measure` | `podLatency`, `serviceLatency` against `--selector` of the run we applied |
| `kube-burner index` | Metrics profile + thanos-querier (`internal/burner/prometheus.go`) |
| `kube-burner check-alerts` | Alerts profile; warnings do not fail apply |

We do **not** reimplement latency histograms. Collected JSON lands in `<out>/kube-burner/collected`.

Not used (yet), and why:

| Upstream feature | Why unused |
|------------------|------------|
| `kube-burner init` as the apply engine | Density: Phase 2 owns OpenShift-safe apply. ObjectPressure: **uses** `init` |
| Churn / jobPause / hooks | Density curve is a controlled apply, not churn |
| Elasticsearch indexer | Local indexer + tarball + `dasm-burner report` |
| TSDB indexer | Deferred (Phase B); local JSON is enough for OVN report cards |
| `global.functionTemplates` | Default go-template vars are enough |
| Read / Patch jobs | Create + measure is the product |

### Phase 4 — OVN + product UI

| kube-burner | dasm-burner |
|-------------|-------------|
| `global.clusterHealth` (nodes Ready) | We keep **our** abort gates: nodes, OOMKilled, managed pod failure %, **OVN pods Ready**. Their clusterHealth is nodes-only; OVN is OpenShift-specific |
| — | `dasm-burner report` + HAWT UI. Serve is observational (does not apply) |
| — | Keycloak SSO on the UI ([KEYCLOAK_SETUP.md](KEYCLOAK_SETUP.md)) |

---

## Commands that touch kube-burner

```
dasm-burner render-kube-burner --config config/smoke.yaml --out ./run
dasm-burner apply --config config/smoke.yaml --measure --i-understand-this-loads-the-control-plane
dasm-burner measure --config config/smoke.yaml --duration 2m
```

UI **Topology → Preview init.yml** is the same renderer (`internal/burner/render.go`). Saving the canvas does not apply.

---

## What a library import would take (not planned)

If we ever imported `github.com/kube-burner/kube-burner/v2` instead of exec:

1. Bump this repo to **Go 1.25+** and drop `GOTOOLCHAIN=local`, or wait until our platform Go matches.
2. Move **client-go** to kube-burner’s version (0.34+) and re-validate OpenShift 4.18 types (Route, SCC).
3. Accept kubevirt / KEDA / Prometheus transitive deps in the product binary.
4. Call `pkg/` (config + measurements factory + prometheus scrape) ourselves — those APIs are not advertised as a library, so we would wrap `cmd` logic or ask upstream for a stable `Measure` / `Index` / `CheckAlerts`.
5. Or embed the binary (still exec) to avoid the module graph.

Until then the pinned **v2.8.1 binary** is the integration. Do not fork kube-burner.
