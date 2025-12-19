#!/usr/bin/env bash
# Copyright 2024 Alexandre Mahdhaoui
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# build-ipxe.sh - Build custom iPXE binary with embedded Shaper boot script
#
# This script clones the iPXE repository and builds undionly.kpxe with an
# embedded script that chainloads to shaper.local.
#
# Usage: ./hack/build-ipxe.sh [output-dir]
#
# Dependencies:
#   - git
#   - make
#   - gcc (or cross-compiler for target architecture)
#   - binutils
#   - perl
#   - mtools (for ISO images)
#
# On Ubuntu/Debian: apt install git make gcc binutils perl mtools liblzma-dev

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
EMBED_SCRIPT="${PROJECT_ROOT}/build/ipxe/embed.ipxe"
OUTPUT_DIR="${1:-${PROJECT_ROOT}/build/ipxe}"
IPXE_REPO="https://github.com/ipxe/ipxe.git"
IPXE_TAG="v1.21.1"
BUILD_DIR="${PROJECT_ROOT}/.tmp/ipxe-build"

echo "Building custom iPXE with embedded Shaper script..."
echo "  Embed script: ${EMBED_SCRIPT}"
echo "  Output dir:   ${OUTPUT_DIR}"
echo "  iPXE version: ${IPXE_TAG}"

# Verify embed script exists
if [[ ! -f "${EMBED_SCRIPT}" ]]; then
    echo "ERROR: Embed script not found: ${EMBED_SCRIPT}"
    exit 1
fi

# Clone iPXE if not already present
if [[ ! -d "${BUILD_DIR}" ]]; then
    echo "Cloning iPXE repository..."
    mkdir -p "$(dirname "${BUILD_DIR}")"
    git clone --depth 1 --branch "${IPXE_TAG}" "${IPXE_REPO}" "${BUILD_DIR}"
else
    echo "Using existing iPXE source at ${BUILD_DIR}"
fi

# Copy embed script to iPXE source
cp "${EMBED_SCRIPT}" "${BUILD_DIR}/src/embed.ipxe"

# Build iPXE with embedded script
echo "Building undionly.kpxe..."
cd "${BUILD_DIR}/src"
make clean >/dev/null 2>&1 || true
make bin/undionly.kpxe EMBED=embed.ipxe NO_WERROR=1

# Copy output
mkdir -p "${OUTPUT_DIR}"
cp "${BUILD_DIR}/src/bin/undionly.kpxe" "${OUTPUT_DIR}/undionly.kpxe"

echo ""
echo "Build complete!"
echo "  Output: ${OUTPUT_DIR}/undionly.kpxe"
echo ""
echo "To use this iPXE binary:"
echo "  1. Place it on your TFTP server"
echo "  2. Configure DHCP to serve it as the boot file"
echo "  3. Ensure 'shaper.local' resolves to your shaper-api server"
