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

## Второй класс проблемы: `@latest` у тестовых импортов зависимостей

Minimal toolchain (выше) — про случай «локальный Go **старее** `go.mod`». Есть
симметричный класс: проект запинен на **старую** версию Go, а `go mod tidy` не
может разрешить граф, потому что тянет чужой `@latest`.

### Механика

`go mod tidy` резолвит не только импорты самого модуля, но и импорты **тестов
пакетов, которые модуль импортирует**. Такой пакет может не требоваться ни
одним модулем в графе — тогда для него нет версии в `require`, и tidy берёт
`@latest`. Если `@latest` объявляет `go >= X`, а проект запинен ниже X, под
`GOTOOLCHAIN=local` подняться некуда, и tidy падает целиком:

```
go: toolchain upgrade needed to resolve go.opentelemetry.io/otel/exporters/otlp/otlptrace
go: ...@v1.47.0 requires go >= 1.26.0 (running go 1.24.4; GOTOOLCHAIN=local)
```

Реальный кейс (TOOLS-51): goat-проект на `tools.golang_version: "1.24"`.
Цепочка — `goat-services` → `testcontainers-go` → `docker/client` (*tested by*)
→ `docker/testutil` → `otlptrace`. Шаг `post_generate: go_mod_tidy` падал, а
ошибка post_generate прерывает генерацию — **первичная генерация goat-проекта
на тулчейне 1.24 не доходила до конца**.

Почему дефект долго не был виден: у разработчика с Go >= 1.26 и
`GOTOOLCHAIN=auto` tidy просто докачивает новый тулчейн и проходит. Воспроизводится
только там, где тулчейну запрещено подниматься, — в частности в официальных
`golang:*` образах, которые ставят `GOTOOLCHAIN=local`.

### Политика

1. **Чиним пином, а не `-e`.** `go mod tidy -e` проводит tidy мимо
   неразрешимых пакетов и этим маскирует весь класс ошибок зависимостей для
   всех проектов — в цель `tidy` его добавлять нельзя.
2. **Версия пина — из той же релизной линейки, что соседние модули в
   `go.mod`.** Для otlptrace это `v1.33.0` под уже присутствующий
   `go.opentelemetry.io/otel v1.33.0`, плюс `go.opentelemetry.io/proto/otlp
   v1.4.0` — ровно то, что требует сам otlptrace. Так MVS не двигает остальной
   граф.
3. **Пин помечается комментарием** с причиной и условием снятия — как уже
   сделано для `github.com/gofrs/uuid/v5` в `go.mod.tmpl`.
4. **Снимать пин можно**, когда минимальная поддерживаемая версия Go догонит
   требование `@latest`. Проверка — не глазами, а прогоном
   `make integration-test-goat`.
5. **Поднимать `tools.golang_version` вместо пина — не вариант**: это сужает
   поддерживаемый диапазон Go всем потребителям ради чужой тестовой
   зависимости.

### Чем это закрыто в CI

`TestIntegrationRESTEnvsGoat` (`test/docker-integration/integration_test.go`)
генерирует фикстуру `rest-envs-goat` (goat + Go 1.24 + `go_mod_tidy` в
`post_generate`) внутри образа `golang:1.24.4`. Тест отдельно проверяет свои
предпосылки — `GOTOOLCHAIN=local` в образе и `go 1.24` в сгенерированном
`go.mod`, — чтобы не превратиться в «зелёный по неинтересной причине».
Локально: `make integration-test-goat`.

## Применение

| Сценарий | Действие |
|---|---|
| Локально перед коммитом | `make check-go-version` |
| Локально починить | поправить `go <X>` в `go.mod` до `max(deps)` |
| Bump обоснован, нужно зафиксировать | создать/обновить `.go-version-rationale` |
| Зависимость требует более нового Go | `make check-go-version` подскажет конкретное `dep@ver` |

## Что **нельзя** делать

- `go tool <X>` в шаблонах `*.tmpl` для `<X>` не из minimal-набора (см. список выше). Если очень нужно — добавить preflight check `go tool -n <X> >/dev/null 2>&1` с понятным WARN-сообщением, **без** маскировки через `2>/dev/null || true`. Именно `-n` (печатает путь до тула, exit 0 если есть, exit 2 + `no such tool` если нет): пробовать `go tool <X> help` нельзя — селектор `help` есть не у всех тулов (`go tool covdata help` → exit 2 даже на полном toolchain), такая проба всегда рапортует «тула нет».
- Bump `go <X>` в `go.mod` без согласованного rationale.
- Подъём `tools.golang_version` в `project.yaml` тестовых конфигов «для синхронизации с основной версией». Тестовые конфиги — отдельная история, цельтесь в реальный минимум.

## Связанные файлы

- `scripts/check-go-mod-version.sh` — проверка для нашего репо (strict, within minor)
- `internal/pkg/templater/embedded/templates/main/scripts/check-go-mod-version.sh.tmpl` — для сгенерированного проекта (lenient, lower bound)
- `.github/workflows/go-version-check.yml` — CI job нашего репо
- `internal/pkg/templater/embedded/templates/main/.github/workflows/go-version-check.yml.tmpl` — CI job сгенерированного проекта
- `Makefile` / `Makefile.tmpl` — target `check-go-version`
- `internal/pkg/templater/embedded/templates/main/go.mod.tmpl` — пины транзитивных зависимостей (см. «Второй класс проблемы»)
- `test/docker-integration/integration_test.go` — `TestIntegrationRESTEnvsGoat`, страховка от регрессии этих пинов
