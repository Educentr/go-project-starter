# Удалённые спеки в `path:`

Поле `path:` (REST `rest.path`, gRPC `grpc.path`, JSON Schema `jsonschema.path` и `jsonschema.schemas[].path`) принимает не только локальные файлы, но и URI: HTTP(S) ссылки и git-репозитории. Удалённые источники скачиваются при `make regenerate` и копируются в `api/` проекта так же, как и локальные. Воспроизводимая сборка без интернета: после первого `regenerate` файлы лежат в репозитории проекта.

## Грамматика URI

| Форма | Источник | Пример |
|---|---|---|
| `./x.yaml`, `/abs/x.yaml`, `x.yaml` | локальный файл (как раньше) | `./api.yaml` |
| `git+ssh://git@host/org/repo.git@<ref>#<subpath>` | git, SSH (ssh-agent/ssh-config) | `git+ssh://git@github.com/org/specs.git@v1.0.0#openapi/api.yaml` |
| `git+https://host/org/repo.git@<ref>#<subpath>[?token_env=NAME]` | git, HTTPS (опц. токен) | `git+https://github.com/org/specs.git@main?token_env=GITHUB_TOKEN#api/users.yaml` |
| `https://...[?token_env=NAME]`, `http://...` | прямая ссылка (опц. Bearer-токен) | `https://raw.githubusercontent.com/org/specs/main/api.yaml` |

Правила:

