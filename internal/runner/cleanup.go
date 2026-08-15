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
	Orphans    int      `json:"orphans,omitempty"`
}

// CleanupWaitTimeout scales namespace termination wait for large OVN deletes.
// Floor 15m, +5s per NS, cap 45m.
func CleanupWaitTimeout(nsCount int) time.Duration {
	d := 15*time.Minute + time.Duration(nsCount)*5*time.Second
	if d < 15*time.Minute {
		d = 15 * time.Minute
	}
	if d > 45*time.Minute {
		d = 45 * time.Minute
	}
	return d
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
	if opts.DryRun {
		log("dry-run — skip namespace delete")
	} else if len(names) == 0 {
		log("no managed namespace objects — will still reap labeled orphans")
	} else {
		for _, name := range names {
			log("deleting " + name)
			if err := opts.Cluster.DeleteNamespace(ctx, name); err != nil {
				log("FAILED delete " + name + ": " + err.Error())
				return res, fmt.Errorf("delete namespace %s: %w", name, err)
			}
		}
		if opts.Wait {
			timeout := opts.WaitTimeout
			if timeout <= 0 {
				timeout = CleanupWaitTimeout(len(names))
			}
			log(fmt.Sprintf("waiting up to %s for namespaces to terminate (%d targeted)", timeout, len(names)))
			deadline := time.Now().Add(timeout)
			for time.Now().Before(deadline) {
				left, err := opts.Cluster.ListManagedNamespaces(ctx, opts.RunID)
				if err != nil {
					return res, err
				}
				res.Remaining = left
				if len(left) == 0 {
					log("all targeted namespaces gone")
					break
				}
				log(fmt.Sprintf("still terminating: %d left · %s remaining", len(left), time.Until(deadline).Round(time.Second)))
				if err := sleep(ctx, 5*time.Second); err != nil {
					return res, err
				}
			}
			if len(res.Remaining) > 0 {
				log(fmt.Sprintf("timed out with %d remaining", len(res.Remaining)))
				return res, fmt.Errorf("timed out waiting for %d namespace(s) to terminate", len(res.Remaining))
			}
		} else {
			log("delete issued (not waiting for termination)")
		}
	}

	if reaper, ok := opts.Cluster.(kube.ManagedReaper); ok {
		n, err := reaper.ReapLabeled(ctx, opts.RunID, opts.DryRun, log)
		res.Orphans = n
		if err != nil {
			return res, err
		}
	}
	return res, nil
}
