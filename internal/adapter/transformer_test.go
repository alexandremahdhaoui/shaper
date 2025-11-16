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

package adapter_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/internal/util/certutil"
	"github.com/alexandremahdhaoui/shaper/internal/util/fakes/transformerserverfake"
	"github.com/alexandremahdhaoui/shaper/internal/util/httputil"
	"github.com/alexandremahdhaoui/shaper/internal/util/mocks/mockadapter"
	"github.com/alexandremahdhaoui/shaper/internal/util/testutil"
	"github.com/alexandremahdhaoui/shaper/pkg/generated/transformerserver"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestButaneTransformer(t *testing.T) {
	var transformer adapter.Transformer

	setup := func(t *testing.T) {
		t.Helper()

		transformer = adapter.NewButaneTransformer()
	}

	t.Run("Transform", func(t *testing.T) {
		setup(t)

		inputCfg := types.TransformerConfig{Kind: types.ButaneTransformerKind}
		inputContent := []byte(`
variant: fcos
version: 1.5.0
passwd:
  users:
    - name: core
`)

		inputSelectors := types.IPXESelectors{
			UUID:      uuid.New(),
			Buildarch: "arm64",
		}

		expected := []byte(`{"ignition":{"version":"3.4.0"},"passwd":{"users":[{"name":"core"}]}}`)

		ctx := context.Background()
		actual, err := transformer.Transform(ctx, inputCfg, inputContent, inputSelectors)
		assert.NoError(t, err)
		assert.Equal(t, expected, actual)
	})
}

