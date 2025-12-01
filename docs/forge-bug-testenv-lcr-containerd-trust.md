# Forge Bug: testenv-lcr integration with testenv-kind has multiple issues

## Summary

When using `testenv-lcr` with `testenv-kind`, there are three separate issues that prevent containerd inside the Kind node from pulling images from the local registry.

## Environment

- Forge version: v0.16.0
- Kind version: v0.27.0
- Kubernetes version: v1.34.0

## Root Causes (Confirmed 2025-11-30)

### Issue 1: DNS Resolution - Host /etc/hosts leaks to Kind

**Problem**: testenv-lcr adds `/etc/hosts` entry on the **host machine**:
```
127.0.0.1 testenv-lcr.testenv-lcr.svc.cluster.local
```

This entry leaks into the Kind container through Docker's DNS resolution chain:
1. Kind node runs `getent hosts testenv-lcr.testenv-lcr.svc.cluster.local`
2. NSS checks `/etc/hosts` inside Kind - no match
3. DNS lookup to Docker's DNS (172.18.0.1)
4. Docker DNS forwards to host's systemd-resolved (127.0.0.53)
5. Host's systemd-resolved checks host's `/etc/hosts` which HAS the entry
6. Returns `127.0.0.1` instead of the ClusterIP

**Evidence**:
```bash
# Inside Kind node (wrong - resolves to 127.0.0.1)
$ docker exec <kind-node> getent hosts testenv-lcr.testenv-lcr.svc.cluster.local
127.0.0.1       localhost testenv-lcr.testenv-lcr.svc.cluster.local

# Pod-level DNS works correctly (uses CoreDNS)
$ kubectl run -it debug --image=busybox -- nslookup testenv-lcr.testenv-lcr.svc.cluster.local
Address: 10.96.236.151  # Correct ClusterIP
```

**Fix needed in forge**: Add `/etc/hosts` entry INSIDE Kind node directly instead of (or in addition to) the host machine:
```bash
CLUSTER_IP=$(kubectl get svc -n testenv-lcr testenv-lcr -o jsonpath='{.spec.clusterIP}')
docker exec <kind-node> bash -c "echo '$CLUSTER_IP testenv-lcr.testenv-lcr.svc.cluster.local' >> /etc/hosts"
```

### Issue 2: containerd TLS Configuration Missing

**Problem**: containerd inside Kind is not configured to trust the testenv-lcr's self-signed TLS certificate.

**Error**:
```
tls: failed to verify certificate: x509: certificate signed by unknown authority
```

**Fix needed in forge**: Configure containerd inside Kind with proper CA certificate (NOT skip_verify):

1. The containerd `config.toml` in Kind already has `config_path = "/etc/containerd/certs.d"` by default.

2. Create directory and copy CA certificate:
```bash
# Create certs.d directory for registry
docker exec <kind-node> mkdir -p "/etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000"

# Copy CA certificate from testenv-lcr secret
kubectl get secret -n testenv-lcr testenv-lcr-tls -o jsonpath='{.data.ca\.crt}' | base64 -d > /tmp/testenv-lcr-ca.crt
docker cp /tmp/testenv-lcr-ca.crt <kind-node>:/etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/ca.crt
```

3. Create `/etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/hosts.toml` with CA reference:
```toml
server = "https://testenv-lcr.testenv-lcr.svc.cluster.local:5000"

[host."https://testenv-lcr.testenv-lcr.svc.cluster.local:5000"]
  capabilities = ["pull", "resolve"]
  ca = "/etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/ca.crt"
```

4. Restart containerd: `docker exec <kind-node> systemctl restart containerd`

**IMPORTANT**: Do NOT use `skip_verify = true` as this disables TLS verification entirely. Use the CA certificate for proper TLS validation.

### Issue 3: imagePullSecret Registry URL Missing Port

**Problem**: The imagePullSecret created by testenv-lcr has the registry URL **without the port**, but images use the port in their reference.

