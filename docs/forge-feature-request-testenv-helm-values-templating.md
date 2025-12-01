# Forge Feature Request: Testenv Environment Variable Injection in Helm Values

## Summary

Allow testenv-helm-install to reference environment variables from previous testenv sub-engines (like testenv-lcr) using templating syntax in helm chart values.

## Problem Statement

When using `testenv-lcr` with dynamic port allocation, the registry host and port are determined at runtime. However, `testenv-helm-install` requires hardcoded values in `forge.yaml`:

```yaml
# Current: Hardcoded port that doesn't match dynamic allocation
values:
  image:
    repository: testenv-lcr.testenv-lcr.svc.cluster.local:5000/shaper-controller
```

But `testenv-lcr` actually allocates a dynamic NodePort (e.g., 31906) and pushes images to:
```
testenv-lcr.testenv-lcr.svc.cluster.local:31906/shaper-controller:latest
```

This mismatch causes `ImagePullBackOff` errors because the helm chart references the wrong port.

## Proposed Solution

### Option 1: Template Syntax in Helm Values

Allow templating syntax to reference testenv environment variables:

```yaml
- engine: go://testenv-helm-install
  spec:
    charts:
      - name: shaper-controller
        values:
          image:
            repository: "{{ .Env.TESTENV_LCR_HOST }}/shaper-controller"
            tag: latest
```

Where `TESTENV_LCR_HOST` is set by the `testenv-lcr` engine to the full registry address (e.g., `testenv-lcr.testenv-lcr.svc.cluster.local:31906`).

### Option 2: Automatic Image Reference Replacement

Provide a special placeholder that gets automatically replaced:

```yaml
values:
  image:
    repository: "{{ testenv-lcr }}/shaper-controller"
```

The forge orchestrator would replace `{{ testenv-lcr }}` with the actual registry host from the testenv-lcr engine output.

### Option 3: ValueReferences with Testenv Sources

Extend the existing `valueReferences` mechanism to support testenv outputs:

```yaml
- engine: go://testenv-helm-install
  spec:
    charts:
      - name: shaper-controller
        valueReferences:
          - source: testenv://testenv-lcr
            key: registryHost
            path: image.repository
            template: "{{ .Value }}/shaper-controller"
```

## Environment Variables to Expose

The `testenv-lcr` engine should expose these environment variables to subsequent engines:

| Variable | Description | Example Value |
|----------|-------------|---------------|
| `TESTENV_LCR_HOST` | Full registry host with port | `testenv-lcr.testenv-lcr.svc.cluster.local:31906` |
| `TESTENV_LCR_HOSTNAME` | Registry hostname only | `testenv-lcr.testenv-lcr.svc.cluster.local` |
| `TESTENV_LCR_PORT` | Registry port | `31906` |
| `TESTENV_LCR_NAMESPACE` | Kubernetes namespace | `testenv-lcr` |

## Implementation Notes

1. **Testenv Engine Output**: Each testenv sub-engine should be able to export key-value pairs that become available to subsequent engines
2. **Templating Engine**: Use Go's `text/template` for consistency with other parts of forge
3. **Order Dependency**: The testenv-helm-install engine runs after testenv-lcr, so the values are available
4. **Error Handling**: Fail fast if a referenced variable is not set

## Example Full Configuration

```yaml
test:
  - name: integration
    testenv:
      - engine: go://testenv-kind
      - engine: go://testenv-lcr
        spec:
          enabled: true
          namespace: testenv-lcr
          # Exports: TESTENV_LCR_HOST, TESTENV_LCR_PORT, etc.
      - engine: go://testenv-helm-install
        spec:
          charts:
            - name: shaper-controller
              path: ./charts/shaper-controller
              namespace: shaper-system
              values:
                image:
                  repository: "{{ .Env.TESTENV_LCR_HOST }}/shaper-controller"
                  tag: latest
                imagePullSecrets:
                  - name: testenv-lcr-credentials
```

## Current Workaround

None available - the port mismatch causes integration tests to fail with `ImagePullBackOff`.

## Impact

- **Severity**: Blocker for integration tests using testenv-lcr with helm-install
- **Affected Components**: testenv-lcr, testenv-helm-install
- **Related Issues**: Port mismatch between image push and helm chart values

## References

- Forge testenv documentation
- Helm values templating patterns
- Go text/template package
