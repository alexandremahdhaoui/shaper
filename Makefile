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

# ------------------------------------------------------- ENVS ------------------------------------------------------- #

PROJECT    := shaper

COMMIT_SHA := $(shell git rev-parse --short HEAD)
TIMESTAMP  := $(shell date --utc --iso-8601=seconds)
VERSION    ?= $(shell git describe --tags --always --dirty)

CHARTS     := $(shell ./hack/list-subprojects.sh charts)
CONTAINERS := $(shell ./hack/list-subprojects.sh containers)
CMDS       := $(shell ./hack/list-subprojects.sh cmd)

GO_BUILD_LDFLAGS ?= -X main.BuildTimestamp=$(TIMESTAMP) -X main.CommitSHA=$(COMMIT_SHA) -X main.Version=$(VERSION)

# ------------------------------------------------------- VERSIONS --------------------------------------------------- #

# renovate: datasource=github-release depName=kubernetes-sigs/controller-tools
CONTROLLER_GEN_VERSION := v0.19.0
# renovate: datasource=github-release depName=mvdan/gofumpt
GOFUMPT_VERSION        := v0.6.0
# renovate: datasource=github-release depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION  := v1.63.4
# renovate: datasource=github-release depName=gotestyourself/gotestsum
GOTESTSUM_VERSION      := v1.13.0
# renovate: datasource=github-release depName=vektra/mockery
MOCKERY_VERSION        := v3.5.5
# renovate: datasource=github-release depName=oapi-codegen/oapi-codegen
OAPI_CODEGEN_VERSION   := v2.5.0
# renovate: datasource=github-release depName=alexandremahdhaoui/forge
FORGE_VERSION        := v0.3.1
# renovate: datasource=github-release depName=mikefarah/yq
YQ_VERSION             := v4.44.5

# ------------------------------------------------------- TOOLS ------------------------------------------------------ #

CONTAINER_ENGINE   ?= docker
KIND_BINARY        ?= kind
# unsetting KIND_BINARY_PREFIX
KIND_BINARY_PREFIX ?=


FORGE := go run github.com/alexandremahdhaoui/forge/cmd

YQ                  := go run github.com/mikefarah/yq/v4@$(YQ_VERSION)
CONTROLLER_GEN      := go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
GO_GEN              := go generate
GOFUMPT             := go run mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
GOLANGCI_LINT       := go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOTESTSUM           := go run gotest.tools/gotestsum@$(GOTESTSUM_VERSION) --format pkgname-and-test-fails --format-hide-empty-pkg
MOCKERY             := go run github.com/vektra/mockery/v3@$(MOCKERY_VERSION)
OAPI_CODEGEN        := go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
OAPI_CODEGEN_HELPER := OAPI_CODEGEN="$(OAPI_CODEGEN)" $(FORGE)/oapi-codegen-helper@$(FORGE_VERSION)

BUILD_CONTAINER     := CONTAINER_ENGINE="$(CONTAINER_ENGINE)" BUILD_ARGS="GO_BUILD_LDFLAGS=$(GO_BUILD_LDFLAGS)" $(FORGE)/build-container@$(FORGE_VERSION)
KINDENV             := KIND_BINARY_PREFIX="$(KIND_BINARY_PREFIX)" KIND_BINARY="$(KIND_BINARY)" $(FORGE)/kindenv@$(FORGE_VERSION)
LOCAL_CONTAINER_REG := CONTAINER_ENGINE="$(CONTAINER_ENGINE)" PREPEND_CMD="sudo" $(FORGE)/local-container-registry@$(FORGE_VERSION)

CLEAN_MOCKS := rm -rf ./internal/util/mocks

KUBECONFIG := $(abspath $(shell $(YQ) '.kindenv.kubeconfigPath' forge.yaml))

.PHONY: modules
modules: ## Run go mod tidy
	go mod tidy

# ------------------------------------------------------- GENERATE --------------------------------------------------- #

CRDS_DEST_DIR     := charts/$(PROJECT)-crds/templates/crds
WEBHOOK_DEST_DIR := charts/$(PROJECT)/templates/webhook
RBAC_DEST_DIR    := charts/$(PROJECT)/templates/rbac

