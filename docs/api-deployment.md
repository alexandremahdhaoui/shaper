# Shaper API Deployment Guide

This guide explains how to deploy the shaper-api HTTP server to your Kubernetes cluster.

## Prerequisites

### Required Software

1. **Kubernetes Cluster**: v1.28+ for API compatibility
2. **Helm**: v3.0+ for chart installation
3. **kubectl**: Configured to access your cluster

### Required CRDs

Shaper CRDs must be installed before deploying the API:

```bash
# Install Shaper CRDs
helm install shaper-crds ./charts/shaper-crds

# Verify CRDs are installed
kubectl get crds | grep shaper.amahdha.com
# Should show:
# assignments.shaper.amahdha.com
# profiles.shaper.amahdha.com
```

## Deploying shaper-api

### Using Helm (Recommended)

```bash
# Install with default values
helm install shaper-api ./charts/shaper-api

# Install with custom values
helm install shaper-api ./charts/shaper-api \
  --set config.assignmentNamespace=shaper \
  --set config.profileNamespace=shaper \
  --set config.apiServer.port=8080
```

### Configuration Options

Key configuration values (see `charts/shaper-api/values.yaml` for full list):

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.assignmentNamespace` | Namespace for Assignment CRDs | `default` |
| `config.profileNamespace` | Namespace for Profile CRDs | `default` |
| `config.kubeconfigPath` | Kubeconfig path (use `>>> Kubeconfig From Service Account` for in-cluster) | `>>> Kubeconfig From Service Account` |
| `config.apiServer.port` | API server HTTP port | `30443` |
| `config.probesServer.port` | Health probes port | `8081` |
| `config.probesServer.livenessPath` | Liveness probe path | `/healthz` |
| `config.probesServer.readinessPath` | Readiness probe path | `/readyz` |
| `config.metricsServer.port` | Prometheus metrics port | `8080` |
| `config.metricsServer.path` | Metrics endpoint path | `/metrics` |
| `replicaCount` | Number of API server replicas | `1` |
| `service.type` | Kubernetes service type | `ClusterIP` |
| `service.port` | Service port | `80` |
| `ingress.enabled` | Enable Ingress for external access | `false` |
| `httpRoute.enabled` | Enable Gateway API HttpRoute | `false` |
| `metrics.serviceMonitor.enabled` | Enable Prometheus ServiceMonitor | `false` |

### Verification

```bash
# Check API pod is running
kubectl get pods -l app.kubernetes.io/name=shaper-api

# Check service
kubectl get svc shaper-api

# Test health endpoints
kubectl port-forward svc/shaper-api 8081:8081
curl http://localhost:8081/healthz  # Should return 200 OK
curl http://localhost:8081/readyz   # Should return 200 OK

# Check metrics
kubectl port-forward svc/shaper-api 8080:8080
curl http://localhost:8080/metrics
```

## Testing the API

### Create Test CRDs

First, create a Profile and Assignment:

```bash
# Create a simple Profile
cat <<EOF | kubectl apply -f -
apiVersion: shaper.amahdha.com/v1alpha1
kind: Profile
metadata:
  name: test-profile
  namespace: default
spec:
  ipxeTemplate: |
    #!ipxe
    echo Booting test profile
    echo UUID: \${uuid}
    echo Buildarch: \${buildarch}
    kernel http://boot.example.com/vmlinuz
    initrd http://boot.example.com/initrd.img
    boot
  additionalContent: []
EOF

# Create an Assignment
cat <<EOF | kubectl apply -f -
apiVersion: shaper.amahdha.com/v1alpha1
kind: Assignment
metadata:
  name: test-assignment
  namespace: default
spec:
  subjectSelectors:
    buildarch:
      - x86_64
    uuidList:
      - 12345678-1234-1234-1234-123456789abc
  profileName: test-profile
  isDefault: false
EOF
```

### Test API Endpoints

```bash
# Port-forward API server
kubectl port-forward svc/shaper-api 30443:30443

# Test bootstrap endpoint
curl http://localhost:30443/boot.ipxe
# Should return iPXE script that chainloads to /ipxe

