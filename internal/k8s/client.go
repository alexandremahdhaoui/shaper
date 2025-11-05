/*
Copyright 2024 Alexandre Mahdhaoui

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package k8s provides utilities for creating Kubernetes clients.
package k8s

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	shaperv1alpha1 "github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
)

const (
	// InClusterConfig is the string value that indicates in-cluster config should be used.
	InClusterConfig = "in-cluster"

	// ServiceAccountConfig is an alternative string value that indicates in-cluster config.
	// This is used by shaper-api for compatibility.
	ServiceAccountConfig = ">>> Kubeconfig From Service Account"
)

// NewKubeRestConfig creates a Kubernetes REST config from the given kubeconfig path.
// If kubeconfigPath is "in-cluster" or ">>> Kubeconfig From Service Account", it uses the in-cluster config.
// Otherwise, it loads the kubeconfig from the specified file path.
func NewKubeRestConfig(kubeconfigPath string) (*rest.Config, error) {
	// Check for in-cluster config indicators
	if kubeconfigPath == InClusterConfig || kubeconfigPath == ServiceAccountConfig {
		return rest.InClusterConfig()
	}

	// Load from kubeconfig file
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

// NewKubeClient creates a Kubernetes client with the shaper scheme.
// The scheme includes corev1 and shaperv1alpha1 types.
func NewKubeClient(restConfig *rest.Config) (client.Client, error) { //nolint:ireturn
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
