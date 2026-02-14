//go:build e2e

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

package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrMTLSCertGenerate indicates a failure to generate mTLS certificates.
	ErrMTLSCertGenerate = errors.New("failed to generate mTLS certificates")
	// ErrTLSSecretCreate indicates a failure to create a TLS secret.
	ErrTLSSecretCreate = errors.New("failed to create TLS secret")
	// ErrTLSSecretDelete indicates a failure to delete a TLS secret.
	ErrTLSSecretDelete = errors.New("failed to delete TLS secret")
	// ErrBuildMTLSIPXEISO indicates a failure to build the mTLS iPXE ISO.
	ErrBuildMTLSIPXEISO = errors.New("failed to build mTLS iPXE ISO")
	// ErrBuildIPXEISO indicates a failure to build a plain HTTP iPXE ISO.
	ErrBuildIPXEISO = errors.New("failed to build iPXE ISO")
	// ErrHelmUpgrade indicates a failure to upgrade a Helm release.
	ErrHelmUpgrade = errors.New("failed to upgrade Helm release")
)

// MTLSCertSet contains all certificates and keys needed for mTLS testing.
type MTLSCertSet struct {
	// CACert is the CA certificate in PEM format.
	CACert []byte
	// CAKey is the CA private key in PEM format.
	CAKey []byte
	// ServerCert is the server certificate in PEM format.
	ServerCert []byte
	// ServerKey is the server private key in PEM format.
	ServerKey []byte
	// ClientCert is the client certificate in PEM format.
	ClientCert []byte
	// ClientKey is the client private key in PEM format.
	ClientKey []byte
}

// GenerateMTLSCertSet generates a complete set of mTLS certificates for testing.
// It creates:
// - A CA certificate and key
// - A server certificate with the given DNS name and IP SAN
// - A client certificate with CN "ipxe-client"
func GenerateMTLSCertSet(serverDNS string, serverIP net.IP) (*MTLSCertSet, error) {
	certSet, err := generateMTLSCertSetInternal(serverDNS, serverIP)
	if err != nil {
		return nil, errors.Join(ErrMTLSCertGenerate, err)
	}

	return certSet, nil
}

// generateMTLSCertSetInternal generates the full cert set with access to all keys.
// Uses RSA keys because iPXE requires RSA for client certificate authentication.
func generateMTLSCertSetInternal(serverDNS string, serverIP net.IP) (*MTLSCertSet, error) {
	// Use 2048-bit RSA keys for iPXE compatibility
	const rsaKeyBits = 2048

	// 1. Create CA key and certificate
	caKey, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate CA key: %w", err)
	}

	caCertTemplate := &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"Use in test only!"},
			CommonName:   "Test CA",
		},
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour), // Extended validity for debugging
		IsCA:         true,
		// Root CAs should not have ExtKeyUsage - it constrains issued certs
		// KeyUsage: CertSign for signing certs, CRLSign for signing revocation lists
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caCertTemplate, caCertTemplate, caKey.Public(), caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// 2. Create server key and certificate with DNS SAN and IP SAN
	serverKey, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate server key: %w", err)
	}

	serverCertTemplate := &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"Use in test only!"},
			CommonName:   serverDNS,
		},
		DNSNames:     []string{serverDNS},
		IPAddresses:  []net.IP{serverIP},
		SerialNumber: big.NewInt(2),
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(2 * time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverCertTemplate, caCert, serverKey.Public(), caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create server certificate: %w", err)
	}

	// 3. Create client key and certificate with CN "ipxe-client"
	clientKey, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate client key: %w", err)
	}

	clientCertTemplate := &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"Use in test only!"},
			CommonName:   "ipxe-client",
		},
		SerialNumber: big.NewInt(3),
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(2 * time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientCertTemplate, caCert, clientKey.Public(), caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create client certificate: %w", err)
	}

	// 4. Encode everything to PEM
	caKeyPEM := privateKeyToPEM(caKey)
	serverKeyPEM := privateKeyToPEM(serverKey)
	clientKeyPEM := privateKeyToPEM(clientKey)

	return &MTLSCertSet{
		CACert:     certToPEM(caCertDER),
		CAKey:      caKeyPEM,
		ServerCert: certToPEM(serverCertDER),
		ServerKey:  serverKeyPEM,
		ClientCert: certToPEM(clientCertDER),
		ClientKey:  clientKeyPEM,
	}, nil
}

