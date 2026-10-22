# golang-ci tag
GOLANGCI_TAG:=1.64.8
# Service name
SERVICE_NAME = generator
# Path to the binary
LOCAL_BIN:=$(CURDIR)/bin
# Path to the binary golang-ci
GOLANGCI_BIN:=$(LOCAL_BIN)/golangci-lint
# Minimal Golang version
MIN_GO_VERSION = 1.20.0
# Go language version this repo targets (the "go" directive of go.mod)
GO_MOD_VERSION := $(shell awk '$$1 == "go" { print $$2; exit }' go.mod 2>/dev/null)
BIN_NAME = go-project-starter

# Version info from git
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LD_FLAGS = -X 'main.version=$(VERSION)' -X 'main.commit=$(COMMIT)' -X 'main.buildDate=$(BUILD_DATE)'

# Integration test image parameters
INTEGRATION_GO_VERSION := 1.24.4
INTEGRATION_BUF_VERSION := 1.47.2
INTEGRATION_IMAGE_NAME := go-project-starter-test
INTEGRATION_PARAMS_FILE := test/docker-integration/.image-params
INTEGRATION_CURRENT_PARAMS := $(INTEGRATION_GO_VERSION)-$(INTEGRATION_BUF_VERSION)-$(GITHUB_TOKEN)

##################### Checks to run golang-ci #####################
# GOLANGCI_TAG above is the single source of truth for the linter version, and
# configs/golangci.yml is written in the config format of that major version —
# a binary of another major version either rejects the config or silently drops
# most of it. So, unlike the Makefile of a generated project (which accepts
# anything at or above the tag), the checks below require an *exact* match and
# otherwise build the pinned version into ./bin (see install-lint).
#
# Version of a golangci-lint binary, empty when it cannot be determined.
# Matching the leading semver tolerates both release formats:
#   v1: "golangci-lint has version 1.64.8 built from <commit> on <date>"
#   v2: "golangci-lint has version 2.5.0 built with go1.26.0 from (...) on (...)"
golangci_version = $(shell $(1) --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -n1)
# Go toolchain a binary was built with ("<path>: go1.26.0" -> "1.26.0"), empty
# if unknown. GOTOOLCHAIN=local keeps the probe from switching toolchains.
golangci_go_version = $(shell GOTOOLCHAIN=local go version -m $(1) 2>/dev/null | awk 'NR==1 { sub(/^go/, "", $$2); print $$2 }')
# Non-empty when version $(2) is known and lower than required version $(1);
# an unknown $(2) is left alone so a failing probe never forces a rebuild loop.
version_older = $(if $(1),$(if $(2),$(filter-out $(1),$(word 1, $(sort $(1) $(2))))))
# A binary is unusable when its version is not exactly GOLANGCI_TAG, or when it
# was built with a toolchain older than the Go version this repo targets — the
# latter panics while type-checking ("file requires newer Go version goX.Y").
golangci_unusable = $(if $(filter $(GOLANGCI_TAG),$(call golangci_version,$(1))),,version)$(call version_older,$(GO_MOD_VERSION),$(call golangci_go_version,$(1)))

# Local bin version check
ifneq ($(wildcard $(GOLANGCI_BIN)),)
ifneq ($(call golangci_unusable,$(GOLANGCI_BIN)),)
GOLANGCI_BIN:=
endif
endif

# Global bin version check
GOLANGCI_GLOBAL_BIN:=$(shell which golangci-lint)
ifneq ($(GOLANGCI_GLOBAL_BIN),)
ifeq ($(call golangci_unusable,$(GOLANGCI_GLOBAL_BIN)),)
GOLANGCI_BIN:=$(GOLANGCI_GLOBAL_BIN)
endif
endif
##################### End of golang-ci checks #####################

# Install linter
.PHONY: install-lint
install-lint:
ifeq ($(wildcard $(GOLANGCI_BIN)),)
	$(info "Downloading golangci-lint v$(GOLANGCI_TAG)")
	tmp=$$(mktemp -d) && cd $$tmp && pwd && go mod init temp && go get -d github.com/golangci/golangci-lint/cmd/golangci-lint@v$(GOLANGCI_TAG) && \
		go build -ldflags "-X 'main.version=$(GOLANGCI_TAG)' -X 'main.commit=test' -X 'main.date=test'" -o $(LOCAL_BIN)/golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint && \
		rm -rf $$tmp
GOLANGCI_BIN:=$(LOCAL_BIN)/golangci-lint
endif

.PHONY: check-go-version
check-go-version:
	@./scripts/check-go-mod-version.sh

.PHONY: test
test:
	@go test $$(go list ./... | grep -v /test/docker-integration) -coverprofile=.cover

.PHONY: build
build:
	go build -o $(BIN_NAME) -ldflags="$(LD_FLAGS)" $(PWD)

# Linter will check only diffs with main branch (default)
.PHONY: lint
lint: install-lint
	$(GOLANGCI_BIN) run --config=./configs/golangci.yml ./... --new-from-rev=origin/main --build-tags=$(SERVICE_NAME)