BOILERPLATE_GO_TXT := ./hack/boilerplate.go.txt

.PHONY: generate-oapi
generate-oapi:
	$(OAPI_CODEGEN_HELPER)
	$(GO_GEN) "./..."

.PHONY: generate-crds
generate-crds: 
	$(CONTROLLER_GEN) object:headerFile=$(BOILERPLATE_GO_TXT) paths="./..."
	$(CONTROLLER_GEN) paths="./..." \
		crd:generateEmbeddedObjectMeta=true \
		output:crd:artifacts:config=$(CRDS_DEST_DIR)

.PHONY: generate-rbac
generate-rbac:
	$(CONTROLLER_GEN) paths="./..." \
		rbac:roleName=$(PROJECT) \
		output:rbac:dir=$(RBAC_DEST_DIR)

.PHONY: generate-webhooks
generate-webhooks:
	$(CONTROLLER_GEN) paths="./..." \
		rbac:roleName=$(PROJECT)-webhook \
		webhook \
		output:rbac:dir=$(WEBHOOK_DEST_DIR) \
		output:webhook:dir=$(WEBHOOK_DEST_DIR)

.PHONY: generate-mocks
generate-mocks:
	$(CLEAN_MOCKS)
	$(MOCKERY)


.PHONY: generate
generate: generate-oapi generate-crds generate-rbac generate-webhooks generate-mocks ## Generate REST API server/client code, CRDs and other go generators.

# ------------------------------------------------------- BUILD BINARIES --------------------------------------------- #

.PHONY: build-binary
build-binary: generate
	GO_BUILD_LDFLAGS=$(GO_BUILD_LDFLAGS) ./hack/build-binary.sh "${BINARY_NAME}"

.PHONY: build-binaries
build-binaries: generate ## Build the binaries.
	echo $(CMDS) | \
		GO_BUILD_LDFLAGS=$(GO_BUILD_LDFLAGS) \
		xargs -n1 ./hack/build-binary.sh

# ------------------------------------------------------- BUILD CONTAINERS -------------------------------------------- #

.PHONY: build-container
build-container: generate
	$(BUILD_CONTAINER)

# ------------------------------------------------------- FMT -------------------------------------------------------- #

.PHONY: fmt
fmt:
	$(GOFUMPT) -w .

# ------------------------------------------------------- LINT ------------------------------------------------------- #

.PHONY: lint
lint:
	$(GOLANGCI_LINT) run --fix

# ------------------------------------------------------- TEST ------------------------------------------------------- #

.PHONY: test-chart
test-chart:
	echo TODO: implement 'make `test-chart`'.

.PHONY: test-unit
test-unit:
	GOTESTSUM="$(GOTESTSUM)" TEST_TAG=unit ./hack/test-go.sh

.PHONY: test-integration
test-integration:
	GOTESTSUM="$(GOTESTSUM)" TEST_TAG=integration ./hack/test-go.sh

.PHONY: test-e2e
test-e2e:
	./test/e2e/main.sh full-test

.PHONY: test-setup
test-setup:
	@echo "======================================"
	@echo "Installing kindenv..."
	@echo "======================================"
	$(KINDENV) setup

	@echo "======================================"
	@echo "Installing local-container-registry..."
	@echo "======================================"
	$(LOCAL_CONTAINER_REG)

	@echo "======================================"
	@echo "Applying crds..."
	@echo "======================================"
	KUBECONFIG=$(KUBECONFIG) kubectl apply -f ./charts/shaper-crds/templates/crds/

	@echo "\nPlease run the following command to set up your kubeconfig:\n    export KUBECONFIG=$(KUBECONFIG)\n"

.PHONY: test-sync
test-sync: build-container
	$(LOCAL_CONTAINER_REG) push-all

.PHONY: test-teardown
test-teardown:
	$(LOCAL_CONTAINER_REG) teardown || true
	$(KINDENV) teardown

.PHONY: test
test: test-unit test-setup test-integration test-teardown test-e2e

# ------------------------------------------------------- PRE-PUSH --------------------------------------------------- #

.PHONY: githooks
githooks: ## Set up git hooks to run before a push.
	git config core.hooksPath .githooks

.PHONY: pre-push
pre-push: generate fmt lint test
	git status --porcelain
