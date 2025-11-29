//go:build integration

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

package main_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	testNamespace         = "shaper-controller-test"
	reconciliationTimeout = 30 * time.Second
	pollInterval          = 500 * time.Millisecond
)

// setupTest creates a test Kubernetes client and namespace
func setupTest(t *testing.T) (client.Client, string) {
	t.Helper()

	// Get kubeconfig from environment or default
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		t.Skip("KUBECONFIG not set, skipping integration test")
	}

	// Create scheme and register types
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	// Create client
	cfg, err := config.GetConfig()
	require.NoError(t, err, "failed to get kubeconfig")

	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err, "failed to create client")

	// Create unique test namespace
	namespace := fmt.Sprintf("%s-%s", testNamespace, uuid.NewString()[:8])
	ctx := context.Background()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}
	err = cl.Create(ctx, ns)
	require.NoError(t, err, "failed to create test namespace")

	t.Cleanup(func() {
		ctx := context.Background()
		_ = cl.Delete(ctx, ns)
	})

	return cl, namespace
}

// startController starts the shaper-controller binary in the background
func startController(t *testing.T, kubeconfigPath string) *exec.Cmd {
	t.Helper()

	// Build the controller binary first
	buildCmd := exec.Command("go", "build", "-o", "/tmp/shaper-controller", "./cmd/shaper-controller")
	buildCmd.Dir = "../.." // Go to repo root
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "failed to build controller: %s", string(output))

	// Start controller
	cmd := exec.Command("/tmp/shaper-controller")
	cmd.Env = append(os.Environ(),
		"SHAPER_CONTROLLER_METRICS_ADDR=:18080",
		"SHAPER_CONTROLLER_HEALTH_ADDR=:18081",
		"SHAPER_CONTROLLER_LEADER_ELECTION=false",
		fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath),
	)

	require.NoError(t, cmd.Start(), "failed to start controller")

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})

	// Give controller time to start
	time.Sleep(2 * time.Second)

	return cmd
}

// controllerDeployed checks if the shaper-controller is deployed in the cluster
func controllerDeployed(t *testing.T, cl client.Client) bool {
	t.Helper()

	ctx := context.Background()

	// Check for controller deployment in common namespaces
	namespaces := []string{"shaper-system", "default", "shaper"}

	for _, ns := range namespaces {
		deploymentList := &appsv1.DeploymentList{}
		if err := cl.List(ctx, deploymentList, client.InNamespace(ns)); err != nil {
			continue
		}

		for _, dep := range deploymentList.Items {
			// Look for deployment with "controller" in the name
			if dep.Name == "shaper-controller" {
				// Check if deployment has ready replicas
				if dep.Status.ReadyReplicas > 0 {
					return true
				}
			}
		}
	}

	return false
}

// TestProfileReconciliation tests that Profile resources get status populated
func TestProfileReconciliation(t *testing.T) {
	cl, namespace := setupTest(t)
	ctx := context.Background()

	if !controllerDeployed(t, cl) {
		t.Skip("shaper-controller not deployed, skipping reconciliation test")
	}

	// Create a Profile
	profileName := "test-profile-" + uuid.NewString()[:8]
	profile := &v1alpha1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      profileName,
			Namespace: namespace,
		},
		Spec: v1alpha1.ProfileSpec{
			IPXETemplate: "test template",
			AdditionalContent: []v1alpha1.AdditionalContent{
				{Name: "ignition", Exposed: true, PostTransformations: []v1alpha1.Transformer{}},
				{Name: "config", Exposed: true, PostTransformations: []v1alpha1.Transformer{}},
			},
		},
	}

	err := cl.Create(ctx, profile)
	require.NoError(t, err, "failed to create Profile")

	// Wait for reconciliation - status should be populated with UUIDs
	assert.Eventually(t, func() bool {
		var updatedProfile v1alpha1.Profile
		err := cl.Get(ctx, types.NamespacedName{Name: profileName, Namespace: namespace}, &updatedProfile)
		if err != nil {
			t.Logf("Failed to get profile: %v", err)
			return false
		}

		if updatedProfile.Status.ExposedAdditionalContent == nil {
			t.Log("Status ExposedAdditionalContent is nil")
			return false
		}

		// Check that UUIDs were generated for both exposed content items
		ignitionUUID, ignitionOk := updatedProfile.Status.ExposedAdditionalContent["ignition"]
		configUUID, configOk := updatedProfile.Status.ExposedAdditionalContent["config"]

		if !ignitionOk || !configOk {
			t.Logf("Missing UUIDs: ignition=%v, config=%v", ignitionOk, configOk)
			return false
		}

		// Validate UUID format
		_, ignitionErr := uuid.Parse(ignitionUUID)
		_, configErr := uuid.Parse(configUUID)

		return ignitionErr == nil && configErr == nil
	}, reconciliationTimeout, pollInterval, "Profile status not reconciled")
}

