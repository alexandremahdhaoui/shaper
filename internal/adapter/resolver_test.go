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

	"github.com/google/uuid"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/internal/util/fakes/resolverserverfake"
	"github.com/alexandremahdhaoui/shaper/internal/util/httputil"
	"github.com/alexandremahdhaoui/shaper/internal/util/testutil"
	"github.com/alexandremahdhaoui/shaper/pkg/generated/resolverserver"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestInlineResolver(t *testing.T) {
	var resolver adapter.Resolver

	setup := func() {
		resolver = adapter.NewInlineResolver()
	}

	t.Run("Resolve", func(t *testing.T) {
		setup()

		expected := []byte("test")

		content := types.Content{Inline: string(expected)}
		ipxeSelectors := types.IPXESelectors{}

		out, err := resolver.Resolve(nil, content, ipxeSelectors)
		assert.NoError(t, err)
		assert.Equal(t, expected, out)
	})
}

func TestObjectRefResolver(t *testing.T) {
	var (
		ctx context.Context

		expected      []byte
		content       types.Content
		ipxeSelectors types.IPXESelectors
		object        *unstructured.Unstructured

		cl       *fake.FakeDynamicClient
		resolver adapter.Resolver
	)

	setup := func(t *testing.T) {
		t.Helper()

		ctx = context.Background()

		expected = []byte("qwe")

		content = testutil.NewTypesContentObjectRef()
		require.NoError(t, content.ObjectRef.JSONPath.Parse("{.data.test}"))

		object = &unstructured.Unstructured{}
		object.SetName(content.ObjectRef.Name)
		object.SetNamespace(content.ObjectRef.Namespace)
		object.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   content.ObjectRef.Group,
			Version: content.ObjectRef.Version,
			Kind:    content.ObjectRef.Resource,
		})

		cl = fake.NewSimpleDynamicClient(runtime.NewScheme(), object)
		resolver = adapter.NewObjectRefResolver(cl)

		ipxeSelectors = types.IPXESelectors{}
	}

	t.Run("Resolve", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			setup(t)

			object.SetUnstructuredContent(map[string]any{"data": map[string]any{"test": string(expected)}})
			cl.PrependReactor("get", "ConfigMap", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, object, nil
			})

			actual, err := resolver.Resolve(ctx, content, ipxeSelectors)
			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	})
}

