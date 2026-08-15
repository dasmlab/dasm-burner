package investigation

import "time"

const (
	StatusOpen       = "open"
	StatusHypothesis = "hypothesis"
	StatusExperiment = "experiment"
	StatusPatched    = "patched"
	StatusClosed     = "closed"
)

// Investigation is a repeatable hypothesis we can drive from the UI.
// Catalog items ship in git; PVC overlays status, evidence, and local notes.
type Investigation struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	Pieces      []string   `json:"pieces"`
	Hypothesis  string     `json:"hypothesis"`
	Metric      string     `json:"metric"`
	Protocol    string     `json:"protocol"` // isolated-wave
	TestPlan    []string   `json:"testPlan,omitempty"`
	SourceFiles []FileRef  `json:"sourceFiles,omitempty"`
	PossibleFix *Fix       `json:"possibleFix,omitempty"`
	Evidence    []Evidence `json:"evidence,omitempty"`
	Notes       string     `json:"notes,omitempty"`
	Cluster     string     `json:"cluster,omitempty"`
	OpenShift   string     `json:"openshift,omitempty"`
	Kubernetes  string     `json:"kubernetes,omitempty"`
	Catalog     bool       `json:"catalog"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type FileRef struct {
	Path    string `json:"path"`
	Lines   string `json:"lines,omitempty"`
	Why     string `json:"why"`
	URL     string `json:"url"`
	ForkURL string `json:"forkUrl,omitempty"`
}

type Fix struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Metric string `json:"metric"`
	Action string `json:"action"`
}

type Evidence struct {
	At      time.Time `json:"at"`
	Note    string    `json:"note"`
	RunID   string    `json:"runId,omitempty"`
	Cluster string    `json:"cluster,omitempty"`
}

func Catalog() []Investigation {
	t := time.Date(2026, 8, 15, 15, 30, 0, 0, time.UTC)
	return []Investigation{
		{
			ID:         "watch-cache-shrink-without-full",
			Title:      "Watch cache capacity never shrinks after DELETE unless the ring is full",
			Status:     StatusHypothesis,
			Pieces:     []string{"kube-apiserver"},
			Protocol:   "isolated-wave",
			OpenShift:  "4.21.10",
			Kubernetes: "v1.34.6",
			Catalog:    true,
			CreatedAt:  t,
			UpdatedAt:  t,
			Hypothesis: "watchCache.Delete appends a Deleted event and removes the live object from store, but resizeCacheLocked only grows or shrinks when isCacheFullLocked(). After a density cleanup the ring is usually not full, so capacity stays at the high watermark. Go also will not return RSS to the OS until the kube-apiserver static pod restarts. Two layers: ring capacity + Go heap.",
			Metric:     "apiserver_watch_cache_capacity per resource, plus kube-apiserver RSS (metrics.k8s.io), at baseline / after wave k / after Terminating=0",
			TestPlan: []string{
				"Isolated wave: baseline until kas RSS is flat.",
				"Apply only wave k. Sample capacity + RSS at settle.",
				"Delete that wave only. Sample when Terminating=0 — before any kas restart.",
				"If capacity is still at the high watermark with pods≈0, the shrink branch never ran.",
				"Optional: restart kube-apiserver static pods and confirm RSS drops (Go heap layer).",
				"Later patch: shrink when occupancy << capacity and events are older than eventFreshDuration, without requiring full. Re-run the same investigation.",
			},
			SourceFiles: []FileRef{
				{
					Path:    "staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go",
					Lines:   "256-394",
					Why:     "Delete → processEvent → updateCache; resizeCacheLocked requires isCacheFullLocked for both grow and shrink.",
					URL:     "https://github.com/kubernetes/kubernetes/blob/v1.34.6/staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go#L256-L394",
					ForkURL: "https://github.com/openshift/kubernetes/blob/dfffacdf0ad6e9aa75664c7b3167dd2ddbfc17ba/staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go",
				},
			},
			PossibleFix: &Fix{
				ID:     "watch-cache-shrink-without-full",
				Title:  "Allow watch cache shrink when occupancy is far below capacity",
				Metric: "apiserver_watch_cache_capacity + kas RSS",
				Action: "Patch resizeCacheLocked so shrink does not require isCacheFullLocked when occupancy << capacity and events are stale. Re-run isolated wave k. Compare leftover RSS vs unpatched 4.21.10.",
			},
			Evidence: []Evidence{
				{
					At:      t,
					RunID:   "90d1",
					Cluster: "test-ovn-perf",
					Note:    "After kb-*=0: kas RSS ~9.0 Gi (2248/2823/3937 Mi) vs ~14.4 Gi at cliff and ~7.6 Gi baseline. Master-2 had 35 kas restarts and was the only member that had given RAM back during Terminating; after objects were gone, 0/1 still above baseline. Cleanup deletes did not return to baseline without a cold kas.",
				},
			},
		},
	}
}
