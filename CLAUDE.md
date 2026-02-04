# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go Project Starter is a **code generator** (not a running microservice). It generates production-ready Go microservices from YAML configuration files. The generator uses 78+ embedded templates to produce ~8,000 lines of production-grade code including REST APIs, gRPC services, Kafka consumers, Telegram bots, and complete infrastructure.

## IMPORTANT: Release Checklist

**При каждом релизе go-project-starter выполнить:**

### 1. Runtime Version Update

1. Проверить последний тег в репозитории go-project-starter-runtime
2. Обновить константу `MinRuntimeVersion` в файле `internal/pkg/templater/templater.go`
3. Обновление `MinRuntimeVersion` и зависимости в go.mod должны быть в **одном коммите**

```go
// internal/pkg/templater/templater.go
const MinRuntimeVersion = "vX.Y.Z"  // <- обновить до последней версии runtime
```

### 2. Deprecation Cleanup

1. Проверить `internal/pkg/migrate/migrate.go` на константы `RemovalVersion*`
2. Если текущая версия >= версия удаления:
   - Удалить код обратной совместимости
   - Удалить миграционную логику для этой функции
   - Обновить `DEPRECATION.md`

См. полную политику deprecation в [DEPRECATION.md](DEPRECATION.md)

### 3. GitHub Release (ОБЯЗАТЕЛЬНО)

**Тег != релиз!** После пуша тега обязательно создать GitHub Release.

1. Найти последний релиз (не тег!): `gh release list --limit 1`
2. Получить коммит последнего релиза: `gh release view <version> --json targetCommitish`
3. Посмотреть изменения относительно последнего релиза:
   ```bash
   # Diff между релизами (не тегами!)
   gh release view <prev-version> --json targetCommitish -q .targetCommitish
   git log --oneline <prev-commit>..HEAD
   ```
4. Создать релиз с release notes:
   ```bash
   gh release create vX.Y.Z --title "vX.Y.Z" --notes "Release notes..."
   ```

**Важно:** Release notes составлять относительно коммита последнего релиза, а не последнего тега!

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
└── logger/         # Logger implementations (zerolog, logrus)
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

### Naming Hierarchy

| Name | Source | Example | Access |
|------|--------|---------|--------|
| **ServiceName** | `Main.Name` | `"my-api"` | `constant.ServiceName` |
| **AppName** | `Application.Name` | `"web-app"` | `ds.AppInfo.AppName` |
| **TransportName** | `Transport.Name` | `"api_v1"` | Function parameter |
| **WorkerName** | `Worker.Name` | `"telegram"` | Function parameter |

**OnlineConf paths** (3-level priority):
1. Default from code
2. Transport: `/{serviceName}/transport/rest/{transportName}/{key}`
3. App-specific: `/{serviceName}/transport/rest/{transportName}/{appName}/{key}`

See [docs/NAMING.md](docs/NAMING.md) for detailed documentation.

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

## Manual Testing

### Testing Generator Changes

После изменений в генераторе или шаблонах:

```bash
# Установить и сгенерировать тестовый проект
go install ./cmd/go-project-starter && \
  rm -rf ~/Develop/tmp/test-app && \
  mkdir ~/Develop/tmp/test-app && \
  go-project-starter --configDir=./test/docker-integration/configs/rest-only --target=~/Develop/tmp/test-app
```

### Testing dev_stand Feature

Функция `dev_stand: true` генерирует локальное окружение с OnlineConf.

**Важно:** При изменениях в SQL-шаблонах нужно удалить MySQL volume, иначе init-скрипты не применятся повторно.

```bash
# 1. Остановить контейнеры и удалить volumes (если были запущены ранее)
cd ~/Develop/tmp/test-app && docker compose -f docker-compose-dev.yaml down -v

# 2. Установить генератор и пересоздать проект
go install ./cmd/go-project-starter && \
  rm -rf ~/Develop/tmp/test-app && \
  mkdir ~/Develop/tmp/test-app && \
  go-project-starter --configDir=./test/docker-integration/configs/rest-only --target=~/Develop/tmp/test-app

# 3. Запустить dev-окружение
cd ~/Develop/tmp/test-app && docker compose -f docker-compose-dev.yaml up
```

**Что проверить:**
- OnlineConf Admin UI доступен на http://localhost:8888
- Traefik dashboard на http://localhost:9080
- API на http://localhost:8080
- Sys metrics на http://localhost:8085
- `onlineconf-updater` в статусе healthy (создан файл TREE.cdb)

### Docker Integration Tests

```bash
# Собрать образ и запустить интеграционные тесты
make buildfortest
TEST_IMAGE=go-project-starter-test:latest go test -v -count=1 -run TestIntegrationRESTOnly ./test/docker-integration/...
```
