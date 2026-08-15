package config

import "time"

const (
	DefaultWebImage = "ghcr.io/dasmlab/dasm-burner-web:dev"
	DefaultPort     = int32(8080)
)

// StartingTemplate is the compact canvas / serve default: two namespaces,
// two 1:1 route→service pairs, three replicas. The 2,500-namespace product
// mix remains Default() for CLI plan without a file.
func StartingTemplate() *Config {
	c := Default()
	c.Metadata.Name = "smoke"
	c.Metadata.Description = "2-namespace starting template — still do not apply blindly"
	c.Topology.Namespaces.Count = 2
	c.Topology.Services.PerNamespace = 2
	c.Topology.Routes.PerNamespace = 2
	c.Topology.Workloads.ReplicasPerService = 3
	c.Topology.Relationships.RouteToService = RelOneToOne
	c.Naming.Seed = Seed{Auto: false, Value: 1837291}
	c.Deployment.Mode = DeployBatch
	c.Deployment.BatchSize = 0 // auto breakpoints
	c.Deployment.APIConcurrency = 8
	return c
}

func StartingObjectPressure() *Config {
	c := Default()
	c.Kind = KindObjectPressure
	c.Metadata.Name = "object-pressure"
	c.Metadata.Description = "ObjectPressure basetype — kube-burner init for small etcd/API objects"
	c.Topology.Namespaces.Count = 2
	c.Topology.Services.PerNamespace = 1
	c.Topology.Routes.PerNamespace = 1
	c.Topology.Workloads.ReplicasPerService = 1
	c.Topology.Objects = DefaultPressureObjects()
	c.Naming.Seed = Seed{Auto: false, Value: 424242}
	c.Deployment.Mode = DeployBatch
	c.Deployment.BatchSize = 0
	c.Deployment.APIConcurrency = 20
	c.Deployment.WaitForReady = false
	c.Monitoring.PodLatency.Enabled = false
	c.Monitoring.ServiceLatency.Enabled = false
	return c
}

// DefaultPressureObjects is the stock palette (enable checkboxes in UI).
func DefaultPressureObjects() []PressureObject {
	return PressureCatalog()
}

func Default() *Config {
	return &Config{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:        "route-service-density",
			Description: "OpenShift namespace/route/service density topology",
		},
		Topology: Topology{
			Namespaces: NamespaceSpec{Count: 2500},
			Services:   CountPerNS{PerNamespace: 2},
			Routes:     CountPerNS{PerNamespace: 2},
			Workloads: WorkloadSpec{
				Controller:         ControllerDeployment,
				ReplicasPerService: 3,
			},
			Relationships: RelationshipSpec{
				RouteToService: RelOneToOne,
			},
		},
		Application: Application{
			Image:           DefaultWebImage,
			ImagePullPolicy: "IfNotPresent",
			// Public GHCR — no pull secret required for workload pods.
			ImagePullSecret:     "",
			ImagePullSecretFrom: "",
			Port:                DefaultPort,
			Response: ResponseSpec{
				Type: "podName",
			},
			TLS: RouteTLSSpec{
				Enabled:                       true,
				Termination:                   "edge",
				InsecureEdgeTerminationPolicy: "Redirect",
			},
			AvoidTaints: DefaultAvoidTaints(),
		},
		Naming: Naming{
			Seed:       Seed{Auto: true},
			Namespace:  NamePrefix{Prefix: "ns", RandomLength: 4},
			Service:    NamePrefix{Prefix: "svc", RandomLength: 4},
			Route:      NamePrefix{Prefix: "rt", RandomLength: 4},
			Deployment: NamePrefix{Prefix: "deploy", RandomLength: 4},
		},
		Deployment: Deployment{
			Mode:             DeployBatch,
			BatchSize:        0, // 0 = auto breakpoints (≤8 waves)
			BatchDelay:       Duration(5 * time.Second),
			APIConcurrency:   20,
			WaitForReady:     true,
			ReadinessTimeout: Duration(5 * time.Minute),
		},
		Monitoring: Monitoring{
			Baseline:       BaselineSpec{Duration: Duration(60 * time.Second)},
			Interval:       Duration(15 * time.Second),
			Prometheus:     Toggle{Enabled: true},
			OVNKubernetes:  Toggle{Enabled: true},
			Events:         Toggle{Enabled: true},
			PodLatency:     Toggle{Enabled: true},
			ServiceLatency: Toggle{Enabled: true},
		},
		Safety: Safety{
			Enabled: true,
			AbortOn: AbortOn{
				NodeNotReady:   true,
				MasterNotReady: true,
				EtcdUnhealthy:  true,
				OOMKilled:      true,
				CriticalAlert:  true,
			},
			Thresholds: Thresholds{
				MaxPodFailurePercent: 5,
				MaxAPIErrorPercent:   2,
				MaxNodeNotReady:      0,
			},
			GracePeriod: Duration(30 * time.Second),
		},
		Execution: Execution{
			SteadyState: SteadyState{
				Enabled:  true,
				Duration: Duration(10 * time.Minute),
			},
			Cleanup: Toggle{Enabled: false},
		},
	}
}

