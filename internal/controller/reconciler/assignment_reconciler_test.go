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

func TestAssignmentReconciler_Reconcile(t *testing.T) {
	testUUID1 := uuid.New()
	testUUID2 := uuid.New()

	tests := []struct {
		name           string
		assignment     *v1alpha1.Assignment
		expectUpdate   bool
		expectedLabels map[string]string // labels that should exist
		expectedError  bool
	}{
		{
			name: "Assignment without labels - should add buildarch and UUID labels",
			assignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-assignment",
					Namespace: "default",
				},
				Spec: v1alpha1.AssignmentSpec{
					ProfileName: "test-profile",
					SubjectSelectors: v1alpha1.SubjectSelectors{
						BuildarchList: []v1alpha1.Buildarch{v1alpha1.Arm64, v1alpha1.X8664},
						UUIDList:      []string{testUUID1.String(), testUUID2.String()},
					},
				},
			},
			expectUpdate: true,
			expectedLabels: map[string]string{
				v1alpha1.Arm64BuildarchLabelSelector:     "",
				v1alpha1.X8664BuildarchLabelSelector:     "",
				v1alpha1.NewUUIDLabelSelector(testUUID1): "",
				v1alpha1.NewUUIDLabelSelector(testUUID2): "",
			},
			expectedError: false,
		},
		{
			name: "Assignment with existing labels - should be idempotent",
			assignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-assignment",
					Namespace: "default",
					Labels: map[string]string{
						v1alpha1.Arm64BuildarchLabelSelector:     "",
						v1alpha1.NewUUIDLabelSelector(testUUID1): "",
					},
				},
				Spec: v1alpha1.AssignmentSpec{
					ProfileName: "test-profile",
					SubjectSelectors: v1alpha1.SubjectSelectors{
						BuildarchList: []v1alpha1.Buildarch{v1alpha1.Arm64},
						UUIDList:      []string{testUUID1.String()},
					},
				},
			},
			expectUpdate: false,
			expectedLabels: map[string]string{
				v1alpha1.Arm64BuildarchLabelSelector:     "",
				v1alpha1.NewUUIDLabelSelector(testUUID1): "",
			},
			expectedError: false,
		},
		{
			name: "Assignment with UUID selector but no label - should add UUID label",
			assignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-assignment",
					Namespace: "default",
					Labels:    map[string]string{},
				},
				Spec: v1alpha1.AssignmentSpec{
					ProfileName: "test-profile",
					SubjectSelectors: v1alpha1.SubjectSelectors{
						UUIDList: []string{testUUID1.String()},
					},
				},
			},
			expectUpdate: true,
			expectedLabels: map[string]string{
				v1alpha1.NewUUIDLabelSelector(testUUID1): "",
			},
			expectedError: false,
		},
		{
			name: "Assignment with buildarch selector but no label - should add buildarch label",
			assignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-assignment",
					Namespace: "default",
					Labels:    map[string]string{},
				},
				Spec: v1alpha1.AssignmentSpec{
					ProfileName: "test-profile",
					SubjectSelectors: v1alpha1.SubjectSelectors{
						BuildarchList: []v1alpha1.Buildarch{v1alpha1.I386},
					},
				},
			},
			expectUpdate: true,
			expectedLabels: map[string]string{
				v1alpha1.I386BuildarchLabelSelector: "",
			},
			expectedError: false,
		},
		{
			name: "Assignment with no selectors - no labels needed",
			assignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-assignment",
					Namespace: "default",
				},
				Spec: v1alpha1.AssignmentSpec{
					ProfileName:      "test-profile",
					SubjectSelectors: v1alpha1.SubjectSelectors{},
				},
			},
			expectUpdate:   false,
			expectedLabels: map[string]string{},
			expectedError:  false,
		},
		{
			name: "Assignment with invalid UUID - should skip invalid UUID",
			assignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-assignment",
					Namespace: "default",
				},
				Spec: v1alpha1.AssignmentSpec{
					ProfileName: "test-profile",
					SubjectSelectors: v1alpha1.SubjectSelectors{
						UUIDList: []string{"invalid-uuid", testUUID1.String()},
					},
				},
			},
			expectUpdate: true,
			expectedLabels: map[string]string{
				v1alpha1.NewUUIDLabelSelector(testUUID1): "",
			},
			expectedError: false, // Should not error, just skip invalid UUID
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create scheme and register types
			scheme := runtime.NewScheme()
			err := v1alpha1.AddToScheme(scheme)
			assert.NoError(t, err)

			// Create fake client
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.assignment).
				Build()

			// Create reconciler
			reconciler := &AssignmentReconciler{
				Client: fakeClient,
				Scheme: scheme,
				Log:    logr.Discard(),
			}

			// Reconcile
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.assignment.Name,
					Namespace: tt.assignment.Namespace,
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

			// Fetch updated assignment
			var updatedAssignment v1alpha1.Assignment
			err = fakeClient.Get(context.Background(), req.NamespacedName, &updatedAssignment)
			assert.NoError(t, err)

			// Verify labels
			for expectedKey, expectedValue := range tt.expectedLabels {
				assert.Contains(t, updatedAssignment.Labels, expectedKey,
					"Label %s should exist", expectedKey)
				assert.Equal(t, expectedValue, updatedAssignment.Labels[expectedKey],
					"Label %s should have value %s", expectedKey, expectedValue)
			}

			// Verify only expected labels exist (plus any original non-shaper labels)
			for key := range updatedAssignment.Labels {
				if v1alpha1.IsInternalLabel(key) {
					assert.Contains(t, tt.expectedLabels, key,
						"Unexpected shaper label: %s", key)
				}
			}
		})
	}
}

func TestAssignmentReconciler_Reconcile_NotFound(t *testing.T) {
	// Create scheme
	scheme := runtime.NewScheme()
	err := v1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	// Create fake client WITHOUT the assignment
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Create reconciler
	reconciler := &AssignmentReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Log:    logr.Discard(),
	}

	// Try to reconcile non-existent assignment
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

func TestAssignmentReconciler_Reconcile_Idempotency(t *testing.T) {
	testUUID := uuid.New()

	// Create assignment
	assignment := &v1alpha1.Assignment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-assignment",
			Namespace: "default",
		},
		Spec: v1alpha1.AssignmentSpec{
			ProfileName: "test-profile",
			SubjectSelectors: v1alpha1.SubjectSelectors{
				BuildarchList: []v1alpha1.Buildarch{v1alpha1.Arm64},
				UUIDList:      []string{testUUID.String()},
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
		WithObjects(assignment).
		Build()

	// Create reconciler
	reconciler := &AssignmentReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Log:    logr.Discard(),
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      assignment.Name,
			Namespace: assignment.Namespace,
		},
	}

	// First reconcile - should add labels
	result, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Fetch assignment after first reconcile
	var assignment1 v1alpha1.Assignment
	err = fakeClient.Get(context.Background(), req.NamespacedName, &assignment1)
	assert.NoError(t, err)
	assert.NotEmpty(t, assignment1.Labels)

	// Save labels from first reconcile
	firstLabels := make(map[string]string)
	for k, v := range assignment1.Labels {
		firstLabels[k] = v
	}

	// Second reconcile - should not change labels
	result, err = reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Fetch assignment after second reconcile
	var assignment2 v1alpha1.Assignment
	err = fakeClient.Get(context.Background(), req.NamespacedName, &assignment2)
	assert.NoError(t, err)

	// Labels should remain the same (idempotent)
	assert.Equal(t, firstLabels, assignment2.Labels,
		"Labels should not change on subsequent reconciles")
}
