# shaper-controller

This directory contains the source code for the `shaper-controller` binary.

## Purpose

The `shaper-controller` is a Kubernetes controller that reconciles Profile and Assignment CRDs. It:

- Generates stable UUIDs for exposed additional content in Profiles.
- Adds subject selector labels to Assignments for efficient K8s queries.
- Updates CRD status subresources after reconciliation.

## See Also

- [Main README](../../README.md)
- [Design](../../DESIGN.md)
