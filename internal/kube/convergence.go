package kube

import "github.com/dasmlab/dasm-burner/internal/config"

// Convergence compares desired topology counts to a live snapshot.
type Convergence struct {
	Desired     config.Counts `json:"desired"`
	Actual      Snapshot      `json:"actual"`
	Namespaces  float64       `json:"namespacesPercent"`
	Services    float64       `json:"servicesPercent"`
	Routes      float64       `json:"routesPercent"`
	Deployments float64       `json:"deploymentsPercent"`
	Pods        float64       `json:"podsPercent"`
	ReadyPods   float64       `json:"readyPodsPercent"`
	Overall     float64       `json:"overallPercent"`
}

func ComputeConvergence(desired config.Counts, actual Snapshot) Convergence {
	c := Convergence{Desired: desired, Actual: actual}
	c.Namespaces = pct(actual.Namespaces, desired.Namespaces)
	c.Services = pct(actual.Services, desired.Services)
	c.Routes = pct(actual.Routes, desired.Routes)
	c.Deployments = pct(actual.Deployments, desired.Deployments)
	c.Pods = pct(actual.Pods, desired.Pods)
	c.ReadyPods = pct(actual.ReadyPods, desired.Pods)
	c.Overall = min4(c.Namespaces, c.Services, c.Routes, c.Deployments)
	return c
}

func pct(actual, desired int) float64 {
	if desired <= 0 {
		return 100
	}
	v := 100 * float64(actual) / float64(desired)
	if v > 100 {
		return 100
	}
	return v
}

func min4(a, b, c, d float64) float64 {
	m := a
	for _, v := range []float64{b, c, d} {
		if v < m {
			m = v
		}
	}
	return m
}
