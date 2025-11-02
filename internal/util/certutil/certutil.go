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

package certutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// Inspired from: https://github.com/madflojo/testcerts/blob/main/testcerts.go

// ------------------------------------------------------- CA ------------------------------------------------------- //

// CA is a certificate authority.
type CA struct {
	key      *ecdsa.PrivateKey
	pool     *x509.CertPool
	rootCert *x509.Certificate
}

// NewCA creates a new CA.
func NewCA() (*CA, error) {
	// 1. create a ca cert.
	caCert := &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"Use in test only!"},
		},
		SerialNumber:          big.NewInt(123),
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	// 2. create private key.
	caKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, err // TODO: wrap err
	}

	// 3. self sign a certificate with the CA's own cert and the previously generated private key.
	selfSignedCertRaw, err := x509.CreateCertificate(rand.Reader, caCert, caCert, caKey.Public(), caKey)
	if err != nil {
		return nil, err // TODO: wrap err
	}

	// 4. Add self-signed cert to pool
	certPool := x509.NewCertPool()
	// certPool.AppendCertsFromPEM(selfSignedCertRaw)
	// TODO: check if the code below cannot be replaced with above code.

	selfSignedCert, err := x509.ParseCertificate(selfSignedCertRaw)
	if err != nil {
		return nil, err // TODO: wrap err
	}

	certPool.AddCert(selfSignedCert)

	return &CA{
		key:      caKey,
		pool:     certPool,
		rootCert: selfSignedCert,
	}, nil
}

// Pool returns the CA's cert pool.
func (ca *CA) Pool() *x509.CertPool {
	return ca.pool
}

// Cert returns the CA's root certificate in PEM format.
func (ca *CA) Cert() []byte {
	return certToPEM(ca.rootCert)
}

// ------------------------------------------------ CertifiedKeypair ------------------------------------------------ //

// NewCertifiedKey creates a new certified key.
func (ca *CA) NewCertifiedKey(domains ...string) (*ecdsa.PrivateKey, *x509.Certificate, error) {
	crtTemplate := &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"Use in test only!"},
		},

		DNSNames:     domains,
		SerialNumber: big.NewInt(123),
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(2 * time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	key, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, nil, err // TODO: wrap err
	}

	signedRaw, err := x509.CreateCertificate(rand.Reader, crtTemplate, ca.rootCert, key.Public(), ca.key)
	if err != nil {
		return nil, nil, err // TODO: wrap err
	}

	signed, err := x509.ParseCertificate(signedRaw)
	if err != nil {
		return nil, nil, err // TODO: wrap err
	}

	return key, signed, nil
}

// NewCertifiedKeyPEM creates a new certified key in PEM format.
func (ca *CA) NewCertifiedKeyPEM(domains ...string) (key []byte, cert []byte, err error) {
	k, c, err := ca.NewCertifiedKey(domains...)
	if err != nil {
		return nil, nil, err // TODO: wrap err
	}

	keyPEM, err := privateKeyToPem(k)
	if err != nil {
		return nil, nil, err // TODO: wrap err
	}

	return keyPEM, certToPEM(c), nil
}

func privateKeyToPem(key *ecdsa.PrivateKey) ([]byte, error) {
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("could not marshal private key - %w", err) // TODO: wrap err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}), nil
}

func certToPEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}
