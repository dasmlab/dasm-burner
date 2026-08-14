package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/runner"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

func (s *Server) clusterCapacityAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	live, target, err := s.liveTyped()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	avoid := config.DefaultAvoidTaints()
	podsAsked, waveNS, podsPerNS := 0, 0, 0
	if tmpl, lerr := s.loadActiveOrDefault(); lerr == nil && tmpl != nil {
		if len(tmpl.Application.AvoidTaints) > 0 {
			avoid = tmpl.Application.AvoidTaints
		}
		c := tmpl.Counts()
		podsAsked = c.Pods
		if c.Namespaces > 0 {
			podsPerNS = c.Pods / c.Namespaces
		}
		fake := make([]topology.Namespace, c.Namespaces)
		plan := runner.PlanBatches(tmpl, fake)
		waveNS = plan.Size
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	cap, err := kube.EvaluateDensityCapacity(ctx, live.Clientset(), avoid, podsAsked, waveNS, podsPerNS)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	managed := 0
	if names, err := live.ListManagedNamespaces(ctx, ""); err == nil {
		managed = len(names)
	}
	mcp, _ := kube.ReadMCPRoll(ctx, live.Dynamic(), kube.WorkerMCPName)
	maxPods, _ := kube.ReadWorkerMaxPods(ctx, live.Clientset(), live.Dynamic())
	writeJSON(w, http.StatusOK, map[string]any{
		"cluster":      target.Name,
		"server":       target.Server,
		"source":       target.Source,
		"capacity":     cap,
		"managedTotal": managed,
		"mcp":          mcp,
		"maxPods":      maxPods,
		"warning":      "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) loadActiveOrDefault() (*config.Config, error) {
	return s.cfg()
}

func (s *Server) clusterMaxPodsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getWorkerMaxPods(w, r)
	case http.MethodPost:
		s.postWorkerMaxPods(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getWorkerMaxPods(w http.ResponseWriter, r *http.Request) {
	live, target, err := s.liveTyped()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	st, err := kube.ReadWorkerMaxPods(ctx, live.Clientset(), live.Dynamic())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	sink := s.ensureLogSink()
	sink.appendLog("info", "KUBELET", 0, fmt.Sprintf("current maxPods on %s · desired=%v live=%d–%d · rollout=%s (%s)",
		target.Name, st.Desired, st.ObservedMin, st.ObservedMax, st.Rollout, st.RolloutReason))
	writeJSON(w, http.StatusOK, map[string]any{
		"cluster": target.Name,
		"server":  target.Server,
		"source":  target.Source,
		"maxPods": st,
		"run":     sink.snapshotSlim(),
		"stream":  eventsPath,
		"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) postWorkerMaxPods(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MaxPods int  `json:"maxPods"`
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := kube.ValidateMaxPods(body.MaxPods); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !body.Confirm {
		writeError(w, http.StatusBadRequest, fmt.Errorf("refusing to change maxPods without confirm=true (cleans kb-* NS and serially reboots worker nodes)"))
		return
	}

	m := s.execMgr()
	m.mu.Lock()
	if m.cur != nil && m.cur.Status == "running" {
		m.mu.Unlock()
		writeError(w, http.StatusConflict, fmt.Errorf("cannot change maxPods while a test run is in progress"))
		return
	}
	m.mu.Unlock()

	s.mu.Lock()
	if s.cleanupBusy {
		s.mu.Unlock()
		writeError(w, http.StatusConflict, fmt.Errorf("cleanup or cluster job already in progress"))
		return
	}
	s.cleanupBusy = true
	s.mu.Unlock()

	live, target, err := s.liveTyped()
	if err != nil {
		s.mu.Lock()
		s.cleanupBusy = false
		s.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	sink := s.ensureLogSink()
	sink.appendLog("info", "KUBELET", 0, fmt.Sprintf("maxPods=%d on %s (serial worker roll; will clean managed kb-* first)", body.MaxPods, target.logLine()))

	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"async":    true,
		"maxPods":  body.MaxPods,
		"cluster":  target.Name,
		"run":      sink.snapshotSlim(),
		"stream":   eventsPath,
		"warning":  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		"note":     "Cleans managed namespaces, sets worker MCP maxUnavailable=1, applies KubeletConfig. Watch the live log over SSE.",
	})

	go s.runMaxPodsJob(live, target, body.MaxPods, sink)
}

func (s *Server) liveTyped() (*kube.Live, clusterTarget, error) {
	target, err := s.snapshotTarget()
	if err != nil {
		return nil, target, err
	}
	cl, err := target.client(20, 40)
	if err != nil {
		return nil, target, err
	}
	live, ok := cl.(*kube.Live)
	if !ok || live.Clientset() == nil || live.Dynamic() == nil {
		return nil, target, fmt.Errorf("live OpenShift client required")
	}
	return live, target, nil
}

func (s *Server) runMaxPodsJob(live *kube.Live, target clusterTarget, maxPods int, sink *execRun) {
	defer func() {
		s.mu.Lock()
		s.cleanupBusy = false
		s.mu.Unlock()
	}()
	logf := func(level, kind, msg string) {
		sink.appendLog(level, kind, 0, msg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	names, err := live.ListManagedNamespaces(ctx, "")
	if err != nil {
		logf("error", "CLEANUP", "list managed NS: "+err.Error())
		return
	}
	if len(names) > 0 {
		logf("warn", "CLEANUP", fmt.Sprintf("deleting %d managed kb-* namespace(s) before kubelet roll", len(names)))
		waitTO := runner.CleanupWaitTimeout(len(names))
		_, err := runner.Cleanup(ctx, runner.CleanupOptions{
			Cluster:     live,
			RunID:       "",
			Wait:        true,
			WaitTimeout: waitTO,
			Log:         func(msg string) { logf("info", "CLEANUP", msg) },
		})
		if err != nil {
			logf("error", "CLEANUP", "cleanup failed — aborting maxPods change: "+err.Error())
			return
		}
	} else {
		logf("info", "CLEANUP", "no managed kb-* namespaces")
	}

	logf("info", "MCP", "set worker MachineConfigPool maxUnavailable=1 (serial)")
	if err := kube.EnsureWorkerMCPSerial(ctx, live.Dynamic()); err != nil {
		logf("error", "MCP", err.Error())
		return
	}
	logf("info", "KUBELET", fmt.Sprintf("apply KubeletConfig %s maxPods=%d", kube.WorkerKubeletConfigName, maxPods))
	if err := kube.ApplyWorkerMaxPods(ctx, live.Dynamic(), maxPods); err != nil {
		logf("error", "KUBELET", err.Error())
		return
	}

	deadline := time.Now().Add(90 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		roll, err := kube.ReadMCPRoll(ctx, live.Dynamic(), kube.WorkerMCPName)
		if err != nil {
			logf("warn", "MCP", err.Error())
		} else if roll.Summary != last {
			logf("info", "MCP", roll.Summary)
			last = roll.Summary
			if roll.Updated && !roll.Updating && !roll.Degraded && roll.UpdatedCount == roll.MachineCount && roll.MachineCount > 0 {
				logf("info", "KUBELET", fmt.Sprintf("worker pool rolled · maxPods=%d on %s — re-check capacity then Execute", maxPods, target.Name))
				return
			}
			if roll.Degraded {
				logf("error", "MCP", "worker pool degraded during roll — inspect MachineConfigPool/worker")
				return
			}
		}
		select {
		case <-ctx.Done():
			logf("warn", "MCP", "timed out waiting for worker roll; check: oc get mcp worker")
			return
		case <-time.After(15 * time.Second):
		}
	}
	logf("warn", "MCP", "still rolling after 90m — check: oc get mcp worker / oc get nodes")
}
