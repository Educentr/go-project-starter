# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go Project Starter is a **code generator** (not a running microservice). It generates production-ready Go microservices from YAML configuration files. The generator uses 78+ embedded templates to produce ~8,000 lines of production-grade code including REST APIs, gRPC services, Kafka consumers, Telegram bots, and complete infrastructure.

## IMPORTANT: Runtime Version Update

**При каждом релизе go-project-starter необходимо обновить `MinRuntimeVersion`:**

1. Проверить последний тег в репозитории go-project-starter-runtime
2. Обновить константу `MinRuntimeVersion` в файле `internal/pkg/templater/templater.go`
3. Обновление `MinRuntimeVersion` и зависимости в go.mod должны быть в **одном коммите**

```go
// internal/pkg/templater/templater.go
const MinRuntimeVersion = "vX.Y.Z"  // <- обновить до последней версии runtime
```

## Build/Test/Lint Commands

```bash
# Build the generator
make build

# Install locally for testing
go install ./cmd/go-project-starter

# Run generator with a config
go-project-starter --config=example/config.yaml

# Run tests
make test                # Run tests with coverage
make race                # Run tests with race detector

# Linting (requires golangci-lint v1.52.2)
make install-lint        # Install golangci-lint
make lint                # Check diffs with origin/main
make lint-full           # Full code lint
make lint-fix            # Auto-fix linting issues

# Coverage report
make coverage            # Generate HTML coverage report
```

## Development Workflow

1. Make code changes
2. `go install ./cmd/go-project-starter`
3. Test with: `go-project-starter --config=example/config.yaml`
4. Run `make lint` before committing

## Architecture

### Core Packages (internal/pkg/)

- **config/** - Loads and validates YAML configs via spf13/viper
- **generator/** - Orchestrates generation, runs post-generation steps (git init, goimports, go mod tidy)
- **templater/** - Executes Go text/templates with code preservation via disclaimer markers
  - Templates embedded in `templater/embedded/templates/`
  - Organized by: main/, transport/, worker/, app/, logger/

### Template Structure

```text
templater/embedded/templates/
├── main/           # Project scaffolding (Makefile, Dockerfile, configs)
├── transport/
│   ├── rest/       # Ogen + template-based REST
│   ├── grpc/       # gRPC services
│   └── kafka/      # Kafka consumers
├── worker/         # Background workers (telegram, daemon)
├── app/            # Application layer templates
└── logger/         # Logger implementations (zerolog)
```

### Three-Layer Architecture (Generated Projects)

- **pkg/** - Runtime libraries, no config dependency, maximally reusable
- **internal/pkg/** - Generated core logic, config-agnostic, no logger binding (returns errors up)
- **internal/app/** - Project-specific code, config-aware, can use specific logger

### Key Concepts

- **Application** - Atomically scalable unit (container). Can include HTTP servers, gRPC, workers, drivers
- **Transport** - Protocol layer: REST (ogen/template), gRPC, CLI, Kafka
- **Driver** - External service integration implementing `Runnable` interface (Init, Run, Shutdown, GracefulShutdown)
- **Disclaimer Markers** - Separates generated code from manual code; code below marker survives regeneration

### Generator Types

- `ogen` - OpenAPI 3.0 code generation for REST
- `template` - Custom template-based generation (e.g., `sys` for metrics server)
- `ogen_client` - REST client generation

## Configuration Validation Rules

- REST/gRPC services must be assigned to applications
- Drivers referenced must exist
- No duplicate transport/worker/driver names
- ActiveRecord requires ArgenVersion

## Test Files

- `test/generate_test.go` - Integration tests
- `test/configs/` - Test configurations (config1.yml, example.proto, example.swagger.yml)
