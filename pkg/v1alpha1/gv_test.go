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

package v1alpha1

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUUIDLabelSelectors(t *testing.T) {
	t.Run("Returns correct maps for UUID labels", func(t *testing.T) {
		// Arrange
		uuid1 := uuid.New()
		uuid2 := uuid.New()

		labels := map[string]string{
			NewUUIDLabelSelector(uuid1): "content1",
			NewUUIDLabelSelector(uuid2): "content2",
			"other-label":               "other-value",
		}

		// Act
		idNameMap, reverseMap, err := UUIDLabelSelectors(labels)

		// Assert
		require.NoError(t, err)

		// Verify idNameMap (UUID -> content name)
		assert.Len(t, idNameMap, 2)
		assert.Equal(t, "content1", idNameMap[uuid1])
		assert.Equal(t, "content2", idNameMap[uuid2])

		// Verify reverseMap (content name -> UUID)
		assert.Len(t, reverseMap, 2)
		assert.Equal(t, uuid1, reverseMap["content1"])
		assert.Equal(t, uuid2, reverseMap["content2"])

		// Non-UUID labels should be ignored
		_, exists := reverseMap["other-value"]
		assert.False(t, exists)
	})

	t.Run("Returns empty maps when no UUID labels", func(t *testing.T) {
		// Arrange
		labels := map[string]string{
			"regular-label": "value",
			"another-label": "another-value",
		}

		// Act
		idNameMap, reverseMap, err := UUIDLabelSelectors(labels)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, idNameMap)
		assert.Empty(t, reverseMap)
	})

	t.Run("Returns error for invalid UUID in label", func(t *testing.T) {
		// Arrange
		labels := map[string]string{
			LabelSelector("invalid-uuid", UUIDPrefix): "content1",
		}

		// Act
		idNameMap, reverseMap, err := UUIDLabelSelectors(labels)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, idNameMap)
		assert.Nil(t, reverseMap)
	})

	t.Run("Handles nil labels map", func(t *testing.T) {
		// Act
		idNameMap, reverseMap, err := UUIDLabelSelectors(nil)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, idNameMap)
		assert.Empty(t, reverseMap)
	})
}

func TestNewUUIDLabelSelector(t *testing.T) {
	t.Run("Creates correctly formatted UUID label", func(t *testing.T) {
		// Arrange
		testUUID := uuid.New()

		// Act
		label := NewUUIDLabelSelector(testUUID)

		// Assert
		assert.Contains(t, label, "uuid.shaper.amahdha.com/")
		assert.Contains(t, label, testUUID.String())
		assert.True(t, IsUUIDLabelSelector(label))
	})
}

func TestIsUUIDLabelSelector(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{
			name:     "Valid UUID label",
			key:      "uuid.shaper.amahdha.com/445a4753-3d59-4429-8cea-7db9febdeca",
			expected: true,
		},
		{
			name:     "Non-UUID shaper label",
			key:      "buildarch.shaper.amahdha.com/x86_64",
			expected: false,
		},
		{
			name:     "Regular label",
			key:      "app.kubernetes.io/name",
			expected: false,
		},
		{
			name:     "Empty string",
			key:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsUUIDLabelSelector(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}
