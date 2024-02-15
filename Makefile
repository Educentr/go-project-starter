# golang-ci tag
GOLANGCI_TAG:=1.52.2
# Service name
SERVICE_NAME = generator
# Path to the binary
LOCAL_BIN:=$(CURDIR)/bin
# Path to the binary golang-ci
GOLANGCI_BIN:=$(LOCAL_BIN)/golangci-lint
# Minimal Golang version
MIN_GO_VERSION = 1.20.0
BIN_NAME = scale

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
