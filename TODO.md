# TODOs

- [x] [0001]: Add a true e2e tests that spawns up a VM that must boot using ipxer
  - Multiple test cases
    1. Boot using default assignment for a build-arch
    1. Boot using specific assignment using machine UUID
  - Study ~/workspaces/testenv-vm/docs/runtime-vm-creation.md for the documentation on how to create vms in testenv-vm using the client during test runtime

- [ ] [0002]: e2e tests more coverage:
  1. Multiple concurrent VMs - Only single VM per test
  1. UEFI boot - Only BIOS tested (undionly.kpxe)
  1. arm32/arm64 architectures - Only i386 tested
