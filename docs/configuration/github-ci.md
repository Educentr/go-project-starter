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
  doctl, и версия CLI в CI расходилась с закреплённой в остальных сборках.

Правка сгенерированного файла стиралась следующей регенерацией, и потеря не
краснела: на `ubuntu-latest` с `latest` doctl workflow остаётся зелёным.

Блок `github_ci:` объявляет эти три величины в `project.yaml` — один раз на
проект, на все job обоих файлов.

## Параметры

```yaml
github_ci:
  runs_on:                 # раннер для всех job; по умолчанию ubuntu-latest
    group: my-runners
    labels: [linux, x64]
  job_timeout_minutes: 30  # timeout-minutes у всех job; по умолчанию не выводится
  doctl_version: 1.170.0   # version у action-doctl; по умолчанию не выводится
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

## Область действия

Блок действует только на **GitHub Actions**. GitLab CI (`.gitlab-ci.yml`,
`tags:` / `timeout:`) он не затрагивает.

Не путать с `main.ci` — это список CI-провайдеров, которых нужно
сгенерировать (`github`, `gitlab`), см. [Основные секции](main.md).

## Реализация

- Конфиг: `GithubCIConfig` в `internal/pkg/config/github_ci.go` (парсинг и
  валидация `runs_on` — `ParseRunsOn`), проверка вызывается из
  `config.GetConfig`.
- Нормализованный вид: `ds.GithubCIConfig` / `ds.RunsOnSpec` в
  `internal/pkg/ds/const.go`.
- Шаблоны подставляют одно выражение `{{ $.GithubCI.JobHeader }}` на job —
  оно рендерит и `runs-on:`, и `timeout-minutes:`, поэтому job'ы не могут
  разойтись.
