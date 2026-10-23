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

**Одно исключение из направления зависимостей: `internal/pkg/occonf`.** Сгенерированные
библиотеки в `pkg/` (`pkg/app/restconfig`, `pkg/app/logger`, `pkg/drivers/*`) читают
конфигурацию, и читают её через сгенерированный пакет аксессоров — то есть `pkg/` →
`internal/pkg/occonf`. Альтернатива — чтобы каждая из них собирала пути OnlineConf сама, и
это ровно тот дефект, который убирает карта ключей (см. «OnlineConf Key Map»): путь,
собранный на месте вызова, не виден ни гварду, ни реестру, ни ревью. Эти пакеты и так не
переносимы между проектами (`pkg/drivers/telegram` импортирует `internal/app/constant`,
`pkg/app/restconfig` генерируется под выбранный логгер), так что «maximally reusable» для
них и раньше было неточным. Новых исключений заводить не нужно: всё, что читает
конфигурацию, идёт через `occonf`.

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
2. Transport: `/{serviceName}/transport/rest/{serverName}/{key}` — `serverName` is `{transportName}_{apiVersion}` (`api_v1`), the name the generated main hands to `rest.NewServer`
3. App-specific: `/{serviceName}/transport/rest/{serverName}/{appName}/{key}`

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

## Docker Build Args и Labels

Секция `docker:` помимо `image_prefix` несёт `build_args` (объявление build-аргументов образа) и `labels` (OCI-лейблы финальной стадии). Нужна потому, что `LABEL` статичен и не умеет вызвать `git`: версия/ревизия обязаны приехать через `--build-arg`, а стадия, которая их использует, обязана объявить `ARG` — а builder-стадия Dockerfile целиком генерируемая, и дописанный туда руками `ARG` стирался на каждом `make regenerate`.

Элемент `build_args` — либо имя (`IMAGE_VERSION`), либо объект `{name, default, stages}`. Из каждого аргумента генерируются четыре вещи: `ARG <name>` в перечисленных стадиях Dockerfile, `--build-arg <name>="$(<name>)"` в рецепте `docker-<app>`, `<name> ?= <default>` в Makefile (`?=` — приоритет за env/CLI, так переопределяет CI) и `<name>=${<name>:-}` в `build.args` приложения в `docker-compose-dev.yaml`.

`stages` по умолчанию `[final]`: `ARG` обесценивает build-кеш слоёв **после** себя, а версия меняется на каждом коммите; в финальной стадии `ARG`-блок генерируется в самом её конце (после `COPY`/`CMD`/`EXPOSE`). `[builder, final]` — только когда значение реально нужно при сборке бинарника.

Валидация на загрузке конфига (`config.Docker.Parse`): имя не может совпасть с `ARG`-ами шаблона (`binary`, `GOPROXY`) и переменными сгенерированного Makefile (`DOCKER_TAG`, `RELEASE`, …); ссылка `${NAME}` в значении лейбла требует аргумента, объявленного в стадии `final` (иначе лейбл молча отрендерился бы пустым). Ключи лейблов — только нижний регистр: viper приводит ключи конфига к нижнему регистру, а дотовые ключи (`org.opencontainers.image.version`) он разбивает в вложенные map'ы, которые `Parse` собирает обратно.

Реализация: `internal/pkg/config/docker.go` (парсинг/валидация), `internal/pkg/ds/docker.go` (`ds.DockerConfig` + хелперы шаблонов `BuilderArgs`/`FinalArgs`/`LabelDirective`/`BuildArgFlags`/`ComposeBuildArgs`), шаблоны `app/files/Dockerfile.tmpl`, `main/Makefile.tmpl`, `main/docker-compose-dev.yaml.tmpl`. Полная документация: [docs/configuration/docker.md](docs/configuration/docker.md).

## Lint Config

Секция `tools.lint_config` в `project.yaml` решает, кто владеет `configs/golangci-lint.yml` — файлом, чьи правила формируют вердикт merge-гейта. Два взаимоисключающих ключа (оба сразу — ошибка конфига), по умолчанию ни одного и поведение прежнее (генератор пишет файл целиком):

- `own: true` — файл принадлежит проекту. Генератор его **не пишет, не читает, не диффит и не удаляет**: путь уходит в `Generator.userOwnedPaths()`, то есть в тот же класс, что `go.mod`/`.gitignore`, и выпадает из манифеста. Включение в существующем проекте файл с диска не трогает — ровно поэтому `diffOrphans` проверяет `IsUserOwned` перед тем, как считать выпавший из вывода файл сиротой.
- `overrides: <URI>` — слой переопределений. Мерж идёт **во время генерации**: генератор рендерит шаблон, вливает документ проекта и пишет результат. Итоговый файл по-прежнему сгенерированный (правки выше disclaimer-маркера перезаписываются), в манифест попадает как `generated`; документ переопределений — проектный, в проект не копируется. URI — те же формы, что у `rest[].path`, с учётом `--sourceRef`.

