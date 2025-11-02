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
	"context"
	"testing"

	"github.com/alexandremahdhaoui/shaper/internal/driver/webhook"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func strPtr(s string) *string {
	return &s
}

func TestNewProfile(t *testing.T) {
	p := webhook.NewProfile()
	assert.NotNil(t, p)
}

func TestProfile_Default_Success(t *testing.T) {
	tests := []struct {
		name         string
		inputProfile *v1alpha1.Profile
		verifyLabels func(t *testing.T, profile *v1alpha1.Profile)
	}{
		{
			name: "profile with exposed content gets UUID labels",
			inputProfile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-profile",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.ProfileSpec{
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "ignition-config",
							Exposed: true,
							Inline:  strPtr("test data"),
						},
					},
				},
			},
			verifyLabels: func(t *testing.T, profile *v1alpha1.Profile) {
				foundUUID := false
				foundContentName := false
				for k, v := range profile.Labels {
					if v1alpha1.IsUUIDLabelSelector(k) {
						foundUUID = true
						if v == "ignition-config" {
							foundContentName = true
						}
					}
				}
				assert.True(t, foundUUID, "should have UUID label")
				assert.True(t, foundContentName, "UUID label should map to content name")
			},
		},
		{
			name: "profile with non-exposed content has no UUID labels",
			inputProfile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-profile-2",
					Labels: make(map[string]string),
				},
				Spec: v1alpha1.ProfileSpec{
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "internal-config",
							Exposed: false,
							Inline:  strPtr("internal data"),
						},
					},
				},
			},
			verifyLabels: func(t *testing.T, profile *v1alpha1.Profile) {
				for k := range profile.Labels {
					assert.False(t, v1alpha1.IsUUIDLabelSelector(k), "should not have UUID labels for non-exposed content")
				}
			},
		},
		{
			name: "profile preserves existing UUID labels on update",
			inputProfile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-profile-3",
					Labels: map[string]string{
						v1alpha1.NewUUIDLabelSelector(uuid.New()): "old-config",
					},
				},
				Spec: v1alpha1.ProfileSpec{
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "old-config",
							Exposed: true,
							Inline:  strPtr("preserved data"),
						},
					},
				},
			},
			verifyLabels: func(t *testing.T, profile *v1alpha1.Profile) {
				foundOldConfig := false
				for _, v := range profile.Labels {
					if v == "old-config" {
						foundOldConfig = true
					}
				}
				assert.True(t, foundOldConfig, "should preserve old content name with its UUID")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := webhook.NewProfile()
			ctx := context.Background()
			err := p.Default(ctx, tt.inputProfile)

			assert.NoError(t, err)
			tt.verifyLabels(t, tt.inputProfile)
		})
	}
}

