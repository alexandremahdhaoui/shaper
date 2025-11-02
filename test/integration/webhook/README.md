# Webhook Integration Tests

This directory contains integration tests for the shaper-webhook admission webhooks.

## Overview

These tests verify that the Assignment and Profile webhooks correctly validate and mutate CRDs according to the business rules implemented in `internal/driver/webhook/`.

## Prerequisites

1. **Kubernetes cluster**: A kind cluster (or similar) with:
   - CRDs installed
   - cert-manager installed
   - shaper-webhooks deployed

2. **Environment setup**:
   ```bash
   # Set up test environment
   make test-setup
   export KUBECONFIG=$(kind get kubeconfig-path --name shaper-test)

   # Install cert-manager
   kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.0/cert-manager.yaml
   kubectl wait --for=condition=Available --timeout=300s -n cert-manager deployment/cert-manager-webhook

   # Apply CRDs
   kubectl apply -f charts/shaper-crds/templates/crds/

   # Deploy webhook
   helm install shaper-webhooks charts/shaper-webhooks
   kubectl wait --for=condition=Available --timeout=300s deployment/shaper-webhooks
   ```

## Running Tests

### Run all webhook integration tests
```bash
make test-webhook-integration
```

### Run specific test
```bash
go test -v ./test/integration/webhook/... -run TestAssignmentValidation
```

### Run tests without cluster
Tests will skip gracefully if `KUBECONFIG` is not set:
```bash
go test ./test/integration/webhook/...
# Output: SKIP (KUBECONFIG not set)
```

## Test Structure

- `webhook_test.go`: Main test file with all integration tests
- `fixtures/`: YAML fixtures for testing validation/mutation

### Test Coverage

**Assignment Webhook:**
- ✅ Valid assignments are accepted
- ✅ Invalid UUID format is rejected
- ✅ Invalid buildarch is rejected
- ✅ Default assignments with UUID selectors are rejected
- ✅ Labels are added correctly (mutation)

**Profile Webhook:**
- ✅ Valid profiles are accepted
- ✅ Multiple content sources are rejected
- ✅ Invalid JSONPath expressions are rejected
- ✅ UUID labels are added for exposed content (mutation)

## Fixtures

See `fixtures/` directory for example valid/invalid CRDs used in tests.

## Troubleshooting

### Webhook not ready
```bash
kubectl get pods -l app.kubernetes.io/name=shaper-webhooks
kubectl logs -l app.kubernetes.io/name=shaper-webhooks
```

### Certificate issues
```bash
kubectl get certificate
kubectl describe certificate shaper-webhooks-cert
```

### Webhook configuration
```bash
kubectl get validatingwebhookconfigurations
kubectl get mutatingwebhookconfigurations
```
