# Shaper API Deployment
**Deploy the iPXE boot server to serve network boot configurations from Kubernetes.**

> "We went from managing config files to declaring CRDs. Deployment took 5 minutes."
> - Platform Engineer

## Problem

Production iPXE servers need health checks, metrics, scaling, and secure access. This guide covers deploying shaper-api with all operational concerns addressed.

## Contents

- [Quick Start](#quick-start)
- [How do I configure it?](#how-do-i-configure-it)
- [How do I expose it externally?](#how-do-i-expose-it-externally)
- [How do I monitor it?](#how-do-i-monitor-it)
- [How do I secure it?](#how-do-i-secure-it)
- [FAQ](#faq)

## Quick Start

```bash
# Install CRDs first
helm install shaper-crds ./charts/shaper-crds

# Install API server
helm install shaper-api ./charts/shaper-api

# Verify
kubectl get pods -l app.kubernetes.io/name=shaper-api
kubectl port-forward svc/shaper-api 8081:8081
curl http://localhost:8081/healthz
```

## How do I configure it?

Key values in `charts/shaper-api/values.yaml`:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `config.assignmentNamespace` | `default` | Namespace for Assignments |
| `config.profileNamespace` | `default` | Namespace for Profiles |
| `config.apiServer.port` | `30443` | API HTTP port |
| `config.probesServer.port` | `8081` | Health probes port |
| `config.metricsServer.port` | `8080` | Metrics port |
| `replicaCount` | `1` | Pod replicas |
| `service.type` | `ClusterIP` | Service type |
| `autoscaling.enabled` | `false` | Enable HPA |

Example with custom namespaces:

```bash
helm install shaper-api ./charts/shaper-api \
  --set config.assignmentNamespace=shaper \
  --set config.profileNamespace=shaper
```

## How do I expose it externally?

**Ingress:**
```bash
helm upgrade shaper-api ./charts/shaper-api \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set "ingress.hosts[0].host=shaper.example.com" \
  --set "ingress.hosts[0].paths[0].path=/" \
  --set "ingress.hosts[0].paths[0].pathType=Prefix"
```

**Gateway API:**
```bash
helm upgrade shaper-api ./charts/shaper-api \
  --set httpRoute.enabled=true \
  --set "httpRoute.hostnames[0]=shaper.example.com" \
  --set "httpRoute.parentRefs[0].name=my-gateway"
```

**LoadBalancer:**
```bash
helm upgrade shaper-api ./charts/shaper-api \
  --set service.type=LoadBalancer
```

## How do I monitor it?

**Prometheus metrics** at `/metrics` on port 8080.

Enable ServiceMonitor:
```bash
helm upgrade shaper-api ./charts/shaper-api \
  --set metrics.serviceMonitor.enabled=true \
  --set "metrics.serviceMonitor.labels.prometheus=kube-prometheus"
```

**Health endpoints:**
- Liveness: `:8081/healthz`
- Readiness: `:8081/readyz`

**Logs:**
```bash
kubectl logs -f -l app.kubernetes.io/name=shaper-api
```

## How do I secure it?

**RBAC** is auto-configured. The ServiceAccount gets read access to Profiles, Assignments, ConfigMaps, and Secrets.

**Network Policy example:**
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: shaper-api-ingress
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: shaper-api
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - port: 30443
```

**Production recommendations:**
- Use Ingress/Gateway with TLS termination
- Place behind API gateway for authentication
- Deploy in isolated network segment

## FAQ

**Q: API pod not starting?**
A: Check CRDs are installed: `kubectl get crds | grep shaper`. Install with `helm install shaper-crds ./charts/shaper-crds`.

**Q: Assignment not found errors?**
A: Verify assignments exist: `kubectl get assignments -A`. Check namespace config matches where CRDs are deployed.

**Q: Profile not found errors?**
A: Verify the profile referenced in the assignment exists: `kubectl get profiles -A`.

**Q: Connection timeouts?**
A: Check service endpoints: `kubectl get endpoints shaper-api`. Verify no blocking NetworkPolicies.

**Q: High memory usage?**
A: The API caches K8s objects. Increase limits if needed:
```bash
helm upgrade shaper-api ./charts/shaper-api \
  --set resources.limits.memory=1Gi
```

## Uninstall

```bash
helm uninstall shaper-api
helm uninstall shaper-crds  # Warning: deletes all Profiles/Assignments
```

## Links

- [Webhook Deployment](./webhook-deployment.md) - admission webhooks setup
- [Architecture](../ARCHITECTURE.md) - system design
- [Helm Values](../charts/shaper-api/values.yaml) - full configuration reference
