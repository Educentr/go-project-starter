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
| `logger` | Нет | Тип логгера: `zerolog` (по умолчанию) или `logrus` |
| `author` | Нет | Автор проекта |
| `use_active_record` | Нет | Включает генерацию кода для PostgreSQL |
| `dev_stand` | Нет | Генерировать docker-compose-dev.yaml с OnlineConf |
| `skip_service_init` | Нет | Пропустить генерацию Service layer |
| `ci` | Нет | Список CI провайдеров для генерации (см. [CI/CD провайдеры](#cicd-провайдеры)) |

### Выбор логгера

Генератор поддерживает два логгера:

| Логгер | Значение | Описание |
|--------|----------|----------|
| [zerolog](https://github.com/rs/zerolog) | `zerolog` | Высокопроизводительный JSON-логгер с zero allocation. Структурированное логирование с типизированными полями (`.Str()`, `.Int()`, `.Err()`). |
| [logrus](https://github.com/sirupsen/logrus) | `logrus` | Широко распространённый структурированный логгер. Простой API через `.WithField()` / `.WithError()`. |

```yaml
# zerolog (по умолчанию)
main:
  logger: zerolog

# logrus
main:
  logger: logrus
```

Логгер влияет на:

- **Инициализацию** — генерируется `pkg/app/logger/` с функциями `InitZlog` или `InitLogrus`
- **Импорты** — zerolog использует `zlog "github.com/rs/zerolog"`, logrus — `rlog "github.com/Educentr/go-project-starter-runtime/pkg/logger"`
- **Вызовы логирования** — все шаблоны используют абстракцию `{{ .Logger }}`, переключение логгера не требует изменений в пользовательском коде
- **Тесты** — тестовый логгер настраивается автоматически под выбранный тип

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

### CI/CD провайдеры

Поле `ci` управляет генерацией файлов CI/CD пайплайнов.

| Значение | Результат |
|----------|-----------|
| не указано | Генерируются оба: `.github/workflows/ci_cd.yml` и `.gitlab-ci.yml` |
| `[github]` | Только GitHub Actions |
| `[gitlab]` | Только GitLab CI |
| `[github, gitlab]` | Оба (явно) |
| `[]` | Не генерировать CI файлы |

```yaml
# Оба провайдера (по умолчанию, если ci не указан)
main:
  name: myproject

# Только GitHub Actions
main:
  name: myproject
  ci: [github]

# Только GitLab CI
main:
  name: myproject
  ci: [gitlab]

# Без CI/CD файлов
main:
  name: myproject
  ci: []
```

!!! note "Обратная совместимость"
    Если поле `ci` не указано, генерируются оба файла — поведение не изменилось.
    Пустой массив `ci: []` явно отключает генерацию всех CI файлов.

## Секция `git`

Настройки Git репозитория.

```yaml
git:
  repo: https://github.com/org/repo
  module_path: github.com/org/repo
  private_repos:
    - github.com/myorg/internal-pkg
    - gitlab.com/company/*
  private_repos_token_secret: GO_MODULES_TOKEN
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `repo` | Да | URL Git репозитория |
| `module_path` | Да | Go module path |
| `private_repos` | Нет | Список приватных модулей для GOPRIVATE |
| `private_repos_token_secret` | Нет | Имя GitHub-секрета с токеном для приватных модулей. По умолчанию `GH_PAT` |

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

### Имя секрета с токеном

Сгенерированный workflow GitHub Actions (`.github/workflows/ci_cd.yml`) читает токен
для скачивания приватных модулей из секрета `GH_PAT`. Если токены разделены по классу
привилегии — например, узкий read-only токен на модули отдельно от токенов с правом
записи — имя секрета задаётся явно:

```yaml
git:
  repo: github.com/myorg/service
  module_path: github.com/myorg/service
  private_repos: github.com/myorg/internal-pkg
  private_repos_token_secret: GO_MODULES_TOKEN
```

Имя подставляется во все места workflow, где используется этот токен
(`insteadOf` для `go mod download` и build-секрет для `docker build`):

```yaml
GH_TOKEN: ${{ secrets.GO_MODULES_TOKEN }}
```

Детали:

- Значение должно быть корректным именем секрета: `[A-Za-z_][A-Za-z0-9_]*`.
  Иначе генерация падает с ошибкой валидации конфига.
- Если поле не задано, используется `GH_PAT` — вывод у существующих проектов
  не меняется.
- Меняется только имя секрета GitHub Actions. Id build-секрета (`github_token`)
  и имена переменных окружения внутри шагов остаются прежними, поэтому
  `Dockerfile` и `Makefile` править не нужно.
- В `.gitlab-ci.yml` поле не используется: там токен приходит из переменных GitLab CI.

## Секция `tools`

Настройка версий используемых инструментов.

```yaml
tools:
  protobuf_version: "1.7.0"
  golang_version: "1.24"
  ogen_version: "v0.78.0"
  golangci_version: "2.1.6"
  argen_version: "v1.0.0"
  runtime_version: "v0.5.0"
  go_jsonschema_version: "v0.16.0"
  goat_version: "v0.3.1"
  goat_services_version: "v0.1.0"
  lint_config:
    overrides: "./configs/golangci-overrides.yml"
```

### Поля

| Поле | Описание | По умолчанию |
|------|----------|--------------|
| `protobuf_version` | Версия protoc-gen-go | 1.7.0 |
| `golang_version` | Версия Go для сгенерированного проекта | 1.24 |
| `ogen_version` | Версия ogen | v0.78.0 |
| `golangci_version` | Версия golangci-lint | 2.1.6 |
| `lint_exclude_paths` | Пути, которые golangci-lint пропускает (регэкспы, в оба блока `exclusions.paths`); например вендорная копия или сабмодуль | `[]` |
| `lint_config.own` | Проект сам держит `configs/golangci-lint.yml`: генератор его не пишет, не читает и не удаляет | `false` |
| `lint_config.overrides` | Документ переопределений, который генератор вливает в свой шаблон линтера при генерации. URI — как у `rest[].path` | — |
| `argen_version` | Версия argen (ActiveRecord) | v1.0.0 |
| `runtime_version` | Версия go-project-starter-runtime | авто |
| `go_jsonschema_version` | Версия go-jsonschema | v0.16.0 |
| `goat_version` | Версия GOAT тест-фреймворка | авто |
| `goat_services_version` | Версия GOAT services | авто |

`lint_config.own` и `lint_config.overrides` взаимоисключающие. Правила слияния, формы URI
и поведение при регенерации — [Конфиг линтера](lint-config.md).

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
