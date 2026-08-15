package ui

import (
	"net/http"

	"github.com/dasmlab/dasm-burner/internal/sourcemap"
)

func (s *Server) isolationAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"isolation": sourcemap.Isolation,
		"warning":   "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) sourceMapAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cl := s.currentCluster()
	out := map[string]any{
		"cluster": cl.Name,
		"server":  cl.Server,
		"map":     sourcemap.OCP42110,
		"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		"note":    "Pins are for TEST3 OCP 4.21.10. Refresh other versions with oc adm release info $VER --commits.",
	}
	writeJSON(w, http.StatusOK, out)
}