// TestAssignmentReconciliation tests that Assignment resources get labels added
func TestAssignmentReconciliation(t *testing.T) {
	cl, namespace := setupTest(t)
	ctx := context.Background()

	if !controllerDeployed(t, cl) {
		t.Skip("shaper-controller not deployed, skipping reconciliation test")
	}

	testUUID := uuid.New()

	// Create an Assignment
	assignmentName := "test-assignment-" + uuid.NewString()[:8]
	assignment := &v1alpha1.Assignment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      assignmentName,
			Namespace: namespace,
		},
		Spec: v1alpha1.AssignmentSpec{
			ProfileName: "test-profile",
			SubjectSelectors: v1alpha1.SubjectSelectors{
				BuildarchList: []v1alpha1.Buildarch{v1alpha1.Arm64},
				UUIDList:      []string{testUUID.String()},
			},
		},
	}

	err := cl.Create(ctx, assignment)
	require.NoError(t, err, "failed to create Assignment")

	// Wait for reconciliation - labels should be added
	assert.Eventually(t, func() bool {
		var updatedAssignment v1alpha1.Assignment
		err := cl.Get(ctx, types.NamespacedName{Name: assignmentName, Namespace: namespace}, &updatedAssignment)
		if err != nil {
			t.Logf("Failed to get assignment: %v", err)
			return false
		}

		if updatedAssignment.Labels == nil {
			t.Log("Labels are nil")
			return false
		}

		// Check for buildarch label
		buildarchLabel := v1alpha1.LabelSelector(v1alpha1.Arm64.String(), v1alpha1.BuildarchPrefix)
		if _, ok := updatedAssignment.Labels[buildarchLabel]; !ok {
			t.Logf("Missing buildarch label: %s", buildarchLabel)
			return false
		}

		// Check for UUID label
		uuidLabel := v1alpha1.NewUUIDLabelSelector(testUUID)
		if _, ok := updatedAssignment.Labels[uuidLabel]; !ok {
			t.Logf("Missing UUID label: %s", uuidLabel)
			return false
		}

		return true
	}, reconciliationTimeout, pollInterval, "Assignment labels not reconciled")
}

// TestProfileIdempotence tests that updating a Profile doesn't cause infinite reconciliation
func TestProfileIdempotence(t *testing.T) {
	cl, namespace := setupTest(t)
	ctx := context.Background()

	if !controllerDeployed(t, cl) {
		t.Skip("shaper-controller not deployed, skipping idempotence test")
	}

	// Create a Profile
	profileName := "test-profile-idem-" + uuid.NewString()[:8]
	profile := &v1alpha1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      profileName,
			Namespace: namespace,
		},
		Spec: v1alpha1.ProfileSpec{
			IPXETemplate: "test template",
			AdditionalContent: []v1alpha1.AdditionalContent{
				{Name: "ignition", Exposed: true, PostTransformations: []v1alpha1.Transformer{}},
			},
		},
	}

	err := cl.Create(ctx, profile)
	require.NoError(t, err, "failed to create Profile")

	// Wait for initial reconciliation
	time.Sleep(3 * time.Second)

	// Get the profile with status
	var updatedProfile v1alpha1.Profile
	err = cl.Get(ctx, types.NamespacedName{Name: profileName, Namespace: namespace}, &updatedProfile)
	require.NoError(t, err, "failed to get Profile")

	firstUUID := updatedProfile.Status.ExposedAdditionalContent["ignition"]
	require.NotEmpty(t, firstUUID, "UUID not populated after first reconciliation")

	// Trigger another reconciliation by updating an annotation
	updatedProfile.Annotations = map[string]string{"test": "idempotence"}
	err = cl.Update(ctx, &updatedProfile)
	require.NoError(t, err, "failed to update Profile")

	// Wait a bit for any potential reconciliation
	time.Sleep(2 * time.Second)

	// Get profile again and verify UUID hasn't changed (idempotent)
	var finalProfile v1alpha1.Profile
	err = cl.Get(ctx, types.NamespacedName{Name: profileName, Namespace: namespace}, &finalProfile)
	require.NoError(t, err, "failed to get Profile")

	secondUUID := finalProfile.Status.ExposedAdditionalContent["ignition"]
	assert.Equal(t, firstUUID, secondUUID, "UUID changed on subsequent reconciliation (not idempotent)")
}

