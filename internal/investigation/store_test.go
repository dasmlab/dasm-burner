package investigation

import (
	"testing"
	"time"
)

func TestCatalogWatchCacheIsSeeded(t *testing.T) {
	list, err := List(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("catalog must seed at least the watch-cache investigation")
	}
	got, err := Get(t.TempDir(), "watch-cache-shrink-without-full")
	if err != nil {
		t.Fatal(err)
	}
	if got.PossibleFix == nil || len(got.SourceFiles) == 0 {
		t.Fatal("watch-cache investigation must pin files and a possible fix")
	}
	if got.SourceFiles[0].Path != "staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go" {
		t.Fatalf("unexpected file %s", got.SourceFiles[0].Path)
	}
	if got.Status != StatusHypothesis {
		t.Fatalf("status %s", got.Status)
	}
}

func TestOverlayStatusAndEvidence(t *testing.T) {
	dir := t.TempDir()
	cur, err := Get(dir, "watch-cache-shrink-without-full")
	if err != nil {
		t.Fatal(err)
	}
	cur.Status = StatusExperiment
	cur.Notes = "trying shrink-without-full on a fork"
	if err := Save(dir, *cur); err != nil {
		t.Fatal(err)
	}
	got, err := Get(dir, "watch-cache-shrink-without-full")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusExperiment {
		t.Fatalf("status %s", got.Status)
	}
	if got.Hypothesis == "" || !got.Catalog {
		t.Fatal("catalog hypothesis must survive overlay")
	}
	if _, err := AppendEvidence(dir, got.ID, Evidence{
		At:    time.Now().UTC(),
		Note:  "capacity still at high watermark after Terminating=0",
		RunID: "wave-k",
	}); err != nil {
		t.Fatal(err)
	}
	got, err = Get(dir, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Evidence) < 2 {
		t.Fatalf("want catalog evidence plus append, got %d", len(got.Evidence))
	}
}

func TestCreateLocalInvestigation(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, Investigation{
		ID:         "oauth-unauthorized-on-sick-master",
		Title:      "Unauthorized when oauth-apiserver is 0/1 on the fat master",
		Status:     StatusOpen,
		Pieces:     []string{"oauth-apiserver", "kube-apiserver"},
		Hypothesis: "token review fails because the replica is pinned to the dying master",
		Protocol:   "isolated-wave",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := Get(dir, "oauth-unauthorized-on-sick-master")
	if err != nil {
		t.Fatal(err)
	}
	if got.Catalog {
		t.Fatal("local items are not catalog")
	}
	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 {
		t.Fatalf("want catalog + local, got %d", len(list))
	}
}
