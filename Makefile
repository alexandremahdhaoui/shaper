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

GO_BUILD_LDFLAGS ?= "-X main.BuildTimestamp=$(TIMESTAMP) -X main.CommitSHA=$(COMMIT_SHA) -X main.Version=$(VERSION)"

# ------------------------------------------------------- VERSIONS --------------------------------------------------- #

# renovate: datasource=github-release depName=kubernetes-sigs/controller-tools
CONTROLLER_GEN_VERSION := v0.14.0
# renovate: datasource=github-release depName=mvdan/gofumpt
GOFUMPT_VERSION        := v0.6.0
# renovate: datasource=github-release depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION  := v1.63.4
# renovate: datasource=github-release depName=gotestyourself/gotestsum
GOTESTSUM_VERSION      := v1.12.0
# renovate: datasource=github-release depName=vektra/mockery
MOCKERY_VERSION        := v2.42.0
# renovate: datasource=github-release depName=oapi-codegen/oapi-codegen
OAPI_CODEGEN_VERSION   := v2.3.0
# renovate: datasource=github-release depName=alexandremahdhaoui/tooling
TOOLING_VERSION        := v0.1.4
# renovate: datasource=github-release depName=mikefarah/yq
YQ_VERSION             := v4.44.5

# ------------------------------------------------------- TOOLS ------------------------------------------------------ #

CONTAINER_ENGINE   ?= docker
KIND_BINARY        ?= kind
KIND_BINARY_PREFIX ?= sudo

KINDENV_ENVS := KIND_BINARY_PREFIX="$(KIND_BINARY_PREFIX)" KIND_BINARY="$(KIND_BINARY)"

TOOLING := go run github.com/alexandremahdhaoui/tooling/cmd

YQ                  := go run github.com/mikefarah/yq/v4@$(YQ_VERSION)
CONTROLLER_GEN      := go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
KINDENV				      := $(KINDENV_ENVS) $(TOOLING)/kindenv@$(TOOLING_VERSION)
GO_GEN              := go generate
GOFUMPT             := go run mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
GOLANGCI_LINT       := go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOTESTSUM           := go run gotest.tools/gotestsum@$(GOTESTSUM_VERSION) --format pkgname
LOCAL_CONTAINER_REG := $(TOOLING)/local-container-registry@$(TOOLING_VERSION)
MOCKERY             := go run github.com/vektra/mockery/v2@$(MOCKERY_VERSION)
OAPI_CODEGEN        := go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
OAPI_CODEGEN_HELPER := OAPI_CODEGEN="$(OAPI_CODEGEN)" $(TOOLING)/oapi-codegen-helper@$(TOOLING_VERSION)

CLEAN_MOCKS := rm -rf ./internal/util/mocks

KUBECONFIG := $(shell $(YQ) '.kindenv.kubeconfigPath' .project.yaml)

.PHONY: modules
modules: ## Run go mod tidy
	go mod tidy

# ------------------------------------------------------- GENERATE --------------------------------------------------- #

.PHONY: generate
generate: ## Generate REST API server/client code, CRDs and other go generators.
	$(OAPI_CODEGEN_HELPER)
	$(GO_GEN) "./..."

	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	$(CONTROLLER_GEN) paths="./..." \
		crd:generateEmbeddedObjectMeta=true \
		output:crd:artifacts:config=charts/$(PROJECT)/templates/crds

	$(CONTROLLER_GEN) paths="./..." \
		rbac:roleName=$(PROJECT) \
		webhook \
		output:rbac:dir=charts/$(PROJECT)/templates/rbac \
		output:webhook:dir=charts/$(PROJECT)/templates/webhook

	$(CLEAN_MOCKS)
	$(MOCKERY)

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
	CONTAINER_ENGINE=$(CONTAINER_ENGINE) GO_BUILD_LDFLAGS=$(GO_BUILD_LDFLAGS) VERSION=$(VERSION) \
		./hack/build-container.sh "${CONTAINER_NAME}"

.PHONY: build-containers
build-containers: generate
	echo $(CONTAINERS) | \
		CONTAINER_ENGINE=$(CONTAINER_ENGINE) \
		GO_BUILD_LDFLAGS=$(GO_BUILD_LDFLAGS) \
		VERSION=$(VERSION) \
		xargs -n1 ./hack/build-container.sh

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

.PHONY: test-functional
test-functional:
	GOTESTSUM="$(GOTESTSUM)" TEST_TAG=functional ./hack/test-go.sh

.PHONY: test-e2e
test-e2e:
	./test/e2e/main.sh full-test

.PHONY: test-setup
test-setup:
	$(KINDENV) setup
	@echo "Applying crds..."
	KUBECONFIG=$(KUBECONFIG) kubectl apply -f ./charts/shaper/templates/crds/
	@echo "\nPlease run the following command to set up your kubeconfig:\n    export KUBECONFIG=$(KUBECONFIG)\n"

.PHONY: test-teardown
test-teardown:
	$(KINDENV) teardown

.PHONY: test
test: test-unit test-setup test-integration test-functional test-teardown

# ------------------------------------------------------- PRE-PUSH --------------------------------------------------- #

.PHONY: githooks
githooks: ## Set up git hooks to run before a push.
	git config core.hooksPath .githooks

.PHONY: pre-push
pre-push: generate fmt lint test
	git status --porcelain