// privateKeyToPEM encodes an RSA private key to PEM format (PKCS#1).
// Uses PKCS#1 format ("RSA PRIVATE KEY") which is the standard format for RSA keys.
func privateKeyToPEM(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

// certToPEM encodes a DER-encoded certificate to PEM format.
func certToPEM(certDER []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

// CreateTLSSecret creates a K8s Secret with type Opaque containing TLS certificates.
// The secret contains:
//   - "tls.crt": full certificate chain (server cert + CA cert) in PEM format
//     This is critical for iPXE which requires the CA cert to be sent in the TLS handshake
//     since iPXE cannot fetch external roots.
//   - "tls.key": server private key in PEM format
//   - "ca.crt": CA certificate in PEM format (for client cert validation)
func CreateTLSSecret(
	ctx context.Context,
	c client.Client,
	name, namespace string,
	certSet *MTLSCertSet,
) error {
	// Create full certificate chain: server cert + CA cert
	// iPXE requires the server to send the CA cert in the chain because iPXE
	// only embeds fingerprints of trusted roots, not the full certificates.
	// By including the CA in the server's chain, iPXE can verify the signature.
	fullChain := append(certSet.ServerCert, certSet.CACert...)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"tls.crt": fullChain, // Server cert + CA cert chain
			"tls.key": certSet.ServerKey,
			"ca.crt":  certSet.CACert,
		},
	}

	if err := c.Create(ctx, secret); err != nil {
		return errors.Join(ErrTLSSecretCreate, err)
	}

	return nil
}

// DeleteTLSSecret deletes a TLS secret by name and namespace.
func DeleteTLSSecret(ctx context.Context, c client.Client, name, namespace string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if err := c.Delete(ctx, secret); err != nil {
		return errors.Join(ErrTLSSecretDelete, err)
	}

	return nil
}

// MTLSHelmConfig contains configuration for upgrading shaper-api with mTLS.
type MTLSHelmConfig struct {
	// SecretName is the name of the K8s secret containing TLS certificates.
	SecretName string
	// Namespace is the namespace where shaper-api is deployed.
	Namespace string
	// ClientAuth is the client authentication mode: "none", "request", "require".
	ClientAuth string
	// ChartPath is the path to the shaper-api Helm chart.
	ChartPath string
	// NodePort is the NodePort to expose the service on (e.g., 30443).
	NodePort int
}

// UpgradeShaperAPIWithMTLS upgrades the shaper-api Helm release with TLS configuration.
// It runs helm upgrade with TLS values and waits for deployment readiness.
func UpgradeShaperAPIWithMTLS(ctx context.Context, kubeconfig string, config MTLSHelmConfig) error {
	// Build helm upgrade command with TLS values
	// Use 5m timeout to allow for image pulls, cache sync, and pod readiness
	// NOTE: Do NOT use --atomic as it causes rollbacks when upgrade takes longer than expected
	// Use --reuse-values to preserve existing custom values (e.g., image overrides from forge)
	// IMPORTANT: Set all tls.* values explicitly to ensure they override existing values
	args := []string{
		"upgrade", "shaper-api", config.ChartPath,
		"-n", config.Namespace,
		"--wait",
		"--reuse-values",
		"--timeout", "5m",
		"--kubeconfig", kubeconfig,
		// Set tls values explicitly - all fields must be set to override nested structure
		"--set", "tls.enabled=true",
		"--set", fmt.Sprintf("tls.clientAuth=%s", config.ClientAuth),
		"--set", fmt.Sprintf("tls.cert.secretRef.name=%s", config.SecretName),
		"--set", "tls.cert.secretRef.key=tls.crt",
		"--set", fmt.Sprintf("tls.key.secretRef.name=%s", config.SecretName),
		"--set", "tls.key.secretRef.key=tls.key",
		"--set", fmt.Sprintf("tls.ca.secretRef.name=%s", config.SecretName),
		"--set", "tls.ca.secretRef.key=ca.crt",
		"--set", "service.type=NodePort",
		"--set", fmt.Sprintf("service.nodePort=%d", config.NodePort),
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Join(ErrHelmUpgrade, fmt.Errorf("helm upgrade failed: %s: %w", string(output), err))
	}

	return nil
}

