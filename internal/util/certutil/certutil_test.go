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

package certutil_test

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/internal/util/certutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCA verifies that NewCA() successfully creates a CA with valid certificate and key.
func TestNewCA(t *testing.T) {
	// Create new CA
	ca, err := certutil.NewCA()
	require.NoError(t, err, "NewCA should not return error")
	require.NotNil(t, ca, "CA should not be nil")

	// Verify CA has non-nil pool
	pool := ca.Pool()
	assert.NotNil(t, pool, "CA pool should not be nil")

	// Verify CA has non-empty cert in PEM format
	certPEM := ca.Cert()
	assert.NotEmpty(t, certPEM, "CA cert PEM should not be empty")

	// Decode PEM block
	block, rest := pem.Decode(certPEM)
	require.NotNil(t, block, "PEM decoding should succeed")
	assert.Empty(t, rest, "should have consumed all PEM bytes")
	assert.Equal(t, "CERTIFICATE", block.Type, "PEM block should be CERTIFICATE type")

	// Parse certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "certificate parsing should succeed")
	require.NotNil(t, cert, "parsed certificate should not be nil")

	// Verify certificate properties
	assert.True(t, cert.IsCA, "certificate should be marked as CA")
	assert.Contains(t, cert.Subject.Organization, "Use in test only!", "certificate should have test organization")

	// Verify key usage
	assert.Equal(t, x509.KeyUsageDigitalSignature|x509.KeyUsageCertSign|x509.KeyUsageCRLSign,
		cert.KeyUsage, "CA should have correct key usage")

	// Verify extended key usage
	assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth, "should have client auth")
	assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth, "should have server auth")

	// Verify validity period
	assert.True(t, cert.NotBefore.Before(time.Now()), "NotBefore should be in the past")
	assert.True(t, cert.NotAfter.After(time.Now()), "NotAfter should be in the future")

	// Verify basic constraints
	assert.True(t, cert.BasicConstraintsValid, "basic constraints should be valid")
}

// TestCA_Pool verifies that Pool() method returns the CA's certificate pool.
func TestCA_Pool(t *testing.T) {
	// Create CA
	ca, err := certutil.NewCA()
	require.NoError(t, err)
	require.NotNil(t, ca)

	// Get pool
	pool := ca.Pool()
	assert.NotNil(t, pool, "Pool should not be nil")

	// To verify the pool contains the CA certificate, we'll create a cert signed by this CA
	// and verify it against the pool
	_, cert, err := ca.NewCertifiedKey("test.example.com")
	require.NoError(t, err)
	require.NotNil(t, cert)

	// Verify the signed certificate against the CA pool
	opts := x509.VerifyOptions{
		DNSName: "test.example.com",
		Roots:   pool,
	}
	chains, err := cert.Verify(opts)
	assert.NoError(t, err, "Certificate verification should succeed with CA pool")
	assert.NotEmpty(t, chains, "Should have at least one valid chain")
}

// TestCA_Cert verifies that Cert() method returns the CA's certificate in PEM format.
func TestCA_Cert(t *testing.T) {
	// Create CA
	ca, err := certutil.NewCA()
	require.NoError(t, err)
	require.NotNil(t, ca)

	// Get cert PEM
	certPEM := ca.Cert()
	assert.NotEmpty(t, certPEM, "Cert PEM should not be empty")

	// Decode PEM block
	block, rest := pem.Decode(certPEM)
	require.NotNil(t, block, "PEM decoding should succeed")
	assert.Empty(t, rest, "should have consumed all bytes")
	assert.Equal(t, "CERTIFICATE", block.Type, "PEM block type should be CERTIFICATE")

	// Parse certificate from DER bytes
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "certificate parsing should succeed")
	require.NotNil(t, cert, "parsed certificate should not be nil")

	// Verify it's a CA certificate
	assert.True(t, cert.IsCA, "certificate should be marked as CA")
}

