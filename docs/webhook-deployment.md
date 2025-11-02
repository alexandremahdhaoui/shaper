# Shaper Webhook Deployment Guide

This guide explains how to deploy the shaper-webhook admission webhooks to your Kubernetes cluster.

## Prerequisites

### Required Software

1. **Kubernetes Cluster**: v1.28+ with admission webhooks enabled
2. **Helm**: v3.0+ for chart installation
3. **cert-manager**: v1.14.0+ for TLS certificate management
4. **kubectl**: Configured to access your cluster

### Required CRDs

Shaper CRDs must be installed before deploying webhooks:

```bash
# Install Shaper CRDs
helm install shaper-crds ./charts/shaper-crds
```

## Installing cert-manager

The webhook requires TLS certificates managed by cert-manager:

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.0/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --for=condition=Available --timeout=300s -n cert-manager deployment/cert-manager-webhook
```

### Create a Certificate Issuer

```bash
# Create a self-signed issuer for development/testing
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
  namespace: default
spec:
  selfSigned: {}
EOF
```

For production, use a proper CA issuer or Let's Encrypt.

## Deploying shaper-webhooks

### Using Helm (Recommended)

```bash
# Install with default values
helm install shaper-webhooks ./charts/shaper-webhooks

# Install with custom values
helm install shaper-webhooks ./charts/shaper-webhooks \
  --set assignmentNamespace=custom-namespace \
  --set profileNamespace=custom-namespace \
  --set certificate.issuerRef.name=my-issuer
```

### Configuration Options

Key configuration values (see `charts/shaper-webhooks/values.yaml` for full list):

| Parameter | Description | Default |
|-----------|-------------|---------|
| `assignmentNamespace` | Namespace for Assignment CRDs | `default` |
| `profileNamespace` | Namespace for Profile CRDs | `default` |
| `webhookServer.port` | Webhook HTTPS port | `9443` |
| `probesServer.port` | Health probes port | `8081` |
| `metricsServer.port` | Prometheus metrics port | `8080` |
| `certificate.enabled` | Enable cert-manager certificate | `true` |
| `certificate.issuerRef.name` | cert-manager Issuer name | `selfsigned-issuer` |
| `certificate.issuerRef.kind` | cert-manager Issuer kind | `Issuer` |

### Verification

```bash
# Check webhook pod is running
kubectl get pods -l app.kubernetes.io/name=shaper-webhooks

# Check certificate is issued
kubectl get certificate
kubectl describe certificate shaper-webhooks-cert

# Verify webhook configurations
kubectl get validatingwebhookconfigurations shaper-webhooks-validating
kubectl get mutatingwebhookconfigurations shaper-webhooks-mutating

# Test health endpoints
kubectl port-forward svc/shaper-webhooks 8081:8081
curl http://localhost:8081/healthz  # Should return 200 OK
curl http://localhost:8081/readyz   # Should return 200 OK

# Check metrics
curl http://localhost:8080/metrics
```

## Testing the Webhook

### Valid Assignment

```bash
cat <<EOF | kubectl apply -f -
apiVersion: shaper.amahdha.com/v1alpha1
kind: Assignment
metadata:
  name: test-assignment
  namespace: default
spec:
  subjectSelectors:
    buildarch:
      - arm64
    uuidList:
      - 47c6da67-7477-4970-aa03-84e48ff4f6ad
  profileName: test-profile
  isDefault: false
EOF
```

### Invalid Assignment (Should be Rejected)

```bash
# Invalid UUID format - webhook should reject
cat <<EOF | kubectl apply -f -
apiVersion: shaper.amahdha.com/v1alpha1
kind: Assignment
metadata:
  name: invalid-assignment
  namespace: default
spec:
  subjectSelectors:
    buildarch:
      - arm64
    uuidList:
      - not-a-valid-uuid
  profileName: test-profile
  isDefault: false
EOF
# Expected error: invalid UUID format
```

### Verify Mutation

```bash
# Create an assignment and check that labels are added
kubectl apply -f test/integration/webhook/fixtures/valid-assignment.yaml

# Check labels were added by mutating webhook
kubectl get assignment valid-assignment -o yaml | grep -A 10 labels:
# Should see UUID labels like: uuid.shaper.amahdha.com/<uuid>: ""
```

## Troubleshooting

### Webhook Not Ready

```bash
# Check webhook logs
kubectl logs -l app.kubernetes.io/name=shaper-webhooks

# Common issues:
# - Certificate not ready: Check cert-manager logs
# - CA injection failed: Verify cert-manager webhook is running
# - Port conflicts: Check port configuration
```

### Certificate Issues

```bash
# Check certificate status
kubectl describe certificate shaper-webhooks-cert

# Check cert-manager logs
kubectl logs -n cert-manager deployment/cert-manager

# Common issues:
# - Issuer not found: Create the issuer first
# - CA bundle not injected: Check cert-manager webhook
```

### Webhook Configuration Issues

```bash
# Check webhook configuration
kubectl get validatingwebhookconfigurations shaper-webhooks-validating -o yaml

# Verify CA bundle is injected
kubectl get validatingwebhookconfigurations shaper-webhooks-validating -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | base64 -d

# Check webhook endpoints
kubectl get svc shaper-webhooks
```

### Requests Timing Out

```bash
# Check webhook timeouts (default: 10s)
kubectl get validatingwebhookconfigurations shaper-webhooks-validating -o yaml | grep timeoutSeconds

# Check network policies
kubectl get networkpolicies

# Verify service endpoints
kubectl get endpoints shaper-webhooks
```

## Uninstalling

```bash
# Remove webhook helm release
helm uninstall shaper-webhooks

# Clean up webhook configurations (usually auto-deleted)
kubectl delete validatingwebhookconfigurations shaper-webhooks-validating
kubectl delete mutatingwebhookconfigurations shaper-webhooks-mutating

# Remove certificate
kubectl delete certificate shaper-webhooks-cert
```

## Advanced Configuration

### Custom Namespaces

If Assignments and Profiles are in different namespaces:

```bash
helm install shaper-webhooks ./charts/shaper-webhooks \
  --set assignmentNamespace=assignments \
  --set profileNamespace=profiles
```

Note: The webhook ServiceAccount needs RBAC permissions in both namespaces.

### High Availability

For production deployments, increase replica count:

```bash
helm install shaper-webhooks ./charts/shaper-webhooks \
  --set replicaCount=3
```

### Resource Limits

Adjust resource limits based on your workload:

```bash
helm install shaper-webhooks ./charts/shaper-webhooks \
  --set resources.limits.memory=256Mi \
  --set resources.limits.cpu=200m
```

## See Also

- [Integration Tests](../test/integration/webhook/README.md)
- [Configuration Example](../examples/shaper-webhook.example.yaml)
- [Helm Chart Values](../charts/shaper-webhooks/values.yaml)
