# TFTP Boot Files for E2E Tests

This directory contains boot files for PXE boot testing.

## Required Files

- `undionly.kpxe` - iPXE boot file for BIOS systems

## How to Obtain

Download from iPXE:
```bash
curl -o undionly.kpxe http://boot.ipxe.org/undionly.kpxe
```

Or build from source:
```bash
git clone git://git.ipxe.org/ipxe.git
cd ipxe/src
make bin/undionly.kpxe
cp bin/undionly.kpxe /path/to/this/directory/
```

## Note

These files are not committed to git due to their binary nature.
The E2E test will skip if required files are missing.
