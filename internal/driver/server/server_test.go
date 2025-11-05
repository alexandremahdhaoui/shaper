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

package server_test

import (
	"context"
	"testing"

	"github.com/alexandremahdhaoui/shaper/internal/driver/server"
	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/internal/util/mocks/mockcontroller"
	"github.com/alexandremahdhaoui/shaper/pkg/generated/shaperserver"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetIPXEBootstrap(t *testing.T) {
	tests := []struct {
		name          string
		mockBootstrap []byte
		expectedBody  []byte
	}{
		{
			name:          "success with valid iPXE script",
			mockBootstrap: []byte("#!ipxe\nchain http://192.168.1.1/ipxe?uuid=${uuid}&buildarch=${buildarch}"),
			expectedBody:  []byte("#!ipxe\nchain http://192.168.1.1/ipxe?uuid=${uuid}&buildarch=${buildarch}"),
		},
		{
			name:          "success with empty bootstrap script",
			mockBootstrap: []byte(""),
			expectedBody:  []byte(""),
		},
		{
			name: "success with multiline iPXE script",
			mockBootstrap: []byte(`#!ipxe
set base-url http://boot.example.com
kernel ${base-url}/vmlinuz
initrd ${base-url}/initrd
boot`),
			expectedBody: []byte(`#!ipxe
set base-url http://boot.example.com
kernel ${base-url}/vmlinuz
initrd ${base-url}/initrd
boot`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockIPXE := mockcontroller.NewMockIPXE(t)
			mockContent := mockcontroller.NewMockContent(t)

			// Setup mock expectations using EXPECT pattern
			mockIPXE.EXPECT().Boostrap().Return(tt.mockBootstrap)

			// Create server
			srv := server.New(mockIPXE, mockContent)

			// Create request
			ctx := context.Background()
			req := shaperserver.GetIPXEBootstrapRequestObject{}

			// Execute
			resp, err := srv.GetIPXEBootstrap(ctx, req)

			// Assert no error (handler returns error in response body, not as error)
			assert.NoError(t, err)

			// Assert response is 200 type
			resp200, ok := resp.(shaperserver.GetIPXEBootstrap200TextResponse)
			assert.True(t, ok, "expected GetIPXEBootstrap200TextResponse")

			// Assert body matches expected
			assert.Equal(t, tt.expectedBody, []byte(resp200))

			// Verify mock expectations
			mockIPXE.AssertExpectations(t)
		})
	}
}

func TestGetContentByID_Success(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	contentUUID1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	contentUUID2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	contentUUID3 := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	contentUUID4 := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	ignitionConfig := []byte(`{"ignition": {"version": "3.4.0"}}`)
	cloudInitConfig := []byte(`#cloud-config
hostname: test-host`)

	tests := []struct {
		name              string
		contentID         uuid.UUID
		buildarch         shaperserver.GetContentByIDParamsBuildarch
		uuid              uuid.UUID
		mockReturnContent []byte
		expectedBody      []byte
	}{
		{
			name:              "valid ignition config with x86_64",
			contentID:         contentUUID1,
			buildarch:         shaperserver.GetContentByIDParamsBuildarchX8664,
			uuid:              testUUID,
			mockReturnContent: ignitionConfig,
			expectedBody:      ignitionConfig,
		},
		{
			name:              "valid cloud-init config with arm64",
			contentID:         contentUUID2,
			buildarch:         shaperserver.GetContentByIDParamsBuildarchArm64,
			uuid:              testUUID,
			mockReturnContent: cloudInitConfig,
			expectedBody:      cloudInitConfig,
		},
		{
			name:              "empty content with i386",
			contentID:         contentUUID3,
			buildarch:         shaperserver.GetContentByIDParamsBuildarchI386,
			uuid:              testUUID,
			mockReturnContent: []byte{},
			expectedBody:      []byte{},
		},
		{
			name:              "content with arm32",
			contentID:         contentUUID4,
			buildarch:         shaperserver.GetContentByIDParamsBuildarchArm32,
			uuid:              testUUID,
			mockReturnContent: []byte("test content"),
			expectedBody:      []byte("test content"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockIPXE := mockcontroller.NewMockIPXE(t)
			mockContent := mockcontroller.NewMockContent(t)

			// Setup mock expectations
			mockContent.EXPECT().GetByID(
				mock.Anything, // context
				tt.contentID,
				mock.MatchedBy(func(attrs types.IPXESelectors) bool {
					return attrs.Buildarch == string(tt.buildarch) && attrs.UUID == tt.uuid
				}),
			).Return(tt.mockReturnContent, nil)

			// Create server
			srv := server.New(mockIPXE, mockContent)

			// Create request
			ctx := context.Background()
			req := shaperserver.GetContentByIDRequestObject{
				ContentID: tt.contentID,
				Params: shaperserver.GetContentByIDParams{
					Buildarch: tt.buildarch,
					Uuid:      tt.uuid,
				},
			}

			// Execute
			resp, err := srv.GetContentByID(ctx, req)

			// Assert no error
			assert.NoError(t, err)

			// Assert response is 200 type
			resp200, ok := resp.(shaperserver.GetContentByID200TextResponse)
			assert.True(t, ok, "expected GetContentByID200TextResponse")

			// Assert body matches expected
			assert.Equal(t, tt.expectedBody, []byte(resp200))

			// Verify mock expectations
			mockContent.AssertExpectations(t)
		})
	}
}

func TestGetContentByID_Error(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	contentUUID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	tests := []struct {
		name              string
		contentID         uuid.UUID
		buildarch         shaperserver.GetContentByIDParamsBuildarch
		uuid              uuid.UUID
		mockReturnContent []byte
		mockReturnError   error
		errorContains     string
	}{
		{
			name:              "controller returns generic error",
			contentID:         contentUUID,
			buildarch:         shaperserver.GetContentByIDParamsBuildarchX8664,
			uuid:              testUUID,
			mockReturnContent: nil,
			mockReturnError:   assert.AnError,
			errorContains:     "getting config by id",
		},
		{
			name:              "controller returns context canceled",
			contentID:         contentUUID,
			buildarch:         shaperserver.GetContentByIDParamsBuildarchArm64,
			uuid:              testUUID,
			mockReturnContent: nil,
			mockReturnError:   context.Canceled,
			errorContains:     "getting config by id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockIPXE := mockcontroller.NewMockIPXE(t)
			mockContent := mockcontroller.NewMockContent(t)

			// Setup mock expectations
			mockContent.EXPECT().GetByID(
				mock.Anything, // context
				tt.contentID,
				mock.MatchedBy(func(attrs types.IPXESelectors) bool {
					return attrs.Buildarch == string(tt.buildarch) && attrs.UUID == tt.uuid
				}),
			).Return(tt.mockReturnContent, tt.mockReturnError)

			// Create server
			srv := server.New(mockIPXE, mockContent)

			// Create request
			ctx := context.Background()
			req := shaperserver.GetContentByIDRequestObject{
				ContentID: tt.contentID,
				Params: shaperserver.GetContentByIDParams{
					Buildarch: tt.buildarch,
					Uuid:      tt.uuid,
				},
			}

			// Execute
			resp, err := srv.GetContentByID(ctx, req)

			// Assert no error from handler (error is in response body)
			assert.NoError(t, err)

			// Assert response is 500 type
			resp500, ok := resp.(shaperserver.GetContentByID500JSONResponse)
			assert.True(t, ok, "expected GetContentByID500JSONResponse")

			// Assert error message contains expected text
			assert.Contains(t, resp500.Message, tt.errorContains)

			// Verify mock expectations
			mockContent.AssertExpectations(t)
		})
	}
}

