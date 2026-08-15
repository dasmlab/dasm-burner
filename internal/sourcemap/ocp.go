package sourcemap

// TEST3 / OpenShift 4.21.10 pins. Refresh with: oc adm release info $VER --commits
var OCP42110 = ClusterMap{
	OpenShift:   "4.21.10",
	Kubernetes:  "v1.34.6",
	K8sUpstream: "e2af6481599baf6f7b9b252365ca5826f76258c2",
	ReleasePage: "https://amd64.ocp.releases.ci.openshift.org/releasetag/4.21.10",
	Recipe:      "oc adm release info $VER --commits",
	Pieces: []Piece{
		{
			ID:          "kube-apiserver",
			Name:        "kube-apiserver",
			Role:        "First flex. LIST/WATCH cache grows under density. RSS is a ratchet.",
			Repo:        "https://github.com/openshift/kubernetes",
			Clone:       "https://github.com/openshift/kubernetes.git",
			Branch:      "release-4.21",
			PayloadSHA:  "dfffacdf0ad6e9aa75664c7b3167dd2ddbfc17ba",
			UpstreamTag: "v1.34.6",
			CommitURL:   "https://github.com/openshift/kubernetes/commit/dfffacdf0ad6e9aa75664c7b3167dd2ddbfc17ba",
			Files: []FileRef{
				{
					Path:    "staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go",
					Lines:   "256-388",
					Why:     "Delete appends a Deleted event; resize only runs when the ring is full.",
					URL:     "https://github.com/kubernetes/kubernetes/blob/v1.34.6/staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go#L256-L388",
					ForkURL: "https://github.com/openshift/kubernetes/blob/dfffacdf0ad6e9aa75664c7b3167dd2ddbfc17ba/staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go",
				},
			},
			PossibleFix: &PossibleFix{
				ID:     "watch-cache-shrink-without-full",
				Title:  "Watch cache capacity never shrinks after DELETE unless the ring is still full",
				Metric: "apiserver_watch_cache_capacity (per resource) before vs after Terminating=0, with kas RSS",
				Action: "If capacity stays at the high watermark after the store is empty, try shrinking when occupancy << capacity and events are older than eventFreshDuration — today both grow and shrink require isCacheFullLocked(). Static-pod restart remains the only sure RSS reset (Go heap).",
			},
		},
		{
			ID:          "etcd",
			Name:        "etcd",
			Role:        "Dies after API saturates. Process RSS ≠ DB bytes. Delete storm is mutating load.",
			Repo:        "https://github.com/openshift/etcd",
			Clone:       "https://github.com/openshift/etcd.git",
			Branch:      "openshift-4.21-etcd-3.6",
			PayloadSHA:  "806f690e1f140e0aea2eb05ef5f288b756b62895",
			UpstreamTag: "v3.6.x",
			CommitURL:   "https://github.com/openshift/etcd/commit/806f690e1f140e0aea2eb05ef5f288b756b62895",
			Files: []FileRef{
				{Path: "server/mvcc/watchable_store.go", Why: "Watchers from every kas / kubelet.", URL: "https://github.com/openshift/etcd/blob/806f690e1f140e0aea2eb05ef5f288b756b62895/server/mvcc/watchable_store.go"},
				{Path: "server/etcdserver/server.go", Why: "Apply loop / slow apply under delete storm.", URL: "https://github.com/openshift/etcd/blob/806f690e1f140e0aea2eb05ef5f288b756b62895/server/etcdserver/server.go"},
			},
		},
		{
			ID:         "ovn-kube",
			Name:       "ovn-kube",
			Role:       "Follower once API/nodes flap. ovnkube-node RSS stays up while namespaces Terminating.",
			Repo:       "https://github.com/openshift/ovn-kubernetes",
			Clone:      "https://github.com/openshift/ovn-kubernetes.git",
			Branch:     "release-4.21",
			PayloadSHA: "0fd9d309727f67d7648d0fbfa29bdbbdfdf14ae3",
			CommitURL:  "https://github.com/openshift/ovn-kubernetes/commit/0fd9d309727f67d7648d0fbfa29bdbbdfdf14ae3",
			Files: []FileRef{
				{Path: "go-controller/pkg/node/", Why: "Per-worker ovnkube-node.", URL: "https://github.com/openshift/ovn-kubernetes/tree/0fd9d309727f67d7648d0fbfa29bdbbdfdf14ae3/go-controller/pkg/node"},
			},
		},
		{
			ID:         "oauth-apiserver",
			Name:       "oauth-apiserver",
			Role:       "Token review. One 0/1 replica on a sick master → Unauthorized while 2 masters look fine.",
			Repo:       "https://github.com/openshift/oauth-apiserver",
			Clone:      "https://github.com/openshift/oauth-apiserver.git",
			Branch:     "release-4.21",
			PayloadSHA: "71c41b2d8abb0c6ad90dca286baf5d03c1340646",
			CommitURL:  "https://github.com/openshift/oauth-apiserver/commit/71c41b2d8abb0c6ad90dca286baf5d03c1340646",
		},
	},
}

type ClusterMap struct {
	OpenShift   string  `json:"openshift"`
	Kubernetes  string  `json:"kubernetes"`
	K8sUpstream string  `json:"k8sUpstream,omitempty"`
	ReleasePage string  `json:"releasePage,omitempty"`
	Recipe      string  `json:"recipe"`
	Pieces      []Piece `json:"pieces"`
}

type Piece struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Role        string       `json:"role"`
	Repo        string       `json:"repo"`
	Clone       string       `json:"clone"`
	Branch      string       `json:"branch"`
	PayloadSHA  string       `json:"payloadSha"`
	UpstreamTag string       `json:"upstreamTag,omitempty"`
	CommitURL   string       `json:"commitUrl"`
	Files       []FileRef    `json:"files,omitempty"`
	PossibleFix *PossibleFix `json:"possibleFix,omitempty"`
}

type FileRef struct {
	Path    string `json:"path"`
	Lines   string `json:"lines,omitempty"`
	Why     string `json:"why"`
	URL     string `json:"url"`
	ForkURL string `json:"forkUrl,omitempty"`
}

type PossibleFix struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Metric string `json:"metric"`
	Action string `json:"action"`
}