# Check configs/golangci.yml against the config schema of the pinned linter
# version. `golangci-lint run` ignores unknown keys, so a config left behind by
# another major version keeps "working" while its settings are silently dropped
# — this target is what notices. Run it whenever GOLANGCI_TAG changes.
# Not part of the CI gate: the schema is fetched from golangci-lint.run, and a
# docs-site outage must not turn every pull request red.
.PHONY: lint-config-verify
lint-config-verify: install-lint
	$(GOLANGCI_BIN) config verify --config=./configs/golangci.yml

# Run full code lint
.PHONY: lint-full
lint-full: lint
	$(GOLANGCI_BIN) run --config=./configs/golangci.yml ./... --build-tags=$(SERVICE_NAME)

# Linter will check only diffs with main branch (default)
.PHONY: lint-fix
lint-fix: lint
	$(GOLANGCI_BIN) run --fix --config=./configs/golangci.yml ./... --build-tags=$(SERVICE_NAME)

# Install config to your home directory.
.PHONY: install-config
install-config:
	@cp .golangci.yml $(HOME)/.golangci.yaml
	@echo "Golangci config installed to $(HOME)/.golangci.yaml"

# Create test coverage report
.PHONY: coverage
coverage:
	@go tool cover -html=.cover -o coverage.html
	@go tool cover -func .cover | grep "total:"

# Unit tests with the race detector. This is the CI gate (unit-test.yml), so
# -count=1 keeps the test cache from masking a failure on a re-run.
# ./test/docker-integration is excluded: it needs a prebuilt image and has its
# own job (make integration-test).
.PHONY: race
race:
	@go test $$(go list ./... | grep -v /test/docker-integration) -race -parallel=10 -count=1

.PHONY: local-install
local-install:
	@go install -ldflags="$(LD_FLAGS)" ./cmd/go-project-starter

# Build binary for integration tests
.PHONY: buildfortest
buildfortest:
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LD_FLAGS)" -o bin/go-project-starter ./cmd/go-project-starter

# Build test image (with marker file check for rebuild)
.PHONY: build-test-image
build-test-image:
	@if [ ! -f $(INTEGRATION_PARAMS_FILE) ] || [ "$$(cat $(INTEGRATION_PARAMS_FILE))" != "$(INTEGRATION_CURRENT_PARAMS)" ]; then \
		echo "Building integration test image..."; \
		docker build \
			--build-arg GO_VERSION=$(INTEGRATION_GO_VERSION) \
			--build-arg BUF_VERSION=$(INTEGRATION_BUF_VERSION) \
			--build-arg GITHUB_TOKEN=$(GITHUB_TOKEN) \
			-t $(INTEGRATION_IMAGE_NAME):latest \
			-f test/docker-integration/Dockerfile . && \
		echo "$(INTEGRATION_CURRENT_PARAMS)" > $(INTEGRATION_PARAMS_FILE); \
	else \
		echo "Test image up to date, skipping build"; \
	fi

# Integration tests (using testcontainers)
# Requires GITHUB_TOKEN env var for private repos access
.PHONY: integration-test
integration-test: buildfortest build-test-image
	@echo "Running integration tests with testcontainers..."
	@if [ -z "$$GITHUB_TOKEN" ]; then echo "Warning: GITHUB_TOKEN not set, private repos may fail"; fi
	TEST_IMAGE=$(INTEGRATION_IMAGE_NAME):latest GOAT_DISABLE_STDOUT=true go test -v -timeout 30m ./test/docker-integration/...

# Integration tests with verbose output
.PHONY: integration-test-verbose
integration-test-verbose: buildfortest build-test-image
	@echo "Running integration tests with verbose output..."
	TEST_IMAGE=$(INTEGRATION_IMAGE_NAME):latest go test -v -timeout 30m ./test/docker-integration/...

# Run single integration test
.PHONY: integration-test-rest
integration-test-rest: buildfortest build-test-image
	TEST_IMAGE=$(INTEGRATION_IMAGE_NAME):latest GOAT_DISABLE_STDOUT=true go test -v -timeout 15m -run TestIntegrationRESTOnly ./test/docker-integration/...

.PHONY: integration-test-grpc
integration-test-grpc: buildfortest build-test-image
	TEST_IMAGE=$(INTEGRATION_IMAGE_NAME):latest GOAT_DISABLE_STDOUT=true go test -v -timeout 15m -run TestIntegrationGRPCOnly ./test/docker-integration/...

.PHONY: integration-test-telegram
integration-test-telegram: buildfortest build-test-image
	TEST_IMAGE=$(INTEGRATION_IMAGE_NAME):latest GOAT_DISABLE_STDOUT=true go test -v -timeout 15m -run TestIntegrationWorkerTelegram ./test/docker-integration/...

.PHONY: integration-test-combined
integration-test-combined: buildfortest build-test-image
	TEST_IMAGE=$(INTEGRATION_IMAGE_NAME):latest GOAT_DISABLE_STDOUT=true go test -v -timeout 15m -run TestIntegrationCombined ./test/docker-integration/...

.PHONY: integration-test-grafana
integration-test-grafana: buildfortest build-test-image
	TEST_IMAGE=$(INTEGRATION_IMAGE_NAME):latest GOAT_DISABLE_STDOUT=true go test -v -timeout 15m -run TestIntegrationGrafana ./test/docker-integration/...

# Run documentation server locally
.PHONY: docs
docs:
	docker run --rm -p 8000:8000 -v $(PWD):/docs squidfunk/mkdocs-material serve --dev-addr=0.0.0.0:8000 --watch-theme