func TestWebhookResolver(t *testing.T) {
	var (
		ctx context.Context

		expected string

		basicAuthObject *unstructured.Unstructured
		mtlsObject      *unstructured.Unstructured
		content         types.Content
		ipxeSelectors   types.IPXESelectors

		mock *resolverserverfake.Fake

		cl       *fake.FakeDynamicClient
		resolver adapter.Resolver
	)

	setup := func(t *testing.T) func() {
		t.Helper()

		ctx = context.Background()

		// -------------------------------------------------- Content ----------------------------------------------- //

		content = testutil.NewTypesContentWebhook()
		require.NoError(t, content.WebhookConfig.BasicAuthObjectRef.UsernameJSONPath.Parse(`{.data.username}`))
		require.NoError(t, content.WebhookConfig.BasicAuthObjectRef.PasswordJSONPath.Parse(`{.data.password}`))
		require.NoError(t, content.WebhookConfig.MTLSObjectRef.ClientKeyJSONPath.Parse(`{.data.client\.key}`))
		require.NoError(t, content.WebhookConfig.MTLSObjectRef.ClientCertJSONPath.Parse(`{.data.client\.crt}`))
		require.NoError(t, content.WebhookConfig.MTLSObjectRef.CaBundleJSONPath.Parse(`{.data.ca\.crt}`))

		ipxeSelectors = types.IPXESelectors{
			Buildarch: string(resolverserver.Arm64),
			UUID:      uuid.New(),
		}

		// -------------------------------------------------- Webhook Server  --------------------------------------- //

		addr := strings.SplitN(content.WebhookConfig.URL, "/", 2)[0]
		mock = resolverserverfake.New(t, addr)

		clientKey, clientCert, err := mock.CA.NewCertifiedKeyPEM(addr)
		require.NoError(t, err)
		caCert := mock.CA.Cert()

		// -------------------------------------------------- Basic Auth -------------------------------------------- //

		username, password := "qwe123", "321ewq"

		currentHandler := mock.Server.Handler
		mock.Server.Handler = httputil.BasicAuth(currentHandler,
			func(u, p string, _ *http.Request) (bool, error) {
				return u == username && p == password, nil
			},
		)

		basicAuthObject = &unstructured.Unstructured{}
		basicAuthObject.SetUnstructuredContent(map[string]any{"data": map[string]any{
			"username": username,
			"password": password,
		}})

		basicAuthObject.SetName(content.WebhookConfig.BasicAuthObjectRef.Name)
		basicAuthObject.SetNamespace(content.WebhookConfig.BasicAuthObjectRef.Namespace)
		basicAuthObject.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "yoursecret.amahdha.com",
			Version: "v1beta2",
			Kind:    "YourSecret",
		})

		// -------------------------------------------------- mTLS  ------------------------------------------------- //

		mtlsObject = &unstructured.Unstructured{}
		mtlsObject.SetUnstructuredContent(map[string]any{
			"data": map[string]any{
				"client.key": string(clientKey),
				"client.crt": string(clientCert),
				"ca.crt":     string(caCert),
			},
		})

		mtlsObject.SetName(content.WebhookConfig.MTLSObjectRef.Name)
		mtlsObject.SetNamespace(content.WebhookConfig.MTLSObjectRef.Namespace)
		mtlsObject.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "core",
			Version: "v1",
			Kind:    "Secret",
		})

		// -------------------------------------------------- Client and Adapter ------------------------------------ //

		cl = fake.NewSimpleDynamicClient(runtime.NewScheme(), basicAuthObject, mtlsObject)

		objectRefResolver := adapter.NewObjectRefResolver(cl)
		resolver = adapter.NewWebhookResolver(objectRefResolver)

		return func() {
			t.Helper()

			mock.AssertExpectationsAndShutdown()
		}
	}

	t.Run("Resolve", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			defer setup(t)()

			expected = fmt.Sprintf("{\"data\":\"%s + %s\"}\n", ipxeSelectors.Buildarch, ipxeSelectors.UUID.String())

			mock.AppendExpectation(func(_ context.Context, request resolverserver.ResolveRequestObject) (resolverserver.ResolveResponseObject, error) { //nolint:lll
				return resolverserver.Resolve200JSONResponse{
					ResolveRespJSONResponse: resolverserver.ResolveRespJSONResponse{
						Data: ptr.To(fmt.Sprintf(`%s + %s`, request.Params.Buildarch, request.Params.Uuid.String())),
					},
				}, nil
			})

			cl.PrependReactor("get", "YourSecret", func(_ k8stesting.Action) (bool, runtime.Object, error) {
				return true, basicAuthObject, nil
			})

			cl.PrependReactor("get", "Secret", func(_ k8stesting.Action) (bool, runtime.Object, error) {
				return true, mtlsObject, nil
			})

			actual, err := resolver.Resolve(ctx, content, ipxeSelectors)
			require.NoError(t, err)
			assert.Equal(t, expected, string(actual))
		})

		t.Run("Fail", func(t *testing.T) {
			defer setup(t)()

			cl.PrependReactor("get", "YourSecret", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
				basicAuthObject.SetUnstructuredContent(map[string]any{"data": map[string]any{
					"username": "not a username",
					"password": "not a password",
				}})

				return true, basicAuthObject, nil
			})

			cl.PrependReactor("get", "Secret", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, mtlsObject, nil
			})

			expected = `{"message":"Unauthorized"}`
			expected = string(append([]byte(expected), byte(0x0a)))

			actual, err := resolver.Resolve(ctx, content, ipxeSelectors)
			require.NoError(t, err)
			assert.Equal(t, expected, string(actual))
		})
	})
}
