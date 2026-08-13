package ovndiag

import (
	"fmt"
	"sync"
	"time"
)

// Baseline holds per-node watermarks captured before load (or on demand).
type Baseline struct {
	mu         sync.RWMutex
	CapturedAt time.Time
	Restarts   map[string]int // ovn pod name → restart count
	Ready      map[string]bool
	CPU        map[string]float64 // node|container → cores
	Mem        map[string]float64 // node|container → MiB
}

func NewBaseline() *Baseline {
	return &Baseline{
		Restarts: map[string]int{},
		Ready:    map[string]bool{},
		CPU:      map[string]float64{},
		Mem:      map[string]float64{},
	}
}

func resKey(node, container string) string {
	return node + "|" + container
}

func (b *Baseline) Capture(nodes []OVNNodeHealth) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.CapturedAt = time.Now()
	b.Restarts = map[string]int{}
	b.Ready = map[string]bool{}
	b.CPU = map[string]float64{}
	b.Mem = map[string]float64{}
	for _, n := range nodes {
		if n.OVNKube.PodName != "" {
			b.Restarts[n.OVNKube.PodName] = n.OVNKube.Restarts
		}
		b.Ready[n.NodeName] = n.Node.Ready
		for _, r := range n.OVNKube.Resources {
			k := resKey(n.NodeName, r.Container)
			b.CPU[k] = r.CPUCores
			b.Mem[k] = r.MemoryMiB
		}
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

func (b *Baseline) CPUWatermark(node, container string) (float64, bool) {
	if b == nil {
		return 0, false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.CPU[resKey(node, container)]
	return v, ok
}

func (b *Baseline) MemWatermark(node, container string) (float64, bool) {
	if b == nil {
		return 0, false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.Mem[resKey(node, container)]
	return v, ok
}

func (b *Baseline) At() time.Time {
	if b == nil {
		return time.Time{}
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.CapturedAt
}

func (b *Baseline) String() string {
	if b == nil {
		return "nil"
	}
	return fmt.Sprintf("baseline@%s", b.At().Format(time.RFC3339))
}