func TestWebhookTransformer(t *testing.T) {
	var (
		ctx      context.Context
		expected string

		inputConfig     types.TransformerConfig
		inputContent    []byte
		inputAttributes types.IPXESelectors

		objectRefResolver *mockadapter.MockObjectRefResolver
		transformer       adapter.Transformer
		serverMock        *transformerserverfake.Fake
	)

	setup := func(t *testing.T) func() {
		t.Helper()

		ctx = context.Background()
		id := uuid.New() //nolint:varnamelen
		buildarch := "arm64"
		expected = fmt.Sprintf("this has been templated: %s, %s", id.String(), buildarch)

		// -------------------------------------------------- Inputs ------------------------------------------------ //

		inputConfig = testutil.NewTypesTransformerConfigWebhook()
		inputContent = []byte("this should be templated: {{ .uuid }}, {{ .buildarch }}")
		inputAttributes = types.IPXESelectors{
			UUID:      id,
			Buildarch: buildarch,
		}

		// -------------------------------------------------- Client and Adapter ------------------------------------ //

		objectRefResolver = mockadapter.NewMockObjectRefResolver(t)
		transformer = adapter.NewWebhookTransformer(objectRefResolver)

		// -------------------------------------------------- Webhook Server Fake ----------------------------------- //

		addr := strings.SplitN(inputConfig.Webhook.URL, "/", 2)[0]
		serverMock = transformerserverfake.New(t, addr)

		clientKey, clientCert, err := serverMock.CA.NewCertifiedKeyPEM(addr)
		require.NoError(t, err)
		caCert := serverMock.CA.Cert()

		// -------------------------------------------------- mTLS  ------------------------------------------------- //

		objectRefResolver.EXPECT().
			ResolvePaths(mock.Anything, mock.Anything, mock.Anything).
			Return([][]byte{clientKey, clientCert, caCert}, nil).
			Once()

		// -------------------------------------------------- Basic Auth -------------------------------------------- //

		username, password := "qwe123", "321ewq"

		currentHandler := serverMock.Server.Handler
		httputil.BasicAuth(currentHandler, func(u, p string, _ *http.Request) (bool, error) {
			return u == username && p == password, nil
		})

		objectRefResolver.EXPECT().
			ResolvePaths(mock.Anything, mock.Anything, mock.Anything).
			Return([][]byte{[]byte(username), []byte(password)}, nil).
			Once()

		// -------------------------------------------------- Teardown  --------------------------------------------- //

		return func() { //nolint:contextcheck
			t.Helper()

			objectRefResolver.AssertExpectations(t)
			serverMock.AssertExpectationsAndShutdown()
		}
	}

	t.Run("Transform", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			defer setup(t)()

			expected = fmt.Sprintf("{\"data\":\"%s + %s\"}\n", inputAttributes.Buildarch, inputAttributes.UUID.String())

			serverMock.AppendExpectation(func(_ context.Context, request transformerserver.TransformRequestObject) (transformerserver.TransformResponseObject, error) { //nolint:lll
				t.Helper()

				return transformerserver.Transform200JSONResponse{
					TransformRespJSONResponse: transformerserver.TransformRespJSONResponse{
						Data: ptr.To(fmt.Sprintf("%s + %s", request.Body.Attributes.Buildarch, request.Body.Attributes.Uuid.String())), //nolint:lll
					},
				}, nil
			})

			actual, err := transformer.Transform(ctx, inputConfig, inputContent, inputAttributes)
			require.NoError(t, err)
			assert.Equal(t, expected, string(actual))
		})

		t.Run("Failure", func(t *testing.T) {
			defer setup(t)()

			expected = "{\"code\":400,\"message\":\"error\"}\n"

			serverMock.AppendExpectation(func(_ context.Context, _ transformerserver.TransformRequestObject) (transformerserver.TransformResponseObject, error) { //nolint:lll
				t.Helper()

				return transformerserver.Transform400JSONResponse{
					N400JSONResponse: transformerserver.N400JSONResponse{
						Code:    400,
						Message: "error",
					},
				}, nil
			})

			actual, err := transformer.Transform(ctx, inputConfig, inputContent, inputAttributes)
			require.NoError(t, err)
			assert.Equal(t, expected, string(actual))
		})
	})

	// setupMinimal creates basic test infrastructure without mock expectations
	setupMinimal := func(t *testing.T) func() {
		t.Helper()

		ctx = context.Background()
		id := uuid.New() //nolint:varnamelen
		buildarch := "arm64"

		// -------------------------------------------------- Inputs ------------------------------------------------ //

		inputConfig = testutil.NewTypesTransformerConfigWebhook()
		inputContent = []byte("this should be templated: {{ .uuid }}, {{ .buildarch }}")
		inputAttributes = types.IPXESelectors{
			UUID:      id,
			Buildarch: buildarch,
		}

		// -------------------------------------------------- Client and Adapter ------------------------------------ //

		objectRefResolver = mockadapter.NewMockObjectRefResolver(t)
		transformer = adapter.NewWebhookTransformer(objectRefResolver)

		// -------------------------------------------------- Webhook Server Fake ----------------------------------- //

		addr := strings.SplitN(inputConfig.Webhook.URL, "/", 2)[0]
		serverMock = transformerserverfake.New(t, addr)

		// -------------------------------------------------- Teardown  --------------------------------------------- //

		return func() { //nolint:contextcheck
			t.Helper()

			serverMock.AssertExpectationsAndShutdown()
		}
	}

	t.Run("mTLS_InvalidClientCert", func(t *testing.T) {
		defer setupMinimal(t)()

		// Extract addr from webhook URL
		addr := strings.SplitN(inputConfig.Webhook.URL, "/", 2)[0]

		// Create a different CA and generate client cert from it
		wrongCA, err := certutil.NewCA()
		require.NoError(t, err)

		wrongClientKey, wrongClientCert, err := wrongCA.NewCertifiedKeyPEM(addr)
		require.NoError(t, err)

		// Mock resolver to return certificate from wrong CA, then basic auth
		objectRefResolver.EXPECT().
			ResolvePaths(mock.Anything, mock.Anything, mock.Anything).
			Return([][]byte{wrongClientKey, wrongClientCert, serverMock.CA.Cert()}, nil).
			Once()

		username, password := []byte("qwe123"), []byte("321ewq")
		objectRefResolver.EXPECT().
			ResolvePaths(mock.Anything, mock.Anything, mock.Anything).
			Return([][]byte{username, password}, nil).
			Once()

		// Transformer should fail with TLS error
		_, err = transformer.Transform(ctx, inputConfig, inputContent, inputAttributes)
		assert.Error(t, err, "Should fail with TLS error when client cert is from wrong CA")
		assert.Contains(t, err.Error(), "tls", "Error should be TLS-related")
	})

	t.Run("mTLS_NoClientCert", func(t *testing.T) {
		defer setupMinimal(t)()

		// Mock resolver to return empty client cert and key
		objectRefResolver.EXPECT().
			ResolvePaths(mock.Anything, mock.Anything, mock.Anything).
			Return([][]byte{[]byte(""), []byte(""), serverMock.CA.Cert()}, nil).
			Once()

		// Transformer should fail when no client certificate provided (fails before BasicAuth)
		_, err := transformer.Transform(ctx, inputConfig, inputContent, inputAttributes)
		assert.Error(t, err, "Should fail when no client certificate provided")
	})

	t.Run("mTLS_InvalidServerCert", func(t *testing.T) {
		defer setupMinimal(t)()

		// Extract addr from webhook URL
		addr := strings.SplitN(inputConfig.Webhook.URL, "/", 2)[0]

		// Create a different CA for validation
		wrongCA, err := certutil.NewCA()
		require.NoError(t, err)

		// Use correct client cert but wrong CA for server validation
		clientKey, clientCert, err := serverMock.CA.NewCertifiedKeyPEM(addr)
		require.NoError(t, err)

		// Mock resolver to return correct client cert but wrong CA for validation
		objectRefResolver.EXPECT().
			ResolvePaths(mock.Anything, mock.Anything, mock.Anything).
			Return([][]byte{clientKey, clientCert, wrongCA.Cert()}, nil).
			Once()

		username, password := []byte("qwe123"), []byte("321ewq")
		objectRefResolver.EXPECT().
			ResolvePaths(mock.Anything, mock.Anything, mock.Anything).
			Return([][]byte{username, password}, nil).
			Once()

		// Transformer should fail because server cert won't validate with wrong CA
		_, err = transformer.Transform(ctx, inputConfig, inputContent, inputAttributes)
		assert.Error(t, err, "Should fail when server cert cannot be validated")
		assert.Contains(t, err.Error(), "certificate", "Error should be certificate-related")
	})

	t.Run("mTLS_MalformedClientCert", func(t *testing.T) {
		defer setupMinimal(t)()

		// Mock resolver to return malformed client cert
		malformedKey := []byte("-----BEGIN PRIVATE KEY-----\ninvalid\n-----END PRIVATE KEY-----")
		malformedCert := []byte("-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----")

		objectRefResolver.EXPECT().
			ResolvePaths(mock.Anything, mock.Anything, mock.Anything).
			Return([][]byte{malformedKey, malformedCert, serverMock.CA.Cert()}, nil).
			Once()

		// Transformer should fail when trying to parse malformed cert (fails before BasicAuth)
		_, err := transformer.Transform(ctx, inputConfig, inputContent, inputAttributes)
		assert.Error(t, err, "Should fail with malformed client certificate")
	})
}
