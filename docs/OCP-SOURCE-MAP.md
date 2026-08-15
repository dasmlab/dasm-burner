# Control-plane source map

Pin **this cluster first**, then the same recipe works for any OpenShift version.

Live TEST3 (`test-ovn-perf`) on 15 Aug 2026:

| Field | Value |
|-------|--------|
| OpenShift | **4.21.10** |
| Release image | `quay.io/openshift-release-dev/ocp-release@sha256:5d591a70c92a6dfa3b6b948ffe5e5eac7ab339c49005744006aa0dd9d6d98898` |
| `oc version` Kubernetes | **v1.34.6** (`gitCommit` `e2af6481599baf6f7b9b252365ca5826f76258c2` = upstream tag) |
| kube-apiserver image | `ocp-v4.0-art-dev@sha256:670e9e53951ccd839766cbaff061f418b6056ad1f7f853e8a6d1a4dbe19ab330` |
| etcd image | `ocp-v4.0-art-dev@sha256:27dee1a57630f3ba23d1ff83f3425bdc7449be9978f98f4b89d3e289e142af57` |

Payload page: [4.21.10](https://amd64.ocp.releases.ci.openshift.org/releasetag/4.21.10) · changelog [4.21.9 → 4.21.10](https://amd64.ocp.releases.ci.openshift.org/changelog?from=4.21.9&to=4.21.10)

## Refresh for another version

```bash
VER=4.21.10   # or whatever oc get clusterversion reports
oc adm release info "$VER" --commits
oc version
oc get clusterversion version -o jsonpath='{.status.desired.image}{"\n"}'
```

`--commits` is the map. Image SHAs on running pods confirm the payload actually booted.

## Repos and commits (4.21.10)

RH ships **forks**. Read the fork at the payload SHA; use upstream tags only to see what was carried.

| Component | What it is | Repo | Payload commit | Upstream tag to diff |
|-----------|------------|------|----------------|----------------------|
| **kube-apiserver** (hyperkube) | API process on each master | [openshift/kubernetes](https://github.com/openshift/kubernetes) `release-4.21` | [`dfffacdf0ad6e9aa75664c7b3167dd2ddbfc17ba`](https://github.com/openshift/kubernetes/commit/dfffacdf0ad6e9aa75664c7b3167dd2ddbfc17ba) | [kubernetes/kubernetes v1.34.6](https://github.com/kubernetes/kubernetes/releases/tag/v1.34.6) · bump PR [openshift/kubernetes#2634](https://github.com/openshift/kubernetes/pull/2634) |
| **KAO** | flags, serving-cert, static-pod revision | [openshift/cluster-kube-apiserver-operator](https://github.com/openshift/cluster-kube-apiserver-operator) | [`8ee10fb411a0c7f0e91b3e6d9e3bd3843a93e882`](https://github.com/openshift/cluster-kube-apiserver-operator/commit/8ee10fb411a0c7f0e91b3e6d9e3bd3843a93e882) | — |
| **etcd** | etcd 3.6 RH carry | [openshift/etcd](https://github.com/openshift/etcd) | [`806f690e1f140e0aea2eb05ef5f288b756b62895`](https://github.com/openshift/etcd/commit/806f690e1f140e0aea2eb05ef5f288b756b62895) (`CNTRLPLANE-1414` / etcd 3.6) | [etcd-io/etcd v3.6.x](https://github.com/etcd-io/etcd) (k8s 1.34 client libs are **v3.6.4**; payload etcd is the 3.6 rebase) |
| **CEO** | etcd static pod, quorum, member replace | [openshift/cluster-etcd-operator](https://github.com/openshift/cluster-etcd-operator) | [`5c38f917a43058c73479673a05d4e782524a3a41`](https://github.com/openshift/cluster-etcd-operator/commit/5c38f917a43058c73479673a05d4e782524a3a41) | — |
| **ovn-kube** | ovnkube-node / control-plane | [openshift/ovn-kubernetes](https://github.com/openshift/ovn-kubernetes) `release-4.21` | [`0fd9d309727f67d7648d0fbfa29bdbbdfdf14ae3`](https://github.com/openshift/ovn-kubernetes/commit/0fd9d309727f67d7648d0fbfa29bdbbdfdf14ae3) | [ovn-org/ovn-kubernetes](https://github.com/ovn-org/ovn-kubernetes) |
| **CNO** | which CNI, MTU, gateway | [openshift/cluster-network-operator](https://github.com/openshift/cluster-network-operator) | [`259ea6b0`](https://github.com/openshift/cluster-network-operator) (payload short SHA; refresh with `--commits`) | — |
| **oauth-apiserver** | token review (Unauthorized when this replica is 0/1) | [openshift/oauth-apiserver](https://github.com/openshift/oauth-apiserver) | [`71c41b2d8abb0c6ad90dca286baf5d03c1340646`](https://github.com/openshift/oauth-apiserver/commit/71c41b2d8abb0c6ad90dca286baf5d03c1340646) | — |
| **oauth-server** | `oauth-openshift` pods | [openshift/oauth-server](https://github.com/openshift/oauth-server) | [`2b8183592190365c269ca0c92b1955bbad9a0236`](https://github.com/openshift/oauth-server/commit/2b8183592190365c269ca0c92b1955bbad9a0236) | — |
| **auth operator** | rolls oauth when masters flap | [openshift/cluster-authentication-operator](https://github.com/openshift/cluster-authentication-operator) | [`d235c0bb`](https://github.com/openshift/cluster-authentication-operator) | — |

`oc version` `gitCommit` **e2af648** is the **upstream** v1.34.6 commit baked into the binary. The tree to clone for RH carry patches is **openshift/kubernetes @ dfffacdf**.

## Clone (shallow)

```bash
git clone --filter=blob:none --branch release-4.21 \
  https://github.com/openshift/kubernetes.git ocp-k8s-4.21
cd ocp-k8s-4.21 && git checkout dfffacdf0ad6e9aa75664c7b3167dd2ddbfc17ba

git clone --filter=blob:none https://github.com/openshift/etcd.git ocp-etcd
cd ocp-etcd && git checkout 806f690e1f140e0aea2eb05ef5f288b756b62895

git clone --filter=blob:none --branch release-4.21 \
  https://github.com/openshift/ovn-kubernetes.git ocp-ovn-4.21
cd ocp-ovn-4.21 && git checkout 0fd9d309727f67d7648d0fbfa29bdbbdfdf14ae3
```

## First files for RSS / LIST latency / leftover heap

These are the places the density cascade should show up. Paths are relative to **openshift/kubernetes @ dfffacdf** unless noted.

### kube-apiserver — watch cache (DELETE does not shrink the ring)

**This is the leftover-RSS finding.** File:
[`staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go`](https://github.com/kubernetes/kubernetes/blob/v1.34.6/staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go) (upstream v1.34.6; same path on the RH fork).

| Function | Lines (v1.34.6) | What it actually does |
|----------|-----------------|------------------------|
| `Delete` | [256–266](https://github.com/kubernetes/kubernetes/blob/v1.34.6/staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go#L256-L266) | Builds `watch.Event{Type: Deleted}` and calls `processEvent`. The **store** drops the live object. The **event ring does not shrink**. |
| `processEvent` / `updateCache` | [282–368](https://github.com/kubernetes/kubernetes/blob/v1.34.6/staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go#L282-L368) | Every Delete **appends** a `watchCacheEvent` (object + PrevObject) into the cyclic buffer. |
| `resizeCacheLocked` | [370–388](https://github.com/kubernetes/kubernetes/blob/v1.34.6/staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go#L370-L388) | Grow 2x **or** shrink 2x — **both** require `isCacheFullLocked()`. After cleanup the ring is usually *not* full, so capacity stays at the high watermark (100 … 102400). |
| `isCacheFullLocked` | [392–394](https://github.com/kubernetes/kubernetes/blob/v1.34.6/staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go#L392-L394) | `endIndex == startIndex+capacity` |

Go still will not return RSS to the OS after GC. Static-pod restart is the only sure process RSS reset. The **testable** k8s change is: shrink when occupancy ≪ capacity and events are older than `eventFreshDuration`, without requiring full. Metric: `apiserver_watch_cache_capacity` at baseline vs Terminating=0.

UI: Investigations (`watch-cache-shrink-without-full`) · Isolated wave · Source map (kube-apiserver piece).
Catalog + PVC overlay: `docs/INVESTIGATIONS.md`.

### etcd 3.6 — write/delete storm, not DB bytes

In **openshift/etcd @ 806f690e**:

- `server/etcdserver/server.go` — apply loop, slow apply
- `server/mvcc/kvstore.go` / `server/mvcc/watchable_store.go` — watchers (every kas + every kubelet)
- `server/storage/backend/` — bbolt; process RSS ≠ `etcdctl endpoint status` DB size
- `server/etcdserver/api/v3rpc/` — gRPC; timeout here is `etcd_flex` after API saturates

k8s 1.34 still talks etcd via `go.etcd.io/etcd/client/v3 v3.6.4` (see upstream `go.mod`). Payload **server** is the 3.6 rebase above.

### ovn-kube — follower once API/nodes flap

In **openshift/ovn-kubernetes @ 0fd9d309**:

- `go-controller/pkg/node/` — ovnkube-node (per-worker RSS we still saw ~1.2 Gi during Terminating)
- `go-controller/pkg/ovn/` — ovnkube-control-plane
- `go-controller/pkg/informer/` / watch clients — reconnect storm when kas dies

### oauth (Unauthorized while 2 masters look fine)

- [openshift/oauth-apiserver](https://github.com/openshift/oauth-apiserver/tree/71c41b2d8abb0c6ad90dca286baf5d03c1340646) — token review backend
- kube-apiserver aggregator points at that Service; one 0/1 replica on the sick master → `Unauthorized`

## Version map (fill as we hit other clusters)

| OCP | k8s | etcd fork | ovn-kube | notes |
|-----|-----|-----------|----------|-------|
| **4.21.10** | 1.34.6 | 3.6 @ 806f690e | 0fd9d309 | TEST3 / this burn |
| 4.x.y | `oc adm release info 4.x.y --commits` | | | add a row when we pin the next lab |

Do not assume 4.20 is etcd 3.6 — k8s 1.33 and below were 3.5.x.
