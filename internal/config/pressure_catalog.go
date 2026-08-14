package config

import "strings"

const (
	CatCore      = "core"
	CatRBAC      = "rbac"
	CatAuthz     = "authz"
	CatCoord     = "coord"
	CatNet       = "net"
	CatScale     = "scale"
	CatObserve   = "observe"
	CatOpenShift = "openshift"
	CatCustom    = "custom"
)

// PressureCatalog is the selectable ObjectPressure palette. Known Kubernetes /
// OpenShift GVKs (including SubjectAccessReview under authorization.k8s.io)
// live here so the UI does not treat them as custom CRDs. Tenant CRDs still
// come from +Add Custom.
func PressureCatalog() []PressureObject {
	return []PressureObject{
		po("configmap", true, "v1", "ConfigMap", "configmaps", CatCore, 10, false),
		po("secret", true, "v1", "Secret", "secrets", CatCore, 10, false),
		po("serviceaccount", true, "v1", "ServiceAccount", "serviceaccounts", CatCore, 5, false),
		po("event", false, "events.k8s.io/v1", "Event", "events", CatCore, 20, false),
		po("limitrange", false, "v1", "LimitRange", "limitranges", CatCore, 1, false),
		po("resourcequota", false, "v1", "ResourceQuota", "resourcequotas", CatCore, 1, false),
		po("endpointslice", false, "discovery.k8s.io/v1", "EndpointSlice", "endpointslices", CatCore, 5, false),

		po("role", false, "rbac.authorization.k8s.io/v1", "Role", "roles", CatRBAC, 5, false),
		po("rolebinding", true, "rbac.authorization.k8s.io/v1", "RoleBinding", "rolebindings", CatRBAC, 5, false),
		po("clusterrole", false, "rbac.authorization.k8s.io/v1", "ClusterRole", "clusterroles", CatRBAC, 2, true),
		po("clusterrolebinding", false, "rbac.authorization.k8s.io/v1", "ClusterRoleBinding", "clusterrolebindings", CatRBAC, 2, true),

		po("subjectaccessreview", false, "authorization.k8s.io/v1", "SubjectAccessReview", "subjectaccessreviews", CatAuthz, 5, true),
		po("localsubjectaccessreview", false, "authorization.k8s.io/v1", "LocalSubjectAccessReview", "localsubjectaccessreviews", CatAuthz, 5, false),
		po("tokenreview", false, "authentication.k8s.io/v1", "TokenReview", "tokenreviews", CatAuthz, 5, true),

		po("lease", false, "coordination.k8s.io/v1", "Lease", "leases", CatCoord, 10, false),

		po("networkpolicy", false, "networking.k8s.io/v1", "NetworkPolicy", "networkpolicies", CatNet, 2, false),
		po("egressfirewall", false, "k8s.ovn.org/v1", "EgressFirewall", "egressfirewalls", CatNet, 1, false),

		po("horizontalpodautoscaler", false, "autoscaling/v2", "HorizontalPodAutoscaler", "horizontalpodautoscalers", CatScale, 2, false),

		po("servicemonitor", false, "monitoring.coreos.com/v1", "ServiceMonitor", "servicemonitors", CatObserve, 2, false),

		po("imagestream", false, "image.openshift.io/v1", "ImageStream", "imagestreams", CatOpenShift, 2, false),
	}
}

func po(id string, enabled bool, api, kind, resource, cat string, replicas int, cluster bool) PressureObject {
	return PressureObject{
		ID:            id,
		Enabled:       enabled,
		APIVersion:    api,
		Kind:          kind,
		Resource:      resource,
		Category:      cat,
		ReplicasPerNS: replicas,
		TemplateRef:   id,
		ClusterScoped: cluster,
	}
}

// LookupPressureKind resolves a typed kind, plural resource, or GVK against
// the stock catalog (case-insensitive). Used so "subjectaccessreviews" is not
// stored as a custom GVK.
func LookupPressureKind(raw string) (PressureObject, bool) {
	key := normalizeGVK(raw)
	if key == "" {
		return PressureObject{}, false
	}
	for _, o := range PressureCatalog() {
		keys := []string{
			o.ID,
			o.Kind,
			o.Resource,
			o.APIVersion + "/" + o.Kind,
			o.APIVersion + "/" + o.Resource,
		}
		if g := apiGroup(o.APIVersion); g != "" {
			keys = append(keys, g+"/"+o.Kind, g+"/"+o.Resource)
		}
		for _, k := range keys {
			if normalizeGVK(k) == key {
				return o, true
			}
		}
	}
	return PressureObject{}, false
}

// MergePressureCatalog promotes known custom entries to stock GVKs and appends
// any missing catalog kinds (disabled) so older saved templates pick up new
// selectable types.
func MergePressureCatalog(existing []PressureObject) []PressureObject {
	cat := PressureCatalog()
	if len(existing) == 0 {
		return cat
	}
	seenID := map[string]bool{}
	seenGVK := map[string]bool{}
	out := make([]PressureObject, 0, len(existing)+len(cat))
	for _, o := range existing {
		if hit, ok := LookupPressureKind(firstNonEmpty(o.Kind, o.ID, o.Resource)); ok {
			sameGVK := o.APIVersion == "" || normalizeGVK(o.APIVersion) == normalizeGVK(hit.APIVersion)
			if o.Custom || sameGVK {
				hit.Enabled = o.Enabled
				if o.ReplicasPerNS > 0 {
					hit.ReplicasPerNS = o.ReplicasPerNS
				}
				hit.Required = o.Required
				hit.InlineYAML = o.InlineYAML
				hit.WaitForReady = o.WaitForReady
				o = hit
			}
		}
		if o.Custom && o.Category == "" {
			o.Category = CatCustom
		}
		out = append(out, o)
		seenID[o.ID] = true
		seenGVK[normalizeGVK(o.APIVersion+"/"+o.Kind)] = true
	}
	for _, c := range cat {
		if seenID[c.ID] || seenGVK[normalizeGVK(c.APIVersion+"/"+c.Kind)] {
			continue
		}
		c.Enabled = false
		out = append(out, c)
	}
	return out
}

func normalizeGVK(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.Trim(s, "/")
	return s
}

func apiGroup(apiVersion string) string {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" || !strings.Contains(apiVersion, "/") {
		return ""
	}
	i := strings.LastIndex(apiVersion, "/")
	last := apiVersion[i+1:]
	if strings.HasPrefix(last, "v") {
		return apiVersion[:i]
	}
	return apiVersion
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
