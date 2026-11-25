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
  каждой джобе гоняется за общий tool-cache раннера (см. [`doctl`](#doctl));
- `on.push.branches` в обоих файлах был литералом `[main, staging]` — проект,
  который собирает ещё и `release/**`, своей ветки в триггер добавить не мог;
- шаг «Build and push» получал ровно две env-переменные (`REGISTRY_SERVER`,
  `GITHUB_TOKEN`) — проекту, которому в сборку образа нужно значение из
  `vars`/`secrets` репозитория, оставался sed-патч сгенерированного workflow в
  `post_generate`: вторая точка правды, которую каждый regenerate стирал, а
  патч накладывал заново (см. [`build_env`](#build_env)).

Правка сгенерированного файла стиралась следующей регенерацией, и потеря не
краснела: на `ubuntu-latest` с `latest` doctl workflow остаётся зелёным.

Блок `github_ci:` объявляет эти величины в `project.yaml` — один раз на
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
  doctl: install           # откуда брать doctl: install (по умолчанию) или image
  push_branches:           # ветки триггера on.push; по умолчанию [main, staging]
    - main
    - 'release/**'
  build_env:               # переменные шага «Build and push»; по умолчанию нет
    - name: IMAGE_REGISTRY
      value: "${{ vars.IMAGE_REGISTRY }}"
  checks:                  # matrix-job checks; по умолчанию не генерируется
    - name: lint
      run: make lint
  checks_config:
    fail_fast: false
```

Все поля опциональны. Блок целиком необязателен: без него сгенерированные
workflow побайтно совпадают с прежними (`ubuntu-latest`, без
`timeout-minutes`, doctl `latest`, push на `main` и `staging`, без job
`checks`), поэтому регенерация существующих проектов ничего не меняет.

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

### `push_branches`

Список фильтров ветки у триггера `on.push` **обоих** сгенерированных файлов.
По умолчанию — `[main, staging]`, прежний литерал шаблонов.

```yaml
github_ci:
  push_branches:
    - main
    - 'release/**'
    - '!release/**-alpha'
```

```yaml
# .github/workflows/ci_cd.yml
on:
  push:
    branches:
      - main
      - release/**
      - '!release/**-alpha'
```

```yaml
# .github/workflows/go-version-check.yml
on:
  push:
    branches: [main, release/**, '!release/**-alpha']
  pull_request:
```

Список **заменяет** умолчание, а не дополняет его: ветки триггера объявлены в
одном месте, и чтобы прочитать, на что собирается проект, не нужно держать в
голове ещё и неявные `main`/`staging`. Поэтому `main` в списке, где он нужен,
указывается явно.

Принимаются как имена веток, так и
[фильтры-шаблоны GitHub](https://docs.github.com/actions/reference/workflows-and-actions/workflow-syntax#filter-pattern-cheat-sheet):
`*`, `**`, `?`, `+`, диапазоны `[0-9]` и исключающий `!`.

В обоих файлах берётся в одинарные кавычки фильтр, который без них перестал бы
быть той строкой, какой объявлен. Таких два вида:

- **нельзя вывести простым скаляром** — начинается с `!`, содержит `[`;
- **читается YAML как другой тип** — `1.10` (float `1.1`, нуля больше нет),
  `123`, `0x1F`, `1e3`, `true`, `null`, `2026-11-17` (timestamp), а также
  слова, которые считает булевыми YAML 1.1 (`yes`, `no`, `on`, `off`). GitHub
  ждёт в `branches` последовательность строк, поэтому имя ветки, похожее на
  число, обязано дойти до фильтра строкой.

`main` и прочие обычные имена остаются без кавычек, поэтому проект на
умолчании регенерируется побайтно.

Триггер `pull_request` в `go-version-check.yml` не фильтруется по ветке — как
и раньше, проверка идёт на каждый PR; этот ключ на него не влияет.

Валидация (ошибка загрузки конфига, генерация не стартует):

- пустой список (`push_branches: []`) — триггер `on.push` без фильтров ветки
  не выводится, а «вернуть умолчание» значит не задавать ключ вовсе;
- пустой фильтр, пробелы, управляющие символы, `:`, `#`, кавычки — ни одно
  имя ветки git и ни один фильтр GitHub их не содержит, а в YAML они ломают
  скаляр, поэтому это отказ, а не экранирование;
- дубль в списке.

Имя, похожее на число или на булево (`1.10`, `123`, `true`, `no`,
`2026-11-17`), берите в `project.yaml` **в кавычки**: без них YAML ещё на
входе читает его не строкой, и в фильтр уезжает уже приведённое значение
(`1.10` → `1.1`, `true` → `1`). На выводе такие имена квотируются, но
исправить уже прочитанное значение генератор не может.

#### Переменные ветки: что нужно, чтобы на ней собиралось

Триггер — не допуск. Все job'ы `ci_cd.yml` гейтятся на **переменной
репозитория** `ENABLED_<СУФФИКС>`, где суффикс — имя ветки, приведённое к
набору символов, который GitHub допускает в именах переменных и секретов:
всё, что не `[A-Za-z0-9_]`, заменяется на `_`, затем верхний регистр.

| ветка | переменная допуска |
|---|---|
| `main` | `ENABLED_MAIN` |
| `staging` | `ENABLED_STAGING` |
| `release/1.2` | `ENABLED_RELEASE_1_2` |
| `hotfix-x` | `ENABLED_HOTFIX_X` |

Значения: `ENABLED` — сборка и деплой, `ENABLED_ONLY_BUILD` — только сборка.

Ветка **без** такой переменной (или с другим значением) даёт **зелёный run, в
котором ничего не собрано**: workflow запускается, `prepare-env` пишет в лог
`Skip branch …` и аннотацию `Build is not enabled for this branch`, остальные
job'ы пропускаются. Так что добавить фильтр в `push_branches` — половина дела;
вторая половина — завести переменную на каждую ветку, на которой сборка
действительно нужна.

По тому же правилу строятся имена **всех** остальных переменных и секретов
ветки, которые читает деплой (нужны при `ENABLED`): `ENV_TYPE_<СУФФИКС>`,
`OC_HOST_<СУФФИКС>`, `OC_PORT_<СУФФИКС>`, `SSH_HOST_<СУФФИКС>`,
`INTERNAL_SUBNET_<СУФФИКС>`, `PORT_PREFIX_<APP>_TRAEFIK_<СУФФИКС>`,
`PORT_PREFIX_<APP>_<TRANSPORT>_<СУФФИКС>`, `DOMAIN_<TRANSPORT>_<СУФФИКС>` и
секреты `OC_USER_<СУФФИКС>`, `OC_PASSWORD_<СУФФИКС>`. Точный список шаг
`Get branch` в `prepare-env` печатает в лог.

Приведение к `[A-Za-z0-9_]` **склеивает** имена, различающиеся только
разделителем: `release/1.2`, `release-1.2` и `release_1.2` дают один суффикс
`RELEASE_1_2`, то есть читают одни и те же переменные и секреты. Если такие
ветки нужны одновременно, различать их должно не только разделителем.

Имя файла переменных деплоя нормализуется так же, только без верхнего
регистра: ветка `release/1.2` даёт `.env.release_1_2` — с сырым `/` это был бы
путь в несуществующий каталог, и шаг падал бы на первой записи в файл.

Фильтр-шаблон переменную **не** обобщает: `release/**` в триггере — одна
строка, а переменная нужна своя на каждую фактическую ветку
(`ENABLED_RELEASE_1_2`, `ENABLED_RELEASE_1_3`, …). Общего запасного значения
(вида `vars.ENABLED` для веток без своей переменной) нет осознанно: неявный
допуск означал бы сборку и деплой любой ветки, попавшей в фильтр.

Уведомление о сборке (job `artifact_version`, Telegram) приходит о **каждой**
ветке, на которой сборка включена: job гейтится на том же `ENABLE_BUILD`, по
ветке ничего не проверяется. Имя ветки уезжает в сообщение экранированным для
MarkdownV2, поэтому `release/1.2`, `hotfix-x` и `feature_x` доходят: на
неэкранированном спецсимволе Telegram отвечает 400 и сообщения нет.

Имя ветки попадает в шаги не workflow-выражением внутри `run:`, а переменной
окружения — как и остальные значения, которые выбирает не шаблон: см.
«[Значения в `run:` приходят через `env:`](#значения-в-run-приходят-через-env)».

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

Образ раннера должен давать doctl всем job'ам, которые логинятся в реестр: это
джоба GOAT-тестов, матричная джоба `build` и — если хотя бы одна проверка
объявила `registry_login: true` — джоба [`checks`](#checks).

### `build_env`

Переменные окружения, которые добавляются в `env:` шага «Build and push» джобы
`build` — рядом с `REGISTRY_SERVER` и `GITHUB_TOKEN`, которые шаблон ставит сам.
Элемент — объект:

| Поле | Обязательное | Значение |
|---|---|---|
| `name` | Да | Имя переменной, `[A-Za-z_][A-Za-z0-9_]*` |
| `value` | Да | Значение: выражение GitHub Actions или литерал. Попадает в workflow **как есть**, вычисляет его раннер |

```yaml
github_ci:
  build_env:
    - name: IMAGE_REGISTRY
      value: "${{ vars.IMAGE_REGISTRY || format('{0}/{1}', secrets.REGISTRY_LOGIN_SERVER, vars.REGISTRY_CONTAINER) }}"
    - name: UPSTREAM_REPO_URL
      value: "${{ github.server_url }}/${{ github.repository_owner }}/contracts.git"
```

```yaml
      - name: "Build and push"
        id: build_and_push
        env:
          REGISTRY_SERVER: ${{ secrets.REGISTRY_LOGIN_SERVER }}/${{ vars.REGISTRY_CONTAINER }}
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          # github_ci.build_env: the values come verbatim from project.yaml,
          # the runner evaluates the workflow expressions in them.
          IMAGE_REGISTRY: '${{ vars.IMAGE_REGISTRY || format(''{0}/{1}'', secrets.REGISTRY_LOGIN_SERVER, vars.REGISTRY_CONTAINER) }}'
          UPSTREAM_REPO_URL: '${{ github.server_url }}/${{ github.repository_owner }}/contracts.git'
        run: |
          make ${{ matrix.containers }}
          make ${{ matrix.containers }}-push
```

Значение всегда выводится как одинарно-кавычная YAML-строка (`'` внутри
удваивается): выражение вроде `format('{0}/{1}', …)` несёт `'`, `,` и `{}`,
которые plain-скаляр не выдерживает. На вычисление выражения кавычки не влияют —
YAML разбирается до подстановки, раннер видит ту же строку, что в
`project.yaml`.

**Зачем это нужно.** `env:` шага — единственный способ отдать сборке образа
значение из `vars`/`secrets` репозитория: контекст `vars` сам в окружение шага
не попадает, а из Makefile его не достать. Обратный конец цепочки уже закрывает
[`docker.build_args`](docker.md#build_args): переменная окружения шага, у
которой имя совпадает с объявленным build-аргументом, подхватывается
сгенерированным Makefile (`<name> ?= <default>` оставляет приоритет за
окружением) и уходит в образ через `--build-arg`. То есть цепочка целиком —
`vars`/`secrets` → `env:` шага → `?=` в Makefile → `--build-arg` → `ARG` в
Dockerfile.

Проверки на загрузке конфига:

- имя вне `[A-Za-z_][A-Za-z0-9_]*` — ошибка; имена `REGISTRY_SERVER` и
  `GITHUB_TOKEN` **зарезервированы**: второй ключ с тем же именем в `env:`
  победил бы тот, который ставит генератор, и сборка молча пушила бы не туда;
- дубль имени — ошибка (в YAML это повтор ключа, выигрывает последний);
- пустое или многострочное значение — ошибка: хвост многострочного значения
  читался бы как workflow-YAML.

Ключ действует только на шаг «Build and push». Джоба `build-packages`
(`make packages`) и GitLab CI им не затрагиваются; в GitLab переменные CI/CD и
так попадают в окружение job'а. Объявить `build_env` в проекте, который не
собирает docker-образ, нельзя — шаг «Build and push» генерируется вместе с
docker-артефактом, и переменным было бы некуда попасть; это ошибка генерации, а
не молчаливый no-op.

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

`runs_on`, `job_timeout_minutes`, `doctl_version` и [`doctl`](#doctl) действуют
на job `checks` так же, как на остальные job — на то он и объявлен в этом же
блоке.

`fetch-depth: 0` — не деталь: сгенерированный `make lint` вызывается с
`--new-from-rev=$(LINT_BASE_REV)` (по умолчанию `origin/main`), и с дефолтным
shallow-checkout ни этой ссылки, ни базового коммита PR в раннере нет.

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

### Проверки на pull request'е (`pr-checks.yml`)

Те же проверки запускаются и на pull request'е — отдельным сгенерированным
workflow `.github/workflows/pr-checks.yml`:

```yaml
name: PR Checks

on:
  pull_request:
    branches:
      - main
      - staging

jobs:
  checks:
    runs-on: ubuntu-latest
    if: vars.SKIP_CHECKS != 'true'
    env:
      LINT_BASE_REV: ${{ github.event.pull_request.base.sha }}
    # ...матрица и шаги — те же, что у job `checks` в ci_cd.yml
```

Матрица и шаги — **буквально те же**: оба workflow рендерят один общий
фрагмент (`checksJob`, `internal/pkg/templater/embedded/partials/github_checks.tmpl`).
Проверка, которая гейтит мерж, обязана выполняться до мержа, а разошедшаяся
копия давала бы зелёный PR на коде, который мерж потом отклоняет. Объявлять
проверки отдельно для PR не нужно: список один — `github_ci.checks`.

**Почему отдельный файл, а не триггер `pull_request` на `ci_cd.yml`.** Весь
граф `ci_cd.yml` висит на `prepare-env`, который разрешает переменные **ветки**
(`ENABLED_<ВЕТКА>`, хосты деплоя, префиксы портов). На `pull_request`
`GITHUB_REF` равен `refs/pull/<N>/merge`, то есть ветки такой нет и переменной
`ENABLED_REFS_PULL_<N>_MERGE` не существует: job `checks` с таким гейтом ушёл
бы в `skipped` при зелёном run'е — PR без единой проверки, что хуже отсутствия
проверок. А `build`, `build-packages`, `upload-packages` и `deploy` пришлось бы
выключать по одному `if:`, каждый из которых молчит, когда неверен. В отдельном
файле выключать нечего: в нём один job, и этот job — проверки.

Что из этого следует:

- на PR **не** запускаются сборка, публикация пакетов и деплой — их в этом
  файле нет;
- вывод для `push` не меняется: `ci_cd.yml` после выделения фрагмента
  побайтно тот же;
- без ключа `checks` файл не генерируется вовсе (пустая матрица — workflow,
  который GitHub отклоняет);
- `vars.SKIP_CHECKS` действует на оба workflow: это выключатель проверок у
  проекта, а выключатель, работающий в одном месте из двух, выключателем не
  является;
- фильтр ветвей — тот же `push_branches`: это объявление проекта о том, какие
  ветки покрыты его CI, а PR в такую ветку — это пуш в неё с ревью посередине.

**Если PR-workflow в проекте уже свой.** До этой фичи единственным способом
получить проверки на PR был рукописный workflow рядом со сгенерированным. После
регенерации он остаётся на месте — генератор чужих файлов не трогает — и PR
будет проверяться дважды; удалите его. Файл, лежащий по тому же пути
`.github/workflows/pr-checks.yml` и не записанный генератором, регенерацию
**остановит** (`conflict: … exist and were not written by the generator`) до
того, как что-либо будет записано: переименуйте или удалите его.

**База линтера.** `make lint` сравнивает с `$(LINT_BASE_REV)`, по умолчанию
`origin/main` — то есть с `main` **на момент запуска**. На PR это неверная база:
стоит `main` уйти вперёд, и перезапуск краснеет на чужих строках, которых PR не
трогал. Поэтому job выставляет `LINT_BASE_REV` в `github.event.pull_request.base.sha`
— коммит, в который влит PR. Значение приезжает переменной окружения
(`?=` в Makefile читает её, см. [значения в `run:`](#значения-в-run-приходят-через-env)),
а полная история из `fetch-depth: 0` делает этот коммит разрешимым.

**Логин в реестр и форки.** PR из форка секретов не видит, и
`docker/login-action` с пустым паролем ронял бы всю ногу матрицы **до** самой
проверки. Поэтому на PR шаги логина требуют ещё и наличия учётных данных
(`matrix.registry_login && env.REGISTRY_CREDENTIAL != ''`): нога, которой реестр
действительно нужен, падает на своей команде и со своим сообщением, а не на
логине, выполнить который было нечем. На `push` условие прежнее — одна нога
матрицы: пуш несёт секреты репозитория по определению, и молча пропущенный
логин превратил бы незаведённый секрет в красную проверку с непонятным
сообщением.

### Почему матрица, а не job на проверку

- `needs.checks.result` агрегирует все ноги матрицы, поэтому у `build`
  остаётся **одна** запись в `needs` независимо от числа объявленных
  проверок: граф перестаёт меняться при добавлении линтера. Job на проверку
  даёт тот же параллелизм, но растит и `needs`, и число статусов;
- ноги матрицы — отдельные параллельные job'ы, так что wall-clock не растёт с
  числом проверок. Цена — повторный checkout и `setup-go` в каждой ноге.

### Совместимость и отключение

- Нет ключа `checks` — job не генерируется, сгенерированный `ci_cd.yml`
  побайтно совпадает с тем, что было до появления фичи, и `pr-checks.yml` не
  создаётся. Существующие проекты не затрагиваются.
- `vars.SKIP_CHECKS` — лазейка для разового пропуска, повторяет существующую
  `SKIP_GOAT_TESTS`. Пропущенный job даёт `result == 'skipped'`, поэтому
  `build` не блокируется.

### Валидация

- `name` и `run` обязательны;
- `name` уникально в пределах проекта.

Нарушение — ошибка загрузки конфига (`invalid config github_ci section: …`),
генерация не стартует.

## Значения в `run:` приходят через `env:`

**Ни в одном `run:` сгенерированных workflow нет значения, которое выбирает не
шаблон.** Выражение в `run:` раскрывается **до** запуска shell, то есть
значение становится частью исходника скрипта (GitHub Security hardening →
script injection), поэтому каждое такое значение объявляется в `env:` шага или
job'а и читается как `"$VAR"`:

| Источник | Пример | Как приходит |
|---|---|---|
| Имя ветки | `github.ref_name` | `BRANCH` в `env:` шага `Create env file` и job'а `artifact_version` |
| Переменная репозитория | `vars[env.SSH_HOST_NAME]`, `vars.UPLOAD_BUCKET` | `env:` шага, который её читает |
| Секрет репозитория | `secrets[env.OC_PASSWORD_NAME]`, `secrets.MINIO_ACCESS_KEY` | то же |
| Output предыдущего job'а/шага | `needs.prepare-env.outputs.RELEASE`, `steps.vars_deploy.outputs.LEN_EXPECTED` | то же |
| `documentation.headers` | `Authorization: Bearer ${TOKEN}` | `DOCS_HEADER_<n>` в `env:` шага `Deploy to MinIO` |

Для имени ветки или тега это ещё и эскалация прав: `git` допускает в именах
`` ` ``, `$`, `"`, `(` и `;`, поэтому ветка `release/$(cmd)` выполнила бы
команду в job'е, где лежат секреты, а ``release/a`b`` уронила бы шаг
синтаксической ошибкой. Для переменных и секретов репозитория эскалации нет —
их содержимое выбирает тот, кто настраивает репозиторий, — но одинарная кавычка
в значении закрывала `'…'` и остаток строки читался как код, а `ENV_TYPE`
подставлялся вообще без кавычек, так что значение с пробелом или `;` ломало
присваивание. Секрет, вставленный в текст команды, вместе с этим попадал в файл
скрипта на раннере, а не оставался в окружении шага.

Значение, которое уезжает в файл или в `$GITHUB_OUTPUT`, пишется
`printf 'KEY=%s\n' "$VAR"`, а не `echo "KEY=$VAR"`: так оно не попадает и в
строку формата. Содержимое env-файла и `$GITHUB_OUTPUT` при этом прежнее.

Заголовок из `documentation.headers` доходит до `mc` как есть. Секрет в нём
подставляется только формой `${VAR}`: генератор превращает её в
`${{ secrets.VAR }}` внутри `env:` шага. Голый `$VAR` (без фигурных скобок)
больше не раскрывается: раньше заголовок был частью текста команды и shell
подставлял в него переменную окружения раннера, теперь он уходит буквально.

В `run:` остаются только выражения, значение которых shell не может прочитать
как код: `env.<ИМЯ>_NAME` (имя переменной ветки, собранное из нормализованного
до `[A-Za-z0-9_]` имени ветки), `matrix.name`/`matrix.run`/`matrix.containers`
(объявления из `project.yaml` и вывод `make docker-list`) и
`github.repository`/`github.workflow`/`github.run_id`/`github.workspace`. Это
же правило действует на любой новый шаг: статический тест генератора
(`TestGenerateGithubCINoInjectableRunExpr`) проверяет все `run:` обоих
workflow.

## Каждое чтение `needs` объявлено в `needs:` своего job'а

Контекст `needs` содержит **только** job'ы, перечисленные в `needs:` самого
читающего job'а. Чтение любого другого — не ошибка, о которой GitHub сообщает:
выражение раскрывается в `null`, поэтому условие на нём ложно, job молча
пропускается, а run остаётся **зелёным**.

Именно так никогда не запускался `upload-packages`: он ждал только
`build-packages`, а условие читал у `prepare-env`
(`needs.prepare-env.outputs.ENABLE_BUILD == 'true'`). Пакеты собирались и не
загружались, причём в проекте с docker-артефактом вместе с ним пропускался и
`deploy` — тот ждёт `upload-packages`, а в его `if:` нет `always()`, так что
неявный `success()` переносил пропуск дальше: не выкатывалось ничего.

Правило действует на **весь** job, а не на его `if:`: `null` в
`strategy.matrix`, в `env:` job'а или шага и в `run:` так же нем, и только у
условия пропуск — осмысленный результат `null`.

Проверяют это статические тесты генератора в `test/ci_needs_test.go`:
`TestWorkflowJobsOnlyReadNeedsTheyDeclare` сверяет все чтения `needs.<job>` с
`needs:` этого job'а на наборе фикстур (списки `needs:` условные, поэтому
нужен набор), а `TestDeployIsNotSkippedByAnUnreachableUpload` называет
последствие на проекте, где есть и docker-артефакт, и загрузка пакетов.

## Доступ к приватным Go-модулям

Джоба, которая скачивает Go-модули на раннере, получает пару шагов — при
условии, что проект объявил приватные модули в `git.private_repos`:

```yaml
      - name: Configure Git for private repos
        env:
          GH_TOKEN: ${{ secrets.GH_PAT }}
        run: |
          git config --global url."https://x-access-token:${GH_TOKEN}@github.com/".insteadOf "https://github.com/"

      - name: Set GOPRIVATE
        run: |
          go env -w GOPRIVATE="github.com/myorg/*"
```

Имя секрета задаётся `git.private_repos_token_secret` (по умолчанию `GH_PAT`),
список модулей — `git.private_repos` (см. [Основные секции](main.md)). Значение
`GOPRIVATE` здесь — ровно то же, что в сгенерированном `Makefile`, в
builder-стадии `Dockerfile-<app>` и в `.gitlab-ci.yml`: его вычисляет один
хелпер `GeneratorParams.GoPrivate`.

**Это один блок, а не два шага.** `GOPRIVATE` уводит модуль с
`proxy.golang.org` на прямой git-fetch — ровно тот fetch, которому нужен токен
из первого шага. Поставить один шаг без другого не нужно никогда, и рендерить
их по отдельности нечем.

**И блок целиком условный.** Без `git.private_repos` прокси отдаёт все
зависимости, уводить с него нечего и токен не нужен — поэтому блока нет ни в
одной джобе. Раньше в `ci_cd.yml` он стоял безусловно, потому что `GOPRIVATE`
всегда включал `github.com/Educentr`; этого префикса больше нет
([почему](main.md#приватные-go-модули)). Условие живёт в самом фрагменте, а не
в каждом месте вызова, — у вызовов остаётся только обязанность быть
left-trimmed (`{{- template … }}`), иначе на месте неотрендеренного блока
остаётся пустая строка.

**Задан блок в одном месте** — `{{ define "privateModulesAccess" }}` в
`internal/pkg/templater/embedded/partials/github_workflows.tmpl`; шаблоны
workflow рендерят его через `{{ template "privateModulesAccess" $ }}`
(механизм — [Архитектура генератора](../development/generator.md#общие-фрагменты-embeddedpartials)).
До этого блок был скопирован в каждую джобу руками, и копии разошлись: в
`checks` осталось захардкоженное имя секрета, в `build` — другое имя
env-переменной и потерянный шаг `GOPRIVATE`.

### Какие джобы его получают

Все перечисленные ниже — **при объявленном `git.private_repos`**; без него блока
нет ни в одной из них.

| Джоба | Блок | Почему |
|---|---|---|
| `checks` | есть | выполняет команды проверок (`make lint`, `make test`, …) |
| `run-goat-tests-<app>` | есть | `make goat-tests-<app>` |
| `build-packages` | есть | `go install nfpm`, `make packages` |
| `check-go-mod-version` (`go-version-check.yml`) | есть | скачивает прямые зависимости |
| `build` | **нет никогда** | Go на раннере не запускается, см. ниже |

**`build`.** Джоба вызывает только `make docker-<app>` и
`make docker-<app>-push`, то есть собирает образ. Модули скачиваются **внутри**
образа, и credentials туда приходят BuildKit-секретом
(`docker build --secret id=github_token,env=GITHUB_TOKEN`), который отдаёт шаг
«Build and push» своей env-переменной. `git config` на раннере в этой сборке не
участвует — шаг был no-op и удалён. Этот build-секрет передаётся независимо от
`git.private_repos`: внутри образа он единственный способ отдать credentials
сборке, и когда приватных модулей нет, сборка его просто не использует.

## Область действия

Блок действует только на **GitHub Actions**. GitLab CI (`.gitlab-ci.yml`,
`tags:` / `timeout:` / `rules:` / проверки) он не затрагивает: у
`.gitlab-ci.yml` своя модель job'ов, и аналогичные ключи получит отдельный
блок `gitlab_ci:`. Это
касается и `doctl`: шаблон GitLab ставит doctl своими средствами и этим ключом
не управляется.

Не путать с `main.ci` — это список CI-провайдеров, которых нужно
сгенерировать (`github`, `gitlab`), см. [Основные секции](main.md).

## Реализация

- Конфиг: `GithubCIConfig` в `internal/pkg/config/github_ci.go` (парсинг и
  валидация `runs_on` — `ParseRunsOn`, нормализация `doctl` — `DoctlMode`,
  веток — `validatePushBranches`, проверок — `validateChecks`), проверка
  вызывается из `config.GetConfig` и получает `main.registry_type`: режим
  doctl валидируется против типа реестра.
- Нормализованный вид: `ds.GithubCIConfig` / `ds.RunsOnSpec` / `ds.DoctlMode` /
  `ds.ChecksConfig` в `internal/pkg/ds/const.go`; шаблон ветвится по
  `{{ $.GithubCI.DoctlFromImage }}`, поэтому нулевой конфиг оставляет шаг
  установки.
- Шаблоны подставляют одно выражение `{{ $.GithubCI.JobHeader }}` на job —
  оно рендерит и `runs-on:`, и `timeout-minutes:`, поэтому job'ы не могут
  разойтись.
- Ветки триггера рендерят `PushBranchesBlock` (блочная последовательность
  `ci_cd.yml`) и `PushBranchesFlow` (поточная последовательность
  `go-version-check.yml`) — два стиля вывода одного списка, чтобы оба файла
  остались в своей исторической форме. Кавычки ставятся по фильтру, а не по
  списку: умолчание `main`/`staging` выводится как раньше.
- Имена переменных ветки строит шаг `Get branch` job'а `prepare-env`
  (`main/.github/workflows/ci_cd.yml.tmpl`) — несколько строк shell вместо
  `noliran/branch-based-secrets@v1`: тот подставлял имя ветки в имя переменной
  как есть, то есть для `release/1.2` просил `ENABLED_RELEASE/1.2` — имя,
  которого GitHub иметь не может, из-за чего любая ветка со `/` или `-`
  давала зелёный run без сборки. Суффикс для `main`/`staging` не изменился,
  поэтому переменные существующих проектов продолжают работать. Проверяется
  `TestGenerateGithubCIBranchVarNames` в `test/github_ci_test.go`: он
  **исполняет** шаг из сгенерированного YAML и смотрит на имена, которые тот
  выставил.
- Имена и команды проверок попадают в YAML дословно из `project.yaml`, поэтому
  шаблон прогоняет их через хелпер `YAMLQuote`. Через него же выводятся
  значения `build_env` — по той же причине.
- `build_env` разбирает и валидирует `ParseBuildEnv`
  (`internal/pkg/config/github_ci.go`), нормализованный вид —
  `ds.GithubCIEnvVar` (`internal/pkg/ds/github_ci.go`). Что проект собирает
  docker-образ, проверяется в `Generator.processConfig`
  (`internal/pkg/generator/generator.go`): эффективный список артефактов —
  объединение артефактов приложений, то есть известен только после того, как
  они все обработаны. Поведение сгенерированного файла —
  `TestGenerateGithubCIBuildEnv`, `TestGenerateWithoutBuildEnv` и
  `TestGenerateBuildEnvWithoutDocker` в `test/github_ci_test.go`; второй из них
  держит зарезервированные имена в синхроне с шаблоном, сравнивая набор ключей
  `env:` шага целиком.
- Доступ к приватным модулям задан фрагментом `privateModulesAccess`
  (`internal/pkg/templater/embedded/partials/github_workflows.tmpl`), общий
  набор фрагментов подмешивается в каждый шаблон (`addSharedPartials`,
  `internal/pkg/templater/partials.go`). Что блок задан один раз, стерегут
  `TestPrivateModulesAccessIsDefinedOnce` и `TestPrivateModulesAccessCallers`,
  список джоб — `TestGeneratePrivateModulesAccessPerJob` (`test/`).
- Job `checks` задан тем же способом — фрагментом `checksJob`
  (`internal/pkg/templater/embedded/partials/github_checks.tmpl`); `ci_cd.yml`
  рендерит его с `$.ChecksOnPush`, `pr-checks.yml` — с
  `$.ChecksOnPullRequest` (`ChecksRender` в
  `internal/pkg/templater/checks.go`). Различия двух рендеров живут под одним
  `.PullRequest`, поэтому сами проверки разойтись не могут; что копии нет и
  что оба вызова на месте, стерегут `TestChecksJobIsDefinedOnce` и
  `TestChecksJobCallers`, поведение сгенерированных файлов —
  `test/pr_checks_test.go`.
- `pr-checks.yml` выпадает из набора вывода, если проверок нет: фильтр по
  `GithubCI.Checks.Enabled()` в `templater.GetMainTemplates`
  (`internal/pkg/templater/fs.go`) — там же, где отбрасываются остальные
  условные артефакты.