**Secret content (wrong)**:
```json
{"auths":{"testenv-lcr.testenv-lcr.svc.cluster.local":{"username":"...","password":"..."}}}
```

**Image reference (has port)**:
```
testenv-lcr.testenv-lcr.svc.cluster.local:5000/shaper-controller-container:latest
```

Kubernetes/containerd does **exact URL matching** for credentials, so the port mismatch causes auth failure.

**Error**:
```
authorization failed: no basic auth credentials
```

**Fix needed in forge**: Include `:5000` in the registry URL when creating imagePullSecrets:
```json
{"auths":{"testenv-lcr.testenv-lcr.svc.cluster.local:5000":{"username":"...","password":"..."}}}
```

### Issue 4: Missing selfsigned-issuer for shaper-webhooks (Related)

**Problem**: The shaper-webhooks Helm chart expects a `selfsigned-issuer` Issuer in the target namespace for cert-manager to issue webhook certificates, but this issuer doesn't exist.

**Error**:
```
secret "shaper-webhooks-cert" not found
```

**Fix**: This is not a forge bug but should be documented. Users must create the issuer:
```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
  namespace: shaper-system
spec:
  selfSigned: {}
```

## Steps to Reproduce

1. Configure a test stage with `testenv-kind`, `testenv-lcr`, and `testenv-helm-install`
2. Push images to the testenv-lcr registry (works via port-forward)
3. Install a Helm chart that references images from `testenv-lcr.testenv-lcr.svc.cluster.local:5000`
4. Observe that the deployment times out waiting for pods to become ready

## Manual Workaround (Complete, Tested 2025-11-30)

All three issues must be fixed manually after testenv-lcr creates the environment:

```bash
#!/bin/bash
set -e

# Configuration
NODE_NAME=$(docker ps --filter name=forge-test --format '{{.Names}}' | head -1)
KUBECONFIG=/path/to/testenv/kubeconfig

# Get ClusterIP
REGISTRY_IP=$(kubectl --kubeconfig $KUBECONFIG get svc -n testenv-lcr testenv-lcr -o jsonpath='{.spec.clusterIP}')

echo "=== Fix 1: Add /etc/hosts inside Kind node ==="
docker exec $NODE_NAME bash -c "echo '$REGISTRY_IP testenv-lcr.testenv-lcr.svc.cluster.local' >> /etc/hosts"

echo "=== Fix 2: Configure containerd with CA certificate ==="

# Create certs.d directory
docker exec $NODE_NAME mkdir -p "/etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000"

# Extract CA certificate from testenv-lcr secret
kubectl --kubeconfig $KUBECONFIG get secret -n testenv-lcr testenv-lcr-tls -o jsonpath='{.data.ca\.crt}' | base64 -d > /tmp/testenv-lcr-ca.crt

# Copy CA into Kind node
docker cp /tmp/testenv-lcr-ca.crt $NODE_NAME:/etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/ca.crt

# Create hosts.toml with CA certificate (NOT skip_verify)
docker exec $NODE_NAME bash -c 'cat > /etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/hosts.toml << EOF
server = "https://testenv-lcr.testenv-lcr.svc.cluster.local:5000"

[host."https://testenv-lcr.testenv-lcr.svc.cluster.local:5000"]
  capabilities = ["pull", "resolve"]
  ca = "/etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/ca.crt"
EOF'

# Restart containerd
docker exec $NODE_NAME systemctl restart containerd

echo "=== Fix 3: Recreate imagePullSecret with port ==="

# Get credentials from existing secret
USERNAME=$(kubectl --kubeconfig $KUBECONFIG get secret -n shaper-system testenv-lcr-credentials -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d | jq -r '.auths | to_entries[0].value.username')
PASSWORD=$(kubectl --kubeconfig $KUBECONFIG get secret -n shaper-system testenv-lcr-credentials -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d | jq -r '.auths | to_entries[0].value.password')

# Recreate secrets with port in URL
for NS in shaper-system default; do
  kubectl --kubeconfig $KUBECONFIG delete secret -n $NS testenv-lcr-credentials --ignore-not-found
  kubectl --kubeconfig $KUBECONFIG create secret docker-registry testenv-lcr-credentials \
    --docker-server=testenv-lcr.testenv-lcr.svc.cluster.local:5000 \
    --docker-username="$USERNAME" \
    --docker-password="$PASSWORD" \
    -n $NS
done

echo "=== Verifying image pull works ==="
# Test pulling image with crictl
docker exec $NODE_NAME crictl pull --creds "$USERNAME:$PASSWORD" testenv-lcr.testenv-lcr.svc.cluster.local:5000/shaper-controller-container:latest

echo "=== Delete failing pods to trigger retry ==="
kubectl --kubeconfig $KUBECONFIG delete pods -n shaper-system --all

echo "=== Done! ==="
```

