package ovndiag

import (
	"context"
	"sync"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Watch periodically samples the cluster and writes snapshots under runDir.
type Watch struct {
	CS       kubernetes.Interface
	Dyn      dynamic.Interface
	Baseline *Baseline
	RunDir   string
	RunID    string
	Cluster  string
	Interval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
}

// Start begins background sampling until Stop.
func (w *Watch) Start(parent context.Context) {
	if w == nil || w.CS == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return
	}
	interval := w.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		// immediate sample
		w.tick(ctx, 0)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				w.tick(ctx, 0)
			}
		}
	}()
}

func (w *Watch) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}

func (w *Watch) tick(ctx context.Context, batchID int) {
	sctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	snap, err := SampleWith(sctx, w.CS, w.Baseline, w.RunID, w.Cluster, batchID, SampleOpts{
		ScanLogs:    true,
		MaxLogPods:  4,
		EventWindow: 10 * time.Minute,
		Dyn:         w.Dyn,
	})
	if err != nil || snap == nil {
		return
	}
	_, _ = WriteSnapshot(w.RunDir, snap)
}
