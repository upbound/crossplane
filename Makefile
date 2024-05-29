# ====================================================================================
# Setup Project

PROJECT_NAME := crossplane
PROJECT_REPO := github.com/crossplane/$(PROJECT_NAME)

PLATFORMS ?= linux_amd64 linux_arm64 linux_arm linux_ppc64le darwin_amd64 darwin_arm64 windows_amd64
# -include will silently skip missing files, which allows us
# to load those files with a target in the Makefile. If only
# "include" was used, the make command would fail and refuse
# to run a target until the include commands succeeded.
-include build/makelib/common.mk

# ====================================================================================
# Setup Output

-include build/makelib/output.mk

# ====================================================================================
# Setup Go

# Set a sane default so that the nprocs calculation below is less noisy on the initial
# loading of this file
NPROCS ?= 1

# each of our test suites starts a kube-apiserver and running many test suites in
# parallel can lead to high CPU utilization. by default we reduce the parallelism
# to half the number of CPU cores.
GO_TEST_PARALLEL := $(shell echo $$(( $(NPROCS) / 2 )))

GO_STATIC_PACKAGES = $(GO_PROJECT)/cmd/crossplane $(GO_PROJECT)/cmd/crank
GO_TEST_PACKAGES = $(GO_PROJECT)/test/e2e
GO_LDFLAGS += -X $(GO_PROJECT)/internal/version.version=$(shell echo $(VERSION) | sed 's/[\.,-]up.*//' )
GO_SUBDIRS += cmd internal apis pkg
GO111MODULE = on
GOLANGCILINT_VERSION = 1.58.2
GO_LINT_ARGS ?= "--fix"

-include build/makelib/golang.mk

# ====================================================================================
# Setup Kubernetes tools

HELM_VERSION = v3.14.4
KIND_VERSION = v0.21.0
-include build/makelib/k8s_tools.mk

# ====================================================================================
# Setup Images
# Due to the way that the shared build logic works, images should
# all be in folders at the same level (no additional levels of nesting).

REGISTRY_ORGS ?= docker.io/upbound xpkg.upbound.io/upbound
IMAGES = crossplane
-include build/makelib/imagelight.mk

# ====================================================================================
# Targets

# run `make help` to see the targets and options

# We want submodules to be set up the first time `make` is run.
# We manage the build/ folder and its Makefiles as a submodule.
# The first time `make` is run, the includes of build/*.mk files will
# all fail, and this target will be run. The next time, the default as defined
# by the includes will be run instead.
fallthrough: submodules
	@echo Initial setup complete. Running make again . . .
	@make

CRD_DIR = cluster/crds
CRD_PATCH_DIR = cluster/crd-patches

# See patch files for details.
crds.patch: $(KUBECTL)
	@$(INFO) patching generated CRDs
	@mkdir -p $(WORK_DIR)/patch
	@$(KUBECTL) patch --local --type=json -f $(CRD_DIR)/pkg.crossplane.io_deploymentruntimeconfigs.yaml --patch-file $(CRD_PATCH_DIR)/pkg.crossplane.io_deploymentruntimeconfigs.yaml -o yaml > $(WORK_DIR)/patch/pkg.crossplane.io_deploymentruntimeconfigs.yaml
	@mv $(WORK_DIR)/patch/pkg.crossplane.io_deploymentruntimeconfigs.yaml $(CRD_DIR)/pkg.crossplane.io_deploymentruntimeconfigs.yaml
	@$(OK) patched generated CRDs

crds.clean:
	@$(INFO) cleaning generated CRDs
	@find $(CRD_DIR) -name '*.yaml' -exec sed -i.sed -e '1,1d' {} \; || $(FAIL)
	@find $(CRD_DIR) -name '*.yaml.sed' -delete || $(FAIL)
	@$(OK) cleaned generated CRDs

generate.run: gen-kustomize-crds gen-chart-license

gen-chart-license:
	@cp -f LICENSE cluster/charts/crossplane/LICENSE

generate.done: crds.clean crds.patch

