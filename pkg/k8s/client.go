package k8s

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps the Kubernetes clientset and REST config.
type Client struct {
	Clientset  *kubernetes.Clientset
	RestConfig *rest.Config
}

// NewClient creates a Kubernetes client using standard kubeconfig resolution.
// It respects KUBECONFIG env var, --kubeconfig flag, and ~/.kube/config.
// If kubeContext is non-empty it overrides the current context in the kubeconfig.
func NewClient(kubeconfig, kubeContext string) (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		configOverrides.CurrentContext = kubeContext
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Client{
		Clientset:  clientset,
		RestConfig: restConfig,
	}, nil
}
