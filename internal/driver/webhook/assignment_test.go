//go:build unit

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

package webhook_test

import (
	"context"
	"testing"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	"github.com/alexandremahdhaoui/shaper/internal/driver/webhook"
	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/internal/util/mocks/mockadapter"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewAssignment(t *testing.T) {
	mockAssignment := mockadapter.NewMockAssignment(t)
	mockProfile := mockadapter.NewMockProfile(t)

	webhook := webhook.NewAssignment(mockAssignment, mockProfile)

	assert.NotNil(t, webhook)
}

func TestAssignment_Default_Success(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name            string
		inputAssignment *v1alpha1.Assignment
		verifyLabels    func(t *testing.T, assignment *v1alpha1.Assignment)
	}{
		{
			name: "assignment with UUID list creates labels",
			inputAssignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-assignment",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						UUIDList: []string{testUUID.String()},
					},
					ProfileName: "test-profile",
				},
			},
			verifyLabels: func(t *testing.T, assignment *v1alpha1.Assignment) {
				// Should have UUID label
				foundUUID := false
				for k, v := range assignment.Labels {
					if v1alpha1.IsUUIDLabelSelector(k) && v == "" {
						foundUUID = true
					}
				}
				assert.True(t, foundUUID, "should have UUID label")
			},
		},
		{
			name: "assignment with buildarch list creates labels",
			inputAssignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-assignment-2",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						BuildarchList: []v1alpha1.Buildarch{v1alpha1.X8664},
					},
					ProfileName: "test-profile",
				},
			},
			verifyLabels: func(t *testing.T, assignment *v1alpha1.Assignment) {
				// Should have buildarch label
				buildarchs := assignment.GetBuildarchList()
				assert.Contains(t, buildarchs, v1alpha1.X8664)
			},
		},
		{
			name: "empty buildarch list sets all allowed buildarchs",
			inputAssignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-assignment-3",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						BuildarchList: []v1alpha1.Buildarch{},
					},
					ProfileName: "test-profile",
				},
			},
			verifyLabels: func(t *testing.T, assignment *v1alpha1.Assignment) {
				// Should have all buildarch labels
				buildarchs := assignment.GetBuildarchList()
				assert.Equal(t, 4, len(buildarchs), "should have all 4 allowed buildarchs")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAssignment := mockadapter.NewMockAssignment(t)
			mockProfile := mockadapter.NewMockProfile(t)

			w := webhook.NewAssignment(mockAssignment, mockProfile)

			ctx := context.Background()
			err := w.Default(ctx, tt.inputAssignment)

			assert.NoError(t, err)
			tt.verifyLabels(t, tt.inputAssignment)
		})
	}
}

func TestAssignment_Default_Error(t *testing.T) {
	tests := []struct {
		name        string
		inputObj    runtime.Object
		expectError bool
	}{
		{
			name: "invalid UUID format",
			inputObj: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "invalid-uuid",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						UUIDList: []string{"not-a-valid-uuid"},
					},
					ProfileName: "test-profile",
				},
			},
			expectError: true,
		},
		{
			name:        "non-Assignment object",
			inputObj:    &v1alpha1.Profile{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAssignment := mockadapter.NewMockAssignment(t)
			mockProfile := mockadapter.NewMockProfile(t)

			w := webhook.NewAssignment(mockAssignment, mockProfile)

			ctx := context.Background()
			err := w.Default(ctx, tt.inputObj)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAssignment_ValidateCreate_Success(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name            string
		inputAssignment *v1alpha1.Assignment
		setupMocks      func(*mockadapter.MockAssignment, *mockadapter.MockProfile)
	}{
		{
			name: "valid assignment with UUID and existing profile",
			inputAssignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-assignment",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						UUIDList:      []string{testUUID.String()},
						BuildarchList: []v1alpha1.Buildarch{v1alpha1.X8664},
					},
					ProfileName: "test-profile",
				},
			},
			setupMocks: func(ma *mockadapter.MockAssignment, mp *mockadapter.MockProfile) {
				mp.EXPECT().Get(mock.Anything, "test-profile").Return(types.Profile{}, nil)
				ma.EXPECT().FindBySelectors(mock.Anything, mock.Anything).Return(types.Assignment{}, adapter.ErrAssignmentNotFound)
			},
		},
		{
			name: "valid default assignment",
			inputAssignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "default-assignment",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						BuildarchList: []v1alpha1.Buildarch{v1alpha1.X8664},
					},
					ProfileName: "test-profile",
					IsDefault:   true,
				},
			},
			setupMocks: func(ma *mockadapter.MockAssignment, mp *mockadapter.MockProfile) {
				mp.EXPECT().Get(mock.Anything, "test-profile").Return(types.Profile{}, nil)
				ma.EXPECT().FindDefaultByBuildarch(mock.Anything, "x86_64").Return(types.Assignment{}, adapter.ErrAssignmentNotFound)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAssignment := mockadapter.NewMockAssignment(t)
			mockProfile := mockadapter.NewMockProfile(t)

			tt.setupMocks(mockAssignment, mockProfile)

			w := webhook.NewAssignment(mockAssignment, mockProfile)

			ctx := context.Background()
			// Call Default() first to set labels (mimics admission webhook flow)
			err := w.Default(ctx, tt.inputAssignment)
			assert.NoError(t, err)

			warnings, err := w.ValidateCreate(ctx, tt.inputAssignment)

			assert.NoError(t, err)
			assert.Nil(t, warnings)
		})
	}
}

