//go:build unit

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

package reconciler

import (
	"context"
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestProfileReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name          string
		profile       *v1alpha1.Profile
		expectUpdate  bool
		expectUUIDs   map[string]bool // content names that should have UUIDs
		expectedError bool
	}{
		{
			name: "Profile with no status - should populate UUIDs",
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile",
					Namespace: "default",
				},
				Spec: v1alpha1.ProfileSpec{
					IPXETemplate: "test template",
					AdditionalContent: []v1alpha1.AdditionalContent{
						{Name: "ignition", Exposed: true},
						{Name: "config", Exposed: true},
					},
				},
			},
			expectUpdate: true,
			expectUUIDs: map[string]bool{
				"ignition": true,
				"config":   true,
			},
			expectedError: false,
		},
		{
			name: "Profile with existing status - should be idempotent",
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile",
					Namespace: "default",
				},
				Spec: v1alpha1.ProfileSpec{
					IPXETemplate: "test template",
					AdditionalContent: []v1alpha1.AdditionalContent{
						{Name: "ignition", Exposed: true},
					},
				},
				Status: v1alpha1.ProfileStatus{
					ExposedAdditionalContent: map[string]string{
						"ignition": "existing-uuid",
					},
				},
			},
			expectUpdate: false,
			expectUUIDs: map[string]bool{
				"ignition": true,
			},
			expectedError: false,
		},
		{
			name: "Profile with mixed exposed/non-exposed content",
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile",
					Namespace: "default",
				},
				Spec: v1alpha1.ProfileSpec{
					IPXETemplate: "test template",
					AdditionalContent: []v1alpha1.AdditionalContent{
						{Name: "exposed-config", Exposed: true},
						{Name: "inline-config", Exposed: false},
					},
				},
			},
			expectUpdate: true,
			expectUUIDs: map[string]bool{
				"exposed-config": true,
			},
			expectedError: false,
		},
		{
			name: "Profile with no additional content",
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile",
					Namespace: "default",
				},
				Spec: v1alpha1.ProfileSpec{
					IPXETemplate:      "test template",
					AdditionalContent: []v1alpha1.AdditionalContent{},
				},
			},
			expectUpdate:  false,
			expectUUIDs:   map[string]bool{},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create scheme and register types
			scheme := runtime.NewScheme()
			err := v1alpha1.AddToScheme(scheme)
			assert.NoError(t, err)

			// Create fake client with status subresource
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.profile).
				WithStatusSubresource(tt.profile).
				Build()

			// Create reconciler
			reconciler := &ProfileReconciler{
				Client: fakeClient,
				Scheme: scheme,
				Log:    logr.Discard(),
			}

			// Reconcile
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.profile.Name,
					Namespace: tt.profile.Namespace,
				},
			}

			result, err := reconciler.Reconcile(context.Background(), req)

			// Verify error expectation
			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, result)

			// Fetch updated profile
			var updatedProfile v1alpha1.Profile
			err = fakeClient.Get(context.Background(), req.NamespacedName, &updatedProfile)
			assert.NoError(t, err)

			// Verify status has expected UUIDs
			if len(tt.expectUUIDs) > 0 {
				assert.NotNil(t, updatedProfile.Status.ExposedAdditionalContent)
				assert.Equal(t, len(tt.expectUUIDs), len(updatedProfile.Status.ExposedAdditionalContent))

				for contentName := range tt.expectUUIDs {
					assert.Contains(t, updatedProfile.Status.ExposedAdditionalContent, contentName)
					contentUUID := updatedProfile.Status.ExposedAdditionalContent[contentName]

					// Verify UUID format (unless it's an existing UUID we set)
					if contentUUID != "existing-uuid" {
						_, err := uuid.Parse(contentUUID)
						assert.NoError(t, err, "UUID should be valid format")
					}
				}
			}

			// Verify non-exposed content doesn't have UUIDs
			for _, content := range tt.profile.Spec.AdditionalContent {
				if !content.Exposed {
					assert.NotContains(t, updatedProfile.Status.ExposedAdditionalContent, content.Name,
						"Non-exposed content should not have UUID")
				}
			}
		})
	}
}

func TestProfileReconciler_Reconcile_NotFound(t *testing.T) {
	// Create scheme
	scheme := runtime.NewScheme()
	err := v1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	// Create fake client WITHOUT the profile
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Create reconciler
	reconciler := &ProfileReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Log:    logr.Discard(),
	}

	// Try to reconcile non-existent profile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)

	// Should return no error when resource is not found
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestProfileReconciler_Reconcile_Idempotency(t *testing.T) {
	// Create profile
	profile := &v1alpha1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-profile",
			Namespace: "default",
		},
		Spec: v1alpha1.ProfileSpec{
			IPXETemplate: "test template",
			AdditionalContent: []v1alpha1.AdditionalContent{
				{Name: "ignition", Exposed: true},
			},
		},
	}

	// Create scheme
	scheme := runtime.NewScheme()
	err := v1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(profile).
		WithStatusSubresource(profile).
		Build()

	// Create reconciler
	reconciler := &ProfileReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Log:    logr.Discard(),
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      profile.Name,
			Namespace: profile.Namespace,
		},
	}

	// First reconcile - should generate UUID
	result, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Fetch profile after first reconcile
	var profile1 v1alpha1.Profile
	err = fakeClient.Get(context.Background(), req.NamespacedName, &profile1)
	assert.NoError(t, err)
	assert.NotEmpty(t, profile1.Status.ExposedAdditionalContent)
	firstUUID := profile1.Status.ExposedAdditionalContent["ignition"]
	assert.NotEmpty(t, firstUUID)

	// Second reconcile - should not change UUID
	result, err = reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Fetch profile after second reconcile
	var profile2 v1alpha1.Profile
	err = fakeClient.Get(context.Background(), req.NamespacedName, &profile2)
	assert.NoError(t, err)

	// UUID should remain the same (idempotent)
	secondUUID := profile2.Status.ExposedAdditionalContent["ignition"]
	assert.Equal(t, firstUUID, secondUUID, "UUID should not change on subsequent reconciles")
}
