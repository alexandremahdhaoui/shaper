//go:build unit

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
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTestCA(t *testing.T) {
	ca, err := NewTestCA()
	require.NoError(t, err)
	assert.NotNil(t, ca)
	assert.NotNil(t, ca.ca)
}

func TestTestCA_CACertPEM(t *testing.T) {
	ca, err := NewTestCA()
	require.NoError(t, err)

	certPEM := ca.CACertPEM()
	assert.NotEmpty(t, certPEM)

	// Verify it's valid PEM
	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM(certPEM)
	assert.True(t, ok, "CA cert should be valid PEM")
}

func TestTestCA_GenerateServerCert(t *testing.T) {
	ca, err := NewTestCA()
	require.NoError(t, err)

	keyPEM, certPEM, err := ca.GenerateServerCert("localhost", "example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, keyPEM)
	assert.NotEmpty(t, certPEM)

	// Verify certificate can be parsed
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	assert.NotNil(t, cert)

	// Verify certificate is signed by CA
	certPool := ca.CertPool()
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	opts := x509.VerifyOptions{
		Roots:     certPool,
		DNSName:   "localhost",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	_, err = x509Cert.Verify(opts)
	assert.NoError(t, err, "Certificate should be verifiable by CA")
}

func TestTestCA_GenerateClientCert(t *testing.T) {
	ca, err := NewTestCA()
	require.NoError(t, err)

	keyPEM, certPEM, err := ca.GenerateClientCert("test-client")
	require.NoError(t, err)
	assert.NotEmpty(t, keyPEM)
	assert.NotEmpty(t, certPEM)

	// Verify certificate can be parsed
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	assert.NotNil(t, cert)

	// Verify certificate is signed by CA
	certPool := ca.CertPool()
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	opts := x509.VerifyOptions{
		Roots:     certPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	_, err = x509Cert.Verify(opts)
	assert.NoError(t, err, "Client certificate should be verifiable by CA")
}

func TestWriteCertAndKey(t *testing.T) {
	ca, err := NewTestCA()
	require.NoError(t, err)

	keyPEM, certPEM, err := ca.GenerateServerCert("localhost")
	require.NoError(t, err)

	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	err = WriteCertAndKey(certPEM, keyPEM, certPath, keyPath)
	require.NoError(t, err)

	// Verify files exist
	assert.FileExists(t, certPath)
	assert.FileExists(t, keyPath)

	// Verify file contents
	writtenCert, err := os.ReadFile(certPath)
	require.NoError(t, err)
	assert.Equal(t, certPEM, writtenCert)

	writtenKey, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	assert.Equal(t, keyPEM, writtenKey)

	// Verify file permissions (key should be 0600)
	keyInfo, err := os.Stat(keyPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), keyInfo.Mode().Perm())
}

func TestCreateServerTLSConfig(t *testing.T) {
	ca, err := NewTestCA()
	require.NoError(t, err)

	serverKeyPEM, serverCertPEM, err := ca.GenerateServerCert("localhost")
	require.NoError(t, err)

	caCertPEM := ca.CACertPEM()

	tlsConfig, err := CreateServerTLSConfig(serverCertPEM, serverKeyPEM, caCertPEM)
	require.NoError(t, err)
	assert.NotNil(t, tlsConfig)

	// Verify TLS config has correct settings
	assert.Equal(t, tls.RequireAndVerifyClientCert, tlsConfig.ClientAuth)
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
	assert.NotNil(t, tlsConfig.ClientCAs)
	assert.Len(t, tlsConfig.Certificates, 1)
}

func TestCreateClientTLSConfig(t *testing.T) {
	ca, err := NewTestCA()
	require.NoError(t, err)

	clientKeyPEM, clientCertPEM, err := ca.GenerateClientCert("test-client")
	require.NoError(t, err)

	caCertPEM := ca.CACertPEM()

	tlsConfig, err := CreateClientTLSConfig(clientCertPEM, clientKeyPEM, caCertPEM)
	require.NoError(t, err)
	assert.NotNil(t, tlsConfig)

	// Verify TLS config has correct settings
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
	assert.NotNil(t, tlsConfig.RootCAs)
	assert.Len(t, tlsConfig.Certificates, 1)
}

func TestCreateServerTLSConfig_InvalidCert(t *testing.T) {
	_, err := CreateServerTLSConfig([]byte("invalid"), []byte("invalid"), []byte("invalid"))
	assert.Error(t, err, "Should fail with invalid certificates")
}

func TestCreateClientTLSConfig_InvalidCert(t *testing.T) {
	_, err := CreateClientTLSConfig([]byte("invalid"), []byte("invalid"), []byte("invalid"))
	assert.Error(t, err, "Should fail with invalid certificates")
}

func TestMTLS_Integration(t *testing.T) {
	// This test verifies that a client and server can perform mTLS handshake
	// using certificates generated by our utilities.

	ca, err := NewTestCA()
	require.NoError(t, err)

	// Generate server cert
	serverKeyPEM, serverCertPEM, err := ca.GenerateServerCert("localhost")
	require.NoError(t, err)

	// Generate client cert
	clientKeyPEM, clientCertPEM, err := ca.GenerateClientCert("test-client")
	require.NoError(t, err)

	caCertPEM := ca.CACertPEM()

	// Create server TLS config
	serverTLSConfig, err := CreateServerTLSConfig(serverCertPEM, serverKeyPEM, caCertPEM)
	require.NoError(t, err)

	// Create client TLS config
	clientTLSConfig, err := CreateClientTLSConfig(clientCertPEM, clientKeyPEM, caCertPEM)
	require.NoError(t, err)

	// Verify both configs are not nil
	assert.NotNil(t, serverTLSConfig)
	assert.NotNil(t, clientTLSConfig)

	// In a real integration test, we would:
	// 1. Start an HTTPS server with serverTLSConfig
	// 2. Make a request with a client using clientTLSConfig
	// 3. Verify the mTLS handshake succeeds
	// This is tested in the WebhookResolver/Transformer mTLS tests (Task 5.2/5.3)
}
