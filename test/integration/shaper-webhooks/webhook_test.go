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

package webhook_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
)

const testTimeout = 30 * time.Second

// TestAssignmentValidation tests validation of Assignment CRDs
func TestAssignmentValidation(t *testing.T) {
	require.NotEmpty(t, os.Getenv("KUBECONFIG"), "KUBECONFIG must be set by forge testenv")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	cl := newTestClient(t)
	defer cleanupAssignments(t, cl)
	defer cleanupProfiles(t, cl)

	require.True(t, webhooksDeployed(t, cl), "shaper-webhooks must be deployed by forge testenv")

	// Create the test-profile that assignments reference
	_ = createTestProfile(t, cl, ctx)

	tests := []struct {
		name        string
		fixture     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid assignment accepted",
			fixture:     "valid-assignment.yaml",
			expectError: false,
		},
		{
			name:        "invalid UUID rejected",
			fixture:     "invalid-assignment-uuid.yaml",
			expectError: true,
			errorMsg:    "UUID",
		},
		{
			name:        "invalid buildarch rejected",
			fixture:     "invalid-assignment-buildarch.yaml",
			expectError: true,
			errorMsg:    "buildarch",
		},
		{
			name:        "default assignment with UUID rejected",
			fixture:     "invalid-assignment-default.yaml",
			expectError: true,
			errorMsg:    "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignment := loadAssignmentFixture(t, tt.fixture)

			err := cl.Create(ctx, assignment)

			if tt.expectError {
				require.Error(t, err, "expected validation error but got none")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "error message should contain expected text")
				}
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)

				// Clean up the created resource
				_ = cl.Delete(ctx, assignment)
			}
		})
	}
}

// TestAssignmentMutation tests mutation (defaulting) of Assignment CRDs
func TestAssignmentMutation(t *testing.T) {
	require.NotEmpty(t, os.Getenv("KUBECONFIG"), "KUBECONFIG must be set by forge testenv")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	cl := newTestClient(t)
	defer cleanupAssignments(t, cl)
	defer cleanupProfiles(t, cl)

	require.True(t, webhooksDeployed(t, cl), "shaper-webhooks must be deployed by forge testenv")

	// Create the test-profile that assignments reference
	_ = createTestProfile(t, cl, ctx)

	assignment := loadAssignmentFixture(t, "valid-assignment.yaml")
	assignment.Name = "test-mutation"

	// Create the assignment
	err := cl.Create(ctx, assignment)
	require.NoError(t, err, "failed to create assignment")
	defer func() { _ = cl.Delete(ctx, assignment) }()

	// Get the assignment to verify mutations
	created := &v1alpha1.Assignment{}
	err = cl.Get(ctx, client.ObjectKey{
		Namespace: assignment.Namespace,
		Name:      assignment.Name,
	}, created)
	require.NoError(t, err, "failed to get created assignment")

	// Verify UUID labels were added
	assert.NotEmpty(t, created.Labels, "labels should not be empty after mutation")

	// Count UUID labels
	uuidLabelCount := 0
	for k := range created.Labels {
		if v1alpha1.IsUUIDLabelSelector(k) {
			uuidLabelCount++
		}
	}
	assert.Equal(t, len(assignment.Spec.SubjectSelectors.UUIDList), uuidLabelCount,
		"should have UUID label for each UUID in spec")

	// Verify buildarch labels were added by checking GetBuildarchList()
	buildarchList := created.GetBuildarchList()
	assert.Greater(t, len(buildarchList), 0, "should have at least one buildarch label")
}

// TestProfileValidation tests validation of Profile CRDs
func TestProfileValidation(t *testing.T) {
	require.NotEmpty(t, os.Getenv("KUBECONFIG"), "KUBECONFIG must be set by forge testenv")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	cl := newTestClient(t)
	defer cleanupProfiles(t, cl)

	require.True(t, webhooksDeployed(t, cl), "shaper-webhooks must be deployed by forge testenv")

	tests := []struct {
		name        string
		fixture     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid profile accepted",
			fixture:     "valid-profile.yaml",
			expectError: false,
		},
		{
			name:        "multiple content sources rejected",
			fixture:     "invalid-profile-multiple-content.yaml",
			expectError: true,
			errorMsg:    "exactly 1 content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := loadProfileFixture(t, tt.fixture)

			err := cl.Create(ctx, profile)

			if tt.expectError {
				require.Error(t, err, "expected validation error but got none")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "error message should contain expected text")
				}
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)

				// Clean up the created resource
				_ = cl.Delete(ctx, profile)
			}
		})
	}
}

