package kube

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// RestConfig resolves kubeconfig from --kubeconfig, KUBECONFIG, in-cluster,
// then ~/.kube/config.
func RestConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return clientcmd.BuildConfigFromFlags("", kc)
	}
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		def := filepath.Join(home, ".kube", "config")
		if _, statErr := os.Stat(def); statErr == nil {
			return clientcmd.BuildConfigFromFlags("", def)
		}
	}
	return nil, fmt.Errorf("no kubeconfig found: pass --kubeconfig, set KUBECONFIG, or run in-cluster")
}

func clientsFromConfig(cfg *rest.Config, qps float32, burst int) (kubernetes.Interface, dynamic.Interface, error) {
	if qps > 0 {
		cfg.QPS = qps
	} else {
		cfg.QPS = 50
	}
	if burst > 0 {
		cfg.Burst = burst
	} else {
		cfg.Burst = 100
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	return cs, dyn, nil
}

// NewLive builds a Cluster against the current kubeconfig.
func NewLive(kubeconfigPath string, qps float32, burst int) (Cluster, error) {
	return NewLiveContext(kubeconfigPath, "", qps, burst)
}

// NewLiveContext builds a Cluster for an optional named kubeconfig context.
func NewLiveContext(kubeconfigPath, context string, qps float32, burst int) (Cluster, error) {
	var cfg *rest.Config
	var err error
	if context != "" && kubeconfigPath != "" {
		loading := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
		overrides := &clientcmd.ConfigOverrides{CurrentContext: context}
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loading, overrides).ClientConfig()
	} else {
		cfg, err = RestConfig(kubeconfigPath)
	}
	if err != nil {
		return nil, err
	}
	cs, dyn, err := clientsFromConfig(cfg, qps, burst)
	if err != nil {
		return nil, err
	}
	return &Live{cs: cs, dyn: dyn}, nil
}
