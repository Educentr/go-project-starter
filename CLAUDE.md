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

### 4. Skills Plugin Update (ОБЯЗАТЕЛЬНО)

Инструкции, по которым AI-агенты работают с генератором, лежат в
[`Educentr/claude`](https://github.com/Educentr/claude/tree/main/plugins/go-project-starter) —
агент читает их вместо этого репозитория. Плагин, оставшийся на прошлой версии, продолжает учить
прошлой версии: ставит `go install …@<прошлый тег>` и выдаёт снятые ограничения как актуальные.

```bash
# что в плагине ещё указывает на прошлую версию (код 1 — не догнал)
make check-skills-plugin SKILLS_VERSION=vX.Y.Z
```

Обновляется отдельным PR в `Educentr/claude` (отдельный репозиторий → отдельная задача с меткой
`repo:github.com/Educentr/claude`): версия в version-statement **каждого** скила, пины команд
(`@vX.Y.Z`, `STARTER_VERSION`), ссылки на документацию по тегу, затронутые релизом `references/`,
`version` в `.claude-plugin/plugin.json`.

Что с чем связано и что намеренно не правится (исторические упоминания версий) —
[docs/development/skills-plugin.md](docs/development/skills-plugin.md).

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

# Release checks
make check-skills-plugin SKILLS_VERSION=vX.Y.Z  # Claude skills plugin caught up with the release

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

CLI и queue-worker spec'и (которые парсятся inline во время загрузки конфига) резолвятся через отдельный `resolveInlineSpec` хелпер: для local — joinpath, для remote — материализация в staging прогона и парсинг (в `api/` файл не копируется, рантайму он не нужен). REST/gRPC/JSONSchema — через основной `resolveAllSources()` pre-pass, файл остаётся живым для копирования в `api/`. Резолвер у прогона **один** на оба пути, staging освобождает `Generator.Close()` — см. «Один `specsource.Resolver` на прогон».

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

## Веб-приложение (`web`)

Секция `web` объявляет фронтенд как сущность: `dir` (исходники), `install` (зависимости),
`build` (сборка), `output` (каталог, который раздаёт транспорт `generator_template: static`),
`node_image` и опциональный `codegen`. Команды — проектные, как в `generator`: стартер владеет
проводкой (make-цели, стадия сборки в образе), а не выбором сборщика и генератора клиента. Без
секции вывод не меняется ни на байт (`TestGenerateWithoutWebApp_OutputUnchanged`).

**Привязка фронта к приложению выводится, а не объявляется вторично:** фронт попадает в образ
того приложения, чей `static`-транспорт раздаёт его `output` (`ds.WebApps.ForApp`). `output`,
который никто не раздаёт, — ошибка конфига: это сборка, результат которой не попадает ни в
ответ сервиса, ни в образ.

**`codegen` называет входы по имени, а не по пути**, и состоит из **каналов** — `rest:` и
`jsonschema:`, у каждого свои `cmd` и `inputs`. Команда на канал, а не одна на оба: генератор TS
читает один из двух форматов (`openapi-typescript` — OpenAPI, `json2ts` — JSON Schema), поэтому
единственная команда падает на первой же спеке чужого формата, а вместе с ней — весь
`make generate`, в который входит `generate-web-<name>`. Команде отдаётся **своя копия** спеки,
которую стартер уже забрал (`api/rest/<имя>/<версия>/…`, `api/schema/<имя>/…`) — второй копии не
возникает, в отличие от `contract:`, которому нужны свои `path` и `copy`. Путь приезжает
**относительно `dir`**: все команды записи выполняются в `dir`, где лежит манифест фронта.
Переменные `cmd`: `.Spec`, `.SpecName`, `.Input`; поля `target` нет намеренно — вход может
принести несколько файлов, и единственный путь вывода они бы перезаписали друг другом.

Валидация секции на загрузке конфига: `dir` внутри `output` отвергается (каталог сборки целиком
уходит в `.gitignore` и перезаписывается в образе — исходники в нём молча перестали бы
версионироваться), как и два фронта с одним `output` (в образе выигрывает стадия, скопированная
последней). Многострочная команда (`install`, `build`, `cmd`) — тоже ошибка загрузки: команда
становится одной строкой make-рецепта, хвост второй строки ломает Makefile целиком
(`missing separator`). Та же проверка стоит на командах секции `generator`
(`validateSingleLineCommand` в `internal/pkg/config/contract.go`).

Цели: `install-web-<name>` → в `install-tools`, `build-web-<name>`, `generate-web-<name>` → в
`generate` (после встроенных генераторов, то есть и в `regenerate` через `call_generate`).
**`docker-<app>` от `build-web-<name>` не зависит:** фронт собирается node-стадией внутри
Dockerfile (`COPY --from=web-<name> /src/<output>/ /<output>/` поверх `ADD <output>`), поэтому
node на хосте и в CI-раннере не нужен, а node-тулчейн не попадает в runtime-образ.
`build-web-<name>` — для локального цикла.

Команды рендерятся **до** resolve-пре-пасса: имена скопированных файлов выводятся из
`specsource.ParseSpecSource(...).TargetFilename()` — так же, как их выводит `resolveAllSources`
(иначе Makefile рендерился бы раньше, чем известны пути).

Реализация: `internal/pkg/config/web.go` (типы секции, валидация, рендер команды),
`internal/pkg/ds/web.go` (модель для шаблонов, `ForApp`, константы `static`-каталога и
node-образа), `internal/pkg/generator/web.go` (`processWebApps`), шаблоны
`main/Makefile.tmpl`, `app/files/Dockerfile.tmpl`, `main/.gitignore.tmpl`. Полная документация:
[docs/configuration/web.md](docs/configuration/web.md).

## Rewrite Refs


Opt-in флаг `rewrite_refs: true` в Rest- и JSONSchema-блоках `project.yaml`. После `CopySpecs`/`CopySchemas` стартер сканирует target-директорию (`api/rest/<svc>/<ver>/` или `api/schema/<name>/`) и переписывает кросс-директорные `$ref` (`../common/foo.yaml#/X`) в локальные (`./foo.yaml#/X`) — при условии, что sibling-файл с этим basename уже скопирован рядом и входит в whitelist `SpecTargetFiles`/`SchemaTargetFiles`.

Поддерживает YAML (через `yaml.Node` — комментарии сохраняются) и JSON (через line-regex). По умолчанию выключено, существующие проекты не затрагиваются.

Реализация: `internal/pkg/refrewrite/`. Подключение в `Generator.Generate()` сразу после `CopySpecs`/`CopySchemas` (`internal/pkg/generator/generator.go`). Полная документация: [docs/configuration/rewrite-refs.md](docs/configuration/rewrite-refs.md).

## Metrics Map

Секция `metrics.path` в `project.yaml` — ссылка на **карту метрик**, объявление всех метрик, которые сервису разрешено эмитить. Форма ссылки та же, что у `rest[].path` (local / https / git+ssh / git+https). Карта читается во время генерации и в проект **не копируется** — ничего ниже по цепочке её не читает, артефакт это сгенерированный пакет.

Карта не указана — ничего не включается (ни пакета, ни валидатора, ни панелей), поведение проекта прежнее.

Указана — генерируется:

- `internal/pkg/metrics/psg_metrics_gen.go` — по переменной-коллектору на метрику плюс `RegisterFor(app, prometheus.Registerer)` / `CollectorsFor(app)` (поверхность одного приложения) и `Register(prometheus.Registerer)` / `Collectors()` / `Unregister()` (вся поверхность сервиса). Registerer явный, `promauto` не используется. Вызов `metrics.RegisterFor("<app>", application.GetMetrics())` проставляется в `psg_main_gen.go` (только для app с `sys`-транспортом и только если этому app есть что регистрировать — иначе импорт пакета был бы неиспользованным);
- `configs/golangci-metrics.yml` + цель `make metrics-validate` — валидатор: `forbidigo` запрещает **вызовы** конструкторов метрик (`prometheus.New*`, `promauto.*`, позиционный `prometheus.NewDesc`) вне сгенерированного пакета. Литералы `prometheus.CounterOpts{…}` не запрещены намеренно — сами по себе они ничего не эмитят. Те же правила дублируются в `configs/golangci-lint.yml`, но **гейт — `metrics-validate`**: основной конфиг ставит `exclusions.generated: lax`, что выводит из проверки все `psg_*_gen.go`, включая секцию ручного кода ниже disclaimer-маркера;
- ряд **Service Metrics** в дашборде приложения с канонической панелью на метрику, которую публикует это приложение (counter → `rate`, gauge → значение, histogram → квантили, summary → `quantile`-серия либо `rate(_sum)/rate(_count)`). Пороги и алерты не генерируются.

`name` в карте — **полное** имя метрики, без разбиения на namespace/subsystem/name: имя, собираемое в рантайме, не находится ни поиском по коду, ни дашбордом.

**`type: GaugeSet` — семейство состояния**, про которое тик знает всю правду целиком (состояние флота, аккаунтов, кредов): набор серий публикуется **целиком**, серия, которой нет в последнем наборе, исчезает. Это единственный `type`, за которым не стоит конструктор `client_golang`, — потому что `GaugeVec` такое выразить не может: `Reset()` + заполнение публикует пустое семейство на время заполнения, а `/metrics` опрашивают по чужим часам, и опрос в этом окне видит пропавшее семейство (дыра на панели, ложный no-data у каждого правила). Генерируется коллектор `GaugeSet[S]` (один на пакет, под `{{ if .Metrics.HasGaugeSet }}` — в карте без таких записей вывод побайтно прежний) плюс на каждое объявление: `<Имя>Sample` (по строковому полю на метку + `Value`), `<Имя>Func` и переменная-коллектор. Источник набора ставится методом на коллекторе — `SetFunc` (pull, зовётся на каждом `Collect`) или `Set` (push, законченный результат тика); `Set` — это `SetFunc` над константным источником, поэтому слот один и ручного атомарного свопа в вызывающем коде не остаётся. До первого вызова семейство отдаёт пустой набор (как свежий `GaugeVec`), а не ошибку.

**Плохой набор ограничен своим семейством:** дубль комбинации меток внутри набора сворачивается (выигрывает **последний** сэмпл — паритет с `GaugeVec`, который заполняют по серии), а сэмпл, который отверг `NewConstMetric` (невалидный UTF-8 в значении метки), **пропускается**. Ни то, ни другое не уезжает в реестр как `NewInvalidMetric`: `/metrics` отдаётся через `promhttp.HandlerFor(..., promhttp.HandlerOpts{})`, где по умолчанию `HTTPErrorOnError`, то есть любая ошибка `Gather` — это HTTP 500 на весь ответ и все метрики приложения вне опроса (`up=0`). Тот же класс дефекта, который закрывает сам `GaugeSet`, только шире. Поведение зафиксировано в контракте семейства внутри сгенерированного проекта (`TestStateFamilyBadSampleDoesNotFailTheScrape` в `test/docker-integration`), в репозитории генератора — текстовый пин формы `Collect`.

Сэмпл — **структура**, а не срез значений меток: «неверное число значений меток» становится непишущимся кодом, а не ошибкой рантайма. Цена — метки обязаны выжить превращением в поля, поэтому отвергаются метка, занимающая поле `Value`, и две метки, сворачивающиеся в одно поле (`ockeymap`-подобный класс ошибок: иначе несобираемый пакет в проекте, которого проект не писал). `GaugeSet` несовместим с `external`/`owner` — оба означают «коллектор строится не здесь». Мутабельного глобала при этом не появляется: глобальна только переменная-коллектор (как и у остальных метрик, под `//nolint:gochecknoglobals`), а подменяемое состояние живёт в её `atomic.Pointer`-поле — TOOLS-31 запрещает переприсваиваемые package-level переменные, а не объекты с внутренним состоянием. До этого типа такие семейства (их 24) писались руками и стояли в карте как `external: true`, то есть как чужие метрики, — миграция описана в документации.

**Пространство идентификаторов пакета коллекторов проверяется целиком.** Объявление порождает экспортированные идентификаторы (сворачивание разделителей поднимает первый символ), и их больше одного: переменная-коллектор у каждой записи, плюс `<Имя>Sample` и `<Имя>Func` у `GaugeSet`, плюс `GaugeSet` на весь пакет, как только в карте есть хотя бы одно семейство. Сверяется один набор: эти имена между собой и против того, что пакет объявляет сам (`Register`, `RegisterFor`, `Collectors`, `CollectorsFor`, `Unregister`, `ErrUnknownApp`) — совпадение это ошибка генерации с указанием обеих сторон, а не `done` над пакетом, который дважды объявляет идентификатор и потому не собирается (неэкспортированные имена пакета и локальные имена импортов недостижимы и не резервируются). Зарезервированный набор следует фактическому выводу: `GaugeSet` занят только в карте с семейством — иначе отвергалось бы имя `gauge_set`, которое генератор сегодня обрабатывает верно. Список держит в синхроне с шаблоном `assertDeclarationsCollideWithNothing` (`test/metrics_map_state_family_test.go`), вычитывая идентификаторы обратно из сгенерированного пакета — как `assertAccessorsShadowNothing` для `occonf`.

**Поле `apps` — кто публикует запись.** Карта описывает поверхность **сервиса**, а приложений у сервиса бывает несколько; коллектор, зарегистрированный в процессе, который его никогда не инкрементирует, даёт всегда-нулевую серию, неотличимую от сломанной фичи. `apps: [<app>, …]` (или скаляр через запятую) ограничивает и регистрацию, и панели дашборда перечисленными приложениями; запись без `apps` публикуют все приложения с `sys`-транспортом — поведение до появления поля, так что существующие карты не меняются. Единственная ошибка генерации здесь — имя, которого нет среди приложений проекта (`metricsmap.ErrUnknownApp`): опечатка не должна молча оставить метрику без публикации. Написанный, но пустой список (`apps: ""`, `apps: []`) — тоже ошибка: для `labels` пустой список значит «меток нет», для `apps` он значил бы «публикуют все», то есть противоположность тому, как строка читается.

**Приложение без `sys`-транспорта `apps` называть можно** — и это главный случай, ради которого поле существует: эндпоинта `/metrics` у процесса нет, но собственный реестр быть может (CLI, приборы которого намеренно живут в пакетно-приватных реестрах). Такому приложению достаются **только записи, где оно названо явно**: запись без `apps` по-прежнему значит «все приложения с `/metrics`», иначе серверные метрики сервиса утекли бы в приватный реестр CLI — обратная сторона того же дефекта. Коллекторы попадают в `collectorsByApp`, то есть доступны через `CollectorsFor`/`RegisterFor`, но сгенерированный `main` вызова не содержит и пакет не импортирует (регистрировать некуда) — регистрирует собственный код процесса, в свой реестр. Дашборд такое приложение получает (дашборд следует за datasource), но ряда Service Metrics в нём нет — скрейпить нечем. Приложение, у которого нет ни `/metrics`, ни явно адресованных записей, в `collectorsByApp` не попадает вовсе: пустая запись превратила бы опечатку в `RegisterFor` в успешную регистрацию ничего. Вторую половину вопроса — метрики, которые конструирует компонент генератора (`owner: kafka_producer`), — `apps` не касается: они раскрываются по тому, в каких приложениях этот компонент есть (`AppScope.Components`), поэтому панели продюсера видит только дашборд приложения с продюсером.

Свои метрики генератор описывает той же картой — `internal/pkg/metricsmap/embedded/internal-metrics.yaml` (Go-рантайм, ogen, Kafka-продюсер). Наборы складываются в одно пространство имён, совпадение имён — ошибка генерации, не переопределение. Внутренние поля `owner` (какой компонент конструирует коллектор) и `requires` (какая возможность проекта нужна) зарезервированы за внутренней картой и в карте сервиса отвергаются.

Реализация: `internal/pkg/metricsmap/` (парсинг, валидация, слияние, хелперы кодогенерации; `apps.go` — `AppScope`, `Publishes`, `ValidateApps`; `GaugeSet` — `IsGaugeSet`/`HasLabels`, `gaugeSetProblems`/`sampleFieldProblems` в `validate.go`, `SampleFields`/`SetType` в `codegen.go`; `idents.go` — `ValidateIdents`/`ReservedIdents`, пространство идентификаторов пакета), `internal/pkg/ds/metrics.go` (параметры шаблонов, правила валидатора, `ForApp`/`CollectorsByApp`/`HasGeneratedFor`/`HasGaugeSet`), `internal/pkg/grafana/metrics_panels.go` (панели), `Generator.processMetricsMap()` + `Generator.resolveMetricsApps()` (второй шаг — потому что карта парсится до того, как построены приложения: её наличие нужно `collectFiles`). Полная документация: [docs/configuration/metrics-map.md](docs/configuration/metrics-map.md).

## Ранняя точка расширения `main` (`earlyInit`)

Оба сгенерированных `main` — сервисный (`app/cmd/main.go.tmpl`) и CLI (`app/cmd_cli/main.go.tmpl`) —
зовут `earlyInit()` **после загрузки `.env` и до `occonf.Initialize`**, одной строкой в одном и том
же месте. До этого первый пользовательский код выполнялся уже после того, как дерево конфигурации
собрано (`UserSetFunc.SetFunc` у сервиса, обработчик команды у CLI), поэтому работа, которая обязана
случиться раньше — поправить окружение, из которого `Initialize` строит дерево (снять
`ONLINECONFIG_FROM_ENV`, когда `OC_*`-ключей нет, чтобы открылась существующая `TREE.cdb`), сделать
re-exec, проверить `os.Args`, зафиксировать идентичность процесса, — делалась только из `init()` с
`//nolint:gochecknoinits`, то есть в обход того, что генератор сам запрещает (TOOLS-30).

Механика — та же, что у `customize()` telegram-воркера и `UserSetFunc`: `EmptyEarlyInit` несёт
`earlyInit()`, который ничего не делает, `UserEarlyInit` его встраивает, а метод с тем же именем,
объявленный проектом на `*UserEarlyInit` (свой файл пакета `main` или секция ниже
disclaimer-маркера), перекрывает промотированный. Правки в `go-project-starter-runtime` не нужны —
перекрытие работает внутри пакета, а у CLI `UserSetFunc` нет вовсе.

**Точка одна, и она после `.env`.** `godotenv.Load` не перезаписывает переменные, которые уже есть
в окружении, но добавляет те, которых нет: хук, вызванный раньше, не увидел бы `OC_*` из `.env` и
снял бы `ONLINECONFIG_FROM_ENV` по ошибке, а снятая им переменная вернулась бы, будь она записана в
самом `.env`. Остальным сценариям (`os.Args`, `stdout`, re-exec, экспорт идентичности) годятся обе
точки, поэтому вторая (`BeforeEnv`) не заводится.

Ошибка хука останавливает процесс: логгера на этом шаге ещё нет, поэтому сообщение идёт в `stderr`
через `log` (как и о неудачном `Initialize`), а код выхода — `ExitCodeErrorEarlyInit`, **добавленный
в конец** блока `iota` в обоих шаблонах: коды выше — наблюдаемый контракт процесса, вставка в
середину перенумеровала бы их.

Тесты: `test/early_init_test.go` (объявления, вызов и его позиция между `godotenv.Load` и
`occonf.Initialize` — позиция проверяется по AST, потому что `occonf.Initialize` упомянут и в
комментарии выше вызова; порядок кодов выхода), `assertEarlyInitHook` в
`test/docker-integration/integration_test.go` (проект с перекрытым хуком собирается, хук
действительно выполняется до `Initialize`, отказ даёт `ExitCodeErrorEarlyInit` и команда не
выполняется). Документация: [docs/configuration/applications.md](docs/configuration/applications.md#ранняя-инициализация-процесса-earlyinit),
миграция с `init()` — [DEPRECATION.md](DEPRECATION.md).

## Канал вывода метрик (`applications[].metrics.export`)

**Реестр метрик инициализируется у каждого приложения, всегда** — `application.InitMetrics(mainCtx)`
в `cmd/<app>/psg_main_gen.go` безусловен, как и регистрация коллекторов карты метрик. Реестр
принадлежит приложению, а не эндпоинту `/metrics`: middleware REST-транспорта регистрируется в нём
(`MustRegister`) во время инициализации транспорта, поэтому приложение, которому реестр не создали,
умирало на nil-разыменовании ещё до первого запроса — ровно дефект TOOLS-94, и генератор такую
конфигурацию принимал молча.

Настраивается **канал вывода** — куда метрики уходят: `pull` (`/metrics` на `sys`-транспорте),
`push` (внешний приёмник; значение зарезервировано, не реализовано — генерация падает) и `none`
(собираются и не отдаются). `none` — рабочая конфигурация: до процесса на машине пользователя
скрейпер не дотянется, а лишний слушающий порт — поверхность без читателя.

Ключ **необязателен**: без него канал выводится из транспортов приложения (есть `sys` → `pull`,
нет → `none`), поэтому существующие `project.yaml` не меняются ни в тексте, ни в смысле.
Объявленное значение проверяется против транспортов: `pull` без `sys` и `none` при `sys` — ошибки
конфигурации, а не предпочтение. Проверка живёт на `Config`, а не на `Application.IsValid`, потому
что «sys-ность» транспорта видна только по его `generator_template`.

Реализация: `internal/pkg/config/metrics_export.go` (`AppMetrics`, `Config.ResolveAppMetricsExport`,
сентинелы), `ds.MetricsExport` (`internal/pkg/ds/metrics_export.go`) + `App.MetricsExport`
(`internal/pkg/ds/const.go`), шаблон `app/cmd/main.go.tmpl`. Документация:
[docs/configuration/applications.md](docs/configuration/applications.md#метрики-приложения-metricsexport).

## Docker Build Args, Labels и runtime-стадия приложения

Секция `docker:` помимо `image_prefix` несёт `build_args` (объявление build-аргументов образа) и `labels` (OCI-лейблы финальной стадии). Нужна потому, что `LABEL` статичен и не умеет вызвать `git`: версия/ревизия обязаны приехать через `--build-arg`, а стадия, которая их использует, обязана объявить `ARG` — а builder-стадия Dockerfile целиком генерируемая, и дописанный туда руками `ARG` стирался на каждом `make regenerate`.

Элемент `build_args` — либо имя (`IMAGE_VERSION`), либо объект `{name, default, stages}`. Из каждого аргумента генерируются четыре вещи: `ARG <name>` в перечисленных стадиях Dockerfile, `--build-arg <name>="$(<name>)"` в рецепте `docker-<app>`, `<name> ?= <default>` в Makefile (`?=` — приоритет за env/CLI, так переопределяет CI) и `<name>=${<name>:-}` в `build.args` приложения в `docker-compose-dev.yaml`.

`stages` по умолчанию `[final]`: `ARG` обесценивает build-кеш слоёв **после** себя, а версия меняется на каждом коммите; в финальной стадии `ARG`-блок генерируется в самом её конце (после `COPY`/`CMD`/`EXPOSE`). `[builder, final]` — только когда значение реально нужно при сборке бинарника.

Валидация на загрузке конфига (`config.Docker.Parse`): имя не может совпасть с `ARG`-ами шаблона (`binary`, `GOPROXY`) и переменными сгенерированного Makefile (`DOCKER_TAG`, `RELEASE`, …); ссылка `${NAME}` в значении лейбла требует аргумента, объявленного в стадии `final` (иначе лейбл молча отрендерился бы пустым). Ключи лейблов — только нижний регистр: viper приводит ключи конфига к нижнему регистру, а дотовые ключи (`org.opencontainers.image.version`) он разбивает в вложенные map'ы, которые `Parse` собирает обратно.

**Своя секция `docker:` есть и у приложения** (`applications[].docker`) — она описывает **финальную (runtime) стадию** его `Dockerfile-<app>`, которая раньше была зафиксирована (`FROM alpine` + `CMD ["/service"]`): стадия генерируется, пользовательский маркер спасает только то, что ниже него, поэтому другой дистрибутив или рантайм-зависимость приходилось заводить отдельным `Dockerfile-<app>.custom` с переопределённой целью `docker-<app>`. Поля: `base_image` (по умолчанию `alpine`), `packages` (одна инструкция `RUN` **до** `COPY` бинарника — слой переиспользуется, пока не изменится список; индекс/кеш удаляется в той же инструкции), `package_manager` (`apk`/`apt`/`dnf`; выводится из `base_image` для `alpine`/`debian`/`ubuntu`/`fedora`, для остальных образов с `packages` требуется явно — угаданный менеджер дал бы Dockerfile, падающий на сборке), `workdir` (по умолчанию инструкции нет), `entrypoint` (**заменяет** `CMD`, а не дополняет: `CMD` уехал бы энтрипойнту аргументом), `platforms` (целевые платформы образа). Builder-стадия не настраивается — она компилирует статический Go-бинарник, её образ задаёт `tools.golang_version`. Приложение без секции получает прежнюю стадию (`FROM alpine` + `CMD ["/service"]`).

**Финальная стадия не зависит от shell.** `/etc/onlineconf` создаётся в builder-стадии и приезжает в финальную через `COPY --from=builder` (владелец и права те же — `root:root`, `0755`) вместо прежнего `RUN mkdir -p /etc/onlineconf` — это единственная строка, изменившаяся у **всех** проектов. Без `packages` в финальной стадии не остаётся ни одной `RUN`, поэтому shell-less базовый образ (`gcr.io/distroless/static`, `scratch` — самый частый рантайм статического Go-бинарника) собирается. `packages`, наоборот, требуют от образа shell и пакетного менеджера: для distroless их объявлять нечем, и явный `package_manager` тут не помогает. Та же инвариантность нужна и кросс-сборке: `RUN` в runtime-стадии исполнялся бы для целевой архитектуры, то есть потребовал бы QEMU ради одного `mkdir`.

Платформы образа объявляются **у приложения** (`applications[].docker.platforms`, элемент — `linux/<arch>`), а не на весь проект: приложение, которое запускают на машинах разработчиков, должно работать и на amd64, и на arm64, а серверным приложениям того же проекта хватает одной платформы. Без ключа вывод не меняется ни на байт (`TestGenerateWithoutDockerPlatforms`).

Объявленные платформы меняют Dockerfile и Makefile. Dockerfile **кросс-компилирует вместо эмуляции** (`FROM --platform=$BUILDPLATFORM`, `ARG TARGETARCH`, `GOARCH=${TARGETARCH:?…}` перед `make build_in_docker-<app>`; форма `:?` — гвард против пустого `TARGETARCH`, который молча собрал бы бинарь под архитектуру сборщика). В Makefile сборка идёт через `docker buildx` с явным `--platform`: одна платформа — обычный образ, он попадает в локальное хранилище (`--load`), и публикация остаётся прежним `docker push`; несколько — OCI index, а его локальное хранилище образов держать не умеет, поэтому сборка остаётся в кеше билдера (`--output type=cacheonly`), а публикует index сама сборка с `--push`. Цели зависят от `buildx-builder`, который создаёт билдер с драйвером `docker-container` (драйвер по умолчанию не умеет ни чужую платформу, ни index) — иначе сгенерированный проект требовал бы ручной настройки buildx.

Платформы у приложения без docker-артефакта — ошибка генерации (ключ формирует рецепты образа, генерировать было бы нечего); проверка живёт в `Generator.processConfig`, где известен эффективный список артефактов приложения, а не в `config`, где дефолт артефактов ещё не применён.

**Что проект дописывает в runtime-стадию, он объявляет, а не пишет Dockerfile'ом.** Четыре ключа секции приложения: `copy` (каталоги проекта в образ), `env` (`ENV` финальной стадии), `fetch` (файл по URL) и `stages` (вспомогательная стадия, из которой копируют). Они закрывают остаток живого случая #14 — бинарь из релиза с `${TARGETARCH}` в имени, CLI из npm, каталог `template/` с `ENV` на него, — не снимая инварианта, на котором держатся `packages` и `platforms`: в финальную стадию эмитятся только `COPY` и `ENV`, ни одной новой `RUN`. Проектный фрагмент Dockerfile покрыл бы то же самое, но вместе с этим забрал бы у генератора знание о содержимом стадии, то есть и возможность эту гарантию проверять; к тому же фрагмент после `FROM` не умеет добавить стадию **до** финальной, а npm её требует. Без всех четырёх ключей вывод побайтно прежний (`TestGenerateWithoutDockerAdditions`).

Детали, в которых форма не произвольна:

- **`copy` — одна `COPY <from> <to>` в финальной стадии**, не `ADD` и не транзит через builder (как у `StaticDirs`): `COPY` не требует ни shell, ни целевой архитектуры, поэтому builder ради инварианта не нужен; транзит положил бы каталог проекта в builder-образ по пути `<to>`, где назначение вида `/usr/local` перезаписало бы Go-тулчейн; `ADD` распаковывает локальные tar-архивы.
- **`env` — список `{name, value}`, а не карта.** Viper приводит ключи конфига к нижнему регистру, и `API_TEMPLATE_DIR` как ключ карты молча приехал бы как `api_template_dir`.
- **`fetch` приезжает своей стадией** `FROM scratch` + `ADD --chmod[ --checksum]`, а не `ADD` в финальной стадии: там понадобился бы `ARG TARGETARCH`, обесценивающий кеш слоёв под собой. Режим ставит сама инструкция — `ADD` по URL оставляет файл неисполняемым, а `chmod` потребовал бы shell. `RUN` в стадии нет, поэтому эмуляция ей не нужна.
- **Сумма на архитектуру — это стадия на архитектуру.** Выбрать digest внутри одной стадии Dockerfile не умеет, поэтому карта `платформа → digest` даёт стадию на каждую объявленную платформу плюс селектор `FROM fetch-<name>-$TARGETARCH` (в `FROM` `TARGETARCH` разрешается в глобальной области; BuildKit собирает только ту стадию, на которую пришла ссылка). Отсюда два симметричных отказа на загрузке конфига: один digest при ссылке с `${TARGETARCH}` и карта при ссылке без него. `ADD --checksum` требует frontend 1.6+, то есть Docker Engine 25.0+: директиву `# syntax=` сгенерированный файл нести не может — она обязана быть первой строкой, а там disclaimer.
- **`stages` закреплены за `$BUILDPLATFORM`** (согласовано с TOOLS-137), поэтому `TARGETARCH`/`TARGETOS` объявлены в стадии `ARG`-ами: архитектура цели должна попадать в установку явно, иначе нативный бинарь из optional deps приедет под архитектуру сборщика, — **и с переводом в словарь инструмента**: `--cpu` npm сверяет с `process.arch` Node (`x64`, `ia32`), а `TARGETARCH` несёт имя Docker (`amd64`, `386`), поэтому переданный как есть `--cpu=amd64` не подходит ни под один пакет `@scope/linux-*` и npm молча ставит без нативного бинаря. Рецепт в документации переводит **оба** расхождения (`case` по `amd64` и `386`): `platforms` допускает любую `linux/<арх>`, и на `linux/386` перевод только `amd64` давал бы тот же дефект, обнаруженный позже; `arm64`/`arm` в обоих словарях называются одинаково, за остальными — словарь самого инструмента. Префикс npm копируется целиком (`/opt/npm` → `/usr/local`): в его `bin/` относительные симлинки, и копия одного `bin/` висит в образе. Контекста сборки стадия не получает — пин инструмента живёт в команде, как в секции `generator`.
- **Назначения в образе проверяются одним набором:** `/service`, `/public`, `/etc/onlineconf` заняты самим образом, два объявления с одним `to` отвергаются (в образе победило бы последнее скопированное).
- **Имена стадий проверяются тоже одним набором, и именно сгенерированные**, а не имена записей: одна запись даёт больше одной стадии, поэтому `fetch` с digest на платформу (`fetch-tool-amd64`) сталкивается с записью `fetch`, названной `tool-amd64`, при различающихся именах записей. BuildKit на это отвечает лишь предупреждением `DuplicateStageName`, а `COPY --from` одной записи забирает в образ файл другой — отсюда отказ на загрузке конфига, в любом порядке объявления. Имя записи `fetch` и имя записи `stages` совпадать могут: префиксы стадий разные.
- **Дополнения копируются выше бинаря** (`/service`), по той же причине, по которой выше него стоит установка пакетов: бинарь меняется на каждой сборке, и слои под ним пересобирались бы вместе с ним, включая скопированный префикс инструмента. Побочный эффект — при вложенном пересечении путей последним копируется сам образ, то есть его содержимое проектное дополнение не затирает.

Реализация: `internal/pkg/config/docker.go` (парсинг/валидация обеих секций — `Docker.Parse`, `AppDocker.Parse`), `internal/pkg/config/app_docker_runtime.go` (дополнения runtime-стадии — `parseRuntimeAdditions`), `internal/pkg/ds/app_docker_additions.go` (их модель и директивы), `internal/pkg/ds/docker.go` (`ds.DockerConfig` + хелперы шаблонов `BuilderArgs`/`FinalArgs`/`LabelDirective`/`BuildArgFlags`/`ComposeBuildArgs`; `ds.AppDockerConfig` + `Image`/`PackagesDirective`/`WorkdirDirective`/`StartDirective`/`PlatformFlag`/`BuildOutputFlag`, `Apps.HasDockerPlatforms`), шаблоны `app/files/Dockerfile.tmpl`, `main/Makefile.tmpl`, `main/docker-compose-dev.yaml.tmpl`. Полная документация: [docs/configuration/docker.md](docs/configuration/docker.md).

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
  `Paths()`, `Subscribe(ctx, paths, cb)` и `CheckRequired(ctx, app)`. Плюс `Initialized(ctx)`
  и пара `Clone`/`Release` — для кода, который может выполниться на контексте вне сервисной
  обвязки (юнит-тест на `context.Background()`): спросить про дерево или положить его туда
  иначе нечем, прямое обращение к библиотеке запрещает валидатор. Коду сервиса `Initialized`
  не нужен — инвариант границы валидируется один раз на старте процесса. Плюс
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
  шаблоном-читателем). Поля `accessor`, `read_via` (общий аксессор), `owns` (с какой точки
  поддерево принадлежит сгенерированному коду) и `required_at_init` (см. ниже). В рантайме не
  читается, он для ревью, diff-а и гварда потребителя.

**Сгенерированный код читает OnlineConf только через `occonf`** — все 15 шаблонов с чтениями плюс
`main` (жизненный цикл). Фактические пути, типы и значения по умолчанию при этом не изменились.

**Сигнатуру аксессора ключа сервиса задаёт объявление**, а не место вызова: `required` →
`(T, error)` с `ErrKeyNotSet`; `default` → `(T, error)` с дефолтом; ни того, ни другого →
`(T, bool, error)`. Схема карты **строгая**: поля `path`, `type`, `required`, `default`, `name`,
`description`; неизвестное поле — отказ генерации. `path` — **полный** путь; все записи в одном
дереве (карта одна на сервис). Типы: `string`, `int` (int64), `bool`, `duration`,
`strings`, `struct`. `required` нельзя у `struct` (проверка потребовала бы разобрать значение, а
тип назначения принадлежит вызывающему) и у ключа с runtime-сегментом.

**Сегмент, который вычисляет сам сервис** (идентификатор аккаунта, имя действия), пишется
`<name>` и **становится параметром аксессора** — та же форма и та же сигнатура, что у объявлений
генератора: `/svc/account/<account>/quota` → `AccountQuota(ctx, account)`. Константы пути у такого
ключа нет (путь собирается из аргумента) — вместо неё построитель `<Name>Path(...)`, и в `Paths()`
ключ не попадает; в имя аксессора runtime-сегмент не входит. Нельзя: runtime-сегмент корнем дерева,
два сегмента с одним именем, `required` (`CheckRequired` читает на старте, а путь существует только
в рантайме), **имя, занятое в сгенерированном пакете** — ключевое слово Go, предобъявленный
идентификатор или идентификатор, который пакет использует сам (`ctx`, `dst`, `path`, `v`, `ok`,
`err`, `fallback`, импорты, `serviceTree`): параметр затенил бы его и пакет не собрался бы. Карта
приезжает из контрактного репозитория, поэтому это ошибка **чтения карты**, а не генерация, которая
печатает `done` и оставляет несобираемый проект; список имён — `ockeymap.ReservedParamNames()`, и
его синхронность шаблону держит `assertAccessorsShadowNothing` (`test/onlineconf_map_test.go`),
который вычитывает идентификаторы обратно из сгенерированного пакета. **Поддерева ключ сервиса себе не отдаёт** — закрывать не от кого, вторую половину
карты пишет генератор; вместо этого сравнение формы пути работает в **обе** стороны, и
`<runtimeСегмент>` стоит за один или несколько сегментов с любой из них. Список по-прежнему — один
ключ с `type: struct`: обхода поддерева в OnlineConf нет, `<i>` перечислять нечем. `{измерения}`
в карте сервиса отвергаются: раскрывать её не по чему (TOOLS-89).

**Объявления генератора** (`<шаблон>.ockeys.yaml` рядом с шаблоном; из вывода генерации
исключаются в `GetTemplates`) несут `path`, `name`, `type`, `required`, `shared`, `read_via`,
`open`, `default` (проза), `description`. В пути два вида переменных сегментов: `{service}`,
`{transport}`, `{app}`, `{worker}`, `{client}`, `{event}` — измерения проекта, известные на
генерации, и `<urlPath>`, `<tgID>`, `<action>` — сегменты, которые сгенерированный код вычисляет в
рантайме. **Оба становятся параметрами аксессора**: сгенерированные читатели generic по
измерениям (провайдер REST получает имя сервера и приложения от рантайма). Поэтому **набор
аксессоров не зависит от измерений проекта** — от них зависят строки реестра и проверки на
старте: шаблон, который генерируется, зовёт свои аксессоры безусловно (провайдер REST
генерируется и в проекте, где единственный REST-транспорт — клиент). Запись без `type` —
построитель пути (имя обязано кончаться на `Path`), отдаёт генератору всё поддерево; запись с
`<runtimeСегментом>` — с него и ниже. `open: true` на такой записи **сужает** претензию до
объявленных путей: сам префикс и чтения под ним зарезервированы, остальное поддерево карта
сервиса объявлять может. Ставится только там, где все чтения ниже известны по именам — префикс
воркера (уходит логгеру, этапные ключи объявлены через `read_via`) и префикс middleware метрик
REST открыты, префикс обработчиков REST закрыт (его получает библиотека рантайма, её чтения
здесь не объявлены). До этого интервал тика воркера, который читает рукописный код, объявить
было нечем — TOOLS-87. `shared: true` — аксессор, путь которого целиком собирает
вызывающий (уровень логирования), поддерева не отдаёт, а конкретные пути объявляются у вызывающих
с `read_via: <Имя>`. Запись с `shared: true` — **единственная**, которая начинается не с
`{service}`, а с `<runtimeСегмента>`: префикс приходит уже абсолютным (его собирают
`occonf.RESTAppMetricsPath` / `occonf.WorkerPath`), и приписанное второй раз имя сервиса уводило
чтение на уровень глубже дерева — ровно дефект TOOLS-65. Пустой префикс (процессный логгер)
нормализует в корень дерева сам вызывающий: сборка пути пустой компонент не выбрасывает и дала бы
`//`. `scope` задаёт измерение раскрытия (`service`, `rest_server`, `rest_client`,
`grpc_client`, `kafka_producer`, `kafka_producer_event`, `worker_daemon`, `worker_telegram`,
`telegram_driver`).

**`required_at_init: true`** — второе поле объявления, машиночитаемый ответ на «что обязан
засеять потребитель»: сгенерированный код читает ключ, пока поднимает компонент, и без значения
процесс не стартует. Генерирует оно **ничего** — ни проверки, ни другой формы аксессора; это
строка реестра. Нужно оно потому, что `required` отвечает свойством ключа, а для host клиента
ответ принадлежит проекту: клиент `instantiation: dynamic` ключ не читает, `optional: true`
переживает его отсутствие. Поэтому флаг **раскрывается по фактическому проекту** — у таких
клиентов его в реестре нет, и гвард берёт весь список `required_at_init: true` без исключений,
которые надо вести руками (измерение, по которому раскрытие «прощает» компонент, —
`ProjectSurface.Tolerant`). Помечены: host ogen-клиента, host и port buf-клиента; плюс каждый
`required`-ключ получает флаг в реестре (упавшая проверка на старте и есть «не стартовал»).
Правила: нужен `type`, нельзя с `default`, с `required`, с `shared` и с `<runtimeСегментом>`.

«Прощает» — **по всем обвязкам компонента, а не по одной** (`optionalEverywhere`, как
`readsItsAddress` для `instantiation`): `optional: true` — настройка обвязки, и приложение,
подключившее тот же компонент без неё, без ключа не стартует. REST/gRPC-клиент не проходит
`ValidateFor<App>` на nil-клиенте; Kafka-продюсер никогда не nil (`Init` лишь отключает себя),
его останавливает только проверка `brokers` в `occonf.CheckRequired`. Компонент, не подключённый
ни к одному приложению, толерантным не считается. Толерантность снимает в строке реестра **оба**
флага, и `required_at_init`, и `required` (`addKey` в `ockeymap/gendecl.go`), и по ней же
отбирается проверка на старте (`addGroup`: `key.Required && !sub.tolerant`). Мэппинг в
`ds.onlineConfSurfaceRows` поэтому ничего не ORит: оба флага приезжают уже раскрытыми.

Толерантность принадлежит **компоненту, который называет сегмент пути**, а не записи
`project.yaml`, и опознаётся парой «измерение + имя» (`ockeymap.TolerantComponent`,
`Dimension*`; измерение известно в месте раскрытия, поэтому его передаёт вызывающий, а не
угадывает `substitute`). Имя само по себе компонентом не является: gRPC-клиент `audit` и
Kafka-клиент `audit` — разные поддеревья, и у «толерантен» нет направления, в котором его можно
было бы безопасно переносить с одного на другой. Для Kafka сегмент ещё и не совпадает с записью
конфига: под одним `client` можно объявить несколько продюсеров, а `brokers` у клиента один, —
поэтому ответ сворачивается по всем его продюсерам через **AND** (`tolerantClients` в
`onlineConfSurface`). Взятый от любого одного, продюсер, без которого проект обходится, отвечал бы
за того, без которого не обходится: ключ ушёл бы из реестра, а проверка на старте осталась бы.

**Строка реестра и проверка на старте — одно решение над разным набором приложений.**
`optionalEverywhere` спрашивает про `config.Applications`, а `onlineConfAppSurface` подставляет
туда одно приложение, чьи проверки строит, — поэтому толерантность для реестра значит «optional
во всех обвязках проекта», а для проверки «optional в этой обвязке», и разойтись они не могут:
компонент, `optional` везде, толерантен и там, и там (не помечен и не проверяется нигде), а при
смешанной обвязке строка несёт оба флага и проверка стоит ровно в приложениях без `optional`.
Project-wide объединение (`optionalComponents`) из отбора проверок убрано: именно оно публиковало
`brokers` как обязательный ключ, которого не проверяло ни одно приложение (TOOLS-88).

Порт REST-сервера под флаг не попадает: `transport/rest/<сервер>/port` **получил дефолт** — порт
этого сервера из `project.yaml`, который генератор рендерит в провайдер конфигурации
(`defaultPorts` в `transport/rest/files/pkg/app/restconfig/config_oc.go.tmpl`, данные —
`ds.Apps.RESTServerPorts`). Значение в дереве по-прежнему главнее; ключ нужен только чтобы увести
сервер с объявленного порта. Сервер без `port` (или `port: 0`) дефолта не получает — слушать на
нулевом порту никто не просил.

`required` у ключа генератора **не меняет форму чтения** (читатель компонента, объявленного
`optional: true`, должен уметь увидеть отсутствие) — он означает, что ключ попадает в
`CheckRequired` **того приложения**, которое этот компонент запускает. Поэтому
`CheckRequired(ctx, app)` принимает имя приложения: ключи сервиса проверяются в каждом
**сервисном** приложении, ключи генератора — по компонентам приложения, за вычетом объявленных
`optional` **в этой обвязке** (приложение, подключившее тот же компонент без `optional`, свою
проверку сохраняет). Помечать `required` можно только ключ, которого никакая настройка
`project.yaml` не может сделать ненужным: таких два — `security/tg_token` и
`kafka/<client>/brokers`. host/port клиентов не помечены, потому что
клиент с `instantiation: dynamic` получает адрес аргументом и ключ не читает.
**Сгенерированный `main` CLI-приложения (`app/cmd_cli/main.go.tmpl`) `CheckRequired` не вызывает
намеренно:** исключение держится на том, что ключи карты сервиса проверяются в каждом приложении —
у карты нет измерения приложений, — поэтому CLI не стартовала бы из-за `required`-ключа, который
читает только сервис, хотя короткоживущей команде вся конфигурационная поверхность сервиса и не
нужна. Нужна проверка — вызовите `occonf.CheckRequired(ctx, "<cli>")` в коде команды, то есть
после `occonf.Initialize`.

Совпадение пути из карты сервиса с путём генератора — ошибка генерации с указанием
шаблона-читателя: точное совпадение, совпадение по форме с чтением, объявленным через
`<runtimeСегмент>` (сегмент в угловых скобках матчится с одним **или несколькими** сегментами:
вычисленное в рантайме значение может само содержать `/`, и построитель пути их сохраняет —
middleware отдаёт логгеру `r.URL.Path` целиком, так что уровень для `/v1/users` читается в
`…/mw/metrics/v1/users/log_level`), либо объявление внутри закрытого (не `open`) поддерева.
Форма пути бывает с обеих сторон, поэтому сравнивается не равенство строк, а то, сходятся ли два
пути хотя бы в одной точке (`pathsOverlap`/`reachesSubtree` в `gendecl.go`): ключ сервиса с
`<runtimeСегментом>`, который достаёт конкретный путь генератора или заходит внутрь закрытого
поддерева, отвергается так же, как написанный целиком.
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
version: 6
inputs:
    - {channel: metrics.path, source_kind: git, source_repo: 'ssh://git@github.com/org/contracts.git', source_path: metrics/map.yaml, ref: main, commit: 06b860af…, source_sha256: 2c5e…}
manifest:
    - {path: cmd/publicApi/psg_main_gen.go, kind: generated, hash: 8fe15f…}
    - {path: api/rest/example/v1/example.swagger.yml, kind: copied, hash: 490da3…, source_kind: local, source_path: ./example.swagger.yml, source_sha256: 490da3…}
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

Манифест даёт семь вещей:

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
5. **Запись того, что успел упавший прогон.** Прогон, который записал файлы и затем упал,
   сохраняет манифест по факту записанного — **поверх** манифеста предыдущего прогона, не вместо
   него (`Manifest.Merge`): свои записи теряют только файлы, которые этот прогон перезаписал.
   Иначе скопированные им спеки следующий прогон читал бы как ручную правку (пункт 4) и падал бы
   на них **до** записи манифеста, который говорит обратное, — петля, из которой не выходил ни
   один следующий прогон (TOOLS-80). Записываются только файлы, реально оказавшиеся на диске;
   после того как манифест прогона записан целиком, чекпоинт уже не трогает его (падение в
   `post_generate` оставляет полную картину, а не частичную).

   **Прогон без предыдущего манифеста не чекпоинтится вовсе** (`len(prev) == 0`): манифест — он
   же выключатель между точным и legacy-путём, поэтому частичный манифест объявил бы проект
   «проектом с манифестом», и всё, что записали прогоны до этого, а этот не дошёл перезаписать
   (спеки в `api/`, сгенерированные файлы после точки падения), стало бы конфликтом — та же
   «прогон не проходит без ручной чистки дерева», только через conflict, а не через drift.
   `meta.yaml` остаётся как был, следующий прогон снова идёт legacy-путём, полный манифест
   пишет тот прогон, который дойдёт до конца. Ничего при этом не теряется: без записей все
   проверки, которые чекпоинт кормит (copied-drift, contract-drift, conflict), и так no-op,
   так что в петлю ради которой он есть такой проект попасть не может.
6. **Проверку дерева без генерации** — `--check` (в проекте `make check-generated`). Тот же
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
7. **Происхождение всего, что пришло через резолвер** — из какого дерева контрактов собран
   проект (см. раздел ниже).

**Незнакомую версию манифеста читатель считает недоказанной.** `version` больше, чем знает
бинарь, — отказ и у `--check` (`treecheck.ErrFutureMeta`), и у **генерации**
(`meta.ErrFutureVersion`, `meta.GetMeta`): по манифесту решается, что перезаписать, что удалить
и что считать пользовательским файлом, а на записях с незнакомыми полями такое решение
принимать нельзя. Версия ниже текущей читается как раньше и после прогона переписывается в
текущую — проект на `version: 5` получает `version: 6` без ручных шагов.

Хеши обновляются по диску после `post_generate` (goimports, `go mod tidy`, `make generate`
переписывают только что сгенерированные файлы) — иначе каждый следующий прогон сообщал бы о
дрейфе для всех файлов.

Проект, сгенерированный до `version: 5`, манифеста не имеет: первый прогон работает по-старому
(обход дерева + поиск disclaimer в содержимом, конфликты не детектируются) и записывает манифест,
со второго — точный путь.
Реализация: `internal/pkg/meta/meta.go`, `templater.GetUserCodeFromFiles()`,
`internal/pkg/templater/conflict.go`, `generator.checkCopyTargetConflicts()`, чекпоинт —
`internal/pkg/generator/checkpoint.go` (`runWrites`, `Generator.checkpoint()`, defer в
`Generate()`); проверка — `internal/pkg/treecheck/` (вердикт и его рендер), `runCheck()` в
`cmd/go-project-starter/main.go` (ветка до загрузки конфига), цель `check-generated` в
`main/Makefile.tmpl`. Сентинел потерянного маркера — `templater.ErrDisclaimerNotFound`.

### Происхождение входов (`source_*` в записи, секция `inputs:`)

**Происхождение записывается у каждого входа, прошедшего через резолвер источников**, а не
только у копий секции `contract:`. Поля: `source_kind` (`local`/`http`/`git` — какие из
остальных заполнены), `source_repo` и `source_path` (идентичность источника и путь внутри него —
раздельно, чтобы строку `url@ref#path` не приходилось разбирать), `ref` (эффективный, с учётом
`--sourceRef`), `commit` и `source_sha256`.

`source_sha256` — дайджест **байтов источника, до любой переписи**, и в этом его смысл: `hash`
считается по файлу в проекте, то есть уже после `rewrite_refs`, поэтому с блобом в контрактном
репозитории он не сравним. Для копии как есть они равны; поле пишется **всегда**, иначе
читатель не отличал бы «равно `hash`» от «не посчитано». Токена ни в одном поле нет: значение
живёт только в переменной окружения из `token_env`, а учётные данные, написанные в URL руками,
вырезаются (`specsource.redactRepoURL`). Какая половина userinfo — секрет, решает транспорт: у
`https` userinfo удаляется **целиком** (имя пользователя там неотличимо от токена — `https://<PAT>@…`
штатная форма его записи, и то же правило уже действует у http-источника, где остаются только схема
и хост), у `ssh` остаётся имя (`git@` — без него URL не клонируется), а пароль удаляется.
**Query http-источника не редактируется:** он выбирает содержимое и потому остаётся в
`source_path` как есть (кроме `token_env`, который вырезает разбор). Токен, вписанный в query
руками (`?private_token=…`), окажется в манифесте — он и так лежит открытым текстом в
`project.yaml`; штатный способ передать токен — `token_env`. Вырезать query по угаданному списку
имён параметров означало бы рисковать легитимным селектором.

**Запись собирает резолвер, а не канал** — по одной `specsource.Provenance` на успешный
`Resolve`. Канал в генераторе резолвит через одну из двух функций, и обе записывают результат в
`g.sourceProvenance` (ключ — локальный путь, единственное, что знает копирующий пасс):
`Generator.resolveOne` (пре-пасс `resolveAllSources`: `rest[]`, `grpc`, `jsonschema[]`,
`contract:`) и `Generator.resolveInlineSpec` (входы, парсящиеся при загрузке конфига). Поэтому
новый канал покрывается сам; инвариант держит `TestEveryResolveRecordsProvenance`, который падает
на вызове `Resolve` из функции вне этих двух и на вызове `NewResolver` откуда-либо, кроме
`Generator.sourceResolver` — резолвер у прогона один (см. ниже).

**Секция `inputs:` — для входов, которые в проект не копируются** (`metrics.path`,
`onlineconf.path`, `tools.lint_config.overrides`, spec CLI, контракт queue-воркера): файловой
записи у них нет (их артефакт — сгенерированный код), а вопрос «из какого дерева контрактов это
собрано» к ним относится так же. К тем же полям добавлен `channel` — ключ `project.yaml`,
который вход обслуживает (`workers.<имя>.path`, `applications.cli.<имя>.path`). Записи
нормализованы (сортировка по `channel`, затем по источнику; дубли убраны), секция
опускается целиком, если таких входов нет.

Записи **чекпоинта** упавшего прогона несут то же происхождение: `Manifest.Merge` заменяет
запись по пути целиком, так что запись без провенанса стёрла бы его у перезаписанного файла.
Входы у упавшего прогона полные (все читаются до первой записи), поэтому заменяют прежние, а не
мержатся.

Реализация: `internal/pkg/specsource/provenance.go` (`Provenance`, `provenanceOf`,
дайджесты по видам источника), `internal/pkg/meta/provenance.go` (форма записи, секция
`inputs:`), `internal/pkg/generator/provenance.go` (леджер прогона, `resolveOne`,
`resolveInlineSpec`, имена каналов). Полная документация:
[docs/workflow/regeneration.md](docs/workflow/regeneration.md#происхождение-из-какого-дерева-контрактов-собран-проект).

### Один `specsource.Resolver` на прогон

**Резолвер у прогона один** (`Generator.resolver`, ленивый геттер `Generator.sourceResolver()`), и
он живёт от загрузки конфига до конца `Generate()`. Раньше резолвер строил каждый канал: пять
inline-вызовов плюс пре-пасс — шесть резолверов и до шести клонов, поэтому один `repo@ref`,
названный в разных каналах, за прогон мог разрезолвиться в **разные коммиты**, если ветка
сдвинулась между вызовами. Дерево собиралось из двух состояний контрактов молча. Общий резолвер
означает общий `cloneCache`: `(repo, ref)` клонируется один раз, и все каналы читают один коммит.

Из этого следуют три вещи:

- **Закрывает тот, кто вызвал `New`.** `Generator.Close()` удаляет staging-каталог и клоны;
  `cmd/go-project-starter/main.go` ставит его через `defer` в функции `generate()` — отдельной
  именно потому, что `log.Fatalf` не выполняет `defer`, а оба отказа (`New`, `Generate`) раньше
  заканчивали процесс здесь же. `New`, упав, закрывает то, что успел создать, и возвращает `nil`.
  Staged-файл inline-канала больше не удаляется сразу после парса: его удержание и есть то, что
  держит общий клон.
- **Staging-каталог создаётся при первом использовании** (`Resolver.stagingRoot()`), а не при
  конструировании. Прогон на одних локальных спеках не создаёт его вовсе — ни `gps-*` в
  `os.TempDir()`, ни чего-либо удалять.
- **Проверка наличия `git` отложена до первого git-источника** (`Resolver.ensureGit()`, один раз на
  резолвер). Прежний вопрос «нужен ли прогону git» решался до резолва и по тому подмножеству
  каналов, которое видел конкретный вызывающий (`SkipGitCheck`, `hasGitSource()`); с одним
  резолвером на прогон так его задать нельзя — проект, у которого git-источник только в
  inline-канале, потерял бы проверку, а проект без git-источников приобрёл бы зависимость от `git`.
  Поэтому `ResolverOptions.SkipGitCheck` и `Generator.hasGitSource()` убраны, а `NewResolver` больше
  не возвращает ошибку.

Тесты: `TestOneResolverPerRunAgreesOnTheCommit` (`internal/pkg/generator/resolver_test.go`) двигает
ветку контрактного репозитория **между** inline-резолвом и пре-пассом и требует, чтобы оба канала
записали коммит, на котором прогон начался, в один staging-каталог и с одним клоном; репозиторий
локальный, URI обычный `git+ssh://`, подмену делает `insteadOf` в `GIT_CONFIG_GLOBAL`. Рядом —
освобождение staging при `Close`, при упавшем `New` и отсутствие staging у локального прогона;
`TestGenerateReportsFailuresByReturning` (`cmd/go-project-starter`) держит возврат ошибки вместо
`log.Fatalf` на обоих путях.

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
  `--dry-run` тоже падает: это предпросмотр прогона, а прогон не состоится. Тем же pre-pass'ом
  проверяются спеки контрактов (`Generator.checkContractDrift`): раньше эта проверка стояла
  непосредственно перед `copyContracts`, то есть падала уже после копирования спек REST/gRPC и
  JSON Schema и оставляла дерево, в котором следующий прогон видел ручную правку (TOOLS-80).
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

Тем же pre-pass'ом, **перед** drift-проверкой, идёт проверка коллизий назначения
(`checkCopyDestCollisions`, `internal/pkg/generator/copycollision.go`): имя файла в `api/` —
basename источника, и два **разных** источника (`rest[].path`, `grpc`, `jsonschema[]`), которые
попадают в один файл проекта, — ошибка генерации с общим назначением и всеми `path:`. Один и тот же
источник несколько раз (транспорт в нескольких приложениях) коллизией не считается. Поэтому
`plannedCopies()` назначения не дедуплицирует, а drift-проверка получает план через `dedupCopies()`.
См. [docs/configuration/remote-specs.md](docs/configuration/remote-specs.md#копирование-одно-имя-файла--один-источник).

Реализация: `Manifest.CheckCopiedDrift` + `CopiedDriftError`/`CopiedFileDriftHint`
(`internal/pkg/meta/drift.go`), сбор целей — `Generator.plannedCopies()` → `dedupCopies()`
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
