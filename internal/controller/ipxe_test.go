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

package controller_test

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/utils/ptr"

	"github.com/stretchr/testify/mock"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	"github.com/alexandremahdhaoui/shaper/internal/controller"
	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/internal/util/mocks/mockadapter"
	"github.com/alexandremahdhaoui/shaper/internal/util/mocks/mockcontroller"
	"github.com/alexandremahdhaoui/shaper/internal/util/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestIPXE_FindProfileAndRender(t *testing.T) {
	var (
		ctx            context.Context
		inputSelectors types.IPXESelectors

		assignment *mockadapter.MockAssignment
		profile    *mockadapter.MockProfile
		mux        *mockcontroller.MockResolveTransformerMux

		ipxe controller.IPXE
	)

	setup := func(t *testing.T) func() {
		t.Helper()

		ctx = context.Background()
		inputSelectors = types.IPXESelectors{UUID: uuid.New(), Buildarch: "arm64"}

		assignment = mockadapter.NewMockAssignment(t)
		profile = mockadapter.NewMockProfile(t)
		mux = mockcontroller.NewMockResolveTransformerMux(t)

		ipxe = controller.NewIPXE(assignment, profile, mux)

		return func() {
			t.Helper()

			assignment.AssertExpectations(t)
			profile.AssertExpectations(t)
			mux.AssertExpectations(t)
		}
	}

	t.Run("Success", func(t *testing.T) {
		t.Run("FindBySelectors", func(t *testing.T) {
			t.Run("No additional content", func(t *testing.T) {
				defer setup(t)()

				expected := []byte("expected")
				expectedProfileName := "expected-profile-name"
				expectedProfile := types.Profile{IPXETemplate: string(expected)}
				expectedResolvedAndTransformedContent := make(map[string][]byte)

				expectedAssignment := types.Assignment{
					Name:        "an-assignment",
					ProfileName: expectedProfileName,
				}

				assignment.EXPECT().
					FindBySelectors(ctx, inputSelectors).
					Return(expectedAssignment, nil).
					Once()

				profile.EXPECT().
					Get(ctx, expectedProfileName).
					Return(expectedProfile, nil).
					Once()

				mux.EXPECT().
					ResolveAndTransformBatch(
						ctx,
						expectedProfile.AdditionalContent,
						inputSelectors,
						mock.AnythingOfType("controller.ResolveTransformBatchOption"), // -> controller.ReturnExposedContentURL
					).
					Return(expectedResolvedAndTransformedContent, nil).
					Once()

				actual, err := ipxe.FindProfileAndRender(ctx, inputSelectors)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})

			t.Run("With additional content", func(t *testing.T) {
				for _, tt := range []struct {
					Name          string
					ExposedConfig bool
				}{
					{
						Name:          "With",
						ExposedConfig: true,
					},
					{
						Name:          "Without",
						ExposedConfig: false,
					},
				} {
					t.Run(fmt.Sprintf("%s exposed config", tt.Name), func(t *testing.T) {
						defer setup(t)()

						expected := []byte("kernel")
						expectedProfileName := "expected-profile-name"
						expectedResolvedAndTransformedContent := make(map[string][]byte)

						expectedProfile := types.Profile{
							IPXETemplate:      "kernel",
							AdditionalContent: make(map[string]types.Content),
						}

						for i := 0; i < 3; i++ {
							name := fmt.Sprintf("additionalContent%d", i)
							content := types.Content{
								Name: name,
								PostTransformers: []types.TransformerConfig{{
									Kind: types.ButaneTransformerKind,
								}, {
									Kind:    types.WebhookTransformerKind,
									Webhook: ptr.To(testutil.NewTypesWebhookConfig()),
								}},
								ResolverKind: types.ResolverKind(i),
							}

							if tt.ExposedConfig {
								id := uuid.New()
								content.ExposedUUID = id
								expectedResolvedAndTransformedContent[name] = []byte(fmt.Sprintf(
									"https://localhost:30443/config/%s/%s",
									expectedProfileName,
									id.String(),
								))
							} else {
								expectedResolvedAndTransformedContent[name] = []byte("resolved and transformed")
							}

							expectedProfile.IPXETemplate = fmt.Sprintf(
								"%s --additional-config-url {{ .%s }}",
								expectedProfile.IPXETemplate,
								name,
							)

							expected = append(expected, []byte(" --additional-config-url ")...)
							expected = append(
								expected,
								expectedResolvedAndTransformedContent[name]...)

							expectedProfile.AdditionalContent[name] = content
						}

						expectedAssignment := types.Assignment{
							Name:        "an-assignment",
							ProfileName: expectedProfileName,
						}

						assignment.EXPECT().
							FindBySelectors(ctx, inputSelectors).
							Return(expectedAssignment, nil).
							Once()

						profile.EXPECT().
							Get(ctx, expectedProfileName).
							Return(expectedProfile, nil).
							Once()

						mux.EXPECT().
							ResolveAndTransformBatch(
								ctx,
								expectedProfile.AdditionalContent,
								inputSelectors,
								mock.AnythingOfType("controller.ResolveTransformBatchOption"), // -> controller.ReturnExposedContentURL
							).
							Return(expectedResolvedAndTransformedContent, nil).
							Once()

						actual, err := ipxe.FindProfileAndRender(ctx, inputSelectors)
						assert.NoError(t, err)
						assert.Equal(t, expected, actual)
					})
				}
			})
		})

		t.Run("FindDefaultByBuildarch", func(t *testing.T) {
			defer setup(t)()

			expectedDefaultProfileName := "default-profile-arm64"
			expectedDefaultProfile := types.Profile{
				IPXETemplate: "this is the default profile with {{ .mustBeReturned }}",
				AdditionalContent: map[string]types.Content{
					mustBeReturned: {
						Name: mustBeReturned,
					},
				},
			}

			expectedResolvedAndTransformedAdditionalBatch := map[string][]byte{
				expectedDefaultProfile.AdditionalContent[mustBeReturned].Name: []byte(
					"an additional content",
				),
			}

			expected := []byte(
				fmt.Sprintf("this is the default profile with an additional content"),
			)

			expectedDefaultAssignment := types.Assignment{
				Name:        "a-default-assignment",
				ProfileName: expectedDefaultProfileName,
			}

			assignment.EXPECT().
				FindBySelectors(ctx, inputSelectors).
				Return(types.Assignment{}, adapter.ErrAssignmentNotFound).
				Once()

			assignment.EXPECT().
				FindDefaultByBuildarch(ctx, inputSelectors.Buildarch).
				Return(expectedDefaultAssignment, nil).
				Once()

			profile.EXPECT().
				Get(ctx, expectedDefaultProfileName).
				Return(expectedDefaultProfile, nil).
				Once()

			mux.EXPECT().
				ResolveAndTransformBatch(
					ctx,
					expectedDefaultProfile.AdditionalContent,
					inputSelectors,
					mock.AnythingOfType("controller.ResolveTransformBatchOption"), // -> controller.ReturnExposedContentURL
				).
				Return(expectedResolvedAndTransformedAdditionalBatch, nil).
				Once()

			actual, err := ipxe.FindProfileAndRender(ctx, inputSelectors)
			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	})

	t.Run("Failure", func(t *testing.T) {
		defer setup(t)()

		t.Skip("TODO")
	})
}

func TestIpxe_Bootstrap(t *testing.T) {
	expected := "#!ipxe\nchain ipxe?uuid=${uuid}&buildarch=${buildarch:uristring}\n"
	actual := controller.NewIPXE(nil, nil, nil).Boostrap()

	assert.Equal(t, expected, string(actual))
}
