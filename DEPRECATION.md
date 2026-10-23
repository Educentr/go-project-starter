# Deprecation Policy

## Overview

This document describes the deprecation policy for go-project-starter configuration format and features.

## Deprecation Timeline

- **Deprecated features are removed every 2nd minor release**
- Example: feature deprecated in `0.10.x` will be removed in `0.12.0`

## Current Deprecations

No active deprecations.

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
| `updateHandler` / `updateHandlerIdle` / `unknownCommandHandler` переназначались из `init()` | сеттеры воркера | перенесите в тот же `customize()`, уже в виде вызовов сеттеров — см. TOOLS-31 ниже |
| `testEnvInit = &testEnvInitializerImpl{}` из `init()` в `tests/<app>/init.go` | явный сеттер | `var _ = SetTestEnvInitializer(&testEnvInitializerImpl{})` |

> Эти переменные убирает TOOLS-31, в том же (ещё не вышедшем) релизе. Переносите
> присваивания сразу в сеттеры — таблица в разделе ниже даёт соответствие — иначе
> получите `undefined: unknownCommandHandler` сразу после переноса в `customize()`.

Пример для telegram-воркера (с учётом TOOLS-31):

```go
// Было — internal/app/worker/telegrambot/commands.go
func init() {
	cmdMaps[CmdHelp] = telegram.Cmd{Name: CmdHelp, Private: true, Handler: helpHandler}
	unknownCommandHandler = resetDialog
}

// Стало
func (w *Worker) customize() {
	w.AddCommands(telegram.Cmd{Name: CmdHelp, Private: true, Handler: helpHandler})
	w.SetUnknownCommandHandler(resetDialog)
}
```

`customize()` по умолчанию промотирован из встроенного `EmptyCustomizer` и ничего не
делает; метод на `*Worker` его перекрывает. `Create()` зовёт его после регистрации
команд генератора, поэтому команда проекта с тем же `Name` перекрывает сгенерированную.
Подробности — [docs/configuration/workers.md](docs/configuration/workers.md) и
[docs/testing/goat.md](docs/testing/goat.md).

**Чем это держится.** `gochecknoinits` не видит `.tmpl`-файлы, а
`exclusions.generated: lax` выводит из проверки `psg_*_gen.go` целиком — включая
секцию ручного кода ниже disclaimer-маркера. Поэтому основной гард — не линтер, а
тест по шаблонам (`TestNoInitFuncInTemplates`) плюс проверка результата генерации
(`TestNoInitFuncInGeneratedProjects`); они от конфига проекта не зависят. Если проект
забирает конфиг линтера себе (`tools.lint_config.own`), запрет `init()` в **его** коде
остаётся на нём.

### Мутабельные package-level переменные в сгенерированных проектах (TOOLS-31, unreleased)

`gochecknoglobals` включён в `configs/golangci-lint.yml` сгенерированного проекта (и в
конфиге самого генератора), **с исключением для `_test.go`** — там табличные тесты
держат наборы случаев в package-level переменных, и запрет ничего не защищает.
Продолжение TOOLS-30: там точки расширения перестали требовать `init()`, здесь — быть
переменными, которым проект присваивает новое значение.

**Почему вообще.** У подменяемого глобала три свойства, которых у поля нет: менять его
может кто угодно в пакете (а не только владелец), два экземпляра компонента в одном
процессе делят одно значение, и подмена не видна как вызов — это запись в переменную
чужого пакета. Последнее не теория: `tgSHA` в драйвере telegram писался горутиной
наблюдателя OnlineConf при ротации токена и читался горутинами запросов при подписи
инвойса — гонка, которую видно только если держать в голове обе стороны. Два бота в
одном процессе вдобавок подписывали бы инвойс ключом друг друга.

**Почему без переходного периода** — по той же причине, что в TOOLS-30: оставить старый
глобал deprecated и синхронизировать его с новым полем значит сохранить ровно то, что
убирается, и при этом получить красный merge-гейт от линтера, который включается этим
же релизом. Компилятор показывает каждое место точно.

