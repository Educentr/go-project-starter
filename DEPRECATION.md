# Deprecation Policy

## Overview

This document describes the deprecation policy for go-project-starter configuration format and features.

## Deprecation Timeline

- **Deprecated features are removed every 2nd minor release**
- Example: feature deprecated in `0.10.x` will be removed in `0.12.0`

## Current Deprecations

No active deprecations.

## Breaking changes in config validation

Эти изменения отвергают `project.yaml`, который прежний релиз загружал. Миграция —
правка конфига, но `go-project-starter migrate` их не трогает: автоматически выбрать
замену нельзя, в отвергнутой записи нет правильного значения.

### `auth_params` валидируются на загрузке конфига (TOOLS-90, unreleased)

`Rest.IsValid` валидирует секцию `auth_params` там, где она применима, —
`generator_type: ogen_client`. До этого секция не проверялась **вообще**: валидация жила
в мёртвой ветке `case "auth_params"` switch-а по `generator_type` (значения
`auth_params` у `generator_type` не бывает), поэтому опечатка доезжала до генерации,
а то и до рантайма — клиентом без авторизации.

| Конфиг | Было | Стало | Миграция |
|--------|------|-------|----------|
| `auth_params` на `generator_type` ≠ `ogen_client` (`ogen`, `template`, `sys`) | молча игнорировались | ошибка: `auth_params are only supported for generator_type ogen_client` | удалить секцию; аутентификация сервера — `generator_params.auth_handler` / `auth_scheme` |
| `transport` ≠ `header` | игнорировалось, клиент без `SecuritySource` | ошибка: `auth_params: invalid transport: … (supported: header)` | `transport: header` |
| `type` вне {`apikey`, `bearer`, `basic`} | то же | ошибка: `auth_params: invalid type: … (supported: apikey, bearer, basic)` | один из поддерживаемых типов (таблица в [transports.md](docs/configuration/transports.md#параметры-аутентификации)) |
| задано одно поле из двух | то же | ошибка: `auth_params: both transport and type are required` | указать оба поля либо удалить секцию |

**Почему ошибка, а не игнорирование.** Молча проигнорированная настройка
аутентификации — худший из исходов: проект собирается, клиент ходит во внешний API без
креденшелов, и выясняется это по 401 в рантайме. Ошибка загрузки конфига называет
отвергнутое значение и допустимые; имя REST-транспорта в сообщение не попадает (так
устроены все ошибки секции `rest`), поэтому при нескольких `ogen_client` проверьте
`auth_params` у каждого.

**Почему не «починить ветку как есть».** Мёртвая проверка разрешала только
`type: apikey`, тогда как шаблоны поддерживают `apikey`, `bearer` и `basic` (TOOLS-85).
Перенос ветки без изменений сломал бы валидные проекты с bearer/basic, поэтому список
допустимых значений теперь один на весь генератор — `ds.SupportedAuthTypes()`, та же
функция, на которой ветвится `ds.Transport.UsesSecuritySource`.

## Breaking changes in generated code

Эти изменения касаются не формата `project.yaml`, а **кода, который получает проект**,
поэтому `go-project-starter migrate` их не трогает: он правит конфиг, а здесь менять
надо исходники проекта. Миграция ручная и механическая, компилятор показывает каждое
место.

### `func init()` в сгенерированных проектах (TOOLS-30, unreleased)

`gochecknoinits` включён в `configs/golangci-lint.yml` сгенерированного проекта
(и в конфиге самого генератора), **без исключения для `_test.go`**. Точки расширения
сгенерированного кода переделаны так, что `init()` для них больше не нужен.

**Почему без переходного периода.** Переходный вариант — оставить `cmdMaps`
deprecated-глобалом и вливать его в новый реестр при конструировании — внутренне
противоречив: задача ровно в том, чтобы убрать глобальный мутабельный реестр, а
единственный способ его наполнить (`init()` в своём файле) с этого же релиза
отвергает линтер, который мы включаем. Проект получил бы и глобал, и красный
merge-гейт. Поэтому ломаем сразу: `undefined: cmdMaps` указывает точно на
строку, которую надо перенести, а миграция — перенос блока в новый метод.

| Было | Стало | Миграция |
|------|-------|----------|
| `cmdMaps[CmdX] = telegram.Cmd{…}` из `init()` в пакете telegram-воркера | реестр — поле воркера (`w.cmds`), наполняется из `customize()` | объявите `func (w *Worker) customize()` в своём файле пакета (или ниже disclaimer-маркера) и перенесите регистрацию на `w.AddCommands(telegram.Cmd{…})` |
| `updateHandler` / `updateHandlerIdle` / `unknownCommandHandler` переназначались из `init()` | те же переменные | перенесите присваивания в тот же `customize()` |
| `testEnvInit = &testEnvInitializerImpl{}` из `init()` в `tests/<app>/init.go` | явный сеттер | `var _ = SetTestEnvInitializer(&testEnvInitializerImpl{})` |
| `init()` в пакете `main` (ранняя работа до чтения конфигурации: правка окружения, re-exec, проверка `os.Args`) — обычно с `//nolint:gochecknoinits` | `earlyInit()` — метод, который `main` зовёт после `.env` и до `occonf.Initialize` (TOOLS-123, unreleased) | объявите `func (u *UserEarlyInit) earlyInit() error` в своём файле пакета `main` (или ниже disclaimer-маркера) и перенесите тело `init()` туда; ошибка возвращается наружу, процесс выходит с `ExitCodeErrorEarlyInit` |

Пример для telegram-воркера:

```go
// Было — internal/app/worker/telegrambot/commands.go
func init() {
	cmdMaps[CmdHelp] = telegram.Cmd{Name: CmdHelp, Private: true, Handler: helpHandler}
	unknownCommandHandler = resetDialog
}

// Стало
func (w *Worker) customize() {
	w.AddCommands(telegram.Cmd{Name: CmdHelp, Private: true, Handler: helpHandler})
	unknownCommandHandler = resetDialog
}
```

`customize()` по умолчанию промотирован из встроенного `EmptyCustomizer` и ничего не
делает; метод на `*Worker` его перекрывает. `Create()` зовёт его после регистрации
команд генератора, поэтому команда проекта с тем же `Name` перекрывает сгенерированную.
Подробности — [docs/configuration/workers.md](docs/configuration/workers.md) и
[docs/testing/goat.md](docs/testing/goat.md).

**Ранняя инициализация в `main`.** Для работы, которой надо случиться **до** чтения
конфигурации, точки расширения не было вовсе: первый пользовательский код сервисного
`main` выполнялся после `occonf.Initialize` (`UserSetFunc.SetFunc`), а у CLI — в обработчике
команды, поэтому проекты держали `init()` с `//nolint:gochecknoinits`. Теперь оба `main`
зовут `earlyInit()` — после загрузки `.env` и до `occonf.Initialize` (TOOLS-123):

```go
// Было — cmd/api/init.go
//nolint:gochecknoinits
func init() {
	if !hasAnyOCKey() {
		os.Unsetenv("ONLINECONFIG_FROM_ENV")
	}
}

// Стало — cmd/api/early.go
func (u *UserEarlyInit) earlyInit() error {
	if !hasAnyOCKey() {
		return os.Unsetenv("ONLINECONFIG_FROM_ENV")
	}

	return nil
}
```

Тот же приём, что у `customize()`: метод промотирован из встроенного `EmptyEarlyInit`, метод
на `*UserEarlyInit` его перекрывает. Два имени — `EmptyEarlyInit` и `UserEarlyInit` — появляются
в пакете `main` каждого сгенерированного приложения: проект, у которого уже есть свой тип с таким
именем в `main`, после регенерации получит ошибку о повторном объявлении и его надо переименовать.
Подробности —
[docs/configuration/applications.md](docs/configuration/applications.md#ранняя-инициализация-процесса-earlyinit).

**Чем это держится.** `gochecknoinits` не видит `.tmpl`-файлы, а
`exclusions.generated: lax` выводит из проверки `psg_*_gen.go` целиком — включая
секцию ручного кода ниже disclaimer-маркера. Поэтому основной гард — не линтер, а
тест по шаблонам (`TestNoInitFuncInTemplates`) плюс проверка результата генерации
(`TestNoInitFuncInGeneratedProjects`); они от конфига проекта не зависят. Если проект
забирает конфиг линтера себе (`tools.lint_config.own`), запрет `init()` в **его** коде
остаётся на нём.

### Инициализация метрик у приложений без `sys`-транспорта (TOOLS-94, unreleased)

`application.InitMetrics(mainCtx)` и регистрация коллекторов карты метрик в
`cmd/<app>/psg_main_gen.go` теперь **безусловны** — раньше они генерировались только у
приложения с `sys`-транспортом, и приложение без него получало `nil`-реестр, на котором
middleware REST-транспорта падало (`MustRegister` на `nil`) ещё до первого запроса.

Регенерация такого проекта чинит старт сама. Чинить руками нужно только одно: **ручной
воркэраунд, если он был**.

| Было в ручном коде | Что делать после регенерации |
|--------------------|------------------------------|
| в `GetMiddlewares` транспорта подставлялся `prometheus.NewRegistry()` при `nil` | уберите ветку: реестр больше не `nil` (оставленная ветка просто мертва, компиляции не ломает) |
| ниже disclaimer-маркера свой вызов `metrics.Register(application.GetMetrics())` | **уберите его** — иначе второй `Register` в тот же реестр вернёт `AlreadyRegisteredError`, и сгенерированный код выйдет с `ExitCodeErrorApp` |

Канал вывода метрик стал объявленным свойством приложения
(`applications[].metrics.export`: `pull` / `push` / `none`). Ключ необязателен и по умолчанию
выводится из транспортов, поэтому существующие `project.yaml` менять не нужно —
[docs/configuration/applications.md](docs/configuration/applications.md#метрики-приложения-metricsexport).

### Цели `test-integration-with-deps` и `test-int-up-deps` (TOOLS-96, unreleased)

Обе цели удалены из сгенерированного `Makefile`: ни одна из них не работала с момента
появления. `test-integration-with-deps` зависела от цели `test-integration`, которую
генератор никогда не создавал, а `test-int-up-deps` поднимала compose-сервис `wiremock`,
которого нет ни в одном сгенерированном compose-файле. Заодно из `go.mod.tmpl` убран
`github.com/walkerus/go-wiremock` — он был там только ради этих целей, и ни одна строка
сгенерированного кода его не импортирует (`go mod tidy` выкидывал его и раньше).

**Миграция.** Если проект определил `test-integration` в своей секции (ниже
disclaimer-маркера) и звал `test-integration-with-deps` — перенесите обёртку туда же, к
самой цели:

```make
# ниже disclaimer-маркера, рядом со своей test-integration
.PHONY: test-integration-with-deps
test-integration-with-deps: test-int-up-deps test-integration

.PHONY: test-int-up-deps
test-int-up-deps:
	docker compose up -d --build --force-recreate <ваш сервис>
```

Зависимости интеграционных тестов объявляет проект: что именно поднимать, генератор не знает.
Штатный путь для интеграционных тестов в сгенерированном проекте — GOAT
(`goat_tests`, цель `goat-tests-<app>`), см. [docs/testing/goat.md](docs/testing/goat.md).

## Go version policy

Generated projects target a **supported** Go release. Go upstream supports the
two latest minor releases, so the generator default (`tools.golang_version`,
and `MIN_GO_VERSION` in the generated Makefile) tracks them instead of staying
on an older version.

| Change | Release | Note |
|--------|---------|------|
| Generated projects: Go 1.24 → **1.27** | unreleased | `tools.golang_version` default, `MIN_GO_VERSION`, integration image and CI; `goimports` installed from `@latest` again |

**Why a policy and not a one-off bump:** staying on an EOL release only defers
the next breakage. A dependency that raises its own `go` directive (e.g.
`gofrs/uuid/v5` → `go 1.25`, TOOLS-4) breaks `go mod tidy` for everyone on an
older toolchain, and pinning the tool that broke (`goimports@v0.49.0`) moves
the problem rather than fixing it.

**Impact on existing projects:** a project generated earlier keeps the
`tools.golang_version` written in its own `project.yaml` — regeneration does
not change that value. After raising it, the project's build/CI environment
must provide the new toolchain.

**Impact on this repository's own tests:** the generator test configs target
the new version, so the configs with `post_generate: go_mod_tidy` need a Go
1.27 toolchain. With the default `GOTOOLCHAIN=auto` Go downloads it on first
use; with `GOTOOLCHAIN=local` on an older host, `go mod tidy` in the generated
project fails with `go.mod requires go >= 1.27`. The integration-test CI job
installs 1.27.2 explicitly.

## Removed Deprecations

| Feature | Deprecated In | Removed In | Migration |
|---------|---------------|------------|-----------|
| `empty_config_available` | 0.11.0 | 0.13.0 | Use `optional: true` in application transport config |
| Transport string array format | 0.10.0 | 0.12.0 | Use object format: `- name: transport_name` |

## Migration

Run the migrate command to automatically update your config:

```bash
# Check what will be changed (dry-run)
go-project-starter migrate --dry-run

# Apply migration
go-project-starter migrate
```

The migrate command will:
1. Create a backup of your config (`.bak` file)
2. Convert deprecated formats to new formats
3. Report any issues found

## Adding New Deprecations

When adding a new deprecation:

1. Add a new version constant in `internal/pkg/migrate/migrate.go`:
   ```go
   const (
       RemovalVersionTransportStringArray = "0.12.0"
       RemovalVersionNewFeature           = "0.14.0"  // NEW
   )
   ```

2. Add migration logic in `migrate.go`

3. Add warning collection in `config/config.go` `collectDeprecationWarnings()`

4. Update this table with the new deprecation

## Release Checklist

Before each release, check if any deprecations should be removed:

1. Check `internal/pkg/migrate/migrate.go` for `RemovalVersion*` constants
2. If current release version >= removal version:
   - Remove backward compatibility code
   - Remove migration logic for that feature
   - Update this document