Правила слияния: мапы сливаются рекурсивно, скаляры и списки заменяются, явный `null` удаляет ключ. Исключение — списки имён линтеров (`linters.enable/disable`, `formatters.enable/disable`): это операции над множеством, `enable` добавляет, `disable` убирает. `disable` разрешается на этапе генерации: линтер, который генератор включает явно, вычитается из `enable` (golangci-lint отвергает конфиг, где линтер одновременно в `enable` и `disable`), а имя, которого в `enable` нет, — это дефолт golangci-lint и оно уезжает в итоговый `disable`, иначе выключить дефолт нечем.

Без обоих ключей мерж не запускается вовсе и вывод побайтно совпадает с прежним. `configs/golangci-metrics.yml` и `make metrics-validate` (гейт карты метрик) генерируются независимо от секции. `tools.lint_exclude_paths` при `own: true` ни на что не влияет — генерация печатает warning, ключ остаётся валидным.

Реализация: `internal/pkg/lintconfig/` (правила слияния, `ConfigPath`), `Generator.processLintConfig()` / `lintBodyTransform()` / `userOwnedPaths()`, фильтр own-режима в `templater.GetMainTemplates()`, хук `templater.GenerateByTmplWithTransform()` (отдаёт трансформу тело шаблона без disclaimer-блоков — мерж парсит YAML, а блоки это комментарии вокруг него). Полная документация: [docs/configuration/lint-config.md](docs/configuration/lint-config.md).

## OnlineConf Key Map

У каждого сгенерированного проекта **всегда** есть полная карта ключей OnlineConf, к которым он
обращается, и она состоит из двух половин: ключи, которые читает сгенерированный код (объявляет
сам генератор, рядом с шаблонами), и ключи сервиса (`onlineconf.path` в `project.yaml` — ссылка на
карту в контрактном репозитории; форма та же, что у `metrics.path`: local / https / git+ssh /
git+https; `--sourceRef` действует; карта читается во время генерации и в проект **не копируется**).

**`onlineconf.path` добавляет ключи, а не включает фичу.** Пакет, валидатор, проверка на старте и
реестр генерируются в **каждом** проекте; без `onlineconf.path` карта состоит из половины
генератора. Раньше `onlineconf.path` был выключателем — и гвард был выключен ровно там, где
единственным читателем был сгенерированный код.

Генерируется:

- `internal/pkg/occonf/psg_occonf_gen.go` — **единственное** место, где вызывается библиотека
  `onlineconf`: типизированный аксессор на каждый ключ обеих половин, построитель пути
  `<Name>Path(...)`, фасад жизненного цикла (`Initialize`, `StartWatcher`, `StopWatcher`),
  `Paths()`, `Subscribe(ctx, paths, cb)` и `CheckRequired(ctx, app)`. Плюс
  `ActiveRecordConfig(ctx)` — только при `use_active_record: true`: ActiveRecord читает дерево
  сам через `activerecord.ConfigInterface`, а это весь набор читателей библиотеки, поэтому
  узкий интерфейс ему отдать нельзя. Это известная дыра в карте (чтения ActiveRecord не
  объявлены и в реестр не попадают), и она была всегда; теперь её хотя бы нет в проектах без
  ActiveRecord. Пакет назван
  `occonf`, не `onlineconf`: он оборачивает библиотеку с таким именем. Чтение идёт через
  `*IfExists`-формы — форма с дефолтом (`GetString(ctx, path, d...)`) при ошибке обращается к
  `d[0]` и паникует без дефолта;
- `configs/golangci-onlineconf.yml` + цель `make onlineconf-validate` — валидатор: `forbidigo`
  запрещает **любое** обращение к пакету `onlineconf` (`^onlineconf\.`) вне сгенерированного
  пакета. Запрет только `Get*` оставил бы `MakePath` свободным собирать путь, запрет только
  `MakePath` — строковый литерал. `psg_*_gen.go` больше **не** исключён: сгенерированный код
  читает через аксессоры, поэтому исключение не нужно — а оно выводило из проверки и секцию
  ручного кода ниже disclaimer-маркера. Те же правила дублируются в `configs/golangci-lint.yml`,
  но **гейт — `onlineconf-validate`**;
