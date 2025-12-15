# Webhook Deployment
**Validate and mutate Shaper CRDs with Kubernetes admission webhooks.**

> "The webhook caught our invalid UUID before it broke the boot flow."
> - Platform Engineer

## Problem

Without validation, invalid Profiles and Assignments slip into the cluster and cause runtime failures. Admission webhooks catch errors at apply time and auto-populate required labels.

## Contents

- [Quick Start](#quick-start)
- [How do I configure it?](#how-do-i-configure-it)
- [FAQ](#faq)

## Quick Start

```bash
# 1. Install cert-manager (required for TLS)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.0/cert-manager.yaml
kubectl wait --for=condition=Available -n cert-manager deployment/cert-manager-webhook --timeout=300s

# 2. Create issuer
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
EOF

# 3. Install webhooks (CRDs must exist first - see api-deployment.md)
helm install shaper-webhooks ./charts/shaper-webhooks

# 4. Verify
kubectl get pods -l app.kubernetes.io/name=shaper-webhooks
```

## How do I configure it?

| Parameter | Default | Description |
|-----------|---------|-------------|
| `assignmentNamespace` | `default` | Namespace for Assignments |
| `profileNamespace` | `default` | Namespace for Profiles |
| `webhookServer.port` | `9443` | Webhook HTTPS port |
| `probesServer.port` | `8081` | Health probes port |
| `certificate.issuerRef.name` | `selfsigned-issuer` | cert-manager Issuer |
| `replicaCount` | `1` | Pod replicas |

**Custom namespaces:**
```bash
helm install shaper-webhooks ./charts/shaper-webhooks \
  --set assignmentNamespace=shaper \
  --set profileNamespace=shaper
```

**Production (HA):**
```bash
helm install shaper-webhooks ./charts/shaper-webhooks --set replicaCount=3
```

## FAQ

**Q: Pod not starting?**
A: Check cert-manager: `kubectl get certificate`. Ensure issuer exists and certificate is Ready.

**Q: Requests timing out?**
A: Verify service endpoints: `kubectl get endpoints shaper-webhooks`. Check NetworkPolicies.

**Q: Certificate not ready?**
A: Check cert-manager logs: `kubectl logs -n cert-manager deployment/cert-manager`.

**Q: CA bundle not injected?**
A: Verify cert-manager webhook is running and annotations are correct on webhook config.

**Q: How to check webhook logs?**
A: `kubectl logs -l app.kubernetes.io/name=shaper-webhooks`

## Uninstall

```bash
helm uninstall shaper-webhooks
```

## Links

- [API Deployment](./api-deployment.md) - deploy shaper-api first
- [Helm Values](../charts/shaper-webhooks/values.yaml) - full configuration
- [Architecture](../ARCHITECTURE.md) - system design