// DowngradeShaperAPIToHTTP reverts the shaper-api Helm release to HTTP-only configuration.
// It disables TLS and waits for deployment readiness.
func DowngradeShaperAPIToHTTP(ctx context.Context, kubeconfig string, config MTLSHelmConfig) error {
	// First, get current values to preserve important settings like image repository
	getValuesArgs := []string{
		"get", "values", "shaper-api",
		"-n", config.Namespace,
		"--kubeconfig", kubeconfig,
		"-o", "yaml",
	}
	getValuesCmd := exec.CommandContext(ctx, "helm", getValuesArgs...)
	currentValues, err := getValuesCmd.Output()
	if err != nil {
		return errors.Join(ErrHelmUpgrade, fmt.Errorf("failed to get current helm values: %w", err))
	}

	// Write current values to a temp file
	valuesFile, err := os.CreateTemp("", "helm-values-*.yaml")
	if err != nil {
		return errors.Join(ErrHelmUpgrade, fmt.Errorf("failed to create temp values file: %w", err))
	}
	defer func() { _ = os.Remove(valuesFile.Name()) }()

	if _, err := valuesFile.Write(currentValues); err != nil {
		return errors.Join(ErrHelmUpgrade, fmt.Errorf("failed to write values file: %w", err))
	}
	_ = valuesFile.Close()

	// Build helm upgrade command with TLS disabled
	// Use 5m timeout to allow for pod rollout
	// NOTE: Do NOT use --atomic as it causes rollbacks when upgrade takes longer than expected
	// Use -f to load current values, then override with --set for TLS settings
	args := []string{
		"upgrade", "shaper-api", config.ChartPath,
		"-n", config.Namespace,
		"--wait",
		"--timeout", "5m",
		"--kubeconfig", kubeconfig,
		"-f", valuesFile.Name(),
		"--set", "tls.enabled=false",
		"--set", "service.type=ClusterIP",
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Join(ErrHelmUpgrade, fmt.Errorf("helm downgrade failed: %s: %w", string(output), err))
	}

	return nil
}