# Test iPXE endpoint with UUID and buildarch
curl "http://localhost:30443/ipxe?uuid=12345678-1234-1234-1234-123456789abc&buildarch=x86_64"
# Should return the Profile's iPXE template rendered
```

## Exposing the API Externally

### Using Ingress (HTTP)

For development/testing with HTTP:

```bash
helm upgrade shaper-api ./charts/shaper-api \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=shaper.example.com \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.hosts[0].paths[0].pathType=Prefix
```

### Using Gateway API HttpRoute

For modern Kubernetes with Gateway API:

```bash
helm upgrade shaper-api ./charts/shaper-api \
  --set httpRoute.enabled=true \
  --set httpRoute.hostnames[0]=shaper.example.com \
  --set httpRoute.parentRefs[0].name=my-gateway \
  --set httpRoute.parentRefs[0].namespace=default
```

### Using LoadBalancer Service

For bare-metal or cloud environments:

```bash
helm upgrade shaper-api ./charts/shaper-api \
  --set service.type=LoadBalancer \
  --set service.port=80
```

### Using NodePort Service

For direct node access:

```bash
helm upgrade shaper-api ./charts/shaper-api \
  --set service.type=NodePort \
  --set service.port=30080
```

## Monitoring and Observability

### Prometheus Metrics

The shaper-api exposes Prometheus metrics at `/metrics`:

```bash
# Enable ServiceMonitor for Prometheus Operator
helm upgrade shaper-api ./charts/shaper-api \
  --set metrics.enabled=true \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.serviceMonitor.labels.prometheus=kube-prometheus \
  --set metrics.serviceMonitor.interval=30s
```

### Logging

The API server uses structured logging (slog). View logs:

```bash
# Follow logs
kubectl logs -f -l app.kubernetes.io/name=shaper-api

# View recent logs
kubectl logs -l app.kubernetes.io/name=shaper-api --tail=100
```

## Advanced Configuration

### Custom Namespaces

If Assignments and Profiles are in different namespaces:

```bash
helm install shaper-api ./charts/shaper-api \
  --set config.assignmentNamespace=assignments \
  --set config.profileNamespace=profiles
```

**Note:** The API ServiceAccount needs RBAC permissions to read resources in both namespaces.

### High Availability

For production deployments, increase replica count and configure autoscaling:

```bash
helm install shaper-api ./charts/shaper-api \
  --set replicaCount=3 \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=3 \
  --set autoscaling.maxReplicas=10 \
  --set autoscaling.targetCPUUtilizationPercentage=80
```

### Resource Limits

Adjust resource limits based on your workload:

```bash
helm install shaper-api ./charts/shaper-api \
  --set resources.limits.memory=512Mi \
  --set resources.limits.cpu=500m \
  --set resources.requests.memory=256Mi \
  --set resources.requests.cpu=250m
```

### Using External Kubeconfig

For running outside the cluster (development):

```bash
# Create ConfigMap with kubeconfig
kubectl create configmap shaper-kubeconfig \
  --from-file=kubeconfig=/path/to/kubeconfig

# Mount ConfigMap as volume
helm install shaper-api ./charts/shaper-api \
  --set config.kubeconfigPath=/etc/kubeconfig/kubeconfig \
  --set volumes[0].name=kubeconfig \
  --set volumes[0].configMap.name=shaper-kubeconfig \
  --set volumeMounts[0].name=kubeconfig \
  --set volumeMounts[0].mountPath=/etc/kubeconfig \
  --set volumeMounts[0].readOnly=true
```

## Running Locally (Development)

For local development without Kubernetes deployment:

### 1. Set Up Test Environment

```bash
# Source environment variables
. .envrc.example

# Create test Kubernetes cluster with Forge
forge test integration create

# Export kubeconfig (path shown in create output)
export KUBECONFIG=/path/to/test-kubeconfig

# Apply CRDs
kubectl apply -f ./charts/shaper-crds/templates/crds/
```

### 2. Create Configuration File

Create a JSON configuration file (e.g., `config.json`):

```json
{
  "assignmentNamespace": "default",
  "profileNamespace": "default",
  "kubeconfigPath": "/path/to/kubeconfig",
  "probesServer": {
    "livenessPath": "/healthz",
    "readinessPath": "/readyz",
    "port": 8081
  },
  "metricsServer": {
    "path": "/metrics",
    "port": 8080
  },
  "apiServer": {
    "port": 8080
  }
}
```

### 3. Run the Binary

```bash
# Set config path
export IPXER_CONFIG_PATH=./config.json

