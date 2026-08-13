package runner

import (
	"fmt"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

const largeNamespaceThreshold = 10 // used by EnsureSafe; batch caps live in batchplan.go

func EnsureSafe(cfg *config.Config, dryRun, confirm, allowLarge bool) error {
	if dryRun {
		return nil
	}
	if !confirm {
		return fmt.Errorf("refusing to apply without --i-understand-this-loads-the-control-plane")
	}
	n := cfg.Topology.Namespaces.Count
	if n > largeNamespaceThreshold && !allowLarge {
		return fmt.Errorf("refusing to apply %d namespaces (>%d) without --allow-large (abort gates are on; this flag is still required)", n, largeNamespaceThreshold)
	}
	return nil
}

func SplitBatches(namespaces []topology.Namespace, size int) [][]topology.Namespace {
	if size <= 0 || size > len(namespaces) {
		if len(namespaces) == 0 {
			return nil
		}
		return [][]topology.Namespace{namespaces}
	}
	var out [][]topology.Namespace
	for i := 0; i < len(namespaces); i += size {
		end := i + size
		if end > len(namespaces) {
			end = len(namespaces)
		}
		out = append(out, namespaces[i:end])
	}
	return out
}