// BuildIPXEISO builds a plain HTTP iPXE ISO on the DnsmasqServer VM.
// It performs the following steps:
// 1. Creates an embed.ipxe script that chainloads to shaper-api via HTTP
// 2. SCPs embed.ipxe to DnsmasqServer:/tmp/boot-iso/
// 3. SSHs to DnsmasqServer and runs iPXE build with EMBED and NO_WERROR=1
// 4. SCPs ipxe.iso back to local /tmp/
// Returns the local path to the built ISO.
// bridgeIP is the bridge gateway IP for the embed script (e.g., "192.168.100.1").
//
// This is needed because QEMU's built-in iPXE ROM (v1.21.1) does not support
// SMBIOS 3.0 (needed for ${uuid}) and TFTP chainloading fails. Building a custom
// iPXE ISO with the embed script and booting from CDROM works reliably.
func BuildIPXEISO(ctx context.Context, vmClient *VMClient, bridgeIP string) (string, error) {
	if bridgeIP == "" {
		bridgeIP = BridgeGatewayIP
	}
	sshKeyPath := vmClient.SSHKeyPath()
	dnsmasqServerIP := vmClient.DnsmasqIP()
	remoteBootISODir := "/tmp/boot-iso"
	remoteIPXESrcDir := "/tmp/ipxe/src"

	// Create a unique temp file for the ISO to avoid permission issues
	localISOFile, err := os.CreateTemp("", "boot-ipxe-*.iso")
	if err != nil {
		return "", errors.Join(ErrBuildIPXEISO, fmt.Errorf("failed to create temp file for ISO: %w", err))
	}
	localISOPath := localISOFile.Name()
	_ = localISOFile.Close()
	_ = os.Remove(localISOPath) // Remove so SCP can create it

	// SSH options used for all SSH/SCP commands
	sshOpts := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "ConnectTimeout=30",
		"-i", sshKeyPath,
	}

	// 1. Create remote directory
	if err := runSSHCommand(ctx, sshOpts, dnsmasqServerIP, fmt.Sprintf("mkdir -p %s", remoteBootISODir)); err != nil {
		return "", errors.Join(ErrBuildIPXEISO, fmt.Errorf("failed to create remote directory: %w", err))
	}

	// 2. Create and upload embed.ipxe script
	// Uses HTTP on port 30443 (service port) via a dedicated port-forward.
	// Port 30080 (NodePort) cannot be used because kube-proxy iptables rules
	// for the NodePort conflict with kubectl port-forward on the same port,
	// causing connection timeouts from VMs.
	embedScript := fmt.Sprintf(`#!ipxe
dhcp
chain http://%s:30443/ipxe?uuid=${uuid}&buildarch=${buildarch:uristring}
`, bridgeIP)

	localTempDir, err := os.MkdirTemp("", "boot-iso-*")
	if err != nil {
		return "", errors.Join(ErrBuildIPXEISO, fmt.Errorf("failed to create local temp directory: %w", err))
	}
	defer func() { _ = os.RemoveAll(localTempDir) }()

	embedLocalPath := filepath.Join(localTempDir, "embed.ipxe")
	if err := os.WriteFile(embedLocalPath, []byte(embedScript), 0o644); err != nil {
		return "", errors.Join(ErrBuildIPXEISO, fmt.Errorf("failed to write embed.ipxe: %w", err))
	}

	embedRemotePath := fmt.Sprintf("%s/embed.ipxe", remoteBootISODir)
	if err := runSCPUpload(ctx, sshOpts, embedLocalPath, dnsmasqServerIP, embedRemotePath); err != nil {
		return "", errors.Join(ErrBuildIPXEISO, fmt.Errorf("failed to SCP embed.ipxe: %w", err))
	}

	// 3. Build iPXE ISO on DnsmasqServer
	// make clean is important because a previous mTLS build may have left a build with different config.
	// Install genisoimage, isolinux, and syslinux-common (needed for bootable ipxe.iso generation).
	buildCmd := fmt.Sprintf(
		"sudo apt-get update >/dev/null 2>&1 && "+
			"sudo apt-get install -y genisoimage isolinux syslinux-common >/dev/null 2>&1 || true && "+
			"sudo chown -R $(id -u):$(id -g) /tmp/ipxe && "+
			"git config --global --add safe.directory /tmp/ipxe && "+
			"cd %s && make clean && make bin/ipxe.iso EMBED=%s/embed.ipxe NO_WERROR=1",
		remoteIPXESrcDir, remoteBootISODir,
	)
	if err := runSSHCommand(ctx, sshOpts, dnsmasqServerIP, buildCmd); err != nil {
		return "", errors.Join(ErrBuildIPXEISO, fmt.Errorf("failed to build iPXE ISO: %w", err))
	}

	// 4. SCP the built ISO back to local temp path
	remoteISOPath := fmt.Sprintf("%s/bin/ipxe.iso", remoteIPXESrcDir)
	if err := runSCPDownload(ctx, sshOpts, dnsmasqServerIP, remoteISOPath, localISOPath); err != nil {
		return "", errors.Join(ErrBuildIPXEISO, fmt.Errorf("failed to SCP ipxe.iso to local: %w", err))
	}

	return localISOPath, nil
}

// BuildMTLSIPXEParams contains parameters for building an mTLS-enabled iPXE ISO.
type BuildMTLSIPXEParams struct {
	// CertSet contains the mTLS certificates (client cert/key and CA).
	CertSet *MTLSCertSet
	// ShaperAPIURL is the HTTPS URL to chainload to (e.g., "https://shaper-api.local:30443").
	ShaperAPIURL string
}