# Run from source
go run ./cmd/shaper-api

# Or build and run binary
forge build shaper-api-binary
./build/bin/shaper-api
```

### 4. Test Endpoints

```bash
# Bootstrap
curl http://localhost:8080/boot.ipxe

# iPXE with selectors
curl "http://localhost:8080/ipxe?uuid=test-uuid&buildarch=x86_64"

# Health checks
curl http://localhost:8081/healthz
curl http://localhost:8081/readyz

# Metrics
curl http://localhost:8080/metrics
```

## Troubleshooting

### API Not Ready

```bash
# Check API logs
kubectl logs -l app.kubernetes.io/name=shaper-api

# Common issues:
# - CRDs not installed: Install shaper-crds chart first
# - RBAC permissions: Check ServiceAccount has read access to Profiles/Assignments
# - Kubeconfig issues: Verify config.kubeconfigPath is correct
```

### Assignment Not Found Errors

```bash
# Check if Assignments exist
kubectl get assignments -A

# Check namespace configuration
kubectl describe deployment shaper-api | grep -A 10 "Environment"

# Verify RBAC permissions
kubectl auth can-i list assignments --as=system:serviceaccount:default:shaper-api
```

### Profile Not Found Errors

```bash
# Check if Profiles exist
kubectl get profiles -A

# Verify Profile referenced in Assignment exists
kubectl get assignment <assignment-name> -o jsonpath='{.spec.profileName}'
kubectl get profile <profile-name>
```

### Connection Timeouts

```bash
# Check service endpoints
kubectl get endpoints shaper-api

# Check network policies
kubectl get networkpolicies

# Test pod connectivity
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl http://shaper-api.default.svc.cluster.local/boot.ipxe
```

### High Memory Usage

The API may consume memory caching templates and Kubernetes objects:

```bash
# Check memory usage
kubectl top pod -l app.kubernetes.io/name=shaper-api

# Increase memory limits if needed
helm upgrade shaper-api ./charts/shaper-api \
  --set resources.limits.memory=1Gi
```

## Uninstalling

```bash
# Remove API helm release
helm uninstall shaper-api

# Optionally remove CRDs (warning: deletes all Profiles/Assignments)
helm uninstall shaper-crds
```

## Security Considerations

### RBAC Permissions

The shaper-api requires the following Kubernetes permissions:

- **Assignments**: `list`, `get`, `watch`
- **Profiles**: `list`, `get`, `watch`
- **ConfigMaps**: `get` (for ObjectRef resolvers)
- **Secrets**: `get` (for ObjectRef resolvers)

These are automatically configured by the Helm chart's ServiceAccount and RoleBinding.

### Network Policies

In production, consider restricting network access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: shaper-api-policy
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: shaper-api
  policyTypes:
    - Ingress
  ingress:
    # Allow from ingress controller
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - protocol: TCP
          port: 30443
```

### Authentication

For production deployments, consider:

1. **mTLS**: Use cert-manager to issue client certificates for iPXE clients
2. **API Gateway**: Place behind an API gateway with authentication
3. **Network Segmentation**: Deploy in isolated network with firewall rules

## Performance Tuning

### Caching

The API currently makes Kubernetes API calls for every request. For high-traffic deployments, consider:

1. Implementing informer-based caching (future enhancement)
2. Using a reverse proxy cache (e.g., Varnish, nginx)
3. Increasing replica count for horizontal scaling

### Resource Allocation

For production workloads serving 100+ requests/second:

```yaml
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 2000m
    memory: 2Gi
```

### Horizontal Pod Autoscaling

```yaml
autoscaling:
  enabled: true
  minReplicas: 5
  maxReplicas: 20
  targetCPUUtilizationPercentage: 70
```

## See Also

- [Webhook Deployment Guide](./webhook-deployment.md)
- [Configuration Example](../examples/shaper-api.example.yaml) (if exists)
- [Helm Chart Values](../charts/shaper-api/values.yaml)
- [API OpenAPI Specification](../api/shaper.v1.yaml)
- [Architecture Documentation](../ARCHITECTURE.md)
