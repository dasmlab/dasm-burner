package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	APIVersion = "benchmark.dasmlab.org/v1"
	Kind       = "OpenShiftNetworkDensity"

	RelOneToOne  = "oneToOne"
	RelOneToMany = "oneToMany"
	RelManyToOne = "manyToOne"

	ControllerDeployment  = "Deployment"
	ControllerStatefulSet = "StatefulSet"
	ControllerPod         = "Pod"

	DeploySequential = "sequential"
	DeployBatch      = "batch"
	DeployRate       = "rate"
)

// Config is the user-facing benchmark document. Phase 1 owns topology +
// naming + application. Later sections are parsed and defaulted so the
// schema stays stable across phases.
type Config struct {
	APIVersion  string      `yaml:"apiVersion"`
	Kind        string      `yaml:"kind"`
	Metadata    Metadata    `yaml:"metadata"`
	Topology    Topology    `yaml:"topology"`
	Application Application `yaml:"application"`
	Naming      Naming      `yaml:"naming"`
	Deployment  Deployment  `yaml:"deployment"`
	Monitoring  Monitoring  `yaml:"monitoring"`
	Safety      Safety      `yaml:"safety"`
	Execution   Execution   `yaml:"execution"`
}

type Metadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

type Topology struct {
	Namespaces    NamespaceSpec    `yaml:"namespaces"`
	Services      CountPerNS       `yaml:"services"`
	Routes        CountPerNS       `yaml:"routes"`
	Workloads     WorkloadSpec     `yaml:"workloads"`
	Relationships RelationshipSpec `yaml:"relationships"`
}

type NamespaceSpec struct {
	Count int `yaml:"count"`
}

type CountPerNS struct {
	PerNamespace int `yaml:"perNamespace"`
}

type WorkloadSpec struct {
	Controller         string `yaml:"controller"`
	ReplicasPerService int    `yaml:"replicasPerService"`
}

type RelationshipSpec struct {
	RouteToService string `yaml:"routeToService"`
	// Phase 2+: used when routeToService is oneToMany.
	ServicesPerRoute int `yaml:"servicesPerRoute,omitempty"`
	// Phase 2+: used when routeToService is manyToOne.
	RoutesPerService int `yaml:"routesPerService,omitempty"`
}

type Application struct {
	Image               string       `yaml:"image"`
	ImagePullPolicy     string       `yaml:"imagePullPolicy,omitempty"`
	ImagePullSecret     string       `yaml:"imagePullSecret,omitempty"`
	ImagePullSecretFrom string       `yaml:"imagePullSecretFrom,omitempty"`
	Port                int32        `yaml:"port"`
	Response            ResponseSpec `yaml:"response"`
	TLS                 RouteTLSSpec `yaml:"tls"`
	// AvoidTaints: workload pods must NOT tolerate these (Execute can override).
	// nil → ApplyDefaults fills DefaultAvoidTaints; empty slice → none.
	AvoidTaints []AvoidTaint `yaml:"avoidTaints,omitempty" json:"avoidTaints,omitempty"`
}

type ResponseSpec struct {
	Type string `yaml:"type"` // podName
}

type RouteTLSSpec struct {
	Enabled                       bool   `yaml:"enabled"`
	Termination                   string `yaml:"termination"`
	InsecureEdgeTerminationPolicy string `yaml:"insecureEdgeTerminationPolicy"`
}

type Naming struct {
	Seed       Seed       `yaml:"seed"`
	Namespace  NamePrefix `yaml:"namespace"`
	Service    NamePrefix `yaml:"service"`
	Route      NamePrefix `yaml:"route"`
	Deployment NamePrefix `yaml:"deployment"`
}

type NamePrefix struct {
	Prefix       string `yaml:"prefix"`
	RandomLength int    `yaml:"randomLength"`
}

// Seed is either "auto" or an integer. The resolved value is written back
// into rendered-config.yaml so a run is reproducible.
type Seed struct {
	Auto  bool
	Value int64
}

func (s Seed) MarshalYAML() (interface{}, error) {
	if s.Auto && s.Value == 0 {
		return "auto", nil
	}
	return s.Value, nil
}

func (s *Seed) UnmarshalYAML(value *yaml.Node) error {
	var asString string
	if err := value.Decode(&asString); err == nil {
		if asString == "" || asString == "auto" {
			s.Auto = true
			s.Value = 0
			return nil
		}
		var n int64
		if _, err := fmt.Sscan(asString, &n); err == nil {
			s.Auto = false
			s.Value = n
			return nil
		}
		return fmt.Errorf("naming.seed: expected auto or integer, got %q", asString)
	}
	var n int64
	if err := value.Decode(&n); err != nil {
		return fmt.Errorf("naming.seed: expected auto or integer")
	}
	s.Auto = false
	s.Value = n
	return nil
}

type Deployment struct {
	Mode             string   `yaml:"mode"`
	BatchSize        int      `yaml:"batchSize"`
	BatchDelay       Duration `yaml:"batchDelay"`
	APIConcurrency   int      `yaml:"apiConcurrency"`
	WaitForReady     bool     `yaml:"waitForReady"`
	ReadinessTimeout Duration `yaml:"readinessTimeout"`
	NamespacesPerSec float64  `yaml:"namespacesPerSecond,omitempty"`
}

type Monitoring struct {
	Baseline       BaselineSpec `yaml:"baseline"`
	Interval       Duration     `yaml:"interval"`
	Prometheus     Toggle       `yaml:"prometheus"`
	OVNKubernetes  Toggle       `yaml:"ovnKubernetes"`
	Events         Toggle       `yaml:"events"`
	PodLatency     Toggle       `yaml:"podLatency"`
	ServiceLatency Toggle       `yaml:"serviceLatency"`
}

type BaselineSpec struct {
	Duration Duration `yaml:"duration"`
}

type Toggle struct {
	Enabled bool `yaml:"enabled"`
}

type Safety struct {
	Enabled     bool       `yaml:"enabled"`
	AbortOn     AbortOn    `yaml:"abortOn"`
	Thresholds  Thresholds `yaml:"thresholds"`
	GracePeriod Duration   `yaml:"gracePeriod"`
}

type AbortOn struct {
	NodeNotReady  bool `yaml:"nodeNotReady"`
	OOMKilled     bool `yaml:"oomKilled"`
	CriticalAlert bool `yaml:"criticalAlert"`
}

type Thresholds struct {
	MaxPodFailurePercent float64 `yaml:"maxPodFailurePercent"`
	MaxAPIErrorPercent   float64 `yaml:"maxApiErrorPercent"`
	MaxNodeNotReady      int     `yaml:"maxNodeNotReady"`
}

type Execution struct {
	SteadyState SteadyState `yaml:"steadyState"`
	Cleanup     Toggle      `yaml:"cleanup"`
}

type SteadyState struct {
	Enabled  bool     `yaml:"enabled"`
	Duration Duration `yaml:"duration"`
}

// Duration wraps time.Duration for YAML strings like "5s".
type Duration time.Duration

func (d Duration) Std() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		var n int64
		if err2 := value.Decode(&n); err2 == nil {
			*d = Duration(time.Duration(n))
			return nil
		}
		return err
	}
	if s == "" {
		*d = 0
		return nil
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg := Default()
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	ApplyDefaults(cfg)
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}
