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

//go:build unit

package adapter_test

import (
	"context"
	"testing"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	"github.com/alexandremahdhaoui/shaper/internal/util/mocks/mockclient"
	"github.com/alexandremahdhaoui/shaper/internal/util/testutil"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	types2 "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestProfile(t *testing.T) {
	var (
		ctx       context.Context
		namespace string

		inputProfileName string

		v1alpha1Profile v1alpha1.Profile
		expectedErr     error

		cl      *mockclient.MockClient
		profile adapter.Profile
	)

	setup := func(t *testing.T) func() {
		t.Helper()

		ctx = context.Background()
		namespace = "test-profile"

		inputProfileName = "profile-name"

		v1alpha1Profile = testutil.NewV1alpha1Profile()

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
}
