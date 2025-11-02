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

package resolverserverfake

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/generated/resolverserver"

	"github.com/alexandremahdhaoui/shaper/internal/util/certutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Expectation = func(
	ctx context.Context,
	request resolverserver.ResolveRequestObject,
) (resolverserver.ResolveResponseObject, error)

type Fake struct {
	t            *testing.T
	expectations []Expectation
	counter      int

	Server http.Server
	CA     *certutil.CA
}

func (f *Fake) Resolve( //nolint:ireturn
	ctx context.Context,
	request resolverserver.ResolveRequestObject,
) (resolverserver.ResolveResponseObject, error) {
	f.t.Helper()

	counter := f.counter
	f.counter++

	return f.expectations[counter](ctx, request)
}

func (f *Fake) Start() *Fake {
	go func() {
		if err := f.Server.ListenAndServeTLS("", ""); !errors.Is(err, http.ErrServerClosed) {
			require.NoError(f.t, err)
		}
	}()

	return f
}

func (f *Fake) AppendExpectation(expectation Expectation) *Fake {
	f.expectations = append(f.expectations, expectation)

	return f
}

func (f *Fake) AssertExpectationsAndShutdown() *Fake {
	f.t.Helper()

	ctx := context.Background()

	assert.Equal(f.t, f.counter, len(f.expectations))
	require.NoError(f.t, f.Server.Shutdown(ctx))

	return f
}

func New(t *testing.T, addr string) *Fake {
	t.Helper()

	serverName := strings.SplitN(addr, ":", 2)[0] // a bit hacky

	// generate mTLS certs
	ca, err := certutil.NewCA()
	require.NoError(t, err)

	serverKey, serverCrt, err := ca.NewCertifiedKeyPEM(serverName)
	require.NoError(t, err)

	fake := &Fake{
		t:            t,
		expectations: make([]Expectation, 0),
		counter:      0,

		CA: ca,
	}

	handler := resolverserver.Handler(resolverserver.NewStrictHandler(fake, nil))

	tlsKeyPair, err := tls.X509KeyPair(serverCrt, serverKey)
	require.NoError(t, err)

	fake.Server = http.Server{
		Addr:    addr,
		Handler: handler,
		TLSConfig: &tls.Config{ // nolint: exhaust
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{tlsKeyPair},
			RootCAs:      ca.Pool(),
			ServerName:   serverName,
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    ca.Pool(),
			// TODO: Parameterize InsecureSkipVerify to test use cases when a user would allow self-signed certs.
			//      We may also have to update the RootCAs var.
			InsecureSkipVerify: false, // TODO?
		},
	}

	fake.Start()

	return fake
}
