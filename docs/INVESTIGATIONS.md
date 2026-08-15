# Investigations

First-class objects for a possible issue, a testable change, and the evidence we collect while we try it.

The live UI page is **Investigations** (`/investigations`). Catalog items ship in git so a chat finding cannot vanish. PVC overlays (`runDir/investigations/<id>.json`) hold status, notes, and extra evidence for this dasm-burner instance.

## What an investigation is

| Field | Role |
|-------|------|
| `id` | Stable slug. Catalog IDs stay forever. |
| `pieces` | Source-map piece IDs (`kube-apiserver`, `etcd`, `ovn-kube`, `oauth-apiserver`, later more). |
| `sourceFiles` | Paths + line ranges + upstream / RH-fork URLs. This is how we keep the four (or N) code pins with the hypothesis. |
| `hypothesis` | What we think is happening. |
| `metric` | What we scrape to accept or reject. |
| `protocol` | Almost always `isolated-wave`. |
| `testPlan` | Repeatable steps from the UI. |
| `possibleFix` | The patch/hack we might try later, still as an experiment — not a product change. |
| `status` | `open` → `hypothesis` → `experiment` → `patched` → `closed`. |
| `evidence` | Dated notes, run IDs, leftover RSS, capacity readings. |

Guest can read. Admin can create, change status, and append evidence.

## Seeded: watch-cache shrink without full

`watch-cache-shrink-without-full` is the leftover-RSS finding from TEST3 / OCP 4.21.10 / k8s v1.34.6.

- File: `staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go` (`Delete`, `updateCache`, `resizeCacheLocked`, `isCacheFullLocked`).
- Protocol: isolated wave k, then delete that wave only, then read `apiserver_watch_cache_capacity` next to kube-apiserver RSS while Terminating=0 — before any kas restart.
- Later: a fork patch that shrinks when occupancy ≪ capacity without requiring the ring to be full. Re-run the same investigation.

See `docs/OCP-SOURCE-MAP.md` and `docs/WAVE-ISOLATION.md`.
