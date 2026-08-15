package investigation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	mu     sync.Mutex
	idSafe = regexp.MustCompile(`[^a-z0-9._-]+`)
)

func dir(runDir string) string {
	return filepath.Join(runDir, "investigations")
}

func pathFor(runDir, id string) string {
	return filepath.Join(dir(runDir), sanitize(id)+".json")
}

func IDFrom(s string) string { return sanitize(s) }

func sanitize(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, "..", "")
	id = idSafe.ReplaceAllString(id, "-")
	id = strings.Trim(id, "-")
	if id == "" {
		return "untitled"
	}
	if len(id) > 80 {
		id = id[:80]
	}
	return id
}

func List(runDir string) ([]Investigation, error) {
	mu.Lock()
	defer mu.Unlock()
	return listLocked(runDir)
}

func listLocked(runDir string) ([]Investigation, error) {
	byID := map[string]Investigation{}
	for _, c := range Catalog() {
		byID[c.ID] = c
	}
	entries, err := os.ReadDir(dir(runDir))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir(runDir), e.Name()))
		if err != nil {
			continue
		}
		var inv Investigation
		if json.Unmarshal(b, &inv) != nil || inv.ID == "" {
			continue
		}
		if base, ok := byID[inv.ID]; ok {
			inv = mergeOverlay(base, inv)
		}
		byID[inv.ID] = inv
	}
	out := make([]Investigation, 0, len(byID))
	for _, v := range byID {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func Get(runDir, id string) (*Investigation, error) {
	mu.Lock()
	defer mu.Unlock()
	id = sanitize(id)
	all, err := listLocked(runDir)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			cp := all[i]
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("investigation %q not found", id)
}

func Save(runDir string, inv Investigation) error {
	mu.Lock()
	defer mu.Unlock()
	return saveLocked(runDir, inv)
}

func saveLocked(runDir string, inv Investigation) error {
	if inv.ID == "" {
		return fmt.Errorf("id required")
	}
	inv.ID = sanitize(inv.ID)
	now := time.Now().UTC()
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = now
	}
	inv.UpdatedAt = now
	if inv.Status == "" {
		inv.Status = StatusOpen
	}
	if err := os.MkdirAll(dir(runDir), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pathFor(runDir, inv.ID), b, 0o644)
}

func AppendEvidence(runDir, id string, ev Evidence) (*Investigation, error) {
	mu.Lock()
	defer mu.Unlock()
	id = sanitize(id)
	all, err := listLocked(runDir)
	if err != nil {
		return nil, err
	}
	var cur *Investigation
	for i := range all {
		if all[i].ID == id {
			cur = &all[i]
			break
		}
	}
	if cur == nil {
		return nil, fmt.Errorf("investigation %q not found", id)
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	cur.Evidence = append(cur.Evidence, ev)
	if err := saveLocked(runDir, *cur); err != nil {
		return nil, err
	}
	return cur, nil
}

func mergeOverlay(base, over Investigation) Investigation {
	out := base
	if over.Status != "" {
		out.Status = over.Status
	}
	if over.Title != "" {
		out.Title = over.Title
	}
	if over.Hypothesis != "" {
		out.Hypothesis = over.Hypothesis
	}
	if over.Metric != "" {
		out.Metric = over.Metric
	}
	if over.Protocol != "" {
		out.Protocol = over.Protocol
	}
	if over.Notes != "" {
		out.Notes = over.Notes
	}
	if over.Cluster != "" {
		out.Cluster = over.Cluster
	}
	if over.OpenShift != "" {
		out.OpenShift = over.OpenShift
	}
	if over.Kubernetes != "" {
		out.Kubernetes = over.Kubernetes
	}
	if len(over.Pieces) > 0 {
		out.Pieces = over.Pieces
	}
	if len(over.TestPlan) > 0 {
		out.TestPlan = over.TestPlan
	}
	if len(over.SourceFiles) > 0 {
		out.SourceFiles = over.SourceFiles
	}
	if over.PossibleFix != nil {
		out.PossibleFix = over.PossibleFix
	}
	if len(over.Evidence) > 0 {
		out.Evidence = over.Evidence
	}
	if !over.CreatedAt.IsZero() {
		out.CreatedAt = over.CreatedAt
	}
	if over.UpdatedAt.After(out.UpdatedAt) {
		out.UpdatedAt = over.UpdatedAt
	}
	out.Catalog = base.Catalog
	return out
}
