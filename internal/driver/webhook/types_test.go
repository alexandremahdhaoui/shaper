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

package webhook_test

import (
	"errors"
	"testing"

	"github.com/alexandremahdhaoui/shaper/internal/driver/webhook"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewUnsupportedResource(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	tests := []struct {
		name             string
		obj              *v1alpha1.Profile // using Profile as a runtime.Object
		inputErrors      []error
		expectedContains []string
	}{
		{
			name:        "single error",
			obj:         &v1alpha1.Profile{},
			inputErrors: []error{err1},
			expectedContains: []string{
				"error 1",
				"webhook does not support resource",
			},
		},
		{
			name:        "multiple errors",
			obj:         &v1alpha1.Profile{},
			inputErrors: []error{err1, err2},
			expectedContains: []string{
				"error 1",
				"error 2",
				"webhook does not support resource",
			},
		},
		{
			name:        "no errors",
			obj:         &v1alpha1.Profile{},
			inputErrors: []error{},
			expectedContains: []string{
				"webhook does not support resource",
			},
		},
		{
			name: "contains GVK info",
			obj: &v1alpha1.Profile{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Profile",
					APIVersion: "shaper.alexandremahdhaoui.com/v1alpha1",
				},
			},
			inputErrors: []error{},
			expectedContains: []string{
				"webhook does not support resource",
				"GroupVersionKind",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute
			err := webhook.NewUnsupportedResource(tt.obj, tt.inputErrors...)

			// Assert error is not nil
			assert.Error(t, err)

			// Assert all expected strings are in error message
			errMsg := err.Error()
			for _, expected := range tt.expectedContains {
				assert.Contains(t, errMsg, expected)
			}

			// Assert errors.Is works for input errors
			for _, inputErr := range tt.inputErrors {
				if inputErr != nil {
					assert.ErrorIs(t, err, inputErr)
				}
			}
		})
	}
}
