# Основные секции

Описание секций `main`, `git` и версий инструментов.

## Секция `main`

Основные настройки проекта.

```yaml
main:
  name: myproject
  registry_type: github
  logger: zerolog
  author: "Your Name"
  use_active_record: true
  dev_stand: true
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя проекта (используется в путях, Docker образах) |
| `registry_type` | Нет | `github`, `digitalocean`, `aws`, или `selfhosted` |
| `logger` | Нет | Тип логгера (сейчас только `zerolog`) |
| `author` | Нет | Автор проекта |
| `use_active_record` | Нет | Включает генерацию кода для PostgreSQL |
| `dev_stand` | Нет | Генерировать docker-compose-dev.yaml с OnlineConf |
| `skip_service_init` | Нет | Пропустить генерацию Service layer |

### Типы Container Registry

| Тип | Registry | Требуемые секреты GitHub Actions |
|-----|----------|----------------------------------|
| `github` | GitHub Container Registry (ghcr.io) | `GHCR_USER`, `GHCR_TOKEN` |
| `digitalocean` | DigitalOcean Container Registry | `REGISTRY_PASSWORD` |
| `aws` | Amazon ECR | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION` |
| `selfhosted` | Любой Docker Registry | `REGISTRY_URL`, `REGISTRY_USERNAME`, `REGISTRY_PASSWORD` |

#### AWS ECR

Для AWS ECR формат URL реестра: `{account-id}.dkr.ecr.{region}.amazonaws.com`

```yaml
main:
  registry_type: aws
```

#### Self-Hosted Registry

Для self-hosted реестров (включая MinIO-backed registry):

```yaml
main:
  registry_type: selfhosted
```

Настройте переменную `REGISTRY_LOGIN_SERVER` в GitHub для указания URL реестра.

## Секция `git`

Настройки Git репозитория.

```yaml
git:
  repo: https://github.com/org/repo
  module_path: github.com/org/repo
  private_repos:
    - github.com/myorg/internal-pkg
    - gitlab.com/company/*
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `repo` | Да | URL Git репозитория |
| `module_path` | Да | Go module path |
| `private_repos` | Нет | Список приватных модулей для GOPRIVATE |

### Приватные Go модули

Если проект зависит от приватных Go модулей, укажите их в `private_repos`:

```yaml
git:
  repo: github.com/myorg/service
  module_path: github.com/myorg/service
  private_repos:
    - github.com/myorg/internal-pkg
    - gitlab.com/company/*
```

Это автоматически добавит в Makefile и Dockerfile:

```bash
GOPRIVATE=github.com/myorg/internal-pkg,gitlab.com/company/*
```

## Секция `tools`

Настройка версий используемых инструментов.

```yaml
tools:
  protobuf_version: "1.7.0"
  golang_version: "1.24"
  ogen_version: "v0.78.0"
  golangci_version: "1.55.2"
  argen_version: "v1.0.0"
  runtime_version: "v0.5.0"
  go_jsonschema_version: "v0.16.0"
  goat_version: "v0.3.1"
  goat_services_version: "v0.1.0"
```

### Поля

| Поле | Описание | По умолчанию |
|------|----------|--------------|
| `protobuf_version` | Версия protoc-gen-go | 1.7.0 |
| `golang_version` | Версия Go для сгенерированного проекта | 1.24 |
| `ogen_version` | Версия ogen | v0.78.0 |
| `golangci_version` | Версия golangci-lint | 1.55.2 |
| `argen_version` | Версия argen (ActiveRecord) | v1.0.0 |
| `runtime_version` | Версия go-project-starter-runtime | авто |
| `go_jsonschema_version` | Версия go-jsonschema | v0.16.0 |
| `goat_version` | Версия GOAT тест-фреймворка | авто |
| `goat_services_version` | Версия GOAT services | авто |

## Секция `post_generate`

Шаги, выполняемые после генерации.

```yaml
post_generate:
  - git_install          # Инициализировать git репозиторий
  - tools_install        # Установить dev инструменты
  - clean_imports        # Организовать imports через goimports
  - executable_scripts   # chmod +x для скриптов
  - call_generate        # Запустить make generate
  - go_mod_tidy          # Запустить go mod tidy
```

### Доступные шаги

| Шаг | Описание |
|-----|----------|
| `git_install` | Инициализировать git репозиторий |
| `tools_install` | Установить ogen, argen, golangci-lint |
| `clean_imports` | Организовать imports через goimports |
| `executable_scripts` | Сделать скрипты исполняемыми |
| `call_generate` | Запустить `make generate` |
| `go_mod_tidy` | Запустить `go mod tidy` |

!!! note "Важно для dev_stand"
    `dev_stand: true` требует `git_install` в `post_generate`, так как OnlineConf добавляется как git submodule.
