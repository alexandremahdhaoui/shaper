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

package testutil

import (
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewV1alpha1ProfileWithExposedContent(t *testing.T) {
	t.Run("Creates profile with exposed content and UUID labels", func(t *testing.T) {
		// Arrange
		uuid1 := uuid.New()
		uuid2 := uuid.New()
		uuid3 := uuid.New()

		items := []ExposedContentItem{
			{
				Name:    "ignition-config",
				UUID:    uuid1,
				Body:    `{"ignition": {"version": "3.0.0"}}`,
				Exposed: true,
			},
			{
				Name:    "cloud-init",
				UUID:    uuid2,
				Body:    "#cloud-config\nruncmd:\n  - echo hello",
				Exposed: true,
			},
			{
				Name:    "internal-param",
				UUID:    uuid3,
				Body:    "secret-value",
				Exposed: false,
			},
		}

		// Act
		profile := NewV1alpha1ProfileWithExposedContent("test-profile", items)

		// Assert
		assert.Equal(t, "test-profile", profile.Name)
		assert.NotNil(t, profile.Labels)
		assert.Len(t, profile.Spec.AdditionalContent, 3)

		// Verify UUID labels for exposed items only
		expectedLabel1 := v1alpha1.NewUUIDLabelSelector(uuid1)
		expectedLabel2 := v1alpha1.NewUUIDLabelSelector(uuid2)
		expectedLabel3 := v1alpha1.NewUUIDLabelSelector(uuid3)

		// Exposed items should have labels
		assert.Equal(t, "ignition-config", profile.Labels[expectedLabel1], "ignition-config should have UUID label")
		assert.Equal(t, "cloud-init", profile.Labels[expectedLabel2], "cloud-init should have UUID label")

		// Non-exposed items should NOT have labels
		_, exists := profile.Labels[expectedLabel3]
		assert.False(t, exists, "internal-param should NOT have UUID label")

		// Verify only 2 labels exist (for the 2 exposed items)
		assert.Len(t, profile.Labels, 2)

		// Verify label format
		assert.Contains(t, expectedLabel1, "uuid.shaper.amahdha.com/")
		assert.Contains(t, expectedLabel2, "uuid.shaper.amahdha.com/")
	})

	t.Run("Creates profile with correct AdditionalContent structure", func(t *testing.T) {
		// Arrange
		uuid1 := uuid.New()
		items := []ExposedContentItem{
			{
				Name:    "test-content",
				UUID:    uuid1,
				Body:    "test body",
				Exposed: true,
			},
		}

		// Act
		profile := NewV1alpha1ProfileWithExposedContent("test-profile", items)

		// Assert
		require.Len(t, profile.Spec.AdditionalContent, 1)
		content := profile.Spec.AdditionalContent[0]

		assert.Equal(t, "test-content", content.Name)
		assert.True(t, content.Exposed)
		assert.NotNil(t, content.Inline)
		assert.Equal(t, "test body", *content.Inline)
		assert.Nil(t, content.PostTransformations)
	})

	t.Run("Creates profile with no labels when no exposed content", func(t *testing.T) {
		// Arrange
		uuid1 := uuid.New()
		items := []ExposedContentItem{
			{
				Name:    "internal-only",
				UUID:    uuid1,
				Body:    "internal",
				Exposed: false,
			},
		}

		// Act
		profile := NewV1alpha1ProfileWithExposedContent("test-profile", items)

		// Assert
		assert.Empty(t, profile.Labels, "No labels should exist for non-exposed content")
		assert.Len(t, profile.Spec.AdditionalContent, 1)
	})

	t.Run("Creates profile with iPXE template referencing content", func(t *testing.T) {
		// Arrange
		uuid1 := uuid.New()
		items := []ExposedContentItem{
			{
				Name:    "my-config",
				UUID:    uuid1,
				Body:    "config-body",
				Exposed: true,
			},
		}

		// Act
		profile := NewV1alpha1ProfileWithExposedContent("test-profile", items)

		// Assert
		assert.Contains(t, profile.Spec.IPXETemplate, "{{ .AdditionalContent.my-config }}")
		assert.Contains(t, profile.Spec.IPXETemplate, "#!ipxe")
	})

	t.Run("Handles empty content items", func(t *testing.T) {
		// Act
		profile := NewV1alpha1ProfileWithExposedContent("test-profile", []ExposedContentItem{})

		// Assert
		assert.Equal(t, "test-profile", profile.Name)
		assert.Empty(t, profile.Labels)
		assert.Empty(t, profile.Spec.AdditionalContent)
		assert.NotEmpty(t, profile.Spec.IPXETemplate)
	})

	t.Run("UUID labels can be parsed back to UUIDs", func(t *testing.T) {
		// Arrange
		uuid1 := uuid.New()
		uuid2 := uuid.New()
		items := []ExposedContentItem{
			{Name: "content1", UUID: uuid1, Body: "body1", Exposed: true},
			{Name: "content2", UUID: uuid2, Body: "body2", Exposed: true},
		}

		// Act
		profile := NewV1alpha1ProfileWithExposedContent("test-profile", items)

		// Assert - Verify UUIDLabelSelectors can parse the labels
		idNameMap, reverseMap, err := v1alpha1.UUIDLabelSelectors(profile.Labels)
		require.NoError(t, err, "Should be able to parse UUID labels")

		assert.Len(t, idNameMap, 2)
		assert.Equal(t, "content1", idNameMap[uuid1])
		assert.Equal(t, "content2", idNameMap[uuid2])

		assert.Len(t, reverseMap, 2)
	})
}
