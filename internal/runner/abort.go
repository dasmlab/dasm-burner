package runner

import (
	"fmt"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/kube"
)

// Decision is the abort-gate verdict for one health snapshot.
type Decision struct {
	Abort  bool        `json:"abort"`
	Reason string      `json:"reason,omitempty"`
	Level  string      `json:"level"` // NORMAL, WARNING, ABORT
	Health kube.Health `json:"health"`
}

func Evaluate(safety config.Safety, h kube.Health) Decision {
	d := Decision{Health: h, Level: "NORMAL"}
	if !safety.Enabled {
		return d
	}
	var reasons []string

	if safety.AbortOn.NodeNotReady && h.NodesNotReady > safety.Thresholds.MaxNodeNotReady {
		reasons = append(reasons, fmt.Sprintf("nodes not Ready=%d (max %d)", h.NodesNotReady, safety.Thresholds.MaxNodeNotReady))
	}
	if safety.AbortOn.OOMKilled && h.OOMKilled > 0 {
		reasons = append(reasons, fmt.Sprintf("OOMKilled pods=%d", h.OOMKilled))
	}
	if h.ManagedPods > 0 && safety.Thresholds.MaxPodFailurePercent > 0 {
		failPct := 100 * float64(h.ManagedPods-h.ManagedReady) / float64(h.ManagedPods)
		if failPct > safety.Thresholds.MaxPodFailurePercent {
			reasons = append(reasons, fmt.Sprintf("managed pod not-ready %.1f%% (max %.1f%%)", failPct, safety.Thresholds.MaxPodFailurePercent))
		}
	}
	if safety.AbortOn.CriticalAlert {
		if h.OVNPods > 0 && h.OVNReady < h.OVNPods {
			reasons = append(reasons, fmt.Sprintf("OVN pods not Ready=%d/%d", h.OVNPods-h.OVNReady, h.OVNPods))
		}
	}
	if len(reasons) == 0 {
		return d
	}
	d.Abort = true
	d.Level = "ABORT"
	d.Reason = reasons[0]
	for i := 1; i < len(reasons); i++ {
		d.Reason += "; " + reasons[i]
	}
	return d
}
