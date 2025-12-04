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

# Git config for private repos (if token provided)
RUN if [ -n "$GITHUB_TOKEN" ]; then \
    git config --global url."https://x-access-token:${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"; \
    fi

# Install buf for proto generation
RUN curl -sSL "https://github.com/bufbuild/buf/releases/download/v${BUF_VERSION}/buf-Linux-x86_64" \
    -o /usr/local/bin/buf && chmod +x /usr/local/bin/buf

# Install protoc-gen-go and protoc-gen-go-grpc
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Install goimports
RUN go install golang.org/x/tools/cmd/goimports@latest

WORKDIR /workspace