// ApplyDefaults fills zero-value fields after YAML unmarshal so a sparse
// user file still produces a complete rendered config.
func ApplyDefaults(c *Config) {
	d := Default()
	if c.APIVersion == "" {
		c.APIVersion = d.APIVersion
	}
	if c.Kind == "" {
		c.Kind = d.Kind
	}
	if c.Metadata.Name == "" {
		c.Metadata.Name = d.Metadata.Name
	}
	if c.Topology.Namespaces.Count == 0 {
		c.Topology.Namespaces.Count = d.Topology.Namespaces.Count
	}
	if c.Topology.Services.PerNamespace == 0 {
		c.Topology.Services.PerNamespace = d.Topology.Services.PerNamespace
	}
	if c.Topology.Routes.PerNamespace == 0 {
		c.Topology.Routes.PerNamespace = d.Topology.Routes.PerNamespace
	}
	if c.Topology.Workloads.Controller == "" {
		c.Topology.Workloads.Controller = d.Topology.Workloads.Controller
	}
	if c.Topology.Workloads.ReplicasPerService == 0 {
		c.Topology.Workloads.ReplicasPerService = d.Topology.Workloads.ReplicasPerService
	}
	if c.Topology.Relationships.RouteToService == "" {
		c.Topology.Relationships.RouteToService = d.Topology.Relationships.RouteToService
	}
	if c.Kind == KindObjectPressure {
		c.Topology.Objects = MergePressureCatalog(c.Topology.Objects)
	}
	for i := range c.Topology.Objects {
		if c.Topology.Objects[i].ReplicasPerNS < 1 {
			c.Topology.Objects[i].ReplicasPerNS = 1
		}
	}
	if c.Application.Image == "" {
		c.Application.Image = d.Application.Image
	}
	if c.Application.ImagePullPolicy == "" {
		c.Application.ImagePullPolicy = d.Application.ImagePullPolicy
	}
	// Legacy private-GHCR templates: strip pull-secret so public images work without
	// copying dasmlab-ghcr-pull from mock-me-system.
	if c.Application.ImagePullSecret == "dasmlab-ghcr-pull" {
		c.Application.ImagePullSecret = ""
		c.Application.ImagePullSecretFrom = ""
	}
	if c.Application.Port == 0 {
		c.Application.Port = d.Application.Port
	}
	if c.Application.Response.Type == "" {
		c.Application.Response.Type = d.Application.Response.Type
	}
	if c.Application.TLS.Termination == "" {
		c.Application.TLS.Termination = d.Application.TLS.Termination
	}
	if c.Application.TLS.InsecureEdgeTerminationPolicy == "" {
		c.Application.TLS.InsecureEdgeTerminationPolicy = d.Application.TLS.InsecureEdgeTerminationPolicy
	}
	// nil means "use product default"; explicit empty YAML list opts out.
	if c.Application.AvoidTaints == nil {
		c.Application.AvoidTaints = append([]AvoidTaint(nil), d.Application.AvoidTaints...)
	}
	if c.Naming.Namespace.Prefix == "" {
		c.Naming.Namespace.Prefix = d.Naming.Namespace.Prefix
	}
	if c.Naming.Namespace.RandomLength == 0 {
		c.Naming.Namespace.RandomLength = d.Naming.Namespace.RandomLength
	}
	if c.Naming.Service.Prefix == "" {
		c.Naming.Service.Prefix = d.Naming.Service.Prefix
	}
	if c.Naming.Service.RandomLength == 0 {
		c.Naming.Service.RandomLength = d.Naming.Service.RandomLength
	}
	if c.Naming.Route.Prefix == "" {
		c.Naming.Route.Prefix = d.Naming.Route.Prefix
	}
	if c.Naming.Route.RandomLength == 0 {
		c.Naming.Route.RandomLength = d.Naming.Route.RandomLength
	}
	if c.Naming.Deployment.Prefix == "" {
		c.Naming.Deployment.Prefix = d.Naming.Deployment.Prefix
	}
	if c.Naming.Deployment.RandomLength == 0 {
		c.Naming.Deployment.RandomLength = d.Naming.Deployment.RandomLength
	}
	if c.Deployment.Mode == "" {
		c.Deployment.Mode = d.Deployment.Mode
	}
	// Legacy smoke templates used batchSize:1 (one NS per wave). Prefer auto breakpoints.
	if c.Deployment.BatchSize == 1 {
		c.Deployment.BatchSize = 0
	}
	// BatchSize 0 means auto breakpoints — do not fill from defaults.
	if c.Deployment.APIConcurrency == 0 {
		c.Deployment.APIConcurrency = d.Deployment.APIConcurrency
	}
	if c.Deployment.Mode == DeployRate && c.Deployment.NamespacesPerSec <= 0 {
		c.Deployment.NamespacesPerSec = 1
	}
}
