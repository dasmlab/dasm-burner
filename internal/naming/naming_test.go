package naming

import (
	"strings"
	"testing"

	"github.com/dasmlab/dasm-burner/internal/config"
)

func TestDeterministic(t *testing.T) {
	n := config.Default().Naming
	n.Seed = config.Seed{Value: 1837291}

	a := NewFactory(n)
	b := NewFactory(n)

	if a.RunID() != b.RunID() {
		t.Fatalf("runID %s vs %s", a.RunID(), b.RunID())
	}
	for i := 1; i <= 20; i++ {
		if a.Name(KindNamespace, i) != b.Name(KindNamespace, i) {
			t.Fatalf("ns %d diverged", i)
		}
	}
}

func TestShape(t *testing.T) {
	n := config.Default().Naming
	n.Seed = config.Seed{Value: 1}
	f := NewFactory(n)
	name := f.Name(KindNamespace, 1)
	if !strings.HasPrefix(name, "kb-") {
		t.Fatalf("name %q", name)
	}
	if !strings.Contains(name, "-ns-00001-") {
		t.Fatalf("expected seq in %q", name)
	}
	if len(name) > 63 {
		t.Fatalf("DNS label too long: %s (%d)", name, len(name))
	}
}

func TestDifferentSeedsDiverge(t *testing.T) {
	n := config.Default().Naming
	n.Seed = config.Seed{Value: 1}
	a := NewFactory(n)
	n.Seed = config.Seed{Value: 2}
	b := NewFactory(n)
	if a.Name(KindService, 1) == b.Name(KindService, 1) {
		t.Fatal("different seeds produced the same service name")
	}
}