// BuildMTLSIPXEISO builds an iPXE ISO with embedded mTLS client certificates on the DnsmasqServer VM.
// It performs the following steps:
// 1. SCPs client.crt, client.key, ca.crt to DnsmasqServer:/tmp/mtls-certs/
// 2. Creates embed.ipxe script that chainloads to the HTTPS shaper-api URL
// 3. SCPs embed.ipxe to DnsmasqServer:/tmp/mtls-certs/
// 4. SSHs to DnsmasqServer and runs iPXE build with CERT, PRIVKEY, TRUST, EMBED, NO_WERROR=1
// 5. SCPs ipxe.iso back to local /tmp/
// Returns the local path to the built ISO.
func BuildMTLSIPXEISO(ctx context.Context, vmClient *VMClient, params BuildMTLSIPXEParams) (string, error) {
	sshKeyPath := vmClient.SSHKeyPath()
	dnsmasqServerIP := vmClient.DnsmasqIP()
	remoteCertDir := "/tmp/mtls-certs"
	remoteIPXESrcDir := "/tmp/ipxe/src"

	// Create a unique temp file for the ISO to avoid permission issues
	// from previous test runs (libvirt may have changed ownership)
	localISOFile, err := os.CreateTemp("", "mtls-ipxe-*.iso")
	if err != nil {
		return "", errors.Join(ErrBuildMTLSIPXEISO, fmt.Errorf("failed to create temp file for ISO: %w", err))
	}
	localISOPath := localISOFile.Name()
	_ = localISOFile.Close()
	_ = os.Remove(localISOPath) // Remove so SCP can create it

	// SSH options used for all SSH/SCP commands
	sshOpts := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "ConnectTimeout=30",
		"-i", sshKeyPath,
	}

	// 1. Create remote cert directory
	if err := runSSHCommand(ctx, sshOpts, dnsmasqServerIP, fmt.Sprintf("mkdir -p %s", remoteCertDir)); err != nil {
		return "", errors.Join(ErrBuildMTLSIPXEISO, fmt.Errorf("failed to create remote cert directory: %w", err))
	}

	// 2. Write cert files to local temp directory, then SCP to remote
	localTempDir, err := os.MkdirTemp("", "mtls-certs-*")
	if err != nil {
		return "", errors.Join(ErrBuildMTLSIPXEISO, fmt.Errorf("failed to create local temp directory: %w", err))
	}
	defer func() { _ = os.RemoveAll(localTempDir) }()

	certFiles := map[string][]byte{
		"client.crt": params.CertSet.ClientCert,
		"client.key": params.CertSet.ClientKey,
		"ca.crt":     params.CertSet.CACert,
	}

	for filename, content := range certFiles {
		localPath := filepath.Join(localTempDir, filename)
		if err := os.WriteFile(localPath, content, 0o600); err != nil {
			return "", errors.Join(ErrBuildMTLSIPXEISO, fmt.Errorf("failed to write %s: %w", filename, err))
		}

		remotePath := fmt.Sprintf("%s/%s", remoteCertDir, filename)
		if err := runSCPUpload(ctx, sshOpts, localPath, dnsmasqServerIP, remotePath); err != nil {
			return "", errors.Join(ErrBuildMTLSIPXEISO, fmt.Errorf("failed to SCP %s: %w", filename, err))
		}
	}

	// Verify cert and key match using openssl (for debugging)
	// Use runSSHCommandWithOutput to capture and return the verification output
	// Also check the PEM key format - iPXE requires PKCS#1 (RSA PRIVATE KEY), not PKCS#8 (PRIVATE KEY)
	verifyCmd := fmt.Sprintf(
		"echo '=== Verifying cert/key match ===' && "+
			"CERT_MOD=$(openssl x509 -noout -modulus -in %s/client.crt | openssl md5) && "+
			"KEY_MOD=$(openssl rsa -noout -modulus -in %s/client.key 2>/dev/null | openssl md5) && "+
			"echo \"Cert modulus: $CERT_MOD\" && "+
			"echo \"Key modulus:  $KEY_MOD\" && "+
			"if [ \"$CERT_MOD\" = \"$KEY_MOD\" ]; then echo 'MATCH: Certificate and key are a valid pair'; else echo 'MISMATCH: Certificate and key do NOT match'; exit 1; fi && "+
			"echo '=== PEM Key Header (should be RSA PRIVATE KEY for PKCS#1) ===' && "+
			"head -1 %s/client.key && "+
			"echo '=== Raw PEM Key (first bytes after base64 decode) ===' && "+
			"cat %s/client.key | grep -v '^-----' | base64 -d | xxd | head -5 && "+
			"echo '=== iPXE expects PKCS#1 DER format (openssl rsa -outform DER) ===' && "+
			"openssl rsa -in %s/client.key -outform DER 2>/dev/null | xxd | head -5",
		remoteCertDir, remoteCertDir, remoteCertDir, remoteCertDir, remoteCertDir,
	)
	verifyOutput, err := runSSHCommandWithOutput(ctx, sshOpts, dnsmasqServerIP, verifyCmd)
	if err != nil {
		return "", errors.Join(ErrBuildMTLSIPXEISO, fmt.Errorf("failed to verify cert/key: %w\nOutput: %s", err, verifyOutput))
	}
	// Log verification output for debugging (visible in test output)
	fmt.Printf("Certificate verification output:\n%s\n", verifyOutput)

	// 3. Create and upload embed.ipxe script
	// The script includes retry logic and uses the provided shaper-api URL
	// CRITICAL: Must call dhcp first to acquire network configuration when booting from CDROM
	embedScript := fmt.Sprintf(`#!ipxe

:start
echo Booting with mTLS client certificate...
echo Acquiring network configuration via DHCP...

:dhcp_retry
dhcp || goto dhcp_failed
echo Got IP address: ${net0/ip}
echo Gateway: ${net0/gateway}
goto chain

:dhcp_failed
echo DHCP failed, retrying in 2 seconds...
sleep 2
goto dhcp_retry

:chain
echo Chainloading from %s/ipxe?uuid=${uuid}&buildarch=${buildarch}

:retry
chain %s/ipxe?uuid=${uuid}&buildarch=${buildarch} || goto failed

:failed
echo Chain failed, retrying in 5 seconds...
sleep 5
goto retry
`, params.ShaperAPIURL, params.ShaperAPIURL)

	embedLocalPath := filepath.Join(localTempDir, "embed.ipxe")
	if err := os.WriteFile(embedLocalPath, []byte(embedScript), 0o644); err != nil {
		return "", errors.Join(ErrBuildMTLSIPXEISO, fmt.Errorf("failed to write embed.ipxe: %w", err))
	}

	embedRemotePath := fmt.Sprintf("%s/embed.ipxe", remoteCertDir)
	if err := runSCPUpload(ctx, sshOpts, embedLocalPath, dnsmasqServerIP, embedRemotePath); err != nil {
		return "", errors.Join(ErrBuildMTLSIPXEISO, fmt.Errorf("failed to SCP embed.ipxe: %w", err))
	}

	// 4. Build iPXE ISO on DnsmasqServer with mTLS certificates embedded
	// NO_WERROR=1 is required to avoid build failures (matches existing pattern in forge.yaml)
	// First, fix ownership and git safe.directory to avoid errors from cloud-init setup
	// The iPXE directory is created by root during cloud-init, so we need to chown it
	// Install genisoimage, isolinux, and syslinux-common (needed for bootable ipxe.iso generation)
	// Enable HTTPS support in iPXE by creating a local config override
	// DEBUG=tls:3 enables TLS debugging output
	//
	// Certificate configuration:
	// - CERT= must contain ONLY the client certificate (not CA), because iPXE matches PRIVKEY to CERT
	// - PRIVKEY= contains the client private key
	// - TRUST= contains the CA certificate for server verification
	// - The server sends its cert + CA in the TLS chain (configured in CreateTLSSecret)
	// Including CA in CERT= causes "could not find certificate corresponding to private key" error
	// DEBUG=tls:3,x509:3,certstore,privkey helps debug certificate/key matching issues
	//
	// IMPORTANT: Patch iPXE Makefile to use -traditional flag for OpenSSL 3.x compatibility.
	// OpenSSL 3.x outputs PKCS#8 DER by default, but iPXE expects PKCS#1 DER.
	// The -traditional flag forces PKCS#1 output format.
	// iPXE's build system runs `openssl rsa -in X -outform DER -out Y` to convert PEM keys to DER.
	// OpenSSL 3.x outputs PKCS#8 DER by default, but iPXE expects PKCS#1 DER.
	// We create a wrapper script that intercepts openssl rsa commands and adds -traditional flag.
	//
	// The wrapper is placed in /tmp/openssl-wrapper and added to PATH before /usr/bin.
	buildCmd := fmt.Sprintf(
		"sudo apt-get update >/dev/null 2>&1 && "+
			"sudo apt-get install -y genisoimage isolinux syslinux-common >/dev/null 2>&1 || true && "+
			"sudo chown -R $(id -u):$(id -g) /tmp/ipxe && "+
			"git config --global --add safe.directory /tmp/ipxe && "+
			"mkdir -p %s/config/local && "+
			"echo -e '#define DOWNLOAD_PROTO_HTTPS\\n#define CRYPTO_HMAC\\n#define CRYPTO_RSA\\n#define CRYPTO_AES_CBC\\n#define CRYPTO_SHA1\\n#define CRYPTO_SHA256\\n#define CERT_CMD\\n#define PRIVKEY_CMD\\n#define IMAGE_TRUST_CMD' > %s/config/local/general.h && "+
			// Create openssl wrapper that adds -traditional for rsa commands
			"mkdir -p /tmp/openssl-wrapper && "+
			"cat > /tmp/openssl-wrapper/openssl << 'WRAPPER_EOF'\n"+
			"#!/bin/bash\n"+
			"# Wrapper to add -traditional flag for OpenSSL 3.x PKCS#1 compatibility\n"+
			"if [[ \"$1\" == \"rsa\" ]]; then\n"+
			"  /usr/bin/openssl rsa -traditional \"${@:2}\"\n"+
			"else\n"+
			"  /usr/bin/openssl \"$@\"\n"+
			"fi\n"+
			"WRAPPER_EOF\n"+
			"chmod +x /tmp/openssl-wrapper/openssl && "+
			"echo '=== Created OpenSSL wrapper for PKCS#1 DER output ===' && "+
			// Verify wrapper works
			"echo '=== Testing wrapper: openssl rsa -outform DER should produce PKCS#1 ===' && "+
			"PATH=/tmp/openssl-wrapper:$PATH openssl rsa -in %s/client.key -outform DER 2>/dev/null | xxd | head -3 && "+
			// Build with wrapper in PATH
			"cd %s && PATH=/tmp/openssl-wrapper:$PATH make clean && PATH=/tmp/openssl-wrapper:$PATH make bin/ipxe.iso CERT=%s/client.crt PRIVKEY=%s/client.key TRUST=%s/ca.crt EMBED=%s/embed.ipxe NO_WERROR=1 DEBUG=tls:3,x509:3,certstore,privkey",
		remoteIPXESrcDir, remoteIPXESrcDir,
		remoteCertDir, // Test wrapper
		remoteIPXESrcDir,
		remoteCertDir, remoteCertDir, remoteCertDir, remoteCertDir,
	)
	if err := runSSHCommand(ctx, sshOpts, dnsmasqServerIP, buildCmd); err != nil {
		return "", errors.Join(ErrBuildMTLSIPXEISO, fmt.Errorf("failed to build iPXE ISO: %w", err))
	}

	// 5. SCP the built ISO back to local temp path
	remoteISOPath := fmt.Sprintf("%s/bin/ipxe.iso", remoteIPXESrcDir)
	if err := runSCPDownload(ctx, sshOpts, dnsmasqServerIP, remoteISOPath, localISOPath); err != nil {
		return "", errors.Join(ErrBuildMTLSIPXEISO, fmt.Errorf("failed to SCP ipxe.iso to local: %w", err))
	}

	return localISOPath, nil
}

