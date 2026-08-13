package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dasmlab/dasm-burner/internal/kube"
)

func TestBuildNarrative(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "apiRequestRate.json"), []byte(`[{"value":1.5},{"value":2.5}]`), 0o644)
	doc, err := Build(kube.Health{NodesReady: 3, OVNPods: 6, OVNReady: 6}, nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Narrative, "Nodes Ready 3") {
		t.Fatalf("narrative:\n%s", doc.Narrative)
	}
	if doc.Metrics["apiRequestRate"].Count != 2 {
		t.Fatalf("%+v", doc.Metrics)
	}
	if err := Write(dir, doc); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.md")); err != nil {
		t.Fatal(err)
	}
}
