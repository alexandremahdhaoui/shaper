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
set -o pipefail

function __usage() {
  cat <<EOF
${0} [CMD]

Available commands:
  setup       This command will set up the e2e test environment.
  run         This command will run the e2e tests.
  teardown    This command will tear down the e2e test environment.

  full-test   This command will set up the environment, run the 
              end-to-end tests, and tear the environment down.
EOF
}

BRIDGE_IFACE=e2e-br0
VETH_BRIDGE=e2e-veth0br0
VETH_CLIENT=e2e-veth0client

WDIR="$(git rev-parse --show-toplevel)"
ASSETS_DIR="${WDIR}/test/e2e/assets"
TEMPDIR="${WDIR}/.tmp/e2e"

DNSMASQ_PID_FILE="${TEMPDIR}/dnsmasq.pid"
DNSMASQ_LOG="${TEMPDIR}/dnsmasq.log"
DNSMASQ_PROCESS_LOG="${TEMPDIR}/dnsmasq.process.log"
DNSMASQ_TFTP_DIR="${TEMPDIR}/tftpboot"
DNSMASQ_CONF_FILE="${TEMPDIR}/dnsmasq.conf"

export BRIDGE_IFACE DNSMASQ_LOG DNSMASQ_TFTP_DIR DNSMASQ_PID_FILE

KIND_CLUSTER_NAME="shaper-e2e"
KUBECONFIG="${TEMPDIR}/${KIND_CLUSTER_NAME}.kubeconfig.yaml"
export KUBECONFIG

HELM_CHART_PATH="${WDIR}/charts/shaper"
HELM_CHART_RELEASE_NAME="shaper-e2e"
HELM_CHART_VALUES_FILE="${ASSETS_DIR}/helm.values.yaml"

function __setup() {
  echo "⏳ Setting up e2e environment..."

  # -- Create e2e temp directory.
  mkdir -p "${DNSMASQ_TFTP_DIR}"

  # -- Generate dnsmasq config
  envsubst <"${ASSETS_DIR}/dnsmasq.conf.tmpl" | tee "${DNSMASQ_CONF_FILE}" 1>/dev/null

  # -- Create bridge interface.
  sudo ip l add dev "${BRIDGE_IFACE}" type bridge
  sudo ip a add 172.16.0.1/24 brd + dev "${BRIDGE_IFACE}"
  sudo ip l set dev "${BRIDGE_IFACE}" up

  # -- Create a veth
  sudo ip link add "${VETH_BRIDGE}" type veth peer name "${VETH_CLIENT}"
  sudo ip l set "${VETH_BRIDGE}" master "${BRIDGE_IFACE}"
  sudo ip l set dev "${VETH_BRIDGE}" up
  sudo ip l set dev "${VETH_CLIENT}" up

  # -- Run dnsmasq.
  echo "⏳ Starting dhcp server..."
  touch "${DNSMASQ_LOG}"
  sudo dnsmasq -d --conf-file="${DNSMASQ_CONF_FILE}" &>"${DNSMASQ_PROCESS_LOG}" &
  echo -n $! | tee "${DNSMASQ_PID_FILE}" 1>/dev/null

  # -- Create a KIND Cluster.
  echo "⏳ Starting kind cluster..."
  sudo kind create cluster --name "${KIND_CLUSTER_NAME}" --kubeconfig "${KUBECONFIG}"
  sudo chown "${USER}" "${KUBECONFIG}"

  # -- Install helm chart in kind cluster.
  helm install "${HELM_CHART_RELEASE_NAME}" "${HELM_CHART_PATH}" --values "${HELM_CHART_VALUES_FILE}"

  # -- Create CRs.
  kubectl apply -f "${ASSETS_DIR}/crs.yaml"

  echo "✅ Successfully set up e2e environment!"
}

function __run() {
  echo "TODO: run command"
  sudo dhclient -v "${VETH_CLIENT}"
}

function __teardown() {
  set +o errexit

  echo "⏳ Tearing down e2e environment..."

  echo "⏳ Deleting kind cluster..."
  sudo kind delete cluster --name "${KIND_CLUSTER_NAME}"

  echo "⏳ Terminating dhcp server..."
  sudo kill -9 "$(cat "${DNSMASQ_PID_FILE}")"
  rm "${DNSMASQ_PID_FILE}"

  echo "⏳ Deleting network interfaces \"${BRIDGE_IFACE}\"..."
  sudo ip l del "${VETH_CLIENT}"
  sudo ip l del dev "${BRIDGE_IFACE}"

  echo "✅ Successfully deleted e2e environment!"
  set -o errexit
}

trap __usage EXIT
CMD="${1}"
trap : EXIT

function main() {
  case "${CMD}" in
  setup)
    __setup
    exit 0
    ;;

  run)
    trap __teardown EXIT
    __run
    trap : EXIT
    exit 0
    ;;

  teardown)
    __teardown
    exit 0
    ;;

  full-test)
    trap __teardown EXIT
    __setup
    __run

    trap : EXIT
    __teardown
    ;;

  *)
    __usage
    exit 1
    ;;
  esac
}

main