## Verification

After applying the fixes, verify that:

1. **DNS resolution works inside Kind node**:
```bash
docker exec <kind-node> getent hosts testenv-lcr.testenv-lcr.svc.cluster.local
# Should return ClusterIP, NOT 127.0.0.1
```

2. **TLS works with CA certificate (not skip_verify)**:
```bash
docker exec <kind-node> curl -s --cacert /etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/ca.crt \
  https://testenv-lcr.testenv-lcr.svc.cluster.local:5000/v2/
# Should return 401 (unauthorized) not TLS error
```

3. **Image pull works**:
```bash
docker exec <kind-node> crictl pull --creds "USER:PASS" testenv-lcr.testenv-lcr.svc.cluster.local:5000/image:tag
# Should succeed
```

## Suggested Forge Fixes

1. **testenv-lcr** should NOT add /etc/hosts entries on the host machine when used with testenv-kind
2. **testenv-lcr** should add /etc/hosts entries INSIDE Kind nodes with the service ClusterIP
3. **testenv-lcr** should configure containerd inside Kind nodes with:
   - Create certs.d directory for the registry
   - Copy CA certificate from testenv-lcr-tls secret into the certs.d directory
   - Create hosts.toml with `ca = "<path>"` (NOT `skip_verify = true`)
   - Restart containerd
4. **testenv-lcr** should include the port (`:5000`) in the registry URL when creating imagePullSecrets

Alternative approach: Use Kind's built-in local registry support which properly handles these configurations.

## TLS Configuration Details (Confirmed Working 2025-11-30)

The containerd TLS configuration requires understanding how containerd resolves registry certificates:

### How containerd finds CA certificates

1. containerd reads `config_path` from `/etc/containerd/config.toml` (default: `/etc/containerd/certs.d`)
2. For each registry, it looks for a directory matching the registry hostname:port
3. Inside that directory, it reads `hosts.toml` for configuration
4. The `hosts.toml` file specifies where to find the CA certificate

### Directory Structure Inside Kind Node

```
/etc/containerd/certs.d/
└── testenv-lcr.testenv-lcr.svc.cluster.local:5000/
    ├── ca.crt      # CA certificate extracted from testenv-lcr-tls secret
    └── hosts.toml  # Configuration pointing to ca.crt
```

### hosts.toml Content (Proper TLS - NOT skip_verify)

```toml
server = "https://testenv-lcr.testenv-lcr.svc.cluster.local:5000"

[host."https://testenv-lcr.testenv-lcr.svc.cluster.local:5000"]
  capabilities = ["pull", "resolve"]
  ca = "/etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/ca.crt"
```

### Why NOT to use skip_verify

Using `skip_verify = true` is insecure and defeats the purpose of TLS:
- Vulnerable to man-in-the-middle attacks
- Provides false sense of security
- Not suitable for production or testing (tests should mirror production behavior)

The proper solution is to configure the CA certificate, which:
- Validates the registry's certificate chain
- Ensures secure communication
- Matches production deployment patterns

### CA Certificate Source