- `configs/onlineconf-tree.yaml` — реестр **всей** поверхности дерева: ключи карты сервиса
  (`origin: service`) и чтения сгенерированного кода (`origin: generated`, с `read_by` —
  шаблоном-читателем). Поля `accessor`, `read_via` (общий аксессор) и `owns` (с какой точки
  поддерево принадлежит сгенерированному коду). В рантайме не читается, он для ревью и diff-а.

**Сгенерированный код читает OnlineConf только через `occonf`** — все 15 шаблонов с чтениями плюс
`main` (жизненный цикл). Фактические пути, типы и значения по умолчанию при этом не изменились.

**Сигнатуру аксессора ключа сервиса задаёт объявление**, а не место вызова: `required` →
`(T, error)` с `ErrKeyNotSet`; `default` → `(T, error)` с дефолтом; ни того, ни другого →
`(T, bool, error)`. Схема карты **строгая**: поля `path`, `type`, `required`, `default`, `name`,
`description`; неизвестное поле — отказ генерации. `path` — **полный** путь; все записи в одном
дереве (карта одна на сервис); шаблоны со списковым сегментом (`…/<i>/…`) не принимаются — список
объявляется одним ключом с `type: struct`. Типы: `string`, `int` (int64), `bool`, `duration`,
`strings`, `struct`. `required` нельзя только у `struct`: проверка потребовала бы разобрать
значение, а тип назначения принадлежит вызывающему.

**Объявления генератора** (`<шаблон>.ockeys.yaml` рядом с шаблоном; из вывода генерации
исключаются в `GetTemplates`) несут `path`, `name`, `type`, `required`, `shared`, `read_via`,
`default` (проза), `description`. В пути два вида переменных сегментов: `{service}`,
`{transport}`, `{app}`, `{worker}`, `{client}`, `{event}` — измерения проекта, известные на
генерации, и `<urlPath>`, `<tgID>`, `<action>` — сегменты, которые сгенерированный код вычисляет в
рантайме. **Оба становятся параметрами аксессора**: сгенерированные читатели generic по
измерениям (провайдер REST получает имя сервера и приложения от рантайма). Поэтому **набор
аксессоров не зависит от измерений проекта** — от них зависят строки реестра и проверки на
старте: шаблон, который генерируется, зовёт свои аксессоры безусловно (провайдер REST
генерируется и в проекте, где единственный REST-транспорт — клиент). Запись без `type` —
построитель пути (имя обязано кончаться на `Path`), отдаёт генератору всё поддерево; запись с
`<runtimeСегментом>` — с него и ниже. `shared: true` — аксессор, путь которого целиком собирает
вызывающий (уровень логирования), поддерева не отдаёт, а конкретные пути объявляются у вызывающих
с `read_via: <Имя>`. Запись с `shared: true` — **единственная**, которая начинается не с
`{service}`, а с `<runtimeСегмента>`: префикс приходит уже абсолютным (его собирают
`occonf.RESTAppMetricsPath` / `occonf.WorkerPath`), и приписанное второй раз имя сервиса уводило
чтение на уровень глубже дерева — ровно дефект TOOLS-65. Пустой префикс (процессный логгер)
нормализует в корень дерева сам вызывающий: сборка пути пустой компонент не выбрасывает и дала бы
`//`. `scope` задаёт измерение раскрытия (`service`, `rest_server`, `rest_client`,
`grpc_client`, `kafka_producer`, `kafka_producer_event`, `worker_daemon`, `worker_telegram`,
`telegram_driver`).

`required` у ключа генератора **не меняет форму чтения** (читатель компонента, объявленного
`optional: true`, должен уметь увидеть отсутствие) — он означает, что ключ попадает в
`CheckRequired` **того приложения**, которое этот компонент запускает. Поэтому
`CheckRequired(ctx, app)` принимает имя приложения: ключи сервиса проверяются всегда, ключи
генератора — по компонентам приложения, за вычетом объявленных `optional`. Помечать `required`
можно только ключ, которого никакая настройка `project.yaml` не может сделать ненужным: таких
два — `security/tg_token` и `kafka/<client>/brokers`. host/port клиентов не помечены, потому что
клиент с `instantiation: dynamic` получает адрес аргументом и ключ не читает.

Совпадение пути из карты сервиса с путём генератора — ошибка генерации с указанием
шаблона-читателя: точное совпадение либо объявление внутри отданного генератору поддерева.
Проверка идёт по **фактическому** проекту: ключ измерения, которого в проекте нет (Kafka без
Kafka-клиента), сервис объявить может — второго читателя не возникает.