// TestAssignmentIdempotence tests that updating an Assignment doesn't cause infinite reconciliation
func TestAssignmentIdempotence(t *testing.T) {
	cl, namespace := setupTest(t)
	ctx := context.Background()

	if !controllerDeployed(t, cl) {
		t.Skip("shaper-controller not deployed, skipping idempotence test")
	}

	testUUID := uuid.New()

	// Create an Assignment
	assignmentName := "test-assignment-idem-" + uuid.NewString()[:8]
	assignment := &v1alpha1.Assignment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      assignmentName,
			Namespace: namespace,
		},
		Spec: v1alpha1.AssignmentSpec{
			ProfileName: "test-profile",
			SubjectSelectors: v1alpha1.SubjectSelectors{
				BuildarchList: []v1alpha1.Buildarch{v1alpha1.Arm64},
				UUIDList:      []string{testUUID.String()},
			},
		},
	}

	err := cl.Create(ctx, assignment)
	require.NoError(t, err, "failed to create Assignment")

	// Wait for initial reconciliation
	time.Sleep(3 * time.Second)

	// Get the assignment with labels
	var updatedAssignment v1alpha1.Assignment
	err = cl.Get(ctx, types.NamespacedName{Name: assignmentName, Namespace: namespace}, &updatedAssignment)
	require.NoError(t, err, "failed to get Assignment")

	firstLabels := make(map[string]string)
	for k, v := range updatedAssignment.Labels {
		firstLabels[k] = v
	}
	require.NotEmpty(t, firstLabels, "Labels not populated after first reconciliation")

	// Trigger another reconciliation by updating an annotation
	updatedAssignment.Annotations = map[string]string{"test": "idempotence"}
	err = cl.Update(ctx, &updatedAssignment)
	require.NoError(t, err, "failed to update Assignment")

	// Wait a bit for any potential reconciliation
	time.Sleep(2 * time.Second)

	// Get assignment again and verify labels haven't changed (idempotent)
	var finalAssignment v1alpha1.Assignment
	err = cl.Get(ctx, types.NamespacedName{Name: assignmentName, Namespace: namespace}, &finalAssignment)
	require.NoError(t, err, "failed to get Assignment")

	assert.Equal(t, firstLabels, finalAssignment.Labels, "Labels changed on subsequent reconciliation (not idempotent)")
}

// TestProfileDeletion tests that deleting a Profile is handled gracefully
func TestProfileDeletion(t *testing.T) {
	cl, namespace := setupTest(t)
	ctx := context.Background()

	// Create a Profile
	profileName := "test-profile-delete-" + uuid.NewString()[:8]
	profile := &v1alpha1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      profileName,
			Namespace: namespace,
		},
		Spec: v1alpha1.ProfileSpec{
			IPXETemplate: "test template",
			AdditionalContent: []v1alpha1.AdditionalContent{
				{Name: "ignition", Exposed: true, PostTransformations: []v1alpha1.Transformer{}},
			},
		},
	}

	err := cl.Create(ctx, profile)
	require.NoError(t, err, "failed to create Profile")

	// Wait for reconciliation
	time.Sleep(2 * time.Second)

	// Delete the Profile
	err = cl.Delete(ctx, profile)
	require.NoError(t, err, "failed to delete Profile")

	// Verify Profile is gone
	assert.Eventually(t, func() bool {
		var deletedProfile v1alpha1.Profile
		err := cl.Get(ctx, types.NamespacedName{Name: profileName, Namespace: namespace}, &deletedProfile)
		return err != nil // Should get "not found" error
	}, 10*time.Second, pollInterval, "Profile not deleted")
}
