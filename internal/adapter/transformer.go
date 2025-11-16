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

package adapter

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/alexandremahdhaoui/shaper/internal/types"
	butaneconfig "github.com/coreos/butane/config"
	butanecommon "github.com/coreos/butane/config/common"

	"k8s.io/client-go/util/jsonpath"
)

var ErrTransformerTransform = errors.New("transforming content")

// --------------------------------------------------- INTERFACE ---------------------------------------------------- //

// Transformer is an interface for transforming content.
type Transformer interface {
	// Transform transforms the content.
	Transform(ctx context.Context, cfg types.TransformerConfig, content []byte, selectors types.IPXESelectors) ([]byte, error)
}

// ----------------------------------------------- BUTANE TRANSFORMER ----------------------------------------------- //

// NewButaneTransformer returns a new butane transformer.
func NewButaneTransformer() Transformer {
	return &butaneTransformer{}
}

type butaneTransformer struct{}

func (t *butaneTransformer) Transform(
	_ context.Context,
	_ types.TransformerConfig,
	content []byte,
	_ types.IPXESelectors,
) ([]byte, error) {
	b, _, err := butaneconfig.TranslateBytes(content, butanecommon.TranslateBytesOptions{Raw: true})
	if err != nil {
		return nil, errors.Join(err, ErrTransformerTransform)
	}

	return b, nil
}

// ---------------------------------------------- WEBHOOK TRANSFORMER ----------------------------------------------- //

// NewWebhookTransformer returns a new webhook transformer.
func NewWebhookTransformer(resolver ObjectRefResolver) Transformer {
	return &webhookTransformer{objectRefResolver: resolver}
}

type webhookTransformer struct {
	objectRefResolver ObjectRefResolver
}

type webhookTransformerRequest struct {
	Content    []byte            `json:"content"`
	Attributes map[string]string `json:"attributes"`
}

func (t *webhookTransformer) Transform(
	ctx context.Context,
	cfg types.TransformerConfig,
	content []byte,
	attributes types.IPXESelectors,
) ([]byte, error) {
	if cfg.Webhook == nil {
		return nil, errors.New("TODO") // TODO: err & wrap err
	}

	requestBody := webhookTransformerRequest{
		Content: content,
		Attributes: map[string]string{ // TODO: use const for keys && a type-conversion func to build that map
			"uuid":      attributes.UUID.String(),
			"buildarch": attributes.Buildarch,
		},
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, errors.Join(err) // TODO: wrap err
	}

	url := fmt.Sprintf("https://%s?uuid=%s&buildarch=%s",
		cfg.Webhook.URL,
		attributes.UUID.String(),
		attributes.Buildarch)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Join(err) // TODO: wrap err
	}

	httpClient := new(http.Client)
	if err := t.mTLSConfig(ctx, httpClient, cfg.Webhook.MTLSObjectRef); err != nil {
		return nil, errors.Join(err, ErrWebhookResolver, ErrResolverResolve)
	}

	if err := t.setBasicAuth(ctx, req, cfg.Webhook.BasicAuthObjectRef); err != nil {
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

func (t *webhookTransformer) mTLSConfig(ctx context.Context, httpClient *http.Client, ref *types.MTLSObjectRef) error {
	if ref == nil {
		return nil
	}

	paths := []*jsonpath.JSONPath{ref.ClientKeyJSONPath, ref.ClientCertJSONPath, ref.CaBundleJSONPath}

	res, err := t.objectRefResolver.ResolvePaths(ctx, paths, ref.ObjectRef)
	if err != nil {
		return errors.Join(err, errResolvingMTLSConfig)
	}

	if len(res) < 3 {
		return errors.New("TODO") // TODO
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
		},
	}

	return nil
}

func (t *webhookTransformer) setBasicAuth(ctx context.Context, req *http.Request, ref *types.BasicAuthObjectRef) error {
	if ref == nil {
		return nil
	}

	paths := []*jsonpath.JSONPath{ref.UsernameJSONPath, ref.PasswordJSONPath}

	res, err := t.objectRefResolver.ResolvePaths(ctx, paths, ref.ObjectRef)
	if err != nil {
		return err // TODO: wrap
	}

	if len(res) < 2 {
		return errors.New("TODO") // TODO
	}

	username := res[0]
	password := res[1]

	req.SetBasicAuth(string(username), string(password))

	return nil
}