Связь объявлений с кодом держат тесты в `internal/pkg/templater/onlineconf_keys_test.go`:
`TestNoTemplateReadsOnlineConfDirectly` (шаблон вне `occonf`, обращающийся к библиотеке) и
`TestOnlineConfDeclarationsMatchAccessorCalls` (аксессор, который шаблон вызывает, обязан быть
объявлен этим же шаблоном, и наоборот).

Реализация: `internal/pkg/ockeymap/` (парсинг, валидация, хелперы кодогенерации; `gendecl.go` —
объявления генератора, раскрытие по проекту и по приложению, проверка совпадений),
`internal/pkg/templater/onlineconf_keys.go` (загрузка объявлений из embed FS, скан шаблонов),
`internal/pkg/ds/onlineconf.go` (единая модель аксессора для обеих половин, правила валидатора,
реестр), `Generator.processOnlineConfMap()` / `onlineConfSurface()` / `onlineConfAppSurface()`.
Полная документация (включая раздел миграции):
[docs/configuration/onlineconf-keys.md](docs/configuration/onlineconf-keys.md).

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

Манифест даёт пять вещей:

1. **Точную orphan-детекцию.** Удаляются только файлы, которые есть в предыдущем манифесте и
   выпали из текущего вывода. Посторонние файлы в дереве (`*_gen.go.orig` от merge-конфликта,
   бэкап сгенерированного файла, индекс code-intelligence инструмента) больше **не читаются** —
   и, значит, не роняют прогон и не удаляются, даже если содержат строку disclaimer.
2. **Детекцию ручных правок выше маркера** (`kind: generated`). Если хеш на диске не совпадает с
   манифестом, генератор печатает `warning: <путь>: generated part was changed …`. Это
   предупреждение, а не ошибка: правки выше маркера перезаписываются (force-режим — отдельная
   задача).
3. **Отказ перезаписать чужой файл (конфликт).** Файл из текущего вывода, который лежит на диске,
   но **записи в предыдущем манифесте не имеет**, генератор не писал — значит он пользовательский.
   Прогон падает **до записи чего-либо**, перечисляя все такие пути сразу:

   ```text
   conflict: 2 files exist and were not written by the generator — rename or remove them:
     - internal/app/constant/psg_constant_gen.go
     - api/rest/example/v1/example.swagger.yml
   ```

   Проверка покрывает все типы: с disclaimer-маркером (раньше молча перезаписывалось всё выше
   маркера), без него (раньше — невнятное `error split disclaimer in file …`) и `isFileIgnored`
   (`.json`, `.keep`, `*.mod` — раньше файл терялся целиком), а также `kind: copied` (спеки
   REST/gRPC, JSON Schema). Исключения: user-owned пути (`go.mod`, `.gitignore`, `README.md`,
   `static/**` — `templater.IsUserOwned`), pending rename (ищется по `OldDestName`, как записал
   предыдущий прогон) и проект без манифеста. Запись с пустым `hash` считается «нашей».
   Сентинел — `templater.ErrTargetConflict` (конкретный тип `*templater.TargetConflictError`
   со списком путей); `--dry-run` сообщает о конфликте так же.
4. **Запрет правки скопированных спек** (`kind: copied`). Расхождение хеша на диске с манифестом —
   **ошибка генерации**, ненулевой код выхода (см. ниже).
5. **Проверку дерева без генерации** — `--check` (в проекте `make check-generated`). Тот же
   вопрос, что п. 2 и 4, но заданный **до** записи, поэтому пригодный как гейт CI: п. 2 сейчас
   печатает предупреждение в момент, когда правка уже затирается. Проверка сверяет **весь**
   манифест с диском, ничего не пишет (включая `meta.yaml` — обновление хешей это работа
   генерации, иначе гейт починил бы себя) и на расхождении печатает по файлу путь, `kind` и оба
   хеша.

   Расхождение: правка выше маркера (`generated`), правка файла целиком (`copied`), **удалённый
   disclaimer-маркер** (сгенерированную область нечем отделить — вместо прежнего
   `error split disclaimer in file …`), **отсутствие файла на диске** (дерево не содержит того,
   что сгенерировано) и симлинк на месте сгенерированного файла. Код ниже маркера — не
   расхождение; посторонние файлы и user-owned пути не проверяются вовсе (п. 1, `IsUserOwned`).

   **Читает ровно `meta.yaml` + перечисленные в нём файлы.** `project.yaml` не загружается, спеки
   не резолвятся, сеть не нужна — поэтому `--config`/`--configDir`/`--sourceRef` вместе с
   `--check` игнорируются с предупреждением, а вердикт не зависит от чужого тика в контрактном
   репозитории. Обратная сторона: отвечает «сгенерированную область не правили», а не «дерево
   актуально по контрактам» — второе по-прежнему только регенерацией.

   Невозможность дать вердикт — **ошибка** (ненулевой код), не зелёный прогон: нет `meta.yaml`
   (`ErrMetaNotFound`), манифеста нет (`ErrNoManifest`, проект старше `version: 5`), `version`
   новее, чем знает бинарь (`ErrFutureMeta` — проверять надо версией из `GPSVERSION`).

