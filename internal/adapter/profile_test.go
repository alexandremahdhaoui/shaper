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

package adapter_test

import (
	"context"
	"testing"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	"github.com/alexandremahdhaoui/shaper/internal/util/mocks/mockclient"
	"github.com/alexandremahdhaoui/shaper/internal/util/testutil"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestProfile(t *testing.T) {
	var (
		ctx       context.Context
		namespace string

		inputProfileName string
		inputContentID   uuid.UUID

		v1alpha1Profile     v1alpha1.Profile
		v1alpha1ProfileList v1alpha1.ProfileList
		expectedErr         error

		cl      *mockclient.MockClient
		profile adapter.Profile
	)

	setup := func(t *testing.T) func() {
		t.Helper()

		ctx = context.Background()
		namespace = "test-profile"

		inputProfileName = "profile-name"
		inputContentID = uuid.New()

		v1alpha1Profile = testutil.NewV1alpha1Profile()
		v1alpha1ProfileList = v1alpha1.ProfileList{}
		expectedErr = nil // Reset error to nil for each test

		cl = mockclient.NewMockClient(t)
		profile = adapter.NewProfile(cl, namespace)

		return func() {
			t.Helper()

			cl.AssertExpectations(t)
		}
	}

	get := func(t *testing.T) {
		t.Helper()

		cl.EXPECT().
			Get(ctx, types2.NamespacedName{
				Namespace: namespace,
				Name:      inputProfileName,
			}, mock.Anything).
			RunAndReturn(func(_ context.Context, _ types2.NamespacedName, obj client.Object, _ ...client.GetOption) error {
				p := obj.(*v1alpha1.Profile)
				*p = v1alpha1Profile

				return expectedErr
			})
	}

	listByContentID := func(t *testing.T) {
		t.Helper()

		cl.EXPECT().
			List(ctx, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.ObjectList, opts ...client.ListOption) error {
				list := obj.(*v1alpha1.ProfileList)
				*list = v1alpha1ProfileList

				return expectedErr
			})
	}

	t.Run("Get", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			defer setup(t)()

			expected := testutil.NewTypesProfile()

			get(t)

			actual, err := profile.Get(ctx, inputProfileName)
			assert.NoError(t, err)
			assert.Equal(t, expected, testutil.MakeProfileComparable(actual))
		})

		t.Run("Failure", func(t *testing.T) {
			t.Run("Get error", func(t *testing.T) {
				defer setup(t)()

				expectedErr = assert.AnError
				get(t)

				_, err := profile.Get(ctx, inputProfileName)
				assert.ErrorIs(t, err, assert.AnError)
			})
		})
	})

	t.Run("ListByContentID", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			t.Run("Single profile found", func(t *testing.T) {
				defer setup(t)()

				// Arrange
				testUUID := uuid.New()
				inputContentID = testUUID

				profile1 := testutil.NewV1alpha1ProfileWithExposedContent("profile-1", []testutil.ExposedContentItem{
					{Name: "config1", UUID: testUUID, Body: "content1", Exposed: true},
				})

				v1alpha1ProfileList = v1alpha1.ProfileList{
					Items: []v1alpha1.Profile{profile1},
				}

				listByContentID(t)

				// Act
				result, err := profile.ListByContentID(ctx, inputContentID)

				// Assert - Basic checks
				assert.NoError(t, err)
				assert.Len(t, result, 1)

				// Assert - ContentIDToNameMap verification
				assert.NotEmpty(t, result[0].ContentIDToNameMap)
				assert.Len(t, result[0].ContentIDToNameMap, 1, "Should have exactly 1 exposed content")
				assert.Contains(t, result[0].ContentIDToNameMap, testUUID)
				assert.Equal(t, "config1", result[0].ContentIDToNameMap[testUUID])

				// Assert - Verify label format was correct
				expectedLabel := v1alpha1.NewUUIDLabelSelector(testUUID)
				assert.Contains(t, expectedLabel, "uuid.shaper.amahdha.com/")
				assert.Contains(t, expectedLabel, testUUID.String())

				// Assert - Verify content structure
				assert.Contains(t, result[0].AdditionalContent, "config1")
				assert.Equal(t, "content1", result[0].AdditionalContent["config1"].Inline)
				assert.True(t, result[0].AdditionalContent["config1"].Exposed)
				assert.Equal(t, testUUID, result[0].AdditionalContent["config1"].ExposedUUID)
			})

			t.Run("Multiple profiles found", func(t *testing.T) {
				defer setup(t)()

				// Arrange
				testUUID := uuid.New()
				inputContentID = testUUID

				profile1 := testutil.NewV1alpha1ProfileWithExposedContent("profile-1", []testutil.ExposedContentItem{
					{Name: "config1", UUID: testUUID, Body: "content1", Exposed: true},
				})

				profile2 := testutil.NewV1alpha1ProfileWithExposedContent("profile-2", []testutil.ExposedContentItem{
					{Name: "config2", UUID: testUUID, Body: "content2", Exposed: true},
				})

				v1alpha1ProfileList = v1alpha1.ProfileList{
					Items: []v1alpha1.Profile{profile1, profile2},
				}

				listByContentID(t)

				// Act
				result, err := profile.ListByContentID(ctx, inputContentID)

				// Assert - Basic checks
				assert.NoError(t, err)
				assert.Len(t, result, 2)

				// Assert - Both profiles should have the UUID in their ContentIDToNameMap
				assert.Contains(t, result[0].ContentIDToNameMap, testUUID)
				assert.Contains(t, result[1].ContentIDToNameMap, testUUID)

				// Assert - Verify content names match
				assert.Equal(t, "config1", result[0].ContentIDToNameMap[testUUID])
				assert.Equal(t, "config2", result[1].ContentIDToNameMap[testUUID])

				// Assert - Verify both use the same label format
				expectedLabel := v1alpha1.NewUUIDLabelSelector(testUUID)
				assert.Equal(t, profile1.Labels[expectedLabel], "config1")
				assert.Equal(t, profile2.Labels[expectedLabel], "config2")

				// Assert - Verify content is correctly converted
				assert.True(t, result[0].AdditionalContent["config1"].Exposed)
				assert.True(t, result[1].AdditionalContent["config2"].Exposed)
				assert.Equal(t, testUUID, result[0].AdditionalContent["config1"].ExposedUUID)
				assert.Equal(t, testUUID, result[1].AdditionalContent["config2"].ExposedUUID)
			})
		})

		t.Run("Failure", func(t *testing.T) {
			t.Run("NotFound error from API", func(t *testing.T) {
				defer setup(t)()

				// Arrange
				inputContentID = uuid.New()
				expectedErr = apierrors.NewNotFound(schema.GroupResource{}, "test-profile")

				listByContentID(t)

				// Act
				result, err := profile.ListByContentID(ctx, inputContentID)

				// Assert
				assert.Error(t, err)
				assert.ErrorIs(t, err, adapter.ErrProfileNotFound)
				assert.Nil(t, result)
			})

			t.Run("Empty list returned", func(t *testing.T) {
				defer setup(t)()

				// Arrange
				inputContentID = uuid.New()
				v1alpha1ProfileList = v1alpha1.ProfileList{
					Items: []v1alpha1.Profile{}, // Empty list
				}

				listByContentID(t)

				// Act
				result, err := profile.ListByContentID(ctx, inputContentID)

				// Assert
				assert.Error(t, err)
				assert.ErrorIs(t, err, adapter.ErrProfileNotFound)
				assert.Nil(t, result)
			})

			t.Run("API error during List", func(t *testing.T) {
				defer setup(t)()

				// Arrange
				inputContentID = uuid.New()
				expectedErr = assert.AnError

				listByContentID(t)

				// Act
				result, err := profile.ListByContentID(ctx, inputContentID)

				// Assert
				assert.Error(t, err)
				assert.ErrorIs(t, err, assert.AnError)
				assert.Nil(t, result)
			})

			t.Run("Conversion error", func(t *testing.T) {
				defer setup(t)()

				// Arrange
				inputContentID = uuid.New()
				expectedErr = nil // List should succeed, but conversion should fail

				// Create a profile with exposed content but NO UUID label
				// This will cause conversion to fail
				profileWithoutLabel := v1alpha1.Profile{
					Spec: v1alpha1.ProfileSpec{
						IPXETemplate: "test",
						AdditionalContent: []v1alpha1.AdditionalContent{
							{
								Name:    "exposed-without-uuid",
								Exposed: true,
								Inline:  ptr.To("content"),
							},
						},
					},
				}

				v1alpha1ProfileList = v1alpha1.ProfileList{
					Items: []v1alpha1.Profile{profileWithoutLabel},
				}

				listByContentID(t)

				// Act
				result, err := profile.ListByContentID(ctx, inputContentID)

				// Assert
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "additional content is exposed but doesn't have a UUID")
				assert.Nil(t, result)
			})
		})
	})
}
