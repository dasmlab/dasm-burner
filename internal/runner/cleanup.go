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
}

type CleanupResult struct {
	DryRun     bool     `json:"dryRun"`
	RunID      string   `json:"runId"`
	Namespaces []string `json:"namespaces"`
	Remaining  []string `json:"remaining,omitempty"`
}

func Cleanup(ctx context.Context, opts CleanupOptions) (*CleanupResult, error) {
	if opts.Cluster == nil {
		return nil, fmt.Errorf("cluster client is required")
	}
	names, err := opts.Cluster.ListManagedNamespaces(ctx, opts.RunID)
	if err != nil {
		return nil, err
	}
	res := &CleanupResult{DryRun: opts.DryRun, RunID: opts.RunID, Namespaces: names}
	if opts.DryRun {
		return res, nil
	}
	for _, name := range names {
		if err := opts.Cluster.DeleteNamespace(ctx, name); err != nil {
			return res, fmt.Errorf("delete namespace %s: %w", name, err)
		}
	}
	if !opts.Wait {
		return res, nil
	}
	timeout := opts.WaitTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		left, err := opts.Cluster.ListManagedNamespaces(ctx, opts.RunID)
		if err != nil {
			return res, err
		}
		res.Remaining = left
		if len(left) == 0 {
			return res, nil
		}
		if err := sleep(ctx, 2*time.Second); err != nil {
			return res, err
		}
	}
	return res, fmt.Errorf("timed out waiting for %d namespace(s) to terminate", len(res.Remaining))
}
