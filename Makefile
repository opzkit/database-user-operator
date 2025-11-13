# Image URL to use all building/pushing image targets
IMG ?= database-user-operator:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.28.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" GOTOOLCHAIN=go1.25.0+auto go test ./... -coverprofile cover.out

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager ./cmd/manager

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/manager

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for cross-platform support
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=linux/arm64,linux/amd64 --tag ${IMG} -f Dockerfile .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Helm

HELM_CHART_DIR ?= helm/database-user-operator
HELM_DIST_DIR ?= dist

.PHONY: helm-lint
helm-lint: ## Lint the helm chart
	helm lint $(HELM_CHART_DIR)

.PHONY: helm-template
helm-template: ## Generate manifests from helm chart
	helm template database-user-operator $(HELM_CHART_DIR)

.PHONY: helm-package
helm-package: manifests helm-crds ## Package the helm chart
	mkdir -p $(HELM_DIST_DIR)
	helm package $(HELM_CHART_DIR) -d $(HELM_DIST_DIR)

.PHONY: helm-crds
helm-crds: manifests ## Copy CRDs to helm chart
	mkdir -p $(HELM_CHART_DIR)/crds
	cp config/crd/bases/*.yaml $(HELM_CHART_DIR)/crds/

.PHONY: helm-install
helm-install: helm-package ## Install the helm chart
	helm install database-user-operator $(HELM_DIST_DIR)/database-user-operator-*.tgz

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall the helm chart
	helm uninstall database-user-operator

##@ Integration Tests

CLUSTER_NAME ?= database-operator-test

.PHONY: integration-tools
integration-tools: kubectl kind helm ## Install tools required for integration tests
	@echo "Checking required tools for integration tests..."
	@command -v docker >/dev/null 2>&1 || { \
		echo "ERROR: docker is not installed."; \
		if command -v brew >/dev/null 2>&1; then \
			echo "Install with: brew install --cask docker"; \
		else \
			echo "Install from: https://docs.docker.com/get-docker/"; \
		fi; \
		exit 1; \
	}
	@docker info >/dev/null 2>&1 || { echo "ERROR: docker daemon is not running. Please start Docker."; exit 1; }
	@command -v go >/dev/null 2>&1 || { \
		echo "ERROR: go is not installed."; \
		if command -v brew >/dev/null 2>&1; then \
			echo "Install with: brew install go"; \
		else \
			echo "Install from: https://golang.org/doc/install"; \
		fi; \
		exit 1; \
	}
	@echo "✓ All integration test tools available"
	@echo "  - kubectl: $(KUBECTL)"
	@echo "  - kind: $(KIND)"
	@echo "  - helm: $(HELM)"

.PHONY: integration-setup
integration-setup: integration-tools ## Setup kind cluster with databases and operator
	@echo "Setting up integration test environment..."
	@chmod +x test/integration/scripts/setup-cluster.sh
	@PATH="$(LOCALBIN):$$PATH" CLUSTER_NAME=$(CLUSTER_NAME) ENABLE_COVERAGE=true test/integration/scripts/setup-cluster.sh

.PHONY: integration-run
integration-run: manifests generate fmt vet ## Run integration tests (requires cluster setup first)
	@echo "Running integration tests..."
	@go test -v -tags=integration ./test/integration/... -timeout 5m -ginkgo.v -ginkgo.progress

.PHONY: integration-teardown
integration-teardown: ## Teardown kind cluster
	@echo "Tearing down integration test environment..."
	@chmod +x test/integration/scripts/teardown-cluster.sh
	@CLUSTER_NAME=$(CLUSTER_NAME) test/integration/scripts/teardown-cluster.sh

.PHONY: integration-test
integration-test: test integration-setup integration-run integration-teardown ## Run complete integration test suite with coverage
	@$(MAKE) --no-print-directory merge-coverage

.PHONY: integration-rebuild
integration-rebuild: kind ## Rebuild and redeploy operator in existing cluster
	@echo "Rebuilding and redeploying operator..."
	@docker build --build-arg ENABLE_COVERAGE=true -t database-user-operator:test .
	@PATH="$(LOCALBIN):$$PATH" kind load docker-image database-user-operator:test --name=$(CLUSTER_NAME)
	@kubectl rollout restart deployment/database-user-operator -n db-system
	@kubectl rollout status deployment/database-user-operator -n db-system --timeout=2m
	@echo "✓ Operator redeployed successfully"

# Internal target to merge coverage data
.PHONY: merge-coverage
merge-coverage:
	@echo ""
	@echo "Merging coverage data..."
	@mkdir -p coverage
	@echo "Converting integration test coverage..."
	@GOTOOLCHAIN=go1.25.0+auto go tool covdata textfmt -i=/tmp/coverage -o=coverage/integration.out
	@MODULE_PATH=$$(go list -m); \
	sed -i.bak "s|/workspace/|$$MODULE_PATH/|g" coverage/integration.out 2>/dev/null || sed -i '' "s|/workspace/|$$MODULE_PATH/|g" coverage/integration.out
	@rm -f coverage/integration.out.bak
	@echo "Merging unit and integration coverage..."
	@echo "mode: set" > coverage/merged.out
	@tail -n +2 cover.out >> coverage/merged.out
	@tail -n +2 coverage/integration.out >> coverage/merged.out
	@echo "✓ Coverage data merged into coverage/merged.out"
	@echo ""
	@echo "Coverage summary:"
	@go tool cover -func=coverage/merged.out | tail -1
	@echo ""
	@go tool cover -html=coverage/merged.out -o coverage/coverage.html
	@echo "HTML report: coverage/coverage.html"

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= $(LOCALBIN)/kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
KIND ?= $(LOCALBIN)/kind
HELM ?= $(LOCALBIN)/helm

## Tool Versions
KUSTOMIZE_VERSION ?= v5.2.1
CONTROLLER_TOOLS_VERSION ?= v0.19.0
GOLANGCI_LINT_VERSION ?= v1.62.2
KUBECTL_VERSION ?= v1.28.0
KIND_VERSION ?= v0.20.0
HELM_VERSION ?= v3.13.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	test -s $(LOCALBIN)/kustomize || GOBIN=$(LOCALBIN) go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	test -s $(LOCALBIN)/golangci-lint || GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: kubectl
kubectl: $(KUBECTL) ## Download kubectl locally if necessary.
$(KUBECTL): $(LOCALBIN)
	@if [ ! -s $(LOCALBIN)/kubectl ]; then \
		if command -v brew >/dev/null 2>&1 && [ "$$(uname -s)" = "Darwin" ]; then \
			echo "Installing kubectl via Homebrew..."; \
			brew install kubectl && ln -sf $$(brew --prefix kubectl)/bin/kubectl $(LOCALBIN)/kubectl; \
		else \
			echo "Installing kubectl $(KUBECTL_VERSION)..."; \
			OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
			ARCH=$$(uname -m); \
			case $$ARCH in \
				x86_64) ARCH=amd64 ;; \
				aarch64|arm64) ARCH=arm64 ;; \
			esac; \
			KUBECTL_URL="https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/$${OS}/$${ARCH}/kubectl"; \
			curl -fsSL "$$KUBECTL_URL" -o $(LOCALBIN)/kubectl; \
			chmod +x $(LOCALBIN)/kubectl; \
		fi \
	fi

.PHONY: kind
kind: $(KIND) ## Download kind locally if necessary.
$(KIND): $(LOCALBIN)
	@if [ ! -s $(LOCALBIN)/kind ]; then \
		if command -v brew >/dev/null 2>&1 && [ "$$(uname -s)" = "Darwin" ]; then \
			echo "Installing kind via Homebrew..."; \
			brew install kind && ln -sf $$(brew --prefix kind)/bin/kind $(LOCALBIN)/kind; \
		else \
			echo "Installing kind via go install..."; \
			GOBIN=$(LOCALBIN) go install sigs.k8s.io/kind@$(KIND_VERSION); \
		fi \
	fi

.PHONY: helm
helm: $(HELM) ## Download helm locally if necessary.
$(HELM): $(LOCALBIN)
	@if [ ! -s $(LOCALBIN)/helm ]; then \
		if command -v brew >/dev/null 2>&1 && [ "$$(uname -s)" = "Darwin" ]; then \
			echo "Installing helm via Homebrew..."; \
			brew install helm && ln -sf $$(brew --prefix helm)/bin/helm $(LOCALBIN)/helm; \
		else \
			echo "Installing helm $(HELM_VERSION)..."; \
			OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
			ARCH=$$(uname -m); \
			case $$ARCH in \
				x86_64) ARCH=amd64 ;; \
				aarch64|arm64) ARCH=arm64 ;; \
			esac; \
			HELM_URL="https://get.helm.sh/helm-$(HELM_VERSION)-$${OS}-$${ARCH}.tar.gz"; \
			TMP_DIR=$$(mktemp -d); \
			curl -fsSL "$$HELM_URL" | tar -xz -C "$$TMP_DIR"; \
			mv "$$TMP_DIR/$${OS}-$${ARCH}/helm" $(LOCALBIN)/helm; \
			rm -rf "$$TMP_DIR"; \
			chmod +x $(LOCALBIN)/helm; \
		fi \
	fi