func TestGetIPXEBySelectors_Success(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name             string
		buildarch        shaperserver.GetIPXEBySelectorsParamsBuildarch
		uuid             uuid.UUID
		mockReturnScript []byte
		expectedBody     []byte
	}{
		{
			name:             "valid iPXE script with UUID and x86_64",
			buildarch:        shaperserver.X8664,
			uuid:             testUUID,
			mockReturnScript: []byte("#!ipxe\nkernel http://example.com/vmlinuz\nboot"),
			expectedBody:     []byte("#!ipxe\nkernel http://example.com/vmlinuz\nboot"),
		},
		{
			name:             "valid iPXE script with arm64",
			buildarch:        shaperserver.Arm64,
			uuid:             testUUID,
			mockReturnScript: []byte("#!ipxe\nchain http://boot.example.com"),
			expectedBody:     []byte("#!ipxe\nchain http://boot.example.com"),
		},
		{
			name:             "empty script with i386",
			buildarch:        shaperserver.I386,
			uuid:             testUUID,
			mockReturnScript: []byte(""),
			expectedBody:     []byte(""),
		},
		{
			name:      "complex multiline script with arm32",
			buildarch: shaperserver.Arm32,
			uuid:      testUUID,
			mockReturnScript: []byte(`#!ipxe
set base-url http://boot.example.com
kernel ${base-url}/vmlinuz
initrd ${base-url}/initrd
boot`),
			expectedBody: []byte(`#!ipxe
set base-url http://boot.example.com
kernel ${base-url}/vmlinuz
initrd ${base-url}/initrd
boot`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockIPXE := mockcontroller.NewMockIPXE(t)
			mockContent := mockcontroller.NewMockContent(t)

			// Setup mock expectations
			mockIPXE.EXPECT().FindProfileAndRender(
				mock.Anything, // context
				mock.MatchedBy(func(selectors types.IPXESelectors) bool {
					return selectors.Buildarch == string(tt.buildarch) && selectors.UUID == tt.uuid
				}),
			).Return(tt.mockReturnScript, nil)

			// Create server
			srv := server.New(mockIPXE, mockContent)

			// Create request
			ctx := context.Background()
			req := shaperserver.GetIPXEBySelectorsRequestObject{
				Params: shaperserver.GetIPXEBySelectorsParams{
					Buildarch: tt.buildarch,
					Uuid:      tt.uuid,
				},
			}

			// Execute
			resp, err := srv.GetIPXEBySelectors(ctx, req)

			// Assert no error
			assert.NoError(t, err)

			// Assert response is 200 type
			resp200, ok := resp.(shaperserver.GetIPXEBySelectors200TextResponse)
			assert.True(t, ok, "expected GetIPXEBySelectors200TextResponse")

			// Assert body matches expected
			assert.Equal(t, tt.expectedBody, []byte(resp200))

			// Verify mock expectations
			mockIPXE.AssertExpectations(t)
		})
	}
}

