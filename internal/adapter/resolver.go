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

package adapter

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/jsonpath"

	"github.com/alexandremahdhaoui/shaper/internal/types"
)

var (
	ErrResolverResolve   = errors.New("resolving content")
	ErrObjectRefResolver = errors.New("resolving object ref")
	ErrWebhookResolver   = errors.New("resolving webhook")

	errObjectRefMustBeSpecified = errors.New("object ref must be specified")
	errResolvingMTLSConfig      = errors.New("resolving mTLS config")
	errResolvingBasicAuthRef    = errors.New("resolving basic auth ref")

	errWebhookConfigShouldNotBeNil = errors.New("webhook config should not be nil")
)

// --------------------------------------------------- INTERFACE ---------------------------------------------------- //

// Resolver is an interface for resolving content.
type Resolver interface {
	// Resolve resolves the content.
	Resolve(
		ctx context.Context,
		content types.Content,
		attributes types.IPXESelectors,
	) ([]byte, error)
}

// ObjectRefResolver is an interface for resolving object references.
type ObjectRefResolver interface {
	Resolver

	// ResolvePaths resolves the paths in the object reference.
	ResolvePaths(
		ctx context.Context,
		paths []*jsonpath.JSONPath,
		ref types.ObjectRef,
	) ([][]byte, error)
}

// ------------------------------------------------- INLINE RESOLVER ------------------------------------------------ //

// NewInlineResolver returns a new inline resolver.
func NewInlineResolver() Resolver {
	return &inlineResolver{}
}

type inlineResolver struct{}

func (r *inlineResolver) Resolve(
	_ context.Context,
	content types.Content,
	_ types.IPXESelectors,
) ([]byte, error) {
	return []byte(content.Inline), nil
}

// ---------------------------------------------- OBJECT REF RESOLVER ----------------------------------------------- //

// NewObjectRefResolver returns a new object ref resolver.
func NewObjectRefResolver(k8sClient dynamic.Interface) ObjectRefResolver {
	return &objectRefResolver{k8s: k8sClient}
}

type objectRefResolver struct {
	k8s dynamic.Interface
}

func (r *objectRefResolver) Resolve(
	ctx context.Context,
	content types.Content,
	_ types.IPXESelectors,
) ([]byte, error) {
	if content.ObjectRef == nil {
		return nil, errors.Join(errObjectRefMustBeSpecified, ErrResolverResolve)
	}

	ref := *content.ObjectRef

	out, err := r.ResolvePaths(ctx, []*jsonpath.JSONPath{ref.JSONPath}, ref)
	if err != nil {
		return nil, err // TODO: wrap
	}

	if len(out) < 1 {
		return nil, errors.New("TODO") // TODO
	}

	return out[0], nil
}

func (r *objectRefResolver) ResolvePaths(
	ctx context.Context,
	paths []*jsonpath.JSONPath,
	ref types.ObjectRef,
) ([][]byte, error) { //nolint:lll
	obj, err := r.k8s.
		Resource(schema.GroupVersionResource{
			Group:    ref.Group,
			Version:  ref.Version,
			Resource: ref.Resource,
		}).
		Namespace(ref.Namespace).
		Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Join(err, ErrObjectRefResolver)
	}

	out := make([][]byte, 0)

	for _, path := range paths {
		buf := bytes.NewBuffer(make([]byte, 0))
		if err := path.Execute(buf, obj.Object); err != nil {
			return nil, errors.Join(err, ErrObjectRefResolver)
		}

		out = append(out, buf.Bytes())
	}

	return out, nil
}

// ------------------------------------------------ WEBHOOK RESOLVER ------------------------------------------------ //

const (
	buildarchParam = "buildarch"
	uuidParam      = "uuid"
)

// NewWebhookResolver returns a new webhook resolver.
// It requires a k8sClient in order to resolve object reference if needed.
func NewWebhookResolver(resolver ObjectRefResolver) Resolver {
	return &webhookResolver{objectRefResolver: resolver}
}

