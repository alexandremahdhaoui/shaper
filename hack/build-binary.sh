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
    GO_BUILD_LDFLAGS    go linker flags.
EOF
  exit 1
}

trap __usage EXIT

BINARY_NAME="${1}"

export CGO_ENABLED=0

go build \
  -ldflags "${GO_BUILD_LDFLAGS}" \
  -o "build/bin/${BINARY_NAME}" \
  "./cmd/${BINARY_NAME}"

trap 'echo "✅ Binary \"${BINARY_NAME}\" built successfully"' EXIT