// TestProfileMutation tests mutation (defaulting) of Profile CRDs
func TestProfileMutation(t *testing.T) {
	require.NotEmpty(t, os.Getenv("KUBECONFIG"), "KUBECONFIG must be set by forge testenv")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	cl := newTestClient(t)
	defer cleanupProfiles(t, cl)

	require.True(t, webhooksDeployed(t, cl), "shaper-webhooks must be deployed by forge testenv")

	profile := loadProfileFixture(t, "valid-profile-mutation.yaml")

	// Create the profile
	err := cl.Create(ctx, profile)
	require.NoError(t, err, "failed to create profile")
	defer func() { _ = cl.Delete(ctx, profile) }()

	// Get the profile to verify mutations
	created := &v1alpha1.Profile{}
	err = cl.Get(ctx, client.ObjectKey{
		Namespace: profile.Namespace,
		Name:      profile.Name,
	}, created)
	require.NoError(t, err, "failed to get created profile")

	// Verify UUID labels were added for exposed content
	assert.NotEmpty(t, created.Labels, "labels should not be empty after mutation")

	// Count exposed content in spec
	exposedCount := 0
	for _, content := range profile.Spec.AdditionalContent {
		if content.Exposed {
			exposedCount++
		}
	}

	// Count UUID labels
	uuidLabelCount := 0
	for k, v := range created.Labels {
		if v1alpha1.IsUUIDLabelSelector(k) {
			uuidLabelCount++
			// Verify the label value is the content name
			assert.NotEmpty(t, v, "UUID label should have content name as value")
		}
	}

	assert.Equal(t, exposedCount, uuidLabelCount,
		"should have UUID label for each exposed content")

	// Test idempotency - update should preserve UUIDs
	// Use retry loop to handle optimistic concurrency conflicts from controller reconciliation
	var updateErr error
	for i := 0; i < 3; i++ {
		// Get fresh version before update
		fresh := &v1alpha1.Profile{}
		err = cl.Get(ctx, client.ObjectKey{
			Namespace: profile.Namespace,
			Name:      profile.Name,
		}, fresh)
		require.NoError(t, err, "failed to get fresh profile for update")

		fresh.Spec.IPXETemplate = "#!ipxe\necho Updated template\n"
		updateErr = cl.Update(ctx, fresh)
		if updateErr == nil {
			created = fresh // use the successfully updated object for subsequent checks
			break
		}
		if !errors.IsConflict(updateErr) {
			break // non-conflict error, don't retry
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NoError(t, updateErr, "failed to update profile after retries")

	// Get again and verify UUIDs are preserved
	updated := &v1alpha1.Profile{}
	err = cl.Get(ctx, client.ObjectKey{
		Namespace: profile.Namespace,
		Name:      profile.Name,
	}, updated)
	require.NoError(t, err, "failed to get updated profile")

	// UUIDs should be preserved
	for k := range created.Labels {
		if v1alpha1.IsUUIDLabelSelector(k) {
			assert.Contains(t, updated.Labels, k, "UUID label should be preserved after update")
		}
	}
}

// Helper functions

func newTestClient(t *testing.T) client.Client {
	t.Helper()

	cfg, err := config.GetConfig()
	require.NoError(t, err, "failed to get kubeconfig")

	scheme := runtime.NewScheme()
	err = v1alpha1.AddToScheme(scheme)
	require.NoError(t, err, "failed to add v1alpha1 to scheme")
	err = admissionregistrationv1.AddToScheme(scheme)
	require.NoError(t, err, "failed to add admissionregistration to scheme")

	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err, "failed to create client")

	return cl
}

// webhooksDeployed checks if the shaper webhooks are deployed in the cluster
func webhooksDeployed(t *testing.T, cl client.Client) bool {
	t.Helper()

	ctx := context.Background()

	// Check for MutatingWebhookConfiguration for shaper
	mwcList := &admissionregistrationv1.MutatingWebhookConfigurationList{}
	if err := cl.List(ctx, mwcList); err != nil {
		t.Logf("failed to list MutatingWebhookConfigurations: %v", err)
		return false
	}

	for _, mwc := range mwcList.Items {
		// Check if any webhook configuration is related to shaper
		for _, wh := range mwc.Webhooks {
			if wh.ClientConfig.Service != nil &&
				(wh.ClientConfig.Service.Name == "shaper-webhooks" ||
					wh.ClientConfig.Service.Name == "shaper-webhook") {
				return true
			}
		}
	}

	// Check for ValidatingWebhookConfiguration for shaper
	vwcList := &admissionregistrationv1.ValidatingWebhookConfigurationList{}
	if err := cl.List(ctx, vwcList); err != nil {
		t.Logf("failed to list ValidatingWebhookConfigurations: %v", err)
		return false
	}

	for _, vwc := range vwcList.Items {
		for _, wh := range vwc.Webhooks {
			if wh.ClientConfig.Service != nil &&
				(wh.ClientConfig.Service.Name == "shaper-webhooks" ||
					wh.ClientConfig.Service.Name == "shaper-webhook") {
				return true
			}
		}
	}

	return false
}

func loadAssignmentFixture(t *testing.T, filename string) *v1alpha1.Assignment {
	t.Helper()

	path := filepath.Join("fixtures", filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read fixture file: %s", filename)

	assignment := &v1alpha1.Assignment{}
	err = yaml.Unmarshal(data, assignment)
	require.NoError(t, err, "failed to unmarshal fixture: %s", filename)

	// Ensure labels map is initialized
	if assignment.Labels == nil {
		assignment.Labels = make(map[string]string)
	}

	return assignment
}

func loadProfileFixture(t *testing.T, filename string) *v1alpha1.Profile {
	t.Helper()

	path := filepath.Join("fixtures", filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read fixture file: %s", filename)

	profile := &v1alpha1.Profile{}
	err = yaml.Unmarshal(data, profile)
	require.NoError(t, err, "failed to unmarshal fixture: %s", filename)

	// Ensure labels map is initialized
	if profile.Labels == nil {
		profile.Labels = make(map[string]string)
	}

	return profile
}

// ptr returns a pointer to the given value
func ptr[T any](v T) *T {
	return &v
}

// createTestProfile creates a profile named "test-profile" that assignment fixtures reference
func createTestProfile(t *testing.T, cl client.Client, ctx context.Context) *v1alpha1.Profile {
	t.Helper()

	profile := &v1alpha1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-profile",
			Namespace: "default",
		},
		Spec: v1alpha1.ProfileSpec{
			IPXETemplate: "#!ipxe\necho Test Profile\n",
			AdditionalContent: []v1alpha1.AdditionalContent{
				{
					Name:    "config",
					Exposed: false,
					Inline:  ptr("test-config-content"),
				},
			},
		},
	}

	err := cl.Create(ctx, profile)
	require.NoError(t, err, "failed to create test-profile for assignment tests")

	return profile
}

func cleanupAssignments(t *testing.T, cl client.Client) {
	t.Helper()

	ctx := context.Background()
	list := &v1alpha1.AssignmentList{}
	err := cl.List(ctx, list, client.InNamespace("default"))
	if err != nil {
		t.Logf("failed to list assignments for cleanup: %v", err)
		return
	}

	for _, item := range list.Items {
		err := cl.Delete(ctx, &item)
		if err != nil && !errors.IsNotFound(err) {
			t.Logf("failed to delete assignment %s: %v", item.Name, err)
		}
	}
}

func cleanupProfiles(t *testing.T, cl client.Client) {
	t.Helper()

	ctx := context.Background()
	list := &v1alpha1.ProfileList{}
	err := cl.List(ctx, list, client.InNamespace("default"))
	if err != nil {
		t.Logf("failed to list profiles for cleanup: %v", err)
		return
	}

	for _, item := range list.Items {
		err := cl.Delete(ctx, &item)
		if err != nil && !errors.IsNotFound(err) {
			t.Logf("failed to delete profile %s: %v", item.Name, err)
		}
	}
}
