# OVN-Kube Diagnoser (v0.2 / `development`)

First-class component on the `development` branch. **Not** a kube-burner metrics consumer.

| Concern | Owner |
|---------|--------|
| Workload apply / convergence / Prom index | kube-burner + existing report |
| “Is OVN-Kubernetes becoming unhealthy as load is injected, and what is the earliest evidence?” | **`internal/ovndiag`** |

## Layers

| Layer | Focus | Status |
|-------|--------|--------|
| L1 | Node conditions + Ready vs baseline | yes |
| L2 | OVN-Kube pod health + restart Δ | yes |
| L3 | Pod metrics (metrics.k8s.io) CPU/mem vs baseline | yes (when API available) |
| L4 | nbdb/sbdb/northd/ovn-controller Ready | yes |
| L5 | `k8s.ovn.org/*` annotation consistency | yes |
| L6 | Dataplane: OVS Ready, L3 gateway, FailedCreatePodSandBox, Pending-without-IP, drop-class logs | yes |
| L7 | Batch correlation (OVN603) | yes |
| — | Warning event aggregation | yes |
| — | Targeted log class scanner | yes |
| — | Continuous watch during Execute (45s) | yes (Execute toggle) |
| — | Sample history table + rule catalog | yes |

## Package

```
internal/ovndiag/
  discovery.go resources.go database.go network.go dataplane.go events.go logs.go
  correlate.go watch.go baseline.go sample.go store.go catalog.go types.go
```

UI: **OVN Diagnoser** nav page + Health panel summary.  
API: `GET/POST /api/v1/ovndiag`, `POST .../baseline`, `POST .../sample`

Preview only — see [DEVELOPER.md](DEVELOPER.md).
