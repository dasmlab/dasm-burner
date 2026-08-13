# OVN-Kube Diagnoser (v0.2 / `development`)

First-class component on the `development` branch. **Not** a kube-burner metrics consumer.

| Concern | Owner |
|---------|--------|
| Workload apply / convergence / Prom index | kube-burner + existing report |
| “Is OVN-Kubernetes becoming unhealthy as load is injected, and what is the earliest evidence?” | **`internal/ovndiag`** |

## Architecture

```
BENCHMARK RUN
     │
┌────┴────┐
│         │
kube-burner   OVN Diagnoser (active cluster interrogation)
│             L1 Nodes · L2 ovnkube pods · events · logs · L5 annotations
│                      │
│               Findings + Timeline + Why? + Health state
└──────────┬───────────┘
           ▼
    Diagnostic snapshot / UI panel
```

Execute hooks: baseline at run open; sample on each `BATCH_MEASUREMENT` / `FINAL_MEASUREMENT` (log scan every 3rd batch + final).

## Layers

| Layer | Focus | Status on `development` |
|-------|--------|-------------------------|
| L1 | Node conditions + Ready vs baseline | yes |
| L2 | OVN-Kube pod health + restart Δ | yes |
| L3 | Process CPU/mem | stub (metrics later) |
| L4 | OVN DB latency | stub |
| L5 | `k8s.ovn.org/*` annotation consistency | yes |
| L6 | Dataplane probes | stub |
| L7 | Batch correlation (OVN603) | yes |
| — | Warning event aggregation | yes |
| — | Targeted log class scanner | yes |

Annotation keys checked (observed on OCP 4.21 OVN cluster):  
`node-subnets`, `l3-gateway-config`, `node-primary-ifaddr`, `host-cidrs`, `node-chassis-id`.

## Package layout

```
internal/ovndiag/
  discovery.go   Capability discovery (no OCP minor hard-coding)
  types.go       HealthState, Finding, Evidence, OVNNodeHealth, Snapshot
  catalog.go     Rule IDs OVN001…OVN603
  baseline.go    Per-node / pod watermarks
  sample.go      L1/L2/L5 evaluate + assemble Snapshot
  network.go     Annotation drift
  events.go      Normalized Warning event buckets
  logs.go        Classed log tail (ERROR/WARN/CONNECTION/…)
  correlate.go   Multi-category → OVN603
  store.go       Immutable snapshot under runDir/ovndiag/
```

API: `GET /api/v1/ovndiag`, `POST /api/v1/ovndiag/baseline`, `POST /api/v1/ovndiag/sample`  
UI: Health page — overall / per-node / findings / timeline / Why?

## Preview GitOps

Ship on `development` via preview path only — see [DEVELOPER.md](DEVELOPER.md). Do not merge to `main` until diagnoser is proven on a preview URL.
