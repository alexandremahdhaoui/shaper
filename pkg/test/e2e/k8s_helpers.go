//go:build e2e

// Copyright 2024 Alexandre Mahdhaoui
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"context"
	"errors"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
)

var (
	// ErrK8sClientCreate indicates a failure to create the Kubernetes client.
	ErrK8sClientCreate = errors.New("failed to create Kubernetes client")
	// ErrProfileCreate indicates a failure to create a Profile CRD.
	ErrProfileCreate = errors.New("failed to create Profile")
	// ErrProfileDelete indicates a failure to delete a Profile CRD.
	ErrProfileDelete = errors.New("failed to delete Profile")
	// ErrAssignmentCreate indicates a failure to create an Assignment CRD.
	ErrAssignmentCreate = errors.New("failed to create Assignment")
	// ErrAssignmentDelete indicates a failure to delete an Assignment CRD.
	ErrAssignmentDelete = errors.New("failed to delete Assignment")
)

// NewK8sClient creates a controller-runtime client from the given kubeconfig path.
func NewK8sClient(kubeconfig string) (client.Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, errors.Join(ErrK8sClientCreate, err)
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, errors.Join(ErrK8sClientCreate, err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return nil, errors.Join(ErrK8sClientCreate, err)
	}

	c, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, errors.Join(ErrK8sClientCreate, err)
	}

	return c, nil
}

// CreateProfile creates a Profile CRD with the given name, namespace, and iPXE template.
func CreateProfile(
	ctx context.Context,
	c client.Client,
	name, namespace, ipxeTemplate string,
) (*v1alpha1.Profile, error) {
	profile := &v1alpha1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.ProfileSpec{
			IPXETemplate: ipxeTemplate,
		},
	}

	if err := c.Create(ctx, profile); err != nil {
		return nil, errors.Join(ErrProfileCreate, err)
	}

	return profile, nil
}

// CreateDefaultAssignment creates a default Assignment CRD for the given buildarch.
// The assignment is marked as default using the shaper.amahdha.com/default-assignment label.
func CreateDefaultAssignment(
	ctx context.Context,
	c client.Client,
	name, namespace, profileName string,
	buildarch v1alpha1.Buildarch,
) (*v1alpha1.Assignment, error) {
	labels := make(map[string]string)

	// Add default assignment label
	labels[v1alpha1.DefaultAssignmentLabel] = ""

	// Add buildarch label
	setBuildarchLabel(labels, buildarch)

	assignment := &v1alpha1.Assignment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1alpha1.AssignmentSpec{
			ProfileName: profileName,
			IsDefault:   true,
			SubjectSelectors: v1alpha1.SubjectSelectors{
				BuildarchList: []v1alpha1.Buildarch{buildarch},
				UUIDList:      []string{}, // Required: must be empty array, not nil
			},
		},
	}

	if err := c.Create(ctx, assignment); err != nil {
		return nil, errors.Join(ErrAssignmentCreate, err)
	}

	return assignment, nil
}

// CreateUUIDAssignment creates an Assignment CRD that targets a specific VM UUID.
// The assignment uses the uuid.shaper.amahdha.com/{uuid} label selector.
func CreateUUIDAssignment(
	ctx context.Context,
	c client.Client,
	name, namespace, profileName string,
	vmUUID uuid.UUID,
	buildarch v1alpha1.Buildarch,
) (*v1alpha1.Assignment, error) {
	labels := make(map[string]string)

	// Add UUID label selector
	labels[v1alpha1.NewUUIDLabelSelector(vmUUID)] = ""

	// Add buildarch label
	setBuildarchLabel(labels, buildarch)

	assignment := &v1alpha1.Assignment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1alpha1.AssignmentSpec{
			ProfileName: profileName,
			IsDefault:   false,
			SubjectSelectors: v1alpha1.SubjectSelectors{
				BuildarchList: []v1alpha1.Buildarch{buildarch},
				UUIDList:      []string{vmUUID.String()},
			},
		},
	}

	if err := c.Create(ctx, assignment); err != nil {
		return nil, errors.Join(ErrAssignmentCreate, err)
	}

	return assignment, nil
}

// DeleteProfile deletes a Profile CRD by name and namespace.
func DeleteProfile(ctx context.Context, c client.Client, name, namespace string) error {
	profile := &v1alpha1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if err := c.Delete(ctx, profile); err != nil {
		return errors.Join(ErrProfileDelete, err)
	}

	return nil
}

// DeleteAssignment deletes an Assignment CRD by name and namespace.
func DeleteAssignment(ctx context.Context, c client.Client, name, namespace string) error {
	assignment := &v1alpha1.Assignment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if err := c.Delete(ctx, assignment); err != nil {
		return errors.Join(ErrAssignmentDelete, err)
	}

	return nil
}

// setBuildarchLabel sets the appropriate buildarch label on the labels map.
func setBuildarchLabel(labels map[string]string, buildarch v1alpha1.Buildarch) {
	switch buildarch {
	case v1alpha1.Arm32:
		labels[v1alpha1.Arm32BuildarchLabelSelector] = ""
	case v1alpha1.Arm64:
		labels[v1alpha1.Arm64BuildarchLabelSelector] = ""
	case v1alpha1.I386:
		labels[v1alpha1.I386BuildarchLabelSelector] = ""
	case v1alpha1.X8664:
		labels[v1alpha1.X8664BuildarchLabelSelector] = ""
	}
}
