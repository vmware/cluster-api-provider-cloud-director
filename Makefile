# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

GITCOMMIT := $(shell git rev-parse --short HEAD 2>/dev/null)
GITROOT := $(shell git rev-parse --show-toplevel)
GO_CODE := $(shell ls go.mod go.sum **/*.go)

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec
MANIFEST_DIR = templates

REGISTRY ?= "projects-stg.registry.vmware.com/vmware-cloud-director"
VERSION ?= $(shell cat ${GITROOT}/release/version)
CAPVCD_IMG := cluster-api-provider-cloud-director
CAPVCD_ARTIFACT_IMG := capvcd-manifest-airgapped

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

# go-install-tool will 'go get' any package $2 and install it to $1.
define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(GITROOT)/tools go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef


_SET_DEV_BUILD:
	$(eval VERSION=$(VERSION)-$(GITCOMMIT))


.PHONY: vendor

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

vendor: ## Run go mod vendor
	go mod edit -go=1.19
	go mod tidy -compat=1.19
	go mod vendor


ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
test: manifests generate fmt vet ## Run tests.
	mkdir -p ${ENVTEST_ASSETS_DIR}
	test -f ${ENVTEST_ASSETS_DIR}/setup-envtest.sh || curl -sSLo ${ENVTEST_ASSETS_DIR}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.8.3/hack/setup-envtest.sh
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out

##@ Build

build-within-docker: vendor
	mkdir -p /build/cluster-api-provider-cloud-director
	CGO_ENABLED=0 go build -ldflags "-s -w -X github.com/vmware/$(CAPVCD_IMG)/version.Version=${VERSION}" -o /build/vcloud/cluster-api-provider-cloud-director main.go

generate-capvcd-image: generate fmt vet vendor 
	docker build -f Dockerfile . -t $(CAPVCD_IMG):$(VERSION) --build-arg VERSION=$(VERSION)
	docker tag $(CAPVCD_IMG):$(VERSION) $(REGISTRY)/$(CAPVCD_IMG):$(VERSION)

push-capvcd-image:
	docker push $(REGISTRY)/$(CAPVCD_IMG):$(VERSION)

generate-capvcd-artifact-image:
	sed -e "s/__VERSION__/$(VERSION)/g" config/manager/manager.yaml.template > config/manager/manager.yaml
	sed -e "s/__VERSION__/$(VERSION)/g" -e "s~__REGISTRY__~$(REGISTRY)~g" artifacts/bom.json.template > artifacts/bom.json
	sed -e "s/__VERSION__/$(VERSION)/g" -e "s~__REGISTRY__~$(REGISTRY)~g" artifacts/dependencies.txt.template > artifacts/dependencies.txt
	make release-manifests
	docker build -f ./artifacts/Dockerfile . -t $(CAPVCD_ARTIFACT_IMG):$(VERSION)
	docker tag $(CAPVCD_ARTIFACT_IMG):$(VERSION) $(REGISTRY)/$(CAPVCD_ARTIFACT_IMG):$(VERSION)

push-capvcd-artifact-image:
	docker push $(REGISTRY)/$(CAPVCD_ARTIFACT_IMG):$(VERSION)

dev: _SET_DEV_BUILD prod ## Build and push dev image

prod: generate-capvcd-image generate-capvcd-artifact-image push-capvcd-image push-capvcd-artifact-image ## Build and push release image

##@ Deployment

install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(REGISTRY)/$(CAPVCD_IMG):$(VERSION)
	$(KUSTOMIZE) build config/default | kubectl apply -f -

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

tools-dir:
	mkdir -p $(GITROOT)/tools

CONTROLLER_GEN := $(GITROOT)/tools/controller-gen
controller-gen: tools-dir ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

CONVERSION_GEN_BIN := conversion-gen
CONVERSION_GEN_DOCKERFILE := Dockerfile-ConversionGen
CONVERSION_GEN_CONTAINER := conversion-gen-container
CONVERSION_GEN := $(GITROOT)/tools/$(CONVERSION_GEN_BIN)
conversion: tools-dir ## Download controller-gen locally if necessary.
	docker build . -f $(GITROOT)/$(CONVERSION_GEN_DOCKERFILE) -t conversion
	docker create -ti --name $(CONVERSION_GEN_CONTAINER) conversion:latest bash
	docker cp $(CONVERSION_GEN_CONTAINER):/opt/conversion-gen/conversion-gen $(CONVERSION_GEN)
	docker rm $(CONVERSION_GEN_CONTAINER)


KUSTOMIZE = $(GITROOT)/tools/kustomize
kustomize: tools-dir ## Download kustomize locally if necessary.
	@if [ ! -f $(KUSTOMIZE) ]; then \
		wget "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"; \
		chmod +x ./install_kustomize.sh; \
		./install_kustomize.sh 3.8.7 $(GITROOT)/tools/; \
		rm -f ./install_kustomize.sh; \
	else \
		echo "kustomize already installed."; \
	fi

# Add a target to download and build conversion-gen; and then run it with the below params
generate-conversions: ## Runs Go related generate targets.
	rm -f $(GITROOT)/api/*/zz_generated.conversion.*
	$(CONVERSION_GEN) \
		--input-dirs=./api/v1alpha4,./api/v1beta1 \
		--build-tag=ignore_autogenerated_conversions \
		--output-file-base=zz_generated.conversion \
		--go-header-file=./boilerplate.go.txt

release-manifests: kustomize
	mkdir -p $(MANIFEST_DIR)
	$(KUSTOMIZE) build config/default > $(MANIFEST_DIR)/infrastructure-components.yaml

autogen-files: manifests generate generate-conversions release-manifests
