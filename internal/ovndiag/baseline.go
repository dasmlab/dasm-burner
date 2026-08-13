package ovndiag

import (
	"sync"
	"time"
)

// Baseline holds per-node watermarks captured before load (or on demand).
type Baseline struct {
	mu         sync.RWMutex
	CapturedAt time.Time
	Restarts   map[string]int // ovn pod name → restart count
	Ready      map[string]bool
}

func NewBaseline() *Baseline {
	return &Baseline{
		Restarts: map[string]int{},
		Ready:    map[string]bool{},
	}
}

func (b *Baseline) Capture(nodes []OVNNodeHealth) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.CapturedAt = time.Now()
	b.Restarts = map[string]int{}
	b.Ready = map[string]bool{}
	for _, n := range nodes {
		if n.OVNKube.PodName != "" {
			b.Restarts[n.OVNKube.PodName] = n.OVNKube.Restarts
		}
		b.Ready[n.NodeName] = n.Node.Ready
	}
}

func (b *Baseline) RestartWatermark(pod string) (int, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.Restarts[pod]
	return v, ok
}

func (b *Baseline) ReadyWatermark(node string) (bool, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.Ready[node]
	return v, ok
}

func (b *Baseline) At() time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.CapturedAt
}
