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

# Linting
make install-lint        # Install golangci-lint (version pinned by GOLANGCI_TAG in Makefile)
make lint                # Check diffs with origin/main — this is the CI gate (lint.yml)
make lint-config-verify  # Check configs/golangci.yml against the pinned version's schema
make lint-full           # Full code lint — currently red on main (~600 pre-existing issues)
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

## Remote Specs

`path:` в `project.yaml` (REST/gRPC/JSON Schema/CLI/queue worker) принимает не только локальные файлы, но и URI:

- `./local.yaml` — локально (как раньше)
- `https://host/file.yaml[?token_env=NAME]` — прямая ссылка, опц. Bearer-токен
- `git+ssh://git@host/org/repo.git@<ref>#<subpath>` — git, SSH (через ssh-agent)
- `git+https://host/org/repo.git@<ref>#<subpath>[?token_env=NAME]` — git, HTTPS, опц. токен

Удалённые спеки скачиваются в staging при `make regenerate`, копируются в `api/`, коммитятся в проект — после первого regenerate сборка не требует интернета. Token читается из ENV-переменной (имя в URI), сам токен в YAML не пишется.

Реализация: `internal/pkg/specsource/`. Pre-pass резолва — в `Generator.resolveAllSources()` (`internal/pkg/generator/generator.go`), вызывается из `Generate()` перед `CopySpecs`/`CopySchemas`. Полная документация: [docs/configuration/remote-specs.md](docs/configuration/remote-specs.md).

CLI и queue-worker spec'и (которые парсятся inline во время загрузки конфига) резолвятся через отдельный `resolveInlineSpec` хелпер: для local — joinpath, для remote — материализация во временный staging, парсинг, немедленная очистка (файл рантайму не нужен). REST/gRPC/JSONSchema — через основной `resolveAllSources()` pre-pass, файл остаётся живым для копирования в `api/`.

Известные ограничения v1: subpath = только файл (не директория), токен в `git clone` светится в `ps`.

### Переопределение ref (`--sourceRef`)

Повторяемый флаг `--sourceRef` (env `GPS_SOURCE_REF`) задаёт ref git-источников, у которых ref **не** указан в строке. Две формы: `<ref>` — на все репозитории (допустима одна на запуск), `<репозиторий>:<ref>` — адресно (контракты лежат в нескольких репозиториях, в релиз идёт своя ветка каждого). Запись с явным `@<ref>` в `project.yaml` — пин, не перекрывается ни одной формой. Действует на все каналы (`rest`, `grpc`, `jsonschema`, queue worker, CLI), иначе дерево собралось бы из двух состояний контрактов. Поля в `project.yaml` для дефолтного ref нет осознанно — ref задаёт тот, кто запускает сборку.

Реализация: `internal/pkg/specsource/sourceref.go` — тип `SourceRefs` (разбор, матчинг, учёт использования); единственный конструктор `ParseSourceRefs` (он же валидирует каждый ref через `ValidateRef`, `nil` = нет переопределения). Селектор отделяется по **последнему** `:` — `ValidateRef` запрещает `:` внутри ref. Матчинг по канонической форме `host/owner/repo` (`canonicalRepo`: без схемы, userinfo, порта, `.git`, в нижнем регистре), совпадение по границе сегмента, выигрывает самый длинный селектор. Признак пина — `GitSource.RefPinned` (выставляется в `parseGitURL`), применение — `SourceRefs.apply` → `GitSource.applySourceRef` в начале `Resolver.resolveGit`, поэтому эффективный ref попадает и в `cloneKey`, и в текст ошибок.

