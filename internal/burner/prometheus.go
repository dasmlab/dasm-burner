package burner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/dasmlab/dasm-burner/internal/kube"
)

type Prometheus struct {
	URL       string
	Token     string
	TokenFile string
}

var thanosRouteGVR = schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}

// DiscoverPrometheus finds the OpenShift thanos-querier route and a bearer token.
func DiscoverPrometheus(ctx context.Context, kubeconfigPath, tokenFile string) (*Prometheus, error) {
	cfg, err := kube.RestConfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	u, err := dyn.Resource(thanosRouteGVR).Namespace("openshift-monitoring").Get(ctx, "thanos-querier", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get thanos-querier route: %w", err)
	}
	host, _, _ := unstructuredString(u.Object, "spec", "host")
	if host == "" {
		return nil, fmt.Errorf("thanos-querier route has no spec.host")
	}
	token := strings.TrimSpace(cfg.BearerToken)
	if token == "" && cfg.BearerTokenFile != "" {
		b, err := os.ReadFile(cfg.BearerTokenFile)
		if err == nil {
			token = strings.TrimSpace(string(b))
		}
	}
	if token == "" {
		token = ocWhoAmIToken()
	}
	if token == "" {
		return nil, fmt.Errorf("no bearer token for Prometheus (kubeconfig token or oc whoami -t)")
	}
	if tokenFile == "" {
		tokenFile = "prometheus.token"
	}
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
		return nil, err
	}
	return &Prometheus{
		URL:       "https://" + host,
		Token:     token,
		TokenFile: tokenFile,
	}, nil
}

func unstructuredString(obj map[string]any, fields ...string) (string, bool, error) {
	cur := any(obj)
	for _, f := range fields {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false, nil
		}
		cur, ok = m[f]
		if !ok {
			return "", false, nil
		}
	}
	s, ok := cur.(string)
	return s, ok, nil
}

func ocWhoAmIToken() string {
	out, err := exec.Command("oc", "whoami", "-t").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