| Было | Стало | Миграция |
|------|-------|----------|
| `CustomError[ErrX] = handler` (telegram-воркер) | реестр — поле `w.customErrors` | `w.AddErrorHandler(ErrX, handler)` из `customize()`. Реестр стал срезом: проверка идёт в порядке регистрации, первое совпадение по `errors.Is` выигрывает. Прежний обход карты был рандомизирован, то есть при двух подходящих записях ответ менялся от запроса к запросу |
| `updateHandler = …` / `updateHandlerIdle = …` | поля воркера | `w.SetUpdateHandler(h, idle)` из `customize()`; у синхронного обработчика `idle` = `nil` |
| `unknownCommandHandler = …` | поле воркера | `w.SetUnknownCommandHandler(h)` из `customize()`; сигнатура обработчика не изменилась |
| `handler.UnimplementedResponse` (значение) | функция | `handler.UnimplementedResponse()` |
| `telegram.PackInvoiceData(ctx, d)` / `telegram.UnpackInvoiceData(ctx, &d, s)` | методы драйвера | `srv.TelegramPackInvoiceData(ctx, d)` / `srv.TelegramUnpackInvoiceData(ctx, &d, s)` |
| `telegram.GetTgTokenSHA(ctx)` | метод драйвера | `srv.TelegramTokenSHA(ctx)`. Ключ нужен для проверки подписи initData мини-аппа, поэтому путь к нему снаружи сохранён — вместе с `telegram.MakeSHA256Hash`, которая осталась функцией пакета |
| `daemon.ErrorTimeout` / `NewCycleTimeout` (экспортированные, читали ENV в инициализаторе) | поля, ENV читается в `Create()` | имена ENV-переменных не изменились; константы дефолтов — `DefaultErrorTimeout` / `DefaultNewCycleTimeout`. Присваивание из Go (обычно в тесте проекта, чтобы цикл не держал тест) — `Create().SetLoopTimeouts(…)`. Побочно исправлено: инициализатор срабатывал до `godotenv.Load(".env")`, поэтому значение из `.env` не доезжало — `Create()` зовётся после загрузки |
| `TestRetryConfig.MaxAttempts = 20` (goat-хелперы) | функция + параметр | читать — `TestRetryConfig().MaxAttempts`; **настроить** бэкофф `WaitForAPI` — `WaitForAPIWithRetry(t, url, maxWait, cfg)`, потому что присваивание полю больше ни на что не влияло бы: `WaitForAPI` берёт параметры вызовом |
| `env`, `appConfig`, `testEnvInit` и два `sync.Once` в `tests/<app>/` | одна структура `testEnvState` | кода проекта не касается: читалось через `GetEnv()` / `GetAppConfig()`, подменялось через `SetTestEnvInitializer` — и то и другое осталось |

Пример для telegram-воркера:

```go
// Было — internal/app/worker/telegrambot/dispatch.go
func (w *Worker) customize() {
	updateHandler = (*Worker).dispatchToQueues
	updateHandlerIdle = queuesDrained
	unknownCommandHandler = resetDialog
	CustomError[ErrSubscriptionExpired] = renewPrompt
}

// Стало
func (w *Worker) customize() {
	w.SetUpdateHandler(w.dispatchToQueues, w.queuesDrained)
	w.SetUnknownCommandHandler(resetDialog)
	w.AddErrorHandler(ErrSubscriptionExpired, renewPrompt)
}
```

`idle` — параметр того же сеттера, а не отдельного: `GracefulStop` ждёт не только
пустого канала апдейтов, но и того, что обработчик закончил начатое. Отдельный сеттер
позволял заменить обработчик и промолчать про `idle` — и воркер останавливался на
полпути; здесь про неё приходится сказать хотя бы `nil`.

**Если имя вашего метода совпало со сгенерированным.** Прежняя документация
предлагала объявить в пакете воркера свои `dispatchUpdate` / `updatesIdle` и
присвоить их хукам. Генератор такие имена не занимает (его методы называются
`invokeUpdateHandler` / `updateHandlerIsIdle`) — ровно чтобы это не ломало сборку.
Но если имя всё же столкнулось, компилятор скажет `method Worker.X already
declared at …psg_…_gen.go`, и чинится это переименованием **вашего** метода:
сгенерированный файл перезапишется на следующем `make regenerate`.

**Что осталось package-level** — только с `//nolint:gochecknoglobals` и обоснованием:
переменные под `-ldflags` (`Version`, `BuildCommit`, …), коллекторы Prometheus в
сгенерированном пакете метрик, таблицы required-ключей в сгенерированном пакете
OnlineConf и синглтон тестового окружения goat. Sentinel-ошибки, `regexp.MustCompile`,
`//go:embed` и `var _ I = (*T)(nil)` линтер пропускает сам.

**Чем это держится.** Тем же, чем запрет `init()`, и по той же причине:
`gochecknoglobals` не видит `.tmpl`-файлы, а `exclusions.generated: lax` выводит из
проверки `psg_*_gen.go` целиком — включая секцию ручного кода ниже disclaimer-маркера.
Гарды — `TestNoUnjustifiedGlobalsInTemplates` (скан шаблонов) и
`TestNoUnjustifiedGlobalsInGeneratedProjects` (разбор результата генерации как
настоящего Go — по манифесту, то есть только файлов, которые записал сам стартер).
Второй нужен потому, что шаблон не валидный Go и объявление, появляющееся только после
подстановки, построчному скану не видно.

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