// TestCA_NewCertifiedKey verifies that NewCertifiedKey() generates certificates signed by the CA.
func TestCA_NewCertifiedKey(t *testing.T) {
	tests := []struct {
		name    string
		domains []string
	}{
		{
			name:    "single domain",
			domains: []string{"example.com"},
		},
		{
			name:    "multiple domains",
			domains: []string{"example.com", "*.example.com"},
		},
		{
			name:    "no domains",
			domains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create CA
			ca, err := certutil.NewCA()
			require.NoError(t, err)
			require.NotNil(t, ca)

			// Generate certified key
			key, cert, err := ca.NewCertifiedKey(tt.domains...)
			require.NoError(t, err, "NewCertifiedKey should not return error")
			require.NotNil(t, key, "private key should not be nil")
			require.NotNil(t, cert, "certificate should not be nil")

			// Verify certificate DNSNames
			if len(tt.domains) > 0 {
				assert.Equal(t, tt.domains, cert.DNSNames, "certificate should have correct DNSNames")
			}

			// Verify certificate is not a CA
			assert.False(t, cert.IsCA, "certificate should not be marked as CA")

			// Verify key usage
			assert.Equal(t, x509.KeyUsageDigitalSignature, cert.KeyUsage, "should have digital signature key usage")

			// Verify extended key usage
			assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth, "should have client auth")
			assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth, "should have server auth")

			// Verify certificate is signed by CA
			if len(tt.domains) > 0 {
				opts := x509.VerifyOptions{
					DNSName: tt.domains[0],
					Roots:   ca.Pool(),
				}
				chains, err := cert.Verify(opts)
				assert.NoError(t, err, "certificate should be verifiable with CA pool")
				assert.NotEmpty(t, chains, "should have at least one valid chain")
			}
		})
	}
}

// TestCA_NewCertifiedKeyPEM verifies that NewCertifiedKeyPEM() returns certificate and key in PEM format.
func TestCA_NewCertifiedKeyPEM(t *testing.T) {
	tests := []struct {
		name    string
		domains []string
	}{
		{
			name:    "single domain",
			domains: []string{"test.example.com"},
		},
		{
			name:    "multiple domains",
			domains: []string{"test.example.com", "*.test.example.com", "api.test.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create CA
			ca, err := certutil.NewCA()
			require.NoError(t, err)
			require.NotNil(t, ca)

			// Generate certified key in PEM format
			keyPEM, certPEM, err := ca.NewCertifiedKeyPEM(tt.domains...)
			require.NoError(t, err, "NewCertifiedKeyPEM should not return error")
			assert.NotEmpty(t, keyPEM, "key PEM should not be empty")
			assert.NotEmpty(t, certPEM, "cert PEM should not be empty")

			// Decode and verify key PEM
			keyBlock, keyRest := pem.Decode(keyPEM)
			require.NotNil(t, keyBlock, "key PEM decoding should succeed")
			assert.Empty(t, keyRest, "should have consumed all key PEM bytes")
			assert.Equal(t, "PRIVATE KEY", keyBlock.Type, "PEM block type should be PRIVATE KEY")

			// Parse private key
			privKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
			require.NoError(t, err, "private key parsing should succeed")
			assert.NotNil(t, privKey, "parsed private key should not be nil")

			// Decode and verify cert PEM
			certBlock, certRest := pem.Decode(certPEM)
			require.NotNil(t, certBlock, "cert PEM decoding should succeed")
			assert.Empty(t, certRest, "should have consumed all cert PEM bytes")
			assert.Equal(t, "CERTIFICATE", certBlock.Type, "PEM block type should be CERTIFICATE")

			// Parse certificate
			cert, err := x509.ParseCertificate(certBlock.Bytes)
			require.NoError(t, err, "certificate parsing should succeed")
			require.NotNil(t, cert, "parsed certificate should not be nil")

			// Verify certificate contains the domains
			assert.Equal(t, tt.domains, cert.DNSNames, "certificate should have correct DNSNames")

			// Verify certificate is not a CA
			assert.False(t, cert.IsCA, "certificate should not be marked as CA")

			// Verify certificate can be verified with CA pool
			opts := x509.VerifyOptions{
				DNSName: tt.domains[0],
				Roots:   ca.Pool(),
			}
			chains, err := cert.Verify(opts)
			assert.NoError(t, err, "certificate should be verifiable with CA pool")
			assert.NotEmpty(t, chains, "should have at least one valid chain")
		})
	}
}