type webhookResolver struct {
	objectRefResolver ObjectRefResolver

	// Allow GLOBALLY disabling
	disableTLSInsecureSkipVerify bool
}

func (r *webhookResolver) Resolve(
	ctx context.Context,
	content types.Content,
	attributes types.IPXESelectors,
) ([]byte, error) {
	// TODO: make use of content.WebhookConfig.MTLSObjectRef.TLSInsecureSkipVerify

	if content.WebhookConfig == nil {
		return nil, errors.Join(
			errWebhookConfigShouldNotBeNil,
			ErrWebhookResolver,
			ErrResolverResolve,
		)
	}

	url := fmt.Sprintf("https://%s?%s=%s&%s=%s",
		content.WebhookConfig.URL,
		buildarchParam, attributes.Buildarch,
		uuidParam, attributes.UUID,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Join(err, ErrWebhookResolver, ErrResolverResolve)
	}

	httpClient := new(http.Client)
	if err := r.mTLSConfig(ctx, httpClient, content.WebhookConfig.MTLSObjectRef); err != nil {
		return nil, errors.Join(err, ErrWebhookResolver, ErrResolverResolve)
	}

	if err := r.setBasicAuth(ctx, req, content.WebhookConfig.BasicAuthObjectRef); err != nil {
		return nil, errors.Join(err, ErrWebhookResolver, ErrResolverResolve)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.Join(err, ErrWebhookResolver, ErrResolverResolve)
	}

	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Join(err, ErrWebhookResolver, ErrResolverResolve)
	}

	return out, nil
}

// TODO: lru cache that config?
func (r *webhookResolver) mTLSConfig(
	ctx context.Context,
	httpClient *http.Client,
	ref *types.MTLSObjectRef,
) error {
	if ref == nil {
		return nil
	}

	paths := []*jsonpath.JSONPath{
		ref.ClientKeyJSONPath,
		ref.ClientCertJSONPath,
		ref.CaBundleJSONPath,
	}

	res, err := r.objectRefResolver.ResolvePaths(ctx, paths, ref.ObjectRef)
	if err != nil {
		return errors.Join(err, errResolvingMTLSConfig)
	}

	if nRes := len(res); nRes < 3 {
		return errors.Join(
			fmt.Errorf("expected: 3 results; actual: %d results", nRes),
			errors.New(
				"mTLS configuration expected 1 client key, 1 client crt, and 1 ca bundle/crt",
			),
			errResolvingMTLSConfig,
		)
	}

	clientKey := res[0]
	clientCert := res[1]
	caBundle := res[2]

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caBundle)

	cert, err := tls.X509KeyPair(clientCert, clientKey)
	if err != nil {
		return errors.Join(err, errResolvingMTLSConfig)
	}

	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:      caCertPool,
			Certificates: []tls.Certificate{cert},
			// disableTLSInsecureSkipVerify globally enforce tls verification. //TODO: add test cases.
			InsecureSkipVerify: (!r.disableTLSInsecureSkipVerify) && ref.TLSInsecureSkipVerify,
		},
	}

	return nil
}

func (r *webhookResolver) setBasicAuth(
	ctx context.Context,
	req *http.Request,
	ref *types.BasicAuthObjectRef,
) error {
	if ref == nil {
		return nil
	}

	paths := []*jsonpath.JSONPath{ref.UsernameJSONPath, ref.PasswordJSONPath}

	res, err := r.objectRefResolver.ResolvePaths(ctx, paths, ref.ObjectRef)
	if err != nil {
		return errors.Join(err, errResolvingBasicAuthRef)
	}

	if nRes := len(res); nRes < 2 {
		return errors.Join(
			fmt.Errorf("got: %d results; want: 2 results", nRes),
			errors.New("basic auth credentials expected 1 username, and 1 password"),
			errResolvingBasicAuthRef)
	}

	username, password := res[0], res[1]

	req.SetBasicAuth(string(username), string(password))

	return nil
}