// runSSHCommand executes a command on a remote host via SSH.
func runSSHCommand(ctx context.Context, sshOpts []string, host, command string) error {
	args := append(sshOpts, fmt.Sprintf("ubuntu@%s", host), command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SSH command failed: %w, output: %s", err, string(output))
	}
	return nil
}

// runSSHCommandWithOutput executes a command on a remote host via SSH and returns the output.
func runSSHCommandWithOutput(ctx context.Context, sshOpts []string, host, command string) (string, error) {
	args := append(sshOpts, fmt.Sprintf("ubuntu@%s", host), command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("SSH command failed: %w", err)
	}
	return string(output), nil
}

// runSCPUpload copies a local file to a remote host.
func runSCPUpload(ctx context.Context, sshOpts []string, localPath, host, remotePath string) error {
	args := append(sshOpts, localPath, fmt.Sprintf("ubuntu@%s:%s", host, remotePath))
	cmd := exec.CommandContext(ctx, "scp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SCP upload failed: %w, output: %s", err, string(output))
	}
	return nil
}

// runSCPDownload copies a file from a remote host to local.
func runSCPDownload(ctx context.Context, sshOpts []string, host, remotePath, localPath string) error {
	args := append(sshOpts, fmt.Sprintf("ubuntu@%s:%s", host, remotePath), localPath)
	cmd := exec.CommandContext(ctx, "scp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SCP download failed: %w, output: %s", err, string(output))
	}
	return nil
}
