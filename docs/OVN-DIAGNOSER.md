# OVN-Kube Diagnoser (v0.2)

First-class component on the `development` branch. **Not** a kube-burner metrics consumer.

| Concern | Owner |
|---------|--------|
| Workload apply / convergence / Prom index | kube-burner + existing report |
| “Is OVN-Kubernetes becoming unhealthy as load is injected, and what is the earliest evidence?” | **`internal/ovndiag`** |

## Architecture (target)

```
BENCHMARK RUN
     │
┌────┴────┐
│         │
kube-burner   OVN Diagnoser (continuous inspection)
│             Nodes / OVN pods / events / OVN·OVS / DB / dataplane
│                      │
│               Findings + Timeline + Health state
└──────────┬───────────┘
           ▼
    Diagnostic report / UI panel
```

## Layers

| Layer | Focus | MVP on this branch |
|-------|--------|-------------------|
| L1 | Node conditions + transitions | yes |
| L2 | OVN-Kube pod health + restart deltas | yes |
| L3 | Process CPU/mem (via metrics when available) | stub |
| L4 | OVN DB latency / connectivity | stub |
| L5 | Network config consistency | stub |
| L6 | Dataplane probes (pod/svc/route) | stub |
| L7 | Correlation to batch lifecycle | yes (batch markers) |

## Package layout

```
internal/ovndiag/
  types.go       HealthState, Finding, Evidence, OVNNodeHealth, Snapshot
  catalog.go     Rule IDs OVN001…
  baseline.go    Per-node baseline samples + anomaly vs range
  watch.go       Periodic ClusterWatch → Snapshot
  evaluate.go    L1/L2 evaluators → findings
  correlate.go   Batch markers → timeline
  store.go       Immutable snapshot under runDir/ovndiag/
```

API: `GET /api/v1/ovndiag` (latest), `POST /api/v1/ovndiag/baseline`, `POST /api/v1/ovndiag/sample`  
UI: OVN Diagnoser panel on Health (overall + per-node + timeline).

Capability discovery (OCP/OVN version) gates later layers — do not hard-code OpenShift minor assumptions.