The CA certificate is stored in the `testenv-lcr-tls` secret in the `testenv-lcr` namespace:

```bash
# Extract CA certificate
kubectl get secret -n testenv-lcr testenv-lcr-tls -o jsonpath='{.data.ca\.crt}' | base64 -d

# The secret contains:
# - ca.crt: CA certificate (root of trust)
# - tls.crt: Server certificate signed by CA
# - tls.key: Server private key
```

### Complete TLS Fix Commands

```bash
NODE_NAME=$(docker ps --filter name=forge-test --format '{{.Names}}' | head -1)
KUBECONFIG=/tmp/integration-kubeconfig

# 1. Create certs.d directory for the registry
docker exec $NODE_NAME mkdir -p "/etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000"

# 2. Extract and copy CA certificate
kubectl --kubeconfig $KUBECONFIG get secret -n testenv-lcr testenv-lcr-tls -o jsonpath='{.data.ca\.crt}' | base64 -d > /tmp/testenv-lcr-ca.crt
docker cp /tmp/testenv-lcr-ca.crt $NODE_NAME:/etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/ca.crt

# 3. Create hosts.toml with CA reference
docker exec $NODE_NAME bash -c 'cat > /etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/hosts.toml << EOF
server = "https://testenv-lcr.testenv-lcr.svc.cluster.local:5000"

[host."https://testenv-lcr.testenv-lcr.svc.cluster.local:5000"]
  capabilities = ["pull", "resolve"]
  ca = "/etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/ca.crt"
EOF'

# 4. Restart containerd to pick up new configuration
docker exec $NODE_NAME systemctl restart containerd

# 5. Verify TLS works (should return 401 Unauthorized, NOT TLS error)
docker exec $NODE_NAME curl -s --cacert /etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/ca.crt \
  https://testenv-lcr.testenv-lcr.svc.cluster.local:5000/v2/
```

### Verification Steps

1. **Check containerd config_path is set correctly**:
```bash
docker exec $NODE_NAME cat /etc/containerd/config.toml | grep config_path
# Output: config_path = "/etc/containerd/certs.d"
```

2. **Verify hosts.toml exists and has correct content**:
```bash
docker exec $NODE_NAME cat /etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/hosts.toml
```

3. **Verify CA certificate is valid**:
```bash
docker exec $NODE_NAME openssl x509 -in /etc/containerd/certs.d/testenv-lcr.testenv-lcr.svc.cluster.local:5000/ca.crt -text -noout
```

4. **Test image pull with crictl**:
```bash
USERNAME=$(kubectl --kubeconfig $KUBECONFIG get secret -n shaper-system testenv-lcr-credentials -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d | jq -r '.auths | to_entries[0].value.username')
PASSWORD=$(kubectl --kubeconfig $KUBECONFIG get secret -n shaper-system testenv-lcr-credentials -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d | jq -r '.auths | to_entries[0].value.password')

docker exec $NODE_NAME crictl pull --creds "$USERNAME:$PASSWORD" testenv-lcr.testenv-lcr.svc.cluster.local:5000/shaper-api:latest
# Should succeed without TLS errors
```

## Related Configuration

```yaml
- alias: testenv-integration
  type: testenv
  testenv:
    - engine: go://testenv-kind
    - engine: go://testenv-lcr
      spec:
        enabled: true
        namespace: testenv-lcr
        imagePullSecretName: testenv-lcr-credentials
        imagePullSecretNamespaces:
          - default
          - shaper-system
        images:
          - name: local://shaper-controller-container:latest
          - name: local://shaper-webhook-container:latest
    - engine: go://testenv-helm-install
      spec:
        charts:
          - name: shaper-controller
            sourceType: local
            path: ./charts/shaper-controller
            namespace: shaper-system
            values:
              image:
                repository: testenv-lcr.testenv-lcr.svc.cluster.local:5000/shaper-controller-container
                tag: latest
              imagePullSecrets:
                - name: testenv-lcr-credentials
```