- `<ref>` — branch, tag или commit-SHA. Если опущен (`@` отсутствует) — `HEAD`, и такая запись подчиняется [`--sourceRef`](#переопределение-ref-sourceref). Для воспроизводимости рекомендуется фиксировать тег или SHA — указанный `@<ref>` считается пином и не перекрывается.
- `<subpath>` — путь к файлу внутри репо (фрагмент после `#`). Обязателен для git. Поддерживается **только файл**, не директория.
- `?token_env=NAME` (только для `git+https` и `https`) — имя переменной окружения, в которой лежит токен. **Сам токен в YAML не пишется** и в логах не появляется.
- Порядок query/fragment по RFC 3986: `?token_env=...` идёт **до** `#subpath`.
- SCP-style git-URL (`git@github.com:org/repo.git`) **не поддерживается** — используйте `git+ssh://git@github.com/org/repo.git`.

## Переопределение ref (`--sourceRef`)

Одна ручка задаёт ref для **всех** git-источников проекта разом — чтобы собрать дерево против контрактов релизной ветки или ветки задачи, не трогая `project.yaml`:

```bash
# из сгенерированного проекта
make regenerate SOURCE_REF=release/2026.10

# напрямую
go-project-starter --target . --sourceRef release/2026.10

# то же через окружение (например, в CI)
export GPS_SOURCE_REF=release/2026.10
make regenerate
```

Правило одно: **любой явный `@<ref>` в строке — это пин, и он не перекрывается**.

| Запись в `project.yaml` | `--sourceRef release/X` | Что склонируется |
|---|---|---|
| `git+ssh://git@host/org/specs.git#api.yaml` | задан | `release/X` |
| `git+ssh://git@host/org/specs.git#api.yaml` | не задан | дефолтная ветка (`HEAD`) |
| `git+ssh://git@host/org/specs.git@v1.2.0#api.yaml` | задан | `v1.2.0` (пин сильнее) |
| `https://host/api.yaml` | задан | без изменений — у HTTP-ссылок нет ref |

Подробности:

- Действует на все каналы: `rest.path`, `grpc.path`, `jsonschema.path` / `jsonschema.schemas[].path`, `worker.path` (queue) и `cli.path`. Дерево не может собраться из двух разных состояний контрактов.
- Приоритет: флаг `--sourceRef` > переменная `GPS_SOURCE_REF` > поведение без переопределения.
- Если ветки/тега не существует, регенерация падает с ошибкой, которая называет источник и ref. Тихого возврата к дефолтной ветке нет.
- Один clone на `(repo, ref)`: источник, перекрытый до `release/X`, и источник, пинованный на `release/X`, делят один клон.
- Поля в `project.yaml` для дефолтного ref **нет** — это осознанно. Ref сборки задаёт тот, кто её запускает, а конфиг остаётся одинаковым для всех веток.

## Аутентификация

| Источник | Как настроить |
|---|---|
| `git+ssh://` | Используется ваш ssh-agent / `~/.ssh/config`. Никаких параметров в YAML — то, что работает для `git clone` локально, работает и здесь. |
| `git+https://` без `token_env` | Анонимный clone — для публичных репозиториев. |
| `git+https://` с `token_env=GITHUB_TOKEN` | Перед `make regenerate` экспортируйте: `export GITHUB_TOKEN=ghp_xxx`. Токен подставляется в URL только в памяти процесса git. |
| `https://` без `token_env` | Анонимный GET. |
| `https://` с `token_env=COMPANY_TOKEN` | Перед `make regenerate`: `export COMPANY_TOKEN=...`. Шлётся как `Authorization: Bearer ${COMPANY_TOKEN}`. |

## Примеры

### REST с git-спекой по тегу

```yaml
rest:
  - name: users
    path:
      - git+ssh://git@github.com/org/api-specs.git@v2.3.1#openapi/users.yaml
    api_prefix: /
    version: v1
    port: 8081
    generator_type: ogen
```

После `make regenerate` файл окажется в `api/rest/users/v1/users.yaml`.

### gRPC с приватного репо через токен

```bash
export GITHUB_TOKEN=ghp_xxx
make regenerate
```

```yaml
grpc:
  - name: BillingService
    path: git+https://github.com/org/proto-shared.git@main?token_env=GITHUB_TOKEN#billing/v1/billing.proto
    short: billing
    port: 8090
    generator_type: buf_client
```

### JSON Schema с публичной HTTPS-ссылкой

```yaml
jsonschema:
  - name: events
    schemas:
      - id: order_created
        path: https://raw.githubusercontent.com/org/event-schemas/v1/order_created.json
      - id: order_paid
        path: ./local-schemas/order_paid.json
```

### Несколько спек из одного репо

Один git-репозиторий клонируется один раз за `regenerate`, даже если из него используется несколько файлов:

```yaml
rest:
  - name: users
    path:
      - git+ssh://git@github.com/org/specs.git@v1.0.0#users.yaml
  - name: orders
    path:
      - git+ssh://git@github.com/org/specs.git@v1.0.0#orders.yaml
```

## Поведение `make regenerate`

1. Парсятся все `path:` из `project.yaml`.
2. Для каждого удалённого источника создаётся временный staging-каталог, файл скачивается / клонируется один раз на запуск.
3. Файлы копируются в `api/{rest,grpc,schema}/...` под исходным именем (например, `openapi/users.yaml` → `users.yaml`).
4. Staging чистится в конце.
5. Файлы в `api/` коммитятся в репозиторий проекта.

Локальные пути обрабатываются ровно так же, как раньше — никаких изменений в поведении.

## Troubleshooting

| Симптом | Причина | Что делать |
|---|---|---|
| `'git' executable not found in PATH` | Не установлен `git` | Установить git (`brew install git` / `apt install git`) |
| `token_env variable is empty: GITHUB_TOKEN` | Переменная не экспортирована | `export GITHUB_TOKEN=...` перед `make regenerate` |
| `git clone failed: authentication failed` (ssh) | ssh-agent не запущен или ключ не добавлен | `ssh-add -l`, затем `ssh-add ~/.ssh/id_ed25519` |
| `git clone failed: authentication failed` (https) | Невалидный токен или нет прав на репо | Проверить токен и его scope (для GitHub: `repo`) |
| `git clone failed: repository not found` | Опечатка в URL или нет доступа | Проверить URL, попробовать `git clone <repo>` локально |
| `git clone failed: ssh://...@release/X#api.yaml` | Ветки из `--sourceRef` / `GPS_SOURCE_REF` нет в этом репо | Проверить ветку в репо контрактов; либо пиновать запись через `@<ref>`, чтобы она не следовала за переопределением |
| `invalid --sourceRef: specsource: invalid git ref` | Ref с пробелом или начинающийся с `-` | Передать имя ветки/тега без пробелов и ведущего `-` |
| `subpath not found in repository: openapi/foo.yaml (similar: api/foo.yaml)` | Опечатка в subpath | Использовать предложенный путь или поправить YAML |
| `HTTP request returned non-2xx status: 404 ...` | Неверный URL или ref | Проверить URL в браузере / `curl -I` |
| `directory subpaths are not supported yet` | `#openapi/` (с `/` в конце) | Указать конкретный файл, не директорию |

## Поддерживаемые источники

Удалённые URI работают для всех `path:` в YAML:

- `rest.path` — OpenAPI-спеки (REST/ogen/ogen_client)
- `grpc.path` — `.proto` файлы
- `jsonschema.path` / `jsonschema.schemas[].path` — JSON-схемы
- `worker.path` для `generator_template: queue` — queue-контракты
- `cli.path` для `generator_template: cli` — CLI-команды

Для REST/gRPC/JSON Schema файлы копируются в `api/` (live на протяжении сборки). Для queue/CLI файл материализуется во временную staging-директорию, парсится в Go-структуру и сразу удаляется — в `api/` не копируется (рантайму он не нужен).

## Известные ограничения v1

- **Subpath = файл, не директория.** Поддержка proto-деревьев с импортами — отдельной задачей.
- **Lockfile с pinned commit-SHA** (как `go.sum`) — не реализовано. См. [issue в GitHub](https://github.com/Educentr/go-project-starter/issues) с тегом `specsource`.
- **Токен в `ps`** для `git+https`: при clone токен виден в командной строке процесса. На разделяемых машинах используйте `git+ssh://` или короткоживущие токены.
- **Параллельные клоны не выполняются.** Если у вас 10 удалённых спек — clone идёт последовательно. Для одного `(repo, ref)` клон один.

## См. также

- [Транспорты](transports.md) — `rest.path`, `grpc.path`
- [JSON Schema](applications.md) — `jsonschema.path` / `jsonschema.schemas[].path`
- [`make regenerate`](../workflow/regeneration.md)
- Реализация: `internal/pkg/specsource/`
