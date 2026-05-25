ARG GO_VERSION=1.24.4
FROM golang:${GO_VERSION}-bookworm

ARG BUF_VERSION=1.47.2
ARG GITHUB_TOKEN

ENV DEBIAN_FRONTEND=noninteractive
ENV GOPRIVATE=github.com/Educentr/*

# Install dependencies
RUN apt-get update && apt-get install -y \
    curl \
    git \
    make \
    ca-certificates \
    unzip \
    && rm -rf /var/lib/apt/lists/*

# Install Docker CLI for building images inside container
RUN curl -fsSL https://get.docker.com | sh

# Git config for private repos (if token provided)
RUN if [ -n "$GITHUB_TOKEN" ]; then \
    git config --global url."https://x-access-token:${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"; \
    fi

# Install buf for proto generation
RUN curl -sSL "https://github.com/bufbuild/buf/releases/download/v${BUF_VERSION}/buf-Linux-x86_64" \
    -o /usr/local/bin/buf && chmod +x /usr/local/bin/buf

# Install protoc-gen-go and protoc-gen-go-grpc
# Pin protoc-gen-go-grpc: v1.6.2 (released 2026-05-07) bumped the required
# Go version to 1.25.0, which breaks this image's golang:1.24-bookworm base.
# v1.6.1 is the last version still compatible with Go 1.24.
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.1

# Install goimports
# Pin: golang.org/x/tools@v0.43.0+ requires Go 1.25.0. v0.42.0 is the last
# release compatible with this image's golang:1.24-bookworm base.
RUN go install golang.org/x/tools/cmd/goimports@v0.42.0

WORKDIR /workspace
