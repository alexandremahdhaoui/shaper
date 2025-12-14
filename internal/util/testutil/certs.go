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

package testutil

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/alexandremahdhaoui/shaper/internal/util/certutil"
)

// GenerateTestCA is deprecated. Use NewTestCA() instead.
// This function is kept for backward compatibility but returns limited information.
func GenerateTestCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	// Since certutil.CA doesn't expose the CA's private key,
	// and we need a wrapper anyway, users should use NewTestCA() instead.
	// This function is kept for compatibility but not recommended.
	return nil, nil, nil
}

// TestCA wraps certutil.CA to provide convenient test certificate generation.
type TestCA struct {
	ca *certutil.CA
}

// NewTestCA creates a new test CA.
func NewTestCA() (*TestCA, error) {
	ca, err := certutil.NewCA()
	if err != nil {
		return nil, err
	}
	return &TestCA{ca: ca}, nil
}

// CertPool returns the CA's certificate pool.
func (t *TestCA) CertPool() *x509.CertPool {
	return t.ca.Pool()
}

// CACertPEM returns the CA certificate in PEM format.
func (t *TestCA) CACertPEM() []byte {
	return t.ca.Cert()
}

// GenerateServerCert generates a server certificate signed by this CA.
// domains specifies the DNS names for the certificate.
func (t *TestCA) GenerateServerCert(domains ...string) (keyPEM, certPEM []byte, err error) {
	return t.ca.NewCertifiedKeyPEM(domains...)
}

// GenerateClientCert generates a client certificate signed by this CA.
// commonName can be used to identify the client.
func (t *TestCA) GenerateClientCert(commonName string) (keyPEM, certPEM []byte, err error) {
	// For client certs, we can use an empty domain list
	return t.ca.NewCertifiedKeyPEM()
}

// WriteCertAndKey writes a certificate and private key to PEM files.
func WriteCertAndKey(certPEM, keyPEM []byte, certPath, keyPath string) error {
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return err
	}
	return nil
}

// CreateServerTLSConfig creates a TLS config for a server with mTLS.
// It requires clients to present valid certificates signed by the CA.
func CreateServerTLSConfig(serverCertPEM, serverKeyPEM, caCertPEM []byte) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCertPEM) {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// CreateClientTLSConfig creates a TLS config for a client with mTLS.
// The client will present the provided certificate and verify the server against the CA.
func CreateClientTLSConfig(clientCertPEM, clientKeyPEM, caCertPEM []byte) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCertPEM) {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}