func TestProfile_Default_Error(t *testing.T) {
	p := webhook.NewProfile()
	ctx := context.Background()

	err := p.Default(ctx, &v1alpha1.Assignment{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook does not support resource")
}

func TestProfile_ValidateCreate_Success(t *testing.T) {
	tests := []struct {
		name         string
		inputProfile *v1alpha1.Profile
	}{
		{
			name: "valid profile with inline content",
			inputProfile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "valid-inline",
				},
				Spec: v1alpha1.ProfileSpec{
					IPXETemplate: "#!ipxe\nboot",
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "config",
							Exposed: true,
							Inline:  strPtr("test"),
						},
					},
				},
			},
		},
		{
			name: "valid profile with ObjectRef content",
			inputProfile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "valid-objectref",
				},
				Spec: v1alpha1.ProfileSpec{
					IPXETemplate: "#!ipxe\nboot",
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "config",
							Exposed: true,
							ObjectRef: &v1alpha1.ObjectRef{
								ResourceRef: v1alpha1.ResourceRef{
									Name: "my-config",
								},
								JSONPath: "{.data}",
							},
						},
					},
				},
			},
		},
		{
			name: "valid profile with Webhook content",
			inputProfile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "valid-webhook",
				},
				Spec: v1alpha1.ProfileSpec{
					IPXETemplate: "#!ipxe\nboot",
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "config",
							Exposed: true,
							Webhook: &v1alpha1.WebhookConfig{
								URL: "https://example.com/config",
							},
						},
					},
				},
			},
		},
		{
			name: "valid profile with Butane transformer",
			inputProfile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "valid-butane",
				},
				Spec: v1alpha1.ProfileSpec{
					IPXETemplate: "#!ipxe\nboot",
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "ignition",
							Exposed: true,
							Inline:  strPtr("butane config"),
							PostTransformations: []v1alpha1.Transformer{
								{
									ButaneToIgnition: true,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "valid profile with webhook transformer",
			inputProfile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "valid-webhook-transformer",
				},
				Spec: v1alpha1.ProfileSpec{
					IPXETemplate: "#!ipxe\nboot",
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "config",
							Exposed: true,
							Inline:  strPtr("data"),
							PostTransformations: []v1alpha1.Transformer{
								{
									Webhook: &v1alpha1.WebhookConfig{
										URL: "https://example.com/transform",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := webhook.NewProfile()
			ctx := context.Background()
			warnings, err := p.ValidateCreate(ctx, tt.inputProfile)

			assert.NoError(t, err)
			assert.Nil(t, warnings)
		})
	}
}

func TestProfile_ValidateCreate_Error(t *testing.T) {
	tests := []struct {
		name          string
		inputObj      runtime.Object
		errorContains string
	}{
		{
			name: "content with no configuration",
			inputObj: &v1alpha1.Profile{
				Spec: v1alpha1.ProfileSpec{
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "bad-content",
							Exposed: true,
						},
					},
				},
			},
			errorContains: "exactly 1 content configuration",
		},
		{
			name: "content with multiple configurations",
			inputObj: &v1alpha1.Profile{
				Spec: v1alpha1.ProfileSpec{
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "bad-content",
							Exposed: true,
							Inline:  strPtr("test"),
							ObjectRef: &v1alpha1.ObjectRef{
								ResourceRef: v1alpha1.ResourceRef{
									Name: "config",
								},
								JSONPath: "{.data}",
							},
						},
					},
				},
			},
			errorContains: "exactly 1 content configuration",
		},
		{
			name: "invalid ResourceRef name (empty)",
			inputObj: &v1alpha1.Profile{
				Spec: v1alpha1.ProfileSpec{
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "config",
							Exposed: true,
							ObjectRef: &v1alpha1.ObjectRef{
								ResourceRef: v1alpha1.ResourceRef{
									Name: "",
								},
								JSONPath: "{.data}",
							},
						},
					},
				},
			},
			errorContains: "invalid name",
		},
		{
			name: "invalid ResourceRef name (too long)",
			inputObj: &v1alpha1.Profile{
				Spec: v1alpha1.ProfileSpec{
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "config",
							Exposed: true,
							ObjectRef: &v1alpha1.ObjectRef{
								ResourceRef: v1alpha1.ResourceRef{
									Name: "this-name-is-way-too-long-for-kubernetes-resource-names-exceeding-63-characters-limit",
								},
								JSONPath: "{.data}",
							},
						},
					},
				},
			},
			errorContains: "invalid name",
		},
		{
			name: "invalid JSONPath",
			inputObj: &v1alpha1.Profile{
				Spec: v1alpha1.ProfileSpec{
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "config",
							Exposed: true,
							ObjectRef: &v1alpha1.ObjectRef{
								ResourceRef: v1alpha1.ResourceRef{
									Name: "config",
								},
								JSONPath: "{invalid jsonpath syntax",
							},
						},
					},
				},
			},
			errorContains: "unclosed action",
		},
		{
			name: "transformer with no configuration",
			inputObj: &v1alpha1.Profile{
				Spec: v1alpha1.ProfileSpec{
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "config",
							Exposed: true,
							Inline:  strPtr("test"),
							PostTransformations: []v1alpha1.Transformer{
								{},
							},
						},
					},
				},
			},
			errorContains: "must either enable butaneToIgnition or specify a webhook",
		},
		{
			name: "transformer with multiple configurations",
			inputObj: &v1alpha1.Profile{
				Spec: v1alpha1.ProfileSpec{
					AdditionalContent: []v1alpha1.AdditionalContent{
						{
							Name:    "config",
							Exposed: true,
							Inline:  strPtr("test"),
							PostTransformations: []v1alpha1.Transformer{
								{
									ButaneToIgnition: true,
									Webhook: &v1alpha1.WebhookConfig{
										URL: "http://example.com",
									},
								},
							},
						},
					},
				},
			},
			errorContains: "exactly one configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := webhook.NewProfile()
			ctx := context.Background()
			warnings, err := p.ValidateCreate(ctx, tt.inputObj)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorContains)
			assert.Nil(t, warnings)
		})
	}
}

func TestProfile_ValidateUpdate(t *testing.T) {
	oldProfile := &v1alpha1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-profile",
		},
		Spec: v1alpha1.ProfileSpec{
			IPXETemplate: "#!ipxe\nboot old",
			AdditionalContent: []v1alpha1.AdditionalContent{
				{
					Name:    "config",
					Exposed: true,
					Inline:  strPtr("old data"),
				},
			},
		},
	}

	newProfile := &v1alpha1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-profile",
		},
		Spec: v1alpha1.ProfileSpec{
			IPXETemplate: "#!ipxe\nboot new",
			AdditionalContent: []v1alpha1.AdditionalContent{
				{
					Name:    "config",
					Exposed: true,
					Inline:  strPtr("new data"),
				},
			},
		},
	}

	p := webhook.NewProfile()
	ctx := context.Background()
	warnings, err := p.ValidateUpdate(ctx, oldProfile, newProfile)

	assert.NoError(t, err)
	assert.Nil(t, warnings)
}

func TestProfile_ValidateDelete(t *testing.T) {
	profile := &v1alpha1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-profile",
		},
		Spec: v1alpha1.ProfileSpec{
			IPXETemplate: "#!ipxe\nboot",
		},
	}

	p := webhook.NewProfile()
	ctx := context.Background()
	warnings, err := p.ValidateDelete(ctx, profile)

	assert.NoError(t, err)
	assert.Nil(t, warnings)
}
