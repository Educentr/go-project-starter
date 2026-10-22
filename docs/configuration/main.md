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
| `lint_exclude_paths` | Пути, которые golangci-lint пропускает (регэкспы, в оба блока `exclusions.paths`); например вендорная копия или сабмодуль | `[]` |
| `argen_version` | Версия argen (ActiveRecord) | v1.0.0 |
| `runtime_version` | Версия go-project-starter-runtime | авто |
| `go_jsonschema_version` | Версия go-jsonschema | v0.16.0 |
| `goat_version` | Версия GOAT тест-фреймворка | авто |
| `goat_services_version` | Версия GOAT services | авто |
| `starter_version` | Версия go-project-starter, которую сгенерированный `Makefile` зовёт на регенерацию | авто |

### `starter_version` — пин генератора

Сгенерированный `Makefile` зовёт генератор так же, как зовёт ogen и argen — по версии,
которую сам же и скачивает:

```make
GPSPKG = github.com/Educentr/go-project-starter/cmd/go-project-starter
GPSVERSION = v1.2.3

regenerate:
	go run $(GPSPKG)@$(GPSVERSION) --target ${PWD}
```

Пин — **провенанс**: генератор пишет туда версию *того бинаря, который собрал дерево*, а не
выбранную вручную. Поэтому регенерация записанной версией обязана давать пустой дифф, и это
проверяется в CI. Править строку `GPSVERSION` руками бессмысленно — следующий прогон её перезапишет.

`starter_version` в `project.yaml` читается **только** когда бинарь не может назвать свою версию
(локальная сборка из нетегированного дерева): тогда в пин идёт значение из конфига, а если и его
нет — `latest`, о чём генератор печатает warning. Значение должно быть semver-тегом (`v1.2.3`) либо
литералом `latest`, иначе генерация падает: непроверенный пин сломал бы `make regenerate`.

Поднять версию (единственный выход из пина, потому что `make regenerate` всегда зовёт записанную):

```bash
make update-starter STARTER_VERSION=v1.3.0
```

Цель регенерирует проект указанной версией, и та, будучи генератором дерева, записывает себя
в `GPSVERSION`. `SOURCE_REF` работает здесь так же, как в `make regenerate`.

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
| `call_generate` | Запустить `make generate` — все генераторы, включая моки |
| `call_generate_mock` | Запустить `make mock` — все `//go:generate` проекта |
| `go_mod_tidy` | Запустить `go mod tidy` |

!!! note "`call_generate_mock` отдельно не нужен"
    `call_generate` уже перегенерирует моки, каркас которых пишет генератор:
    `generate-mocks` входит в агрегатор `generate` (см.
    [Makefile Targets Reference](../reference/makefile-targets.md#моки-и-make-generate)).
    `call_generate_mock` сохранён для обратной совместимости и зовёт `make mock` —
    это `go generate ./...`, то есть **все** директивы проекта, включая написанные
    вручную. Для новых конфигураций используйте `call_generate`.

!!! note "Важно для dev_stand"
    `dev_stand: true` требует `git_install` в `post_generate`, так как OnlineConf добавляется как git submodule.
