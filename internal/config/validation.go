package config

import (
	"fmt"
	"strings"
)

func Validate(c *Config) error {
	var errs []string

	if c.APIVersion != APIVersion {
		errs = append(errs, fmt.Sprintf("apiVersion must be %s", APIVersion))
	}
	if c.Kind != Kind {
		errs = append(errs, fmt.Sprintf("kind must be %s", Kind))
	}
	if strings.TrimSpace(c.Metadata.Name) == "" {
		errs = append(errs, "metadata.name is required")
	}

	if c.Topology.Namespaces.Count < 1 {
		errs = append(errs, "topology.namespaces.count must be >= 1")
	}
	if c.Topology.Services.PerNamespace < 1 {
		errs = append(errs, "topology.services.perNamespace must be >= 1")
	}
	if c.Topology.Routes.PerNamespace < 1 {
		errs = append(errs, "topology.routes.perNamespace must be >= 1")
	}
	if c.Topology.Workloads.ReplicasPerService < 1 {
		errs = append(errs, "topology.workloads.replicasPerService must be >= 1")
	}

	switch c.Topology.Workloads.Controller {
	case ControllerDeployment:
	case ControllerStatefulSet, ControllerPod:
		errs = append(errs, fmt.Sprintf("topology.workloads.controller %q is reserved for a later phase; Phase 1 supports %s", c.Topology.Workloads.Controller, ControllerDeployment))
	default:
		errs = append(errs, fmt.Sprintf("topology.workloads.controller must be %s, %s, or %s", ControllerDeployment, ControllerStatefulSet, ControllerPod))
	}

	rel := c.Topology.Relationships.RouteToService
	switch rel {
	case RelOneToOne:
		if c.Topology.Routes.PerNamespace != c.Topology.Services.PerNamespace {
			errs = append(errs, "relationships.routeToService=oneToOne requires routes.perNamespace == services.perNamespace")
		}
	case RelOneToMany, RelManyToOne:
		errs = append(errs, fmt.Sprintf("relationships.routeToService %q is reserved for a later phase; Phase 1 supports %s", rel, RelOneToOne))
	default:
		errs = append(errs, fmt.Sprintf("relationships.routeToService must be %s, %s, or %s", RelOneToOne, RelOneToMany, RelManyToOne))
	}

	if c.Application.Image == "" {
		errs = append(errs, "application.image is required")
	}
	if c.Application.Port < 1 || c.Application.Port > 65535 {
		errs = append(errs, "application.port must be 1-65535")
	}
	if c.Application.Response.Type != "podName" {
		errs = append(errs, `application.response.type must be "podName"`)
	}

	for _, p := range []struct {
		name string
		p    NamePrefix
	}{
		{"naming.namespace", c.Naming.Namespace},
		{"naming.service", c.Naming.Service},
		{"naming.route", c.Naming.Route},
		{"naming.deployment", c.Naming.Deployment},
	} {
		if strings.TrimSpace(p.p.Prefix) == "" {
			errs = append(errs, p.name+".prefix is required")
		}
		if p.p.RandomLength < 1 || p.p.RandomLength > 8 {
			errs = append(errs, p.name+".randomLength must be 1-8")
		}
	}

	if !c.Naming.Seed.Auto && c.Naming.Seed.Value == 0 {
		errs = append(errs, "naming.seed must be auto or a non-zero integer")
	}

	switch c.Deployment.Mode {
	case DeploySequential, DeployBatch, DeployRate:
	default:
		errs = append(errs, fmt.Sprintf("deployment.mode must be %s, %s, or %s", DeploySequential, DeployBatch, DeployRate))
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid config:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// Counts is the intended object arithmetic for a topology. Pods are
// Deployment replicas, not separately created Pod objects.
type Counts struct {
	Namespaces  int `json:"namespaces" yaml:"namespaces"`
	Services    int `json:"services" yaml:"services"`
	Routes      int `json:"routes" yaml:"routes"`
	Deployments int `json:"deployments" yaml:"deployments"`
	Pods        int `json:"pods" yaml:"pods"`
	Pairs       int `json:"pairs" yaml:"pairs"`
	Intended    int `json:"intendedObjects" yaml:"intendedObjects"`
}

func (c *Config) Counts() Counts {
	ns := c.Topology.Namespaces.Count
	svc := ns * c.Topology.Services.PerNamespace
	rt := ns * c.Topology.Routes.PerNamespace
	dep := svc // one Deployment per service in oneToOne
	pods := dep * c.Topology.Workloads.ReplicasPerService
	return Counts{
		Namespaces:  ns,
		Services:    svc,
		Routes:      rt,
		Deployments: dep,
		Pods:        pods,
		Pairs:       svc,
		Intended:    ns + svc + rt + dep,
	}
}
