package config

import (
	"fmt"
	"strings"
)

// AvoidTaint is a cluster taint that workload pods must NOT tolerate.
// Without a matching toleration, the scheduler will not place pods on
// nodes that carry a NoSchedule/NoExecute taint with this key/value.
//
// Accepts kubectl-style strings via ParseAvoidTaint:
//
//	node-role.kubernetes.io=infra:NoSchedule
//	node-role.kubernetes.io/infra:NoSchedule
//	node-role.kubernetes.io=infra
type AvoidTaint struct {
	Key    string `yaml:"key" json:"key"`
	Value  string `yaml:"value,omitempty" json:"value,omitempty"`
	Effect string `yaml:"effect,omitempty" json:"effect,omitempty"` // NoSchedule | PreferNoSchedule | NoExecute
}

// DefaultAvoidTaints keeps density workloads off infra-role nodes
// (per cluster practice: taint node-role.kubernetes.io=infra).
func DefaultAvoidTaints() []AvoidTaint {
	return []AvoidTaint{{
		Key:    "node-role.kubernetes.io",
		Value:  "infra",
		Effect: "NoSchedule",
	}}
}

// ParseAvoidTaint parses KEY[=VALUE][:EFFECT] (kubectl taint shorthand).
func ParseAvoidTaint(s string) (AvoidTaint, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return AvoidTaint{}, fmt.Errorf("empty taint")
	}
	effect := "NoSchedule"
	body := s
	if i := strings.LastIndex(s, ":"); i >= 0 {
		maybe := s[i+1:]
		switch maybe {
		case "NoSchedule", "PreferNoSchedule", "NoExecute":
			effect = maybe
			body = s[:i]
		}
	}
	key, value := body, ""
	if i := strings.Index(body, "="); i >= 0 {
		key = body[:i]
		value = body[i+1:]
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" {
		return AvoidTaint{}, fmt.Errorf("taint %q: missing key", s)
	}
	return AvoidTaint{Key: key, Value: value, Effect: effect}, nil
}

// ParseAvoidTaints parses a list of kubectl-style taint strings.
func ParseAvoidTaints(ss []string) ([]AvoidTaint, error) {
	var out []AvoidTaint
	seen := map[string]bool{}
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		t, err := ParseAvoidTaint(s)
		if err != nil {
			return nil, err
		}
		k := t.String()
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out, nil
}

func (t AvoidTaint) String() string {
	if t.Value != "" {
		if t.Effect != "" {
			return fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect)
		}
		return fmt.Sprintf("%s=%s", t.Key, t.Value)
	}
	if t.Effect != "" {
		return fmt.Sprintf("%s:%s", t.Key, t.Effect)
	}
	return t.Key
}

// MatchesToleration reports whether a pod toleration would allow this avoided taint.
func (t AvoidTaint) MatchesToleration(tolKey, tolValue, tolEffect, tolOp string) bool {
	// Empty-key toleration matches every taint — never keep those when avoiding.
	if tolKey == "" {
		return true
	}
	if tolKey != t.Key {
		return false
	}
	op := strings.ToLower(tolOp)
	if op == "" {
		op = "equal"
	}
	switch op {
	case "exists":
		// Exists matches any value for the key.
	case "equal":
		if t.Value != tolValue {
			return false
		}
	default:
		if t.Value != "" && tolValue != "" && t.Value != tolValue {
			return false
		}
	}
	if t.Effect != "" && tolEffect != "" && t.Effect != tolEffect {
		return false
	}
	return true
}
