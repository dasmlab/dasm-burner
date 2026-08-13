package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

func smokeCfg(namespaces int) *config.Config {
	c := config.Default()
	c.Metadata.Name = "smoke"
	c.Topology.Namespaces.Count = namespaces
	c.Naming.Seed = config.Seed{Value: 1837291}
	c.Deployment.Mode = config.DeployBatch
	c.Deployment.BatchSize = 1
	c.Deployment.BatchDelay = 0
	c.Deployment.APIConcurrency = 8
	c.Deployment.WaitForReady = true
	c.Deployment.ReadinessTimeout = config.Duration(2 * time.Second)
	c.Monitoring.Baseline.Duration = 0
	return c
}

func TestEnsureSafe(t *testing.T) {
	c := smokeCfg(2)
	if err := EnsureSafe(c, true, false, false); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSafe(c, false, false, false); err == nil {
		t.Fatal("expected confirm error")
	}
	big := smokeCfg(2500)
	if err := EnsureSafe(big, false, true, false); err == nil {
		t.Fatal("expected allow-large error")
	}
	if err := EnsureSafe(c, false, true, false); err != nil {
		t.Fatal(err)
	}
}

func TestSplitBatches(t *testing.T) {
	ns := make([]topology.Namespace, 5)
	got := SplitBatches(ns, 2)
	if len(got) != 3 || len(got[0]) != 2 || len(got[2]) != 1 {
		t.Fatalf("%v", got)
	}
}

func TestApplyFakeBatch(t *testing.T) {
	cfg := smokeCfg(2)
	g, err := topology.Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	fake := kube.NewFake()
	rep, err := Run(context.Background(), Options{
		Cluster:      fake,
		Config:       cfg,
		Graph:        g,
		SkipBaseline: true,
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Batches) != 2 {
		t.Fatalf("batches=%d", len(rep.Batches))
	}
	if fake.CreateCalls == 0 {
		t.Fatal("expected creates")
	}
	if rep.Convergence.Overall != 100 {
		t.Fatalf("convergence %+v", rep.Convergence)
	}
	snap, _ := fake.ListManaged(context.Background(), g.RunID)
	if snap.Namespaces != 2 || snap.Services != 4 || snap.Routes != 4 || snap.Deployments != 4 || snap.ReadyPods != 12 {
		t.Fatalf("snap %+v", snap)
	}
}

func TestApplyDryRunNoCluster(t *testing.T) {
	cfg := smokeCfg(2)
	g, err := topology.Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(context.Background(), Options{
		Config: cfg,
		Graph:  g,
		DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || len(rep.Batches) != 2 {
		t.Fatalf("%+v", rep)
	}
	if rep.Batches[0].NS.Created != 1 {
		t.Fatalf("dry-run created %d", rep.Batches[0].NS.Created)
	}
}

func TestApplySequential(t *testing.T) {
	cfg := smokeCfg(2)
	cfg.Deployment.Mode = config.DeploySequential
	g, _ := topology.Generate(cfg)
	rep, err := Run(context.Background(), Options{
		Cluster: kube.NewFake(), Config: cfg, Graph: g, SkipBaseline: true,
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Batches) != 2 {
		t.Fatalf("seq batches=%d", len(rep.Batches))
	}
}

func TestApplyRate(t *testing.T) {
	cfg := smokeCfg(2)
	cfg.Deployment.Mode = config.DeployRate
	cfg.Deployment.NamespacesPerSec = 100
	g, _ := topology.Generate(cfg)
	rep, err := Run(context.Background(), Options{
		Cluster: kube.NewFake(), Config: cfg, Graph: g, SkipBaseline: true,
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Batches) != 2 {
		t.Fatalf("rate batches=%d", len(rep.Batches))
	}
}

func TestIdempotentReapply(t *testing.T) {
	cfg := smokeCfg(2)
	cfg.Deployment.BatchSize = 50
	g, _ := topology.Generate(cfg)
	fake := kube.NewFake()
	opts := Options{Cluster: fake, Config: cfg, Graph: g, SkipBaseline: true, PollInterval: 10 * time.Millisecond}
	if _, err := Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	rep, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Batches[0].NS.Existing != 2 {
		t.Fatalf("expected existing namespaces, got %+v", rep.Batches[0].NS)
	}
}

func TestWaitTimeout(t *testing.T) {
	cfg := smokeCfg(1)
	cfg.Deployment.ReadinessTimeout = config.Duration(50 * time.Millisecond)
	g, _ := topology.Generate(cfg)
	fake := kube.NewFake()
	fake.NeverReady = true
	_, err := Run(context.Background(), Options{
		Cluster: fake, Config: cfg, Graph: g, SkipBaseline: true,
		PollInterval: 10 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "readiness timeout") {
		t.Fatalf("got %v", err)
	}
}

func TestAbortOnNodeNotReady(t *testing.T) {
	cfg := smokeCfg(1)
	cfg.Safety.GracePeriod = 0
	cfg.Safety.AbortOn.NodeNotReady = true
	cfg.Safety.Thresholds.MaxNodeNotReady = 0
	g, _ := topology.Generate(cfg)
	fake := kube.NewFake()
	fake.Health = kube.Health{NodesReady: 2, NodesNotReady: 1, NotReadyNodes: []string{"worker-1"}}
	_, err := Run(context.Background(), Options{
		Cluster: fake, Config: cfg, Graph: g, SkipBaseline: true,
		PollInterval: 10 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "abort:") {
		t.Fatalf("got %v", err)
	}
}

func TestEvaluateNormal(t *testing.T) {
	d := Evaluate(config.Default().Safety, kube.Health{NodesReady: 3, OVNPods: 6, OVNReady: 6})
	if d.Abort {
		t.Fatalf("%+v", d)
	}
}

func TestCleanup(t *testing.T) {
	cfg := smokeCfg(2)
	cfg.Deployment.BatchSize = 50
	g, _ := topology.Generate(cfg)
	fake := kube.NewFake()
	if _, err := Run(context.Background(), Options{
		Cluster: fake, Config: cfg, Graph: g, SkipBaseline: true,
		PollInterval: 10 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := Cleanup(context.Background(), CleanupOptions{Cluster: fake, RunID: g.RunID, Wait: true, WaitTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Namespaces) != 2 {
		t.Fatalf("deleted %+v", res.Namespaces)
	}
	snap, _ := fake.ListManaged(context.Background(), g.RunID)
	if snap.Namespaces != 0 {
		t.Fatalf("still have %+v", snap)
	}
}
