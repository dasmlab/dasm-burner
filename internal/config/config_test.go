package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartingObjectPressureValidates(t *testing.T) {
	c := StartingObjectPressure()
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	if c.Kind != KindObjectPressure {
		t.Fatalf("kind = %s", c.Kind)
	}
	got := c.Counts()
	if got.Namespaces != 2 || got.ByKind["ConfigMap"] != 20 || got.Intended < 2 {
		t.Fatalf("object-pressure counts = %+v", got)
	}
}

func TestLookupPressureKind(t *testing.T) {
	for _, q := range []string{
		"subjectaccessreviews",
		"SubjectAccessReview",
		"authorization.k8s.io/v1/SubjectAccessReview",
		"authorization.k8s.io/v1/subjectaccessreviews",
		"authorization.k8s.io/subjectaccessreviews",
	} {
		got, ok := LookupPressureKind(q)
		if !ok || got.Kind != "SubjectAccessReview" || got.APIVersion != "authorization.k8s.io/v1" || got.Custom {
			t.Fatalf("%q -> %+v ok=%v", q, got, ok)
		}
	}
	if _, ok := LookupPressureKind("services.desjardins.com/v1/User"); ok {
		t.Fatal("tenant CRD should stay custom")
	}
}

func TestMergePromotesCustomSAR(t *testing.T) {
	merged := MergePressureCatalog([]PressureObject{
		{ID: "custom-sar", Enabled: true, Custom: true, APIVersion: "authorization.k8s.io/v1", Kind: "subjectaccessreviews", ReplicasPerNS: 5},
	})
	found := false
	for _, o := range merged {
		if o.Kind == "SubjectAccessReview" {
			found = true
			if o.Custom || o.APIVersion != "authorization.k8s.io/v1" || !o.Enabled || o.ReplicasPerNS != 5 {
				t.Fatalf("promoted SAR = %+v", o)
			}
		}
	}
	if !found {
		t.Fatal("expected SubjectAccessReview in merged catalog")
	}
}

func TestDefaultValidates(t *testing.T) {
	c := Default()
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	got := c.Counts()
	if got.Namespaces != 2500 || got.Services != 5000 || got.Routes != 5000 || got.Deployments != 5000 || got.Pods != 15000 {
		t.Fatalf("default counts = %+v", got)
	}
	if got.Intended != 2500+5000+5000+5000 {
		t.Fatalf("intended = %d", got.Intended)
	}
}

func TestLoadSparseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	body := `
apiVersion: benchmark.dasmlab.org/v1
kind: OpenShiftNetworkDensity
metadata:
  name: smoke
topology:
  namespaces:
    count: 2
naming:
  seed: 42
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Topology.Services.PerNamespace != 2 {
		t.Fatalf("services defaulted to %d", c.Topology.Services.PerNamespace)
	}
	if c.Naming.Seed.Value != 42 || c.Naming.Seed.Auto {
		t.Fatalf("seed = %+v", c.Naming.Seed)
	}
	if c.Counts().Pods != 12 {
		t.Fatalf("smoke pods = %d want 12", c.Counts().Pods)
	}
}

func TestOneToOneMismatch(t *testing.T) {
	c := Default()
	c.Topology.Routes.PerNamespace = 3
	err := Validate(c)
	if err == nil || !strings.Contains(err.Error(), "oneToOne") {
		t.Fatalf("expected oneToOne error, got %v", err)
	}
}

func TestPhase1RejectsLaterControllers(t *testing.T) {
	c := Default()
	c.Topology.Workloads.Controller = ControllerStatefulSet
	if err := Validate(c); err == nil {
		t.Fatal("expected StatefulSet to be rejected in Phase 1")
	}
}
