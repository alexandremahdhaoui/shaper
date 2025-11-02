package main

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	shaperv1alpha1 "github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
)

// newKubeRestConfig creates a Kubernetes REST config from the given kubeconfig path.
// If kubeconfigPath is "in-cluster", it uses the in-cluster config.
func newKubeRestConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath == "in-cluster" {
		return rest.InClusterConfig()
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

// newKubeClient creates a Kubernetes client with the shaper scheme.
func newKubeClient(restConfig *rest.Config) (client.Client, error) { //nolint:ireturn
	// Create scheme
	scheme := runtime.NewScheme()

	// Add core/v1
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	// Add shaper v1alpha1
	if err := shaperv1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	// Create and return client
	return client.New(restConfig, client.Options{Scheme: scheme}) //nolint:exhaustruct
}
