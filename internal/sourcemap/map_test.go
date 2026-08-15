package sourcemap

import "testing"

func TestIsolationHasClosedLoop(t *testing.T) {
	if Isolation.Name == "" || len(Isolation.Steps) != 6 {
		t.Fatalf("want 6 steps, got %d name=%q", len(Isolation.Steps), Isolation.Name)
	}
	if len(Isolation.Causality) < 3 {
		t.Fatal("causality chain missing")
	}
}

func TestOCP42110HasFourPieces(t *testing.T) {
	if len(OCP42110.Pieces) != 4 {
		t.Fatalf("want 4 pieces, got %d", len(OCP42110.Pieces))
	}
	var kas *Piece
	for i := range OCP42110.Pieces {
		if OCP42110.Pieces[i].ID == "kube-apiserver" {
			kas = &OCP42110.Pieces[i]
		}
		if OCP42110.Pieces[i].PayloadSHA == "" || OCP42110.Pieces[i].Clone == "" {
			t.Fatalf("piece %s missing clone/sha", OCP42110.Pieces[i].ID)
		}
	}
	if kas == nil || kas.PossibleFix == nil || len(kas.Files) == 0 {
		t.Fatal("kube-apiserver must pin watch_cache.go and a possible fix")
	}
}