gen-kustomize-crds:
	@$(INFO) Adding all CRDs to Kustomize file for local development
	@rm cluster/kustomization.yaml
	@echo "# This kustomization can be used to remotely install all Crossplane CRDs" >> cluster/kustomization.yaml
	@echo "# by running kubectl apply -k https://github.com/crossplane/crossplane//cluster?ref=master" >> cluster/kustomization.yaml
	@echo "resources:" >> cluster/kustomization.yaml
	@find $(CRD_DIR) -type f -name '*.yaml' | sort | \
		while read filename ;\
		do echo "- $${filename#*/}" >> cluster/kustomization.yaml \
		; done
	@$(OK) All CRDs added to Kustomize file for local development

e2e-tag-images:
	@$(INFO) Tagging E2E test images
	@docker tag $(BUILD_REGISTRY)/$(PROJECT_NAME)-$(TARGETARCH) crossplane-e2e/$(PROJECT_NAME):latest || $(FAIL)
	@$(OK) Tagged E2E test images

# NOTE(negz): There's already a go.test.integration target, but it's weird.
# This relies on make build building the e2e binary.
E2E_TEST_FLAGS ?=

# TODO(negz): Ideally we'd just tell the E2E tests which CLI tools to invoke.
# https://github.com/kubernetes-sigs/e2e-framework/issues/282
E2E_PATH = $(WORK_DIR)/e2e

GOTESTSUM_VERSION ?= v1.11.0
GOTESTSUM := $(TOOLS_HOST_DIR)/gotestsum

$(GOTESTSUM):
	@$(INFO) installing gotestsum
	@GOBIN=$(TOOLS_HOST_DIR) $(GOHOST) install gotest.tools/gotestsum@$(GOTESTSUM_VERSION) || $(FAIL)
	@$(OK) installed gotestsum

e2e-run-tests:
	@$(INFO) Run E2E tests
	@mkdir -p $(E2E_PATH)
	@ln -sf $(KIND) $(E2E_PATH)/kind
	@ln -sf $(HELM) $(E2E_PATH)/helm
	@PATH="$(E2E_PATH):${PATH}" $(GOTESTSUM) --format testname --junitfile $(GO_TEST_OUTPUT)/e2e-tests.xml --raw-command -- $(GO) tool test2json -t -p e2e $(GO_TEST_OUTPUT)/e2e -test.v $(E2E_TEST_FLAGS) || $(FAIL)
	@$(OK) Run E2E tests

e2e.init: build e2e-tag-images

e2e.run: $(GOTESTSUM) $(KIND) $(HELM3) e2e-run-tests

# Update the submodules, such as the common build scripts.
submodules:
	@git submodule sync
	@git submodule update --init --recursive

# Install CRDs into a cluster. This is for convenience.
install-crds: $(KUBECTL) reviewable
	$(KUBECTL) apply -f $(CRD_DIR)

# Uninstall CRDs from a cluster. This is for convenience.
uninstall-crds:
	$(KUBECTL) delete -f $(CRD_DIR)

# NOTE(hasheddan): the build submodule currently overrides XDG_CACHE_HOME in
# order to force the Helm 3 to use the .work/helm directory. This causes Go on
# Linux machines to use that directory as the build cache as well. We should
# adjust this behavior in the build submodule because it is also causing Linux
# users to duplicate their build cache, but for now we just make it easier to
# identify its location in CI so that we cache between builds.
go.cachedir:
	@go env GOCACHE

# This is for running out-of-cluster locally, and is for convenience. Running
# this make target will print out the command which was used. For more control,
# try running the binary directly with different arguments.
run: go.build
	@$(INFO) Running Crossplane locally out-of-cluster . . .
	@# To see other arguments that can be provided, run the command with --help instead
	$(GO_OUT_DIR)/$(PROJECT_NAME) core start --debug

.PHONY: manifests submodules fallthrough test-integration run install-crds uninstall-crds gen-kustomize-crds e2e-tests-compile e2e.test.images

# ====================================================================================
# Special Targets

define CROSSPLANE_MAKE_HELP
Crossplane Targets:
    submodules         Update the submodules, such as the common build scripts.
    run                Run crossplane locally, out-of-cluster. Useful for development.

endef
# The reason CROSSPLANE_MAKE_HELP is used instead of CROSSPLANE_HELP is because the crossplane
# binary will try to use CROSSPLANE_HELP if it is set, and this is for something different.
export CROSSPLANE_MAKE_HELP

crossplane.help:
	@echo "$$CROSSPLANE_MAKE_HELP"

help-special: crossplane.help

.PHONY: crossplane.help help-special
