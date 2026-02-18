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
# Local bin version check
ifneq ($(wildcard $(GOLANGCI_BIN)),)
GOLANGCI_BIN_VERSION:=$(shell $(GOLANGCI_BIN) --version)
ifneq ($(GOLANGCI_BIN_VERSION),)
GOLANGCI_BIN_VERSION_SHORT:=$(shell echo "$(GOLANGCI_BIN_VERSION)" | sed -E 's/.* version (.*) built from .* on .*/\1/g')
else
GOLANGCI_BIN_VERSION_SHORT:=0
endif
ifneq "$(GOLANGCI_TAG)" "$(word 1, $(sort $(GOLANGCI_TAG) $(GOLANGCI_BIN_VERSION_SHORT)))"
GOLANGCI_BIN:=
endif
endif

# Global bin version check
ifneq (, $(shell which golangci-lint))
GOLANGCI_VERSION:=$(shell golangci-lint --version 2> /dev/null )
ifneq ($(GOLANGCI_VERSION),)
GOLANGCI_VERSION_SHORT:=$(shell echo "$(GOLANGCI_VERSION)"|sed -E 's/.* version (.*) built from .* on .*/\1/g')
else
GOLANGCI_VERSION_SHORT:=0
endif
ifeq "$(GOLANGCI_TAG)" "$(word 1, $(sort $(GOLANGCI_TAG) $(GOLANGCI_VERSION_SHORT)))"
GOLANGCI_BIN:=$(shell which golangci-lint)
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

.PHONY: test
test:
	@go test ./... -coverprofile=.cover

.PHONY: build
build:
	go build -o $(BIN_NAME) -ldflags="$(LD_FLAGS)" $(PWD)

# Linter will check only diffs with main branch (default)
.PHONY: lint
lint: install-lint
	$(GOLANGCI_BIN) run --config=./configs/golangci.yml ./... --new-from-rev=origin/main --build-tags=$(SERVICE_NAME)

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

.PHONY: race
race:
	@go test ./... -race -parallel=10

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
