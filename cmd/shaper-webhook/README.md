# shaper-webhook

This directory contains the source code for the `shaper-webhook` binary.

## Purpose

The `shaper-webhook` is a Kubernetes admission webhook server that validates and mutates Profile and Assignment CRDs. It:

- Validates that each Profile content specifies exactly one source (inline, objectRef, or webhook).
- Validates Assignment UUID formats, buildarch values, and default assignment rules.
- Mutates Assignments to add UUID and buildarch labels.
- Mutates Profiles to add UUID labels for exposed content.

## See Also

- [Main README](../../README.md)
- [Design](../../DESIGN.md)