func TestGetIPXEBySelectors_Error(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name             string
		buildarch        shaperserver.GetIPXEBySelectorsParamsBuildarch
		uuid             uuid.UUID
		mockReturnScript []byte
		mockReturnError  error
		errorContains    string
	}{
		{
			name:             "generic controller error",
			buildarch:        shaperserver.X8664,
			uuid:             testUUID,
			mockReturnScript: nil,
			mockReturnError:  assert.AnError,
			errorContains:    "getting ipxe by labels",
		},
		{
			name:             "context canceled",
			buildarch:        shaperserver.Arm64,
			uuid:             testUUID,
			mockReturnScript: nil,
			mockReturnError:  context.Canceled,
			errorContains:    "getting ipxe by labels",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockIPXE := mockcontroller.NewMockIPXE(t)
			mockContent := mockcontroller.NewMockContent(t)

			// Setup mock expectations
			mockIPXE.EXPECT().FindProfileAndRender(
				mock.Anything, // context
				mock.MatchedBy(func(selectors types.IPXESelectors) bool {
					return selectors.Buildarch == string(tt.buildarch) && selectors.UUID == tt.uuid
				}),
			).Return(tt.mockReturnScript, tt.mockReturnError)

			// Create server
			srv := server.New(mockIPXE, mockContent)

			// Create request
			ctx := context.Background()
			req := shaperserver.GetIPXEBySelectorsRequestObject{
				Params: shaperserver.GetIPXEBySelectorsParams{
					Buildarch: tt.buildarch,
					Uuid:      tt.uuid,
				},
			}

			// Execute
			resp, err := srv.GetIPXEBySelectors(ctx, req)

			// Assert no error from handler (error is in response body)
			assert.NoError(t, err)

			// Assert response is 500 type
			resp500, ok := resp.(shaperserver.GetIPXEBySelectors500JSONResponse)
			assert.True(t, ok, "expected GetIPXEBySelectors500JSONResponse")

			// Assert error message contains expected text
			assert.Contains(t, resp500.Message, tt.errorContains)

			// Verify mock expectations
			mockIPXE.AssertExpectations(t)
		})
	}
}

func TestNew(t *testing.T) {
	// Create mocks
	mockIPXE := mockcontroller.NewMockIPXE(t)
	mockContent := mockcontroller.NewMockContent(t)

	// Call constructor
	srv := server.New(mockIPXE, mockContent)

	// Assert non-nil
	assert.NotNil(t, srv)

	// Assert implements interface (compile-time check)
	var _ shaperserver.StrictServerInterface = srv
}
