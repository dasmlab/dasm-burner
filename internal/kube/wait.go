package kube

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/dasmlab/dasm-burner/internal/topology"
)

type ReadyStats struct {
	NamespacesReady  int           `json:"namespacesReady"`
	DeploymentsReady int           `json:"deploymentsReady"`
	RoutesAdmitted   int           `json:"routesAdmitted"`
	Duration         time.Duration `json:"duration"`
}

// WaitReady polls until every object in the batch is ready, or timeout.
func WaitReady(ctx context.Context, c Cluster, batch []topology.Namespace, timeout, interval time.Duration) (ReadyStats, error) {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	deadline := time.Now().Add(timeout)
	start := time.Now()
	var last ReadyStats
	for {
		select {
		case <-ctx.Done():
			last.Duration = time.Since(start)
			return last, ctx.Err()
		default:
		}
		st, err := snapshotReady(ctx, c, batch)
		st.Duration = time.Since(start)
		last = st
		if err != nil {
			return st, err
		}
		if readyComplete(st, batch) {
			return st, nil
		}
		if time.Now().After(deadline) {
			return st, fmt.Errorf("readiness timeout after %s (ns %d/%d, deploy %d/%d, routes %d/%d)",
				timeout, st.NamespacesReady, len(batch), st.DeploymentsReady, deployCount(batch), st.RoutesAdmitted, routeCount(batch))
		}
		sleep := interval
		if remain := time.Until(deadline); remain < sleep {
			sleep = remain
		}
		if sleep < 0 {
			continue
		}
		t := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			t.Stop()
			last.Duration = time.Since(start)
			return last, ctx.Err()
		case <-t.C:
		}
	}
}

func snapshotReady(ctx context.Context, c Cluster, batch []topology.Namespace) (ReadyStats, error) {
	var st ReadyStats
	for _, ns := range batch {
		got, err := c.GetNamespace(ctx, ns.Name)
		if err == nil && got.Status.Phase == corev1.NamespaceActive {
			st.NamespacesReady++
		}
		for _, p := range ns.Pairs {
			d, err := c.GetDeployment(ctx, ns.Name, p.Deployment)
			if err == nil && deploymentReady(d, p.Replicas) {
				st.DeploymentsReady++
			}
			ok, err := c.RouteAdmitted(ctx, ns.Name, p.Route)
			if err == nil && ok {
				st.RoutesAdmitted++
			}
		}
	}
	return st, nil
}

func deploymentReady(d *appsv1.Deployment, replicas int) bool {
	if d.Spec.Replicas != nil {
		replicas = int(*d.Spec.Replicas)
	}
	return int(d.Status.AvailableReplicas) >= replicas && replicas > 0
}

func readyComplete(st ReadyStats, batch []topology.Namespace) bool {
	return st.NamespacesReady == len(batch) &&
		st.DeploymentsReady == deployCount(batch) &&
		st.RoutesAdmitted == routeCount(batch)
}

func deployCount(batch []topology.Namespace) int {
	n := 0
	for _, ns := range batch {
		n += len(ns.Pairs)
	}
	return n
}

func routeCount(batch []topology.Namespace) int { return deployCount(batch) }