Прокидка: `resolveSourceRefs(pflag.CommandLine, os.Getenv)` в `cmd/go-project-starter/main.go` (есть флаги → env игнорируется целиком) → `generator.New(..., sourceRefs)` → `Generator.SourceRefs` → `ResolverOptions.SourceRefs` в **обоих** путях резолва (`resolveAllSources` и `resolveInlineSpec`), один и тот же указатель — по нему накапливается учёт совпадений. Адресный селектор, не совпавший ни с одним источником, — ошибка (`SourceRefs.CheckUsed()` в `Generate()` сразу после `resolveAllSources`): опечатка в имени репозитория не должна молча оставить его на дефолтной ветке. Общая форма «неиспользованной» не считается. В сгенерированном проекте: `make regenerate SOURCE_REF=<ref>` или `SOURCE_REF="<repo>:<ref> <repo>:<ref>"` (`main/Makefile.tmpl`, `$(foreach …)`). Документация: [docs/configuration/remote-specs.md](docs/configuration/remote-specs.md#переопределение-ref-sourceref).

## Свои генераторы (`generator` + `contract`)

Секция `generator` в `project.yaml` объявляет **генератор проекта** — три команды, без модели пакетных менеджеров: `install` (опц.), `cmd`, `lint` (опц.). Версия и способ установки принадлежат экосистеме генератора (`go install …@v1`, `npm ci` по lockfile, `pip install -r`), поэтому пин живёт внутри команды, а не отдельным ключом. Секция `contract` объявляет спеку (`path` — грамматика `rest[].path`; `copy`; `target` опц.) и ссылается на генератор обязательным `generator_name` — вход, из которого ничего не генерируется, контрактом не является.

Три команды ложатся на три цели: `install` → `install-<gen>` в зависимостях `install-tools`, `cmd` → `generate-<gen>` в `generate` (после встроенных, и через него в `regenerate`), `lint` → `lint-<gen>` в зависимостях `lint`. Одна цель на генератор, `cmd`/`lint` вызываются по одному разу на каждую его спеку. `generate-<gen>` от `install-<gen>` не зависит — как встроенные. Без объявленных `generator`/`contract` вывод Makefile не меняется ни на строку (`TestGenerateWithoutCustomGenerators_MakefileUnchanged`).

В `cmd`/`lint` доступны ровно две переменные Go-шаблона: `.Spec` (путь `copy` от корня проекта) и `.Target` (поле `target`). У `install` переменных нет — он выполняется один раз на генератор. Шаблоны валидируются при загрузке конфига: неизвестная переменная, незакрытое `{{`, ссылка на `.Target` в записи без `target` — ошибка загрузки, не пустая строка в команде. Имя генератора — `^[a-z0-9_-]+$`; коллизия со встроенной make-целью (`ogen`, `argen`, `jsonschema`, `lint`, `tools`, …, префикс `test-env-`) тоже ошибка.

Поля `version` нет: что забирать — `@ref` в `path` (пин) либо `--sourceRef`, как у остальных каналов. Что **забрано** — стартер пишет в манифест `.project-config/meta.yaml`: `source` (с вырезанным токеном), эффективный `ref` и `commit` (SHA; для https/local — только `source` плюс `hash`). Вывод `cmd` в манифест не попадает: его пишет не стартер.

**Спеки внутри проекта не правятся** — только в контрактном репозитории. Хеш скопированной спеки на диске ≠ манифесту → **ошибка генерации** (не warning, в отличие от правки выше disclaimer-маркера): там артефакт — сам файл и перегенерация его и исправляет, здесь расхождение означает, что сгенерированный код не соответствует ни одной опубликованной версии контракта.

Реализация: `internal/pkg/config/contract.go` (типы секций, валидация, разбор и рендер шаблонов команд), `internal/pkg/ds/contract.go` (то, что видит шаблон Makefile — генераторы с уже отрендеренными командами), `internal/pkg/generator/contract.go` (`processGenerators`, `resolveContracts`, `checkContractDrift`, `copyContracts`), `main/Makefile.tmpl` (цели). Провенанс приезжает из `specsource.ResolvedSpec.Commit` (`gitHeadCommit`). Полная документация: [docs/configuration/custom-generators.md](docs/configuration/custom-generators.md).

## Rewrite Refs

Opt-in флаг `rewrite_refs: true` в Rest- и JSONSchema-блоках `project.yaml`. После `CopySpecs`/`CopySchemas` стартер сканирует target-директорию (`api/rest/<svc>/<ver>/` или `api/schema/<name>/`) и переписывает кросс-директорные `$ref` (`../common/foo.yaml#/X`) в локальные (`./foo.yaml#/X`) — при условии, что sibling-файл с этим basename уже скопирован рядом и входит в whitelist `SpecTargetFiles`/`SchemaTargetFiles`.

Поддерживает YAML (через `yaml.Node` — комментарии сохраняются) и JSON (через line-regex). По умолчанию выключено, существующие проекты не затрагиваются.

Реализация: `internal/pkg/refrewrite/`. Подключение в `Generator.Generate()` сразу после `CopySpecs`/`CopySchemas` (`internal/pkg/generator/generator.go`). Полная документация: [docs/configuration/rewrite-refs.md](docs/configuration/rewrite-refs.md).

## Metrics Map

Секция `metrics.path` в `project.yaml` — ссылка на **карту метрик**, объявление всех метрик, которые сервису разрешено эмитить. Форма ссылки та же, что у `rest[].path` (local / https / git+ssh / git+https). Карта читается во время генерации и в проект **не копируется** — ничего ниже по цепочке её не читает, артефакт это сгенерированный пакет.

Карта не указана — ничего не включается (ни пакета, ни валидатора, ни панелей), поведение проекта прежнее.

Указана — генерируется:

- `internal/pkg/metrics/psg_metrics_gen.go` — по переменной-коллектору на метрику плюс `Register(prometheus.Registerer)` / `Collectors()` / `Unregister()`. Registerer явный, `promauto` не используется. Вызов `metrics.Register(application.GetMetrics())` проставляется в `psg_main_gen.go` (только для app с `sys`-транспортом);
- `configs/golangci-metrics.yml` + цель `make metrics-validate` — валидатор: `forbidigo` запрещает **вызовы** конструкторов метрик (`prometheus.New*`, `promauto.*`, позиционный `prometheus.NewDesc`) вне сгенерированного пакета. Литералы `prometheus.CounterOpts{…}` не запрещены намеренно — сами по себе они ничего не эмитят. Те же правила дублируются в `configs/golangci-lint.yml`, но **гейт — `metrics-validate`**: основной конфиг ставит `exclusions.generated: lax`, что выводит из проверки все `psg_*_gen.go`, включая секцию ручного кода ниже disclaimer-маркера;
- ряд **Service Metrics** в дашборде с канонической панелью на метрику (counter → `rate`, gauge → значение, histogram → квантили, summary → `quantile`-серия либо `rate(_sum)/rate(_count)`). Пороги и алерты не генерируются.

`name` в карте — **полное** имя метрики, без разбиения на namespace/subsystem/name: имя, собираемое в рантайме, не находится ни поиском по коду, ни дашбордом.

Свои метрики генератор описывает той же картой — `internal/pkg/metricsmap/embedded/internal-metrics.yaml` (Go-рантайм, ogen, Kafka-продюсер). Наборы складываются в одно пространство имён, совпадение имён — ошибка генерации, не переопределение. Внутренние поля `owner` (какой компонент конструирует коллектор) и `requires` (какая возможность проекта нужна) зарезервированы за внутренней картой и в карте сервиса отвергаются.

Реализация: `internal/pkg/metricsmap/` (парсинг, валидация, слияние, хелперы кодогенерации), `internal/pkg/ds/metrics.go` (параметры шаблонов, правила валидатора), `internal/pkg/grafana/metrics_panels.go` (панели), `Generator.processMetricsMap()`. Полная документация: [docs/configuration/metrics-map.md](docs/configuration/metrics-map.md).

## Grafana Annotations

Секция `grafana.annotations[]` в `project.yaml` объявляет записи-запросы, которые эмитятся в `.annotations.list[]` **каждого** сгенерированного дашборда рядом со стоковой записью builtIn. Сами события (деплой, рестарт, краш) постит сервис в рантайме через `POST /api/annotations`; без записи-запроса они складываются в Grafana, но ни на одном графике не рисуются.

Аннотации объявляются на сервис целиком — приложение **не** ссылается на них по имени, как на datasources (`$.Grafana.Annotations` в шаблоне, не `.Application.Grafana`). Запись — чистые данные, маппинг в JSON один к одному.

Поддерживается только теговый запрос к встроенному хранилищу (`datasource: grafana`, uid `-- Grafana --`): у запроса выражением к prometheus/loki принципиально другая форма `target`, это не ещё одно значение того же поля.

Дефолты применяются на границе config → шаблон (`ResolvedIconColor`/`ResolvedLimit`/`ResolvedEnable`/`ResolvedTags`), потому что пустой `iconColor` или `limit: 0` Grafana трактует как «не рисовать». `tags` принимает и список YAML, и строку через запятую — viper режет скаляр по `,` без trim, поэтому `ResolvedTags` обрезает пробелы, иначе тег ` restart` не совпал бы ни с одним событием. `enable` — указатель: явный `false` (объявлена, но по умолчанию выключена) надо отличать от отсутствующего поля.

Валидация (`config.GrafanaAnnotation.IsValid`, `Grafana.validateAnnotations`) отвергает: пустое имя, имя стоковой записи `Annotations & Alerts`, дубли имён, запрос без тегов, неподдерживаемый `datasource`, отрицательный `limit`, control-символы в `name`/`icon_color`/тегах (шаблонный `escapeJSON` их не экранирует → сломанный JSON доски). Объявленные аннотации при полном отсутствии datasources — тоже ошибка: дашборд генерируется только для приложений со ссылкой на datasource, записям было бы некуда попасть.

Реализация: `internal/pkg/grafana/config.go` (тип `Annotation`, константы встроенного хранилища и дефолтов), `internal/pkg/config/structs.go` (`GrafanaAnnotation`, валидация, резолверы), маппинг в `Generator.processConfig()`, вывод — `internal/pkg/templater/embedded/templates/grafana/dashboards/dashboard.json.tmpl`. Полная документация: [docs/configuration/infrastructure.md](docs/configuration/infrastructure.md#аннотации).

## Regeneration Behavior

### Disclaimer Marker (env-файлы и другие файлы с ручными правками)

Все файлы, помеченные disclaimer-комментарием, содержат маркер:

```
If you need you can add your code after this message
```

При регенерации **всё, что выше маркера, перезаписывается** содержимым шаблона. Сохраняется только то, что **ниже** маркера.

Для env-файлов (`.env-{appName}.example`, генерируется из `internal/pkg/templater/embedded/templates/app/files/.env.example.tmpl`) маркер инжектится автоматически с префиксом `#` (см. `internal/pkg/templater/disclaimer.go:81-142`). Точная строка маркера в env-файле:

```
# If you need you can add your code after this message
```

**Правило:** любые ручные блоки в `.env-*.example` (bearer-токены вида `OC_*__transport__rest__*__auth__token`, документация по custom OnlineConf-путям, project-specific переменные) **должны располагаться ниже** этого маркера. Иначе они будут стёрты при следующем `make regenerate`.

Если ручной блок уже потерян — посмотрите git history и перенесите его ниже маркера.

### Манифест прогона (`.project-config/meta.yaml`)

`meta.yaml` хранит версию схемы и **манифест последнего прогона** — по одной записи на каждый
файл, который записал генератор:

```yaml
version: 5
manifest:
    - {path: cmd/publicApi/psg_main_gen.go, kind: generated, hash: 8fe15f…}
    - {path: api/rest/example/v1/example.swagger.yml, kind: copied, hash: 490da3…}
```

- `path` — путь относительно корня проекта, через `/`; записи отсортированы по `path`, по одной
  на строку (meta.yaml меняется на каждом прогоне, который меняет вывод, поэтому конфликты
  должны резолвиться построчно; машинный способ — перегенерировать на смёрженном дереве).
- `kind: generated` — файл из шаблона, `hash` считается **только по сгенерированной части**
  (до строки disclaimer включительно), поэтому код ниже маркера на хеш не влияет.
- `kind: copied` — файл, скопированный как есть (спеки REST/gRPC, JSON Schema), `hash` по всему
  файлу.
- Файлы, которые генерируются один раз и дальше принадлежат пользователю (`go.mod`,
  `.gitignore`, `README.md`, `LICENSE.txt`, `CLAUDE.md`, `static/**`), в манифест **не**
  попадают.

Манифест даёт две вещи:

1. **Точную orphan-детекцию.** Удаляются только файлы, которые есть в предыдущем манифесте и
   выпали из текущего вывода. Посторонние файлы в дереве (`*_gen.go.orig` от merge-конфликта,
   бэкап сгенерированного файла, индекс code-intelligence инструмента) больше **не читаются** —
   и, значит, не роняют прогон и не удаляются, даже если содержат строку disclaimer.
2. **Детекцию ручных правок выше маркера.** Если хеш на диске не совпадает с манифестом,
   генератор печатает `warning: <путь>: generated part was changed …`. Это предупреждение, а не
   ошибка: правки выше маркера перезаписываются (force-режим — отдельная задача).

Хеши обновляются по диску после `post_generate` (goimports, `go mod tidy`, `make generate`
переписывают только что сгенерированные файлы) — иначе каждый следующий прогон сообщал бы о
дрейфе для всех файлов.

Проект, сгенерированный до `version: 5`, манифеста не имеет: первый прогон работает по-старому
(обход дерева + поиск disclaimer в содержимом) и записывает манифест, со второго — точный путь.
Реализация: `internal/pkg/meta/meta.go`, `templater.GetUserCodeFromFiles()`.

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

**Важно:** Тестовые конфиги (`test/docker-integration/configs/`) имеют `post_generate: []`, поэтому после генерации нужно вручную запустить `go mod tidy` перед проверкой сборки:

```bash
cd ~/Develop/tmp/test-app && go mod tidy && go build ./...
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
