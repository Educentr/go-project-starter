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
проект, на все job обоих файлов. Там же живёт список проверок
([`checks`](#checks)), на который гейтятся `build` и `build-packages`.

## Параметры

```yaml
github_ci:
  runs_on:                 # раннер для всех job; по умолчанию ubuntu-latest
    group: my-runners
    labels: [linux, x64]
  job_timeout_minutes: 30  # timeout-minutes у всех job; по умолчанию не выводится
  doctl_version: 1.170.0   # version у action-doctl; по умолчанию не выводится
  checks:                  # matrix-job checks; по умолчанию не генерируется
    - name: lint
      run: make lint
  checks_config:
    fail_fast: false
```

Все поля опциональны. Блок целиком необязателен: без него сгенерированные
workflow побайтно совпадают с прежними (`ubuntu-latest`, без
`timeout-minutes`, doctl `latest`, без job `checks`), поэтому регенерация
существующих проектов ничего не меняет.

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

## `checks`

### Зачем

До появления `checks` единственной проверочной фазой в сгенерированном
`ci_cd.yml` был `run-goat-tests-<app>`. Ни линтер, ни юнит-тесты CI не
запускал, хотя `Makefile` (цели `lint`, `unit-tests`, …) и
`configs/golangci-lint.yml` генератор кладёт в проект. Проверки дописывались
руками отдельными воркфлоу рядом со сгенерированным: каждая со своим триггером
и ни одна — в `needs:`, поэтому упавший линтер **не останавливал** `build` и
`deploy`.

`checks` возвращает эти проверки в один файл и в один граф зависимостей.
Генератору не нужно знать, что проверка делает: достаточно имени и команды —
проект сохраняет свои собственные гварды.

### Формат

```yaml
github_ci:
  checks:
    - name: lint
      run: make lint
    - name: unit-tests
      run: make unit-tests
    - name: guards
      run: make lint-guards
      registry_login: true      # по умолчанию false — линтеру реестр не нужен

  checks_config:
    fail_fast: false            # по умолчанию false
```

Уровень — **проектный**: линт и юнит-тесты относятся к модулю, а не к
приложению, поэтому список один на проект, а не на приложение.

#### Поля `checks[]`

| Поле | Тип | Обязательное | Описание |
|---|---|---|---|
| `name` | string | да | Имя проверки, уникальное в пределах проекта. Становится именем ноги матрицы |
| `run` | string | да | Команда, которую проверка выполняет |
| `registry_login` | bool | нет | `true` — залогиниться в container registry перед запуском. По умолчанию `false` |

#### Поля `checks_config`

| Поле | Тип | По умолчанию | Описание |
|---|---|---|---|
| `fail_fast` | bool | `false` | `true` — первая упавшая проверка отменяет остальные |

`fail_fast: false` здесь несущий, а не косметика: со значением `true` первый
упавший линтер отменяет остальные ноги, и за один push автор узнаёт про одно
нарушение вместо всех.

### Что генерируется

Один job `checks` с матрицей — по ноге на объявленную проверку:

```yaml
  checks:
    runs-on: ubuntu-latest
    needs:
      - prepare-env
    if: needs.prepare-env.outputs.ENABLE_BUILD == 'true' && vars.SKIP_CHECKS != 'true'
    strategy:
      fail-fast: false
      matrix:
        include:
          - name: 'lint'
            run: 'make lint'
            registry_login: false
          - name: 'guards'
            run: 'make lint-guards'
            registry_login: true
    name: checks (${{ matrix.name }})
    steps:
      # checkout (fetch-depth: 0) / setup-go / GOPRIVATE — как в run-goat-tests-<app>
      - name: Run check ${{ matrix.name }}
        run: ${{ matrix.run }}
```

Шаги повторяют `run-goat-tests-<app>`: checkout, `setup-go` с `cache: true`,
git-config для приватных репозиториев, `GOPRIVATE`, отчёт `act10ns/slack@v2`
на `failure()`. Шаг логина в реестр генерируется, только если хотя бы одна
проверка объявила `registry_login: true`, и выполняется только в тех ногах
матрицы, где он включён.

`runs_on`, `job_timeout_minutes` и `doctl_version` действуют на job `checks`
так же, как на остальные job — на то он и объявлен в этом же блоке.

`fetch-depth: 0` — не деталь: сгенерированный `make lint` вызывается с
`--new-from-rev=origin/main`, и с дефолтным shallow-checkout ссылки
`origin/main` в раннере нет.

Гейт у `build` и `build-packages` — тем же приёмом, каким они сегодня
гейтятся на goat-тестах:

```yaml
  build:
    needs:
      - prepare-env
      - checks
    if: |
      always() &&
      needs.prepare-env.outputs.ENABLE_BUILD == 'true' &&
      needs.prepare-env.result == 'success' &&
      (needs.checks.result == 'success' || needs.checks.result == 'skipped')
```

### Почему матрица, а не job на проверку

- `needs.checks.result` агрегирует все ноги матрицы, поэтому у `build`
  остаётся **одна** запись в `needs` независимо от числа объявленных
  проверок: граф перестаёт меняться при добавлении линтера. Job на проверку
  даёт тот же параллелизм, но растит и `needs`, и число статусов;
- ноги матрицы — отдельные параллельные job'ы, так что wall-clock не растёт с
  числом проверок. Цена — повторный checkout и `setup-go` в каждой ноге.

### Совместимость и отключение

- Нет ключа `checks` — job не генерируется, сгенерированный `ci_cd.yml`
  побайтно совпадает с тем, что было до появления фичи. Существующие проекты
  не затрагиваются.
- `vars.SKIP_CHECKS` — лазейка для разового пропуска, повторяет существующую
  `SKIP_GOAT_TESTS`. Пропущенный job даёт `result == 'skipped'`, поэтому
  `build` не блокируется.

### Валидация

- `name` и `run` обязательны;
- `name` уникально в пределах проекта.

Нарушение — ошибка загрузки конфига (`invalid config github_ci section: …`),
генерация не стартует.

## Область действия

Блок действует только на **GitHub Actions**. GitLab CI (`.gitlab-ci.yml`,
`tags:` / `timeout:` / проверки) он не затрагивает: у `.gitlab-ci.yml` своя
модель job'ов, и аналогичные ключи получит отдельный блок `gitlab_ci:`.

Не путать с `main.ci` — это список CI-провайдеров, которых нужно
сгенерировать (`github`, `gitlab`), см. [Основные секции](main.md).

## Реализация

- Конфиг: `GithubCIConfig` в `internal/pkg/config/github_ci.go` (парсинг и
  валидация `runs_on` — `ParseRunsOn`, проверок — `validateChecks`), проверка
  вызывается из `config.GetConfig`.
- Нормализованный вид: `ds.GithubCIConfig` / `ds.RunsOnSpec` /
  `ds.ChecksConfig` в `internal/pkg/ds/const.go`.
- Шаблоны подставляют одно выражение `{{ $.GithubCI.JobHeader }}` на job —
  оно рендерит и `runs-on:`, и `timeout-minutes:`, поэтому job'ы не могут
  разойтись.
- Имена и команды проверок попадают в YAML дословно из `project.yaml`, поэтому
  шаблон прогоняет их через хелпер `YAMLQuote`.
