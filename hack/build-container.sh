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



set -o errexit
set -o nounset

__usage() {
  cat <<EOF
USAGE:

${0} [BINARY_NAME]

Required environment variables:
    CONTAINER_ENGINE    Container engine such as podman or docker.
    GO_BUILD_LDFLAGS    Go linker flags.
    VERSION             Semver tag.
EOF
  exit 1
}

trap __usage EXIT

BINARY_NAME="${1}"

"${CONTAINER_ENGINE}" \
  build \
  . \
  --build-arg "GO_BUILD_LDFLAGS=${GO_BUILD_LDFLAGS}" \
  -t "${BINARY_NAME}:${VERSION}" \
  -f "./containers/${BINARY_NAME}/Containerfile"

trap 'echo "✅ Container image \"${BINARY_NAME}\" built successfully"' EXIT