Хеши обновляются по диску после `post_generate` (goimports, `go mod tidy`, `make generate`
переписывают только что сгенерированные файлы) — иначе каждый следующий прогон сообщал бы о
дрейфе для всех файлов.

Проект, сгенерированный до `version: 5`, манифеста не имеет: первый прогон работает по-старому
(обход дерева + поиск disclaimer в содержимом, конфликты не детектируются) и записывает манифест,
со второго — точный путь.
Реализация: `internal/pkg/meta/meta.go`, `templater.GetUserCodeFromFiles()`,
`internal/pkg/templater/conflict.go`, `generator.checkCopyTargetConflicts()`; проверка —
`internal/pkg/treecheck/` (вердикт и его рендер), `runCheck()` в
`cmd/go-project-starter/main.go` (ветка до загрузки конфига), цель `check-generated` в
`main/Makefile.tmpl`. Сентинел потерянного маркера — `templater.ErrDisclaimerNotFound`.

### Скопированные спеки не правятся внутри проекта (`kind: copied`)

Спеки (`rest[]`/`grpc` → `api/rest/<svc>/<ver>/`, `api/grpc/<svc>/`; `jsonschema[]` →
`api/schema/<name>/`) копируются в проект из контрактного источника. Править их **внутри проекта**
бессмысленно: править надо в контрактном репозитории, на который указывает `path`. Поэтому в
отличие от правки выше disclaimer-маркера (там warning — у автора есть куда перенести код, ниже
маркера) здесь генератор **останавливается**:

```text
failed to generate: a file copied by the generator was edited inside the project tree:
  api/rest/example/v1/example.swagger.yml (source: ./example.swagger.yml)
files copied by the generator are not an edit point: change them at the source (the contract
repository the path points to) and re-run the generator. To drop the local edits, restore the
files from version control.
```

Свойства проверки:

- **Идёт pre-pass'ом до любой записи** (первым делом в `Generate()`), поэтому при ошибке дерево
  проекта остаётся нетронутым — правку ещё можно вытащить и перенести в контрактный репозиторий.
  `--dry-run` тоже падает: это предпросмотр прогона, а прогон не состоится.
- **Сравнивается диск с записанным хешем**, а не с источником: контракт мог законно уехать вперёд
  в своём репозитории, и это не дрейф.
- **В тексте — исходный `path` из `project.yaml`** (для remote — URI с именем token-переменной, без
  значения), а не staging-путь: pre-pass специально выполняется **до** `resolveAllSources`, который
  подменяет `SpecPath` на временный staging.
- **Нет записи — нет ошибки.** Проект до `version: 5` (манифеста нет) и канал, добавленный в
  `project.yaml` после прошлого прогона, проходят молча. Отсутствующий на диске файл тоже не дрейф —
  копирование его создаст.
- **Force-флага нет намеренно.** Выход из ошибки — `git restore` либо удаление файла (удаление —
  не правка: терять нечего, файл скопируется из источника заново). Перенос правки в локальный
  источник — два шага: скопировать в источник **и** вернуть/удалить файл в проекте (проверка
  сравнивает с хешем, не с источником). Для контрактного репо — ветка + `SOURCE_REF`. Пошагово —
  [docs/workflow/regeneration.md](docs/workflow/regeneration.md#спеки-правятся-в-контрактном-репозитории-не-в-проекте).

Реализация: `Manifest.CheckCopiedDrift` + `CopiedDriftError`/`CopiedFileDriftHint`
(`internal/pkg/meta/drift.go`), сбор целей — `Generator.copiedSpecTargets()`
(`internal/pkg/generator/copy.go`), вызов — в начале `Generator.Generate()`. Формулировка
ошибки и подсказка лежат в `meta`, чтобы другие каналы, копирующие файлы в проект, переиспользовали
их, а не придумывали свои.

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
