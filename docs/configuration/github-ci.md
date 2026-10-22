# GitHub CI

## Зачем

Шаблоны GitHub Actions (`.github/workflows/ci_cd.yml` и
`.github/workflows/go-version-check.yml`) раньше задавали литералом три
величины, которые у проекта бывают своими:

- `runs-on: ubuntu-latest` в каждой job — проект, который гоняет CI на
  собственном раннере, параметра не имел;
- `timeout-minutes` не было ни в одной job — зависшая job висела в
  `in_progress` до 6-часового лимита GitHub и всё это время держала слот
  self-hosted раннера;
- `digitalocean/action-doctl@v2` без `version:` — каждая job ставила `latest`
  doctl, и версия CLI в CI расходилась с закреплённой в остальных сборках;
- шаг установки doctl генерировался **всегда** — проект, у которого doctl
  запечён в образ собственного раннера, выключить его не мог, а установка в
  каждой джобе гоняется за общий tool-cache раннера (см. [`doctl`](#doctl)).

Правка сгенерированного файла стиралась следующей регенерацией, и потеря не
краснела: на `ubuntu-latest` с `latest` doctl workflow остаётся зелёным.

Блок `github_ci:` объявляет эти величины в `project.yaml` — один раз на
проект, на все job обоих файлов.

## Параметры

```yaml
github_ci:
  runs_on:                 # раннер для всех job; по умолчанию ubuntu-latest
    group: my-runners
    labels: [linux, x64]
  job_timeout_minutes: 30  # timeout-minutes у всех job; по умолчанию не выводится
  doctl_version: 1.170.0   # version у action-doctl; по умолчанию не выводится
  doctl: install           # откуда брать doctl: install (по умолчанию) или image
```

Все поля опциональны. Блок целиком необязателен: без него сгенерированные
workflow побайтно совпадают с прежними (`ubuntu-latest`, без
`timeout-minutes`, doctl `latest`), поэтому регенерация существующих проектов
ничего не меняет.

### `runs_on`

Принимает три формы.

Одна метка:

```yaml
github_ci:
  runs_on: self-hosted
```

```yaml
# .github/workflows/ci_cd.yml
    runs-on: self-hosted
```

Список меток — job попадёт на раннер, у которого есть **все** указанные метки:

```yaml
github_ci:
  runs_on: [self-hosted, linux, x64]
```

```yaml
    runs-on: [self-hosted, linux, x64]
```

Группа раннеров, опционально суженная метками:

```yaml
github_ci:
  runs_on:
    group: my-runners
    labels: [linux]
```

```yaml
    runs-on: {group: my-runners, labels: [linux]}
```

В объектной форме `group` обязателен, `labels` — нет; других ключей нет.
Метки и имена групп ограничены символами `A-Z a-z 0-9 . _ / -` и одиночными
внутренними пробелами — этого достаточно для любых реальных меток GitHub, а
подстановка остаётся безопасной без кавычек. Значение вне этого набора —
ошибка валидации конфига.

### `job_timeout_minutes`

Целое число больше нуля. Выводится как `timeout-minutes:` у **каждой** job
обоих файлов:

```yaml
github_ci:
  job_timeout_minutes: 30
```

```yaml
  prepare-env:
    runs-on: ubuntu-latest
    timeout-minutes: 30
```

Если параметр не задан, ключ не выводится и действует лимит GitHub в 6 часов.
`0` и отрицательные значения — ошибка валидации: GitHub такой timeout не
принимает, а «выключить» таймаут можно, просто не задавая параметр.

### `doctl_version`

Версия doctl, которую ставит `digitalocean/action-doctl@v2` в шагах логина в
DigitalOcean Container Registry (актуально при
`main.registry_type: digitalocean`):

```yaml
github_ci:
  doctl_version: 1.170.0
```

```yaml
      - name: Install doctl
        uses: digitalocean/action-doctl@v2
        with:
          token: ${{ secrets.REGISTRY_PASSWORD }}
          version: 1.170.0
```

Одно объявление на проект попадает во **все** шаги `action-doctl`. Если
параметр не задан, вход `version:` не выводится и action ставит последний
релиз.

В режиме `doctl: image` параметр не применяется (версию задаёт образ
раннера) — сочетание двух ключей это ошибка валидации, а не молчаливое
игнорирование.

### `doctl`

Откуда в job берётся сам бинарь `doctl`. Два значения:

- `install` (по умолчанию) — doctl ставит `digitalocean/action-doctl@v2`,
  токен уходит в вход `token:` экшена, версия берётся из `doctl_version`.
  Это прежнее поведение, выводится побайтно как раньше;
- `image` — doctl уже есть в образе раннера. Шага установки нет **вовсе**:
  ни экшена, ни загрузки, ни условного fallback'а.

```yaml
github_ci:
  doctl: image
```

```yaml
      - name: Log in to DO Container Registry
        env:
          DIGITALOCEAN_ACCESS_TOKEN: ${{ secrets.REGISTRY_PASSWORD }}
        run: |
          doctl version
          doctl registry login
```

Что даёт `image` проекту на собственном раннере с запечённым doctl:

- **нет гонки за общий tool-cache.** `action-doctl` пишет в
  `RUNNER_TOOL_CACHE` через `@actions/tool-cache`; на self-hosted хосте с
  несколькими слотами этот каталог часто один на всех, и джобы, стартовавшие
  одновременно, ломают друг другу распаковку (`ENOENT … unlink
  '<toolcache>/doctl/<ver>/x64.complete'`, `spawn ETXTBSY`). В шаблоне это
  условие возникает на каждом прогоне: джоба `build` — матрица по
  контейнерам, и каждая её ячейка ставила doctl параллельно;
- **нет мягкого пина версии.** При неудаче загрузки `action-doctl` ставит
  другую версию из списка последних релизов, сообщая об этом только
  `core.warning` — джоба зелёная, версия CLI не та, что объявлена. Здесь
  версию задаёт образ;
- **токена нет в argv.** `action-doctl` делает `doctl auth init -t <token>`,
  и токен виден в списке процессов хоста раннера. `doctl` читает
  `DIGITALOCEAN_ACCESS_TOKEN` из окружения, а для `doctl registry login`
  `auth init` не нужен. Токен объявлен в env **шага**, не job;
- **нет сетевой загрузки** doctl в каждой джобе — одной точкой отказа меньше.

`doctl version` первой строкой — не диагностика, а громкая остановка: если
образ раннера перестал давать doctl, джоба краснеет на этом шаге, а не тихо
доставляет бинарь откуда-то ещё.

Ограничения режима:

- `doctl: image` вместе с `doctl_version` — **ошибка валидации конфига**:
  версию в этом режиме задаёт образ раннера, применить параметр некуда;
- `doctl: image` при `main.registry_type` ≠ `digitalocean` — тоже **ошибка**:
  шаги с doctl генерируются только для DigitalOcean-реестра, то есть ключ ни
  на что бы не повлиял. Молча игнорировать его значит оставить проект с
  ложным ощущением, что doctl настроен. Режим `install` при этом валиден для
  любого `registry_type` — он умолчание, и у остальных реестров doctl-шагов
  просто нет;
- любое другое значение ключа — ошибка валидации.

Образ раннера должен давать doctl всем job'ам, которые пушат образы: это
джоба GOAT-тестов и матричная джоба `build`.

## Область действия

Блок действует только на **GitHub Actions**. GitLab CI (`.gitlab-ci.yml`,
`tags:` / `timeout:`) он не затрагивает — в том числе `doctl`: шаблон GitLab
ставит doctl своими средствами и этим ключом не управляется.

Не путать с `main.ci` — это список CI-провайдеров, которых нужно
сгенерировать (`github`, `gitlab`), см. [Основные секции](main.md).

## Реализация

- Конфиг: `GithubCIConfig` в `internal/pkg/config/github_ci.go` (парсинг и
  валидация `runs_on` — `ParseRunsOn`, нормализация `doctl` — `DoctlMode`),
  проверка вызывается из `config.GetConfig` и получает `main.registry_type`:
  режим doctl валидируется против типа реестра.
- Нормализованный вид: `ds.GithubCIConfig` / `ds.RunsOnSpec` / `ds.DoctlMode`
  в `internal/pkg/ds/const.go`; шаблон ветвится по
  `{{ $.GithubCI.DoctlFromImage }}`, поэтому нулевой конфиг оставляет шаг
  установки.
- Шаблоны подставляют одно выражение `{{ $.GithubCI.JobHeader }}` на job —
  оно рендерит и `runs-on:`, и `timeout-minutes:`, поэтому job'ы не могут
  разойтись.
