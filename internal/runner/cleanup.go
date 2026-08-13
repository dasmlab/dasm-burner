package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/dasmlab/dasm-burner/internal/kube"
)

type CleanupOptions struct {
	Cluster     kube.Cluster
	RunID       string
	DryRun      bool
	Wait        bool
	WaitTimeout time.Duration
	Log         func(msg string)
}

type CleanupResult struct {
	DryRun     bool     `json:"dryRun"`
	RunID      string   `json:"runId"`
	Namespaces []string `json:"namespaces"`
	Remaining  []string `json:"remaining,omitempty"`
}

func Cleanup(ctx context.Context, opts CleanupOptions) (*CleanupResult, error) {
	log := opts.Log
	if log == nil {
		log = func(string) {}
	}
	if opts.Cluster == nil {
		return nil, fmt.Errorf("cluster client is required")
	}
	target := opts.RunID
	if target == "" {
		target = "(all managed)"
	} else {
		target = "run=" + opts.RunID
	}
	log(fmt.Sprintf("listing managed namespaces for %s", target))
	names, err := opts.Cluster.ListManagedNamespaces(ctx, opts.RunID)
	if err != nil {
		return nil, err
	}
	res := &CleanupResult{DryRun: opts.DryRun, RunID: opts.RunID, Namespaces: names}
	log(fmt.Sprintf("found %d namespace(s)", len(names)))
	for _, name := range names {
		log("  · " + name)
	}
	if len(names) == 0 {
		log("nothing to delete")
		return res, nil
	}
	if opts.DryRun {
		log("dry-run — skip delete")
		return res, nil
	}
	for _, name := range names {
		log("deleting " + name)
		if err := opts.Cluster.DeleteNamespace(ctx, name); err != nil {
			log("FAILED delete " + name + ": " + err.Error())
			return res, fmt.Errorf("delete namespace %s: %w", name, err)
		}
	}
	if !opts.Wait {
		log("delete issued (not waiting for termination)")
		return res, nil
	}
	timeout := opts.WaitTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	log(fmt.Sprintf("waiting up to %s for namespaces to terminate", timeout))
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		left, err := opts.Cluster.ListManagedNamespaces(ctx, opts.RunID)
		if err != nil {
			return res, err
		}
		res.Remaining = left
		if len(left) == 0 {
			log("all targeted namespaces gone")
			return res, nil
		}
		log(fmt.Sprintf("still terminating: %d left", len(left)))
		if err := sleep(ctx, 2*time.Second); err != nil {
			return res, err
		}
	}
	log(fmt.Sprintf("timed out with %d remaining", len(res.Remaining)))
	return res, fmt.Errorf("timed out waiting for %d namespace(s) to terminate", len(res.Remaining))
}