func TestAssignment_ValidateCreate_StaticError(t *testing.T) {
	tests := []struct {
		name          string
		inputObj      runtime.Object
		errorContains string
	}{
		{
			name: "invalid UUID in list",
			inputObj: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "invalid-uuid",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						UUIDList: []string{"not-a-uuid"},
					},
					ProfileName: "test-profile",
				},
			},
			errorContains: "invalid UUID",
		},
		{
			name: "default assignment with UUID selectors",
			inputObj: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "invalid-default",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						UUIDList: []string{"550e8400-e29b-41d4-a716-446655440000"},
					},
					ProfileName: "test-profile",
					IsDefault:   true,
				},
			},
			errorContains: "default assignment must not specify",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAssignment := mockadapter.NewMockAssignment(t)
			mockProfile := mockadapter.NewMockProfile(t)

			w := webhook.NewAssignment(mockAssignment, mockProfile)

			ctx := context.Background()
			warnings, err := w.ValidateCreate(ctx, tt.inputObj)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorContains)
			assert.Nil(t, warnings)
		})
	}
}

func TestAssignment_ValidateCreate_DynamicError(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name            string
		inputAssignment *v1alpha1.Assignment
		setupMocks      func(*mockadapter.MockAssignment, *mockadapter.MockProfile)
		errorContains   string
	}{
		{
			name: "profile not found",
			inputAssignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-assignment",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						UUIDList:      []string{testUUID.String()},
						BuildarchList: []v1alpha1.Buildarch{v1alpha1.X8664},
					},
					ProfileName: "nonexistent-profile",
				},
			},
			setupMocks: func(ma *mockadapter.MockAssignment, mp *mockadapter.MockProfile) {
				mp.EXPECT().Get(mock.Anything, "nonexistent-profile").Return(types.Profile{}, adapter.ErrProfileNotFound)
			},
			errorContains: "assignment must specify an existing profileName",
		},
		{
			name: "duplicate UUID assignment",
			inputAssignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-assignment",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						UUIDList:      []string{testUUID.String()},
						BuildarchList: []v1alpha1.Buildarch{v1alpha1.X8664},
					},
					ProfileName: "test-profile",
				},
			},
			setupMocks: func(ma *mockadapter.MockAssignment, mp *mockadapter.MockProfile) {
				mp.EXPECT().Get(mock.Anything, "test-profile").Return(types.Profile{}, nil)
				ma.EXPECT().FindBySelectors(mock.Anything, mock.Anything).Return(types.Assignment{}, nil)
			},
			errorContains: "assignment cannot reference a subject selector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAssignment := mockadapter.NewMockAssignment(t)
			mockProfile := mockadapter.NewMockProfile(t)

			tt.setupMocks(mockAssignment, mockProfile)

			w := webhook.NewAssignment(mockAssignment, mockProfile)

			ctx := context.Background()
			warnings, err := w.ValidateCreate(ctx, tt.inputAssignment)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorContains)
			assert.Nil(t, warnings)
		})
	}
}

func TestAssignment_ValidateUpdate(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name          string
		oldAssignment *v1alpha1.Assignment
		newAssignment *v1alpha1.Assignment
		setupMocks    func(*mockadapter.MockAssignment, *mockadapter.MockProfile)
		expectError   bool
	}{
		{
			name: "valid update",
			oldAssignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-assignment",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						UUIDList:      []string{testUUID.String()},
						BuildarchList: []v1alpha1.Buildarch{v1alpha1.X8664},
					},
					ProfileName: "test-profile",
				},
			},
			newAssignment: &v1alpha1.Assignment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-assignment",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.AssignmentSpec{
					SubjectSelectors: v1alpha1.SubjectSelectors{
						UUIDList:      []string{testUUID.String()},
						BuildarchList: []v1alpha1.Buildarch{v1alpha1.X8664},
					},
					ProfileName: "test-profile-updated",
				},
			},
			setupMocks: func(ma *mockadapter.MockAssignment, mp *mockadapter.MockProfile) {
				mp.EXPECT().Get(mock.Anything, "test-profile-updated").Return(types.Profile{}, nil)
				ma.EXPECT().FindBySelectors(mock.Anything, mock.Anything).Return(types.Assignment{Name: "test-assignment"}, nil)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAssignment := mockadapter.NewMockAssignment(t)
			mockProfile := mockadapter.NewMockProfile(t)

			tt.setupMocks(mockAssignment, mockProfile)

			w := webhook.NewAssignment(mockAssignment, mockProfile)

			ctx := context.Background()
			warnings, err := w.ValidateUpdate(ctx, tt.oldAssignment, tt.newAssignment)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Nil(t, warnings)
		})
	}
}

func TestAssignment_ValidateDelete(t *testing.T) {
	assignment := &v1alpha1.Assignment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-assignment",
			Labels: make(map[string]string),
		},
		Spec: v1alpha1.AssignmentSpec{
			ProfileName: "test-profile",
		},
	}

	mockAssignment := mockadapter.NewMockAssignment(t)
	mockProfile := mockadapter.NewMockProfile(t)

	w := webhook.NewAssignment(mockAssignment, mockProfile)

	ctx := context.Background()
	warnings, err := w.ValidateDelete(ctx, assignment)

	assert.NoError(t, err)
	assert.Nil(t, warnings)
}
