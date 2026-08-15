# Isolated Wave Test Approach / Mode

North star for density work. The live UI page is **Isolated wave** (`/isolation`); this file is the same protocol in git.

A 14-wave dump cannot tell you which batch created kube-apiserver RSS, or whether that RSS ever comes back. Each wave is its own closed loop:

1. **Baseline** — RSS flat, 3/3 Ready, oauth 3/3.
2. **Apply only wave k** — no B{k+1}.
3. **Settle** — Δ RSS, LIST, inflight, etcd apply, ovn-node.
4. **Delete that wave only** — same pace as create.
5. **Give-back** — Terminating=0 vs kas RSS. Leftover floor is ETCD008.
6. **Reset / next k** — leftover floor or kas static-pod restart. Repeat until a wave does not settle.

Causality: API watch-cache → LIST/inflight → etcd → master RAM → oauth on that master → OVN reconnect.

Recovery is a second mutating load. It is part of the loop.

See `docs/OCP-SOURCE-MAP.md` for `watch_cache.go` Delete / `resizeCacheLocked`.
The leftover-RSS finding is investigation `watch-cache-shrink-without-full` — UI **Investigations**, `docs/INVESTIGATIONS.md`.
