package cleanupwatch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const maxSamples = 200

// Sample is one periodic cluster observation during cleanup.
type Sample struct {
	At                 time.Time `json:"at"`
	NodesReady         int       `json:"nodesReady"`
	NodesNotReady      int       `json:"nodesNotReady"`
	MemoryPressure     int       `json:"memoryPressure"`
	DiskPressure       int       `json:"diskPressure"`
	PIDPressure        int       `json:"pidPressure"`
	MonitoringReady    int       `json:"monitoringReady"`
	MonitoringTotal    int       `json:"monitoringTotal"`
	MonitoringOOM      int       `json:"monitoringOOM"`
	MonitoringRestarts int       `json:"monitoringRestarts"`
	NotReadyNodes      []string  `json:"notReadyNodes,omitempty"`
}

// Incident is a notable transition detected between samples.
type Incident struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"` // node_not_ready | node_recovered | memory_pressure | prometheus_oom | pod_restart_burst
	Message string    `json:"message"`
	Node    string    `json:"node,omitempty"`
}

// Observation is frozen into a cleanup report.
type Observation struct {
	Samples           []Sample   `json:"samples,omitempty"`
	Incidents         []Incident `json:"incidents,omitempty"`
	Summary           string     `json:"summary,omitempty"`
	MaxNotReady       int        `json:"maxNotReady"`
	MaxNotReadyDurSec int64      `json:"maxNotReadyDurationSec,omitempty"`
	MonitoringOOM     int        `json:"monitoringOOMTotal"`
	WorstNodes        []string   `json:"worstNodes,omitempty"`
}

// Watcher samples nodes + openshift-monitoring during cleanup.
type Watcher struct {
	cs       kubernetes.Interface
	interval time.Duration
	logf     func(level, msg string)

	mu        sync.Mutex
	samples   []Sample
	incidents []Incident
	prev      *Sample
	notReady  map[string]time.Time // node -> since
	maxNR     int
	maxNRDur  time.Duration
	oomTotal  int
	stopCh    chan struct{}
	doneCh    chan struct{}
}

func Start(cs kubernetes.Interface, interval time.Duration, logf func(level, msg string)) *Watcher {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if logf == nil {
		logf = func(string, string) {}
	}
	w := &Watcher{
		cs:       cs,
		interval: interval,
		logf:     logf,
		notReady: map[string]time.Time{},
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
	go w.loop()
	return w
}

func (w *Watcher) Stop() Observation {
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
	<-w.doneCh
	return w.Snapshot()
}

func (w *Watcher) Snapshot() Observation {
	w.mu.Lock()
	defer w.mu.Unlock()
	obs := Observation{
		Samples:           append([]Sample(nil), w.samples...),
		Incidents:         append([]Incident(nil), w.incidents...),
		MaxNotReady:       w.maxNR,
		MaxNotReadyDurSec: int64(w.maxNRDur / time.Second),
		MonitoringOOM:     w.oomTotal,
	}
	seen := map[string]bool{}
	for _, inc := range w.incidents {
		if inc.Node != "" && !seen[inc.Node] {
			obs.WorstNodes = append(obs.WorstNodes, inc.Node)
			seen[inc.Node] = true
		}
	}
	obs.Summary = summarize(obs)
	return obs
}

func (w *Watcher) loop() {
	defer close(w.doneCh)
	w.tick(context.Background())
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-w.stopCh:
			w.tick(context.Background())
			return
		case <-t.C:
			w.tick(context.Background())
		}
	}
}

func (w *Watcher) tick(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	s, err := sampleOnce(ctx, w.cs)
	if err != nil {
		w.logf("warn", "CLUSTER sample: "+err.Error())
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.samples) >= maxSamples {
		w.samples = w.samples[1:]
	}
	w.samples = append(w.samples, s)
	if s.NodesNotReady > w.maxNR {
		w.maxNR = s.NodesNotReady
	}
	if s.MonitoringOOM > w.oomTotal {
		w.oomTotal = s.MonitoringOOM
	}
	now := s.At
	curNR := map[string]bool{}
	for _, n := range s.NotReadyNodes {
		curNR[n] = true
		if _, ok := w.notReady[n]; !ok {
			w.notReady[n] = now
			inc := Incident{At: now, Kind: "node_not_ready", Message: "node NotReady: " + n, Node: n}
			w.incidents = append(w.incidents, inc)
			w.logf("warn", "CLUSTER "+inc.Message)
		}
	}
	for n, since := range w.notReady {
		if curNR[n] {
			if d := now.Sub(since); d > w.maxNRDur {
				w.maxNRDur = d
			}
			continue
		}
		dur := now.Sub(since)
		if dur > w.maxNRDur {
			w.maxNRDur = dur
		}
		inc := Incident{At: now, Kind: "node_recovered", Message: fmt.Sprintf("node recovered: %s (NotReady %s)", n, dur.Round(time.Second)), Node: n}
		w.incidents = append(w.incidents, inc)
		w.logf("info", "CLUSTER "+inc.Message)
		delete(w.notReady, n)
	}
	if w.prev != nil {
		if s.MemoryPressure > w.prev.MemoryPressure {
			inc := Incident{At: now, Kind: "memory_pressure", Message: fmt.Sprintf("MemoryPressure nodes %d→%d", w.prev.MemoryPressure, s.MemoryPressure)}
			w.incidents = append(w.incidents, inc)
			w.logf("warn", "CLUSTER "+inc.Message)
		}
		if s.MonitoringOOM > w.prev.MonitoringOOM {
			inc := Incident{At: now, Kind: "prometheus_oom", Message: fmt.Sprintf("monitoring OOMKilled %d→%d", w.prev.MonitoringOOM, s.MonitoringOOM)}
			w.incidents = append(w.incidents, inc)
			w.logf("warn", "CLUSTER "+inc.Message)
		}
		if delta := s.MonitoringRestarts - w.prev.MonitoringRestarts; delta >= 3 {
			inc := Incident{At: now, Kind: "pod_restart_burst", Message: fmt.Sprintf("monitoring restarts +%d (now %d)", delta, s.MonitoringRestarts)}
			w.incidents = append(w.incidents, inc)
			w.logf("warn", "CLUSTER "+inc.Message)
		}
	} else {
		w.logf("info", fmt.Sprintf("CLUSTER watch start nodes Ready %d/%d monitoring %d/%d",
			s.NodesReady, s.NodesReady+s.NodesNotReady, s.MonitoringReady, s.MonitoringTotal))
	}
	cp := s
	w.prev = &cp
}

