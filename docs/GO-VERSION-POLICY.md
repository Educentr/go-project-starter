# Go Version Policy

## TL;DR

Версия `go <X>` в `go.mod` — **минимальная** из всех совместимых с зависимостями, без косметических округлений вверх. Любой bump поверх максимума `go`-требований direct dependencies обязан сопровождаться обоснованием (используемая Go-фича или жёсткое требование зависимости).

Проверка автоматизирована: CI запускает `scripts/check-go-mod-version.sh`, локально — `make check-go-version`.

## Зачем эта политика существует

История из марта 2026: коммит [e79f163](#) поднял `go.mod` 1.24.4 → 1.26.1 «как косметическое обновление до последней версии Go». В исходниках Go-1.26-фичи не использовались, никакая зависимость не требовала 1.26. Спустя несколько недель в репорте всплыл свежий класс проблемы.

### Корневая причина: minimal toolchain в `GOTOOLCHAIN=auto`

Когда локально установленный Go **строго меньше** версии из `go.mod`, Go (с включённым по умолчанию `GOTOOLCHAIN=auto`) скачивает toolchain через module proxy:

```
~/godev/pkg/mod/golang.org/toolchain@v0.0.1-go<ver>.linux-amd64/
```

Содержимое этого toolchain — **намеренно урезанное** (для скорости загрузки):

```
pkg/tool/linux_amd64/   ← всего 8 файлов:
  asm  cgo  compile  cover  fix  link  preprofile  vet
```

Отсутствуют: `covdata`, `buildid`, `pack`, `test2json`, `doc`, `pprof`, `trace`, `addr2line`, `nm`, `objdump`.

Полный набор тулзов есть только в **системном** дистрибутиве Go (`/usr/local/go`, `apt install golang-X`, `brew install go`, `go.dev/dl/*.tar.gz`).

### Эффект

Любой вызов `go tool <X>` для `<X>` не из minimal-списка работает у разработчика (на полном Go), но **молча или громко ломается у пользователя**, чей локальный Go старее `go.mod` версии.

Конкретный кейс, из-за которого писалась эта политика — `go tool covdata textfmt` в шаблоне `goat-tests-<app>` (Makefile.tmpl). Он использовался с `2>/dev/null || true`, превращая отсутствие covdata в silent broken coverage без видимой ошибки.

### Почему это **повторяющаяся** проблема

Каждый bump `go <X>` в `go.mod` сдвигает границу «локальный Go < go.mod», расширяя круг пострадавших. Через год bump на 1.28 затронет всех, кто остался на 1.26. Это **структурный** баг, не разовая ошибка.

## Политика

### 1. Минимум `go.mod` = `max(go-requirement)` direct dependencies

Чтобы посчитать вручную:

```bash
for d in $(go list -m -f '{{if and (not .Indirect) (not .Main)}}{{.Path}}@{{.Version}}{{end}}' all); do
    awk '/^go [0-9]+\.[0-9]+/ {print $2"  "FILENAME; exit}' \
        "$(go env GOMODCACHE)/cache/download/${d%@*}/@v/${d#*@}.mod"
done | sort -V | tail -3
```

Эта же логика — в `scripts/check-go-mod-version.sh`.

### 2. Bump только при использовании Go-фичи / жёстком требовании

Bump допустим если:

- В коде реально используется API, появившийся в новой версии Go (`iter.Seq`, новые `slices`/`maps` функции, `range over int/func`, новый stdlib API).
- Зависимость в `go.mod` требует более новый Go (видно через `awk '/^go /' .mod`-файлов из module cache).

Bump «потому что вышла новая версия» — **запрещён**. В PR-описании bump'а версия должен быть назван конкретный артефакт (фича/функция/зависимость).

### 3. Обход через `.go-version-rationale`

Если bump обоснован, но связь не парсится автоматически (например, используется фича, которую `go vet` не отлавливает), в корне репо создаётся файл `.go-version-rationale` с описанием:

```
$ cat .go-version-rationale
Bumped to go 1.27 — uses iter.Pull2 in pkg/streaming/iter.go
introduced in Go 1.27 (https://pkg.go.dev/iter#Pull2).
```

`scripts/check-go-mod-version.sh` читает этот файл и пропускает policy violation если он не пуст.

### 4. CI проверяет автоматически

Workflow `.github/workflows/go-version-check.yml` запускает скрипт на каждый PR. Зелёный билд — обязательное условие мерджа.

Та же проверка генерируется и в каждый сгенерированный проект:
- `scripts/check-go-mod-version.sh` (lenient — только lower bound, потому что верхняя граница в сгенерированном проекте задаётся `tools.golang_version` в `project.yaml`, и это решение пользователя)
- `.github/workflows/go-version-check.yml`
- `make check-go-version`

## Применение

| Сценарий | Действие |
|---|---|
| Локально перед коммитом | `make check-go-version` |
| Локально починить | поправить `go <X>` в `go.mod` до `max(deps)` |
| Bump обоснован, нужно зафиксировать | создать/обновить `.go-version-rationale` |
| Зависимость требует более нового Go | `make check-go-version` подскажет конкретное `dep@ver` |

## Что **нельзя** делать

- `go tool <X>` в шаблонах `*.tmpl` для `<X>` не из minimal-набора (см. список выше). Если очень нужно — добавить preflight check `go tool <X> help >/dev/null 2>&1` с понятным WARN-сообщением, **без** маскировки через `2>/dev/null || true`.
- Bump `go <X>` в `go.mod` без согласованного rationale.
- Подъём `tools.golang_version` в `project.yaml` тестовых конфигов «для синхронизации с основной версией». Тестовые конфиги — отдельная история, цельтесь в реальный минимум.

## Связанные файлы

- `scripts/check-go-mod-version.sh` — проверка для нашего репо (strict, within minor)
- `internal/pkg/templater/embedded/templates/main/scripts/check-go-mod-version.sh.tmpl` — для сгенерированного проекта (lenient, lower bound)
- `.github/workflows/go-version-check.yml` — CI job нашего репо
- `internal/pkg/templater/embedded/templates/main/.github/workflows/go-version-check.yml.tmpl` — CI job сгенерированного проекта
- `Makefile` / `Makefile.tmpl` — target `check-go-version`