func sampleOnce(ctx context.Context, cs kubernetes.Interface) (Sample, error) {
	s := Sample{At: time.Now()}
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return s, err
	}
	for _, n := range nodes.Items {
		if nodeReady(n) {
			s.NodesReady++
		} else {
			s.NodesNotReady++
			s.NotReadyNodes = append(s.NotReadyNodes, n.Name)
		}
		for _, c := range n.Status.Conditions {
			if c.Status != corev1.ConditionTrue {
				continue
			}
			switch c.Type {
			case corev1.NodeMemoryPressure:
				s.MemoryPressure++
			case corev1.NodeDiskPressure:
				s.DiskPressure++
			case corev1.NodePIDPressure:
				s.PIDPressure++
			}
		}
	}
	pods, err := cs.CoreV1().Pods("openshift-monitoring").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, p := range pods.Items {
			if !isMonitoringPod(p.Name) {
				continue
			}
			s.MonitoringTotal++
			if podReady(p) {
				s.MonitoringReady++
			}
			s.MonitoringRestarts += restartCount(p)
			if oomKilled(p) {
				s.MonitoringOOM++
			}
		}
	}
	return s, nil
}

func isMonitoringPod(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "prometheus") || strings.Contains(n, "alertmanager") || strings.Contains(n, "thanos")
}

func nodeReady(n corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func podReady(p corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func restartCount(p corev1.Pod) int {
	n := 0
	for _, cs := range p.Status.ContainerStatuses {
		n += int(cs.RestartCount)
	}
	return n
}

func oomKilled(p corev1.Pod) bool {
	for _, cs := range p.Status.ContainerStatuses {
		if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
			return true
		}
		if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
			return true
		}
	}
	return false
}

func summarize(obs Observation) string {
	if len(obs.Incidents) == 0 && obs.MaxNotReady == 0 && obs.MonitoringOOM == 0 {
		return "No node/monitoring incidents observed during cleanup."
	}
	parts := []string{}
	if obs.MaxNotReady > 0 {
		parts = append(parts, fmt.Sprintf("max NotReady nodes=%d", obs.MaxNotReady))
	}
	if obs.MaxNotReadyDurSec > 0 {
		parts = append(parts, fmt.Sprintf("longest NotReady=%ds", obs.MaxNotReadyDurSec))
	}
	if obs.MonitoringOOM > 0 {
		parts = append(parts, fmt.Sprintf("monitoring OOM=%d", obs.MonitoringOOM))
	}
	if n := len(obs.Incidents); n > 0 {
		parts = append(parts, fmt.Sprintf("%d incident(s)", n))
	}
	if len(obs.WorstNodes) > 0 {
		parts = append(parts, "nodes: "+strings.Join(obs.WorstNodes, ", "))
	}
	return strings.Join(parts, " · ")
}

// DetectTransitions is exported for unit tests (pure incident logic helpers).
func DetectNodeTransition(prevNotReady map[string]time.Time, now time.Time, notReady []string) (still map[string]time.Time, incidents []Incident) {
	still = map[string]time.Time{}
	cur := map[string]bool{}
	for _, n := range notReady {
		cur[n] = true
		if since, ok := prevNotReady[n]; ok {
			still[n] = since
		} else {
			still[n] = now
			incidents = append(incidents, Incident{At: now, Kind: "node_not_ready", Message: "node NotReady: " + n, Node: n})
		}
	}
	for n, since := range prevNotReady {
		if cur[n] {
			continue
		}
		incidents = append(incidents, Incident{At: now, Kind: "node_recovered", Message: fmt.Sprintf("node recovered: %s (NotReady %s)", n, now.Sub(since).Round(time.Second)), Node: n})
	}
	return still, incidents
}
