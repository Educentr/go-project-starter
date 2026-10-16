# Параметры запуска

Все доступные флаги и параметры Go Project Starter.

## Основные параметры

### --config

Путь к файлу конфигурации проекта.

```bash
go-project-starter --config=my-config.yaml
go-project-starter --config=configs/production.yaml
```

**По умолчанию:** `project.yaml`

### --configDir

Директория, содержащая файлы конфигурации.

```bash
go-project-starter --configDir=.project-config --config=project.yaml
```

**По умолчанию:** текущая директория

Полезно для организации:

```
.project-config/
├── project.yaml      # Основная конфигурация
├── openapi.yaml      # API спецификация
└── users.proto       # Protobuf файлы
```

### --target

Целевая директория для генерации проекта.

```bash
go-project-starter --config=config.yaml --target=./my-service
```

**По умолчанию:** текущая директория

### --dry-run

Режим предпросмотра — показать изменения без записи файлов.

```bash
go-project-starter --dry-run --config=config.yaml --target=./my-service
```

Вывод показывает:

- Какие файлы будут созданы
- Какие файлы будут изменены
- Какие файлы будут удалены

### --sourceRef

Git-ref для удалённых спек в `project.yaml`, у которых ref не указан в строке. Флаг **повторяемый**, значение — в одной из двух форм:

| Форма | Применяется к |
|---|---|
| `<ref>` | все репозитории (такая запись допустима одна на запуск) |
| `<репозиторий>:<ref>` | только источники этого репозитория |

```bash
# все непинованные источники из одной ветки
go-project-starter --target=. --sourceRef=release/2026.10

# у каждого репозитория контрактов свой ref
go-project-starter --target=. \
  --sourceRef=github.com/org/specs:release/2026.10 \
  --sourceRef=gitlab.example.com/org/proto:main
```

**По умолчанию:** пусто — каждый источник берётся с тем ref, что написан в конфиге (без `@<ref>` это дефолтная ветка репозитория).

Записи с явным `@<ref>` (`...repo.git@v1.2.0#api.yaml`) — это пин: они не перекрываются. Действует на `rest.path`, `grpc.path`, `jsonschema`, `worker.path` (queue) и `cli.path`; на `https://`-ссылки не действует — у них нет ref. Селектор репозитория можно писать полностью (`github.com/org/specs`) или хвостом пути (`org/specs`, `specs`); адресная запись, не совпавшая ни с одним источником, — ошибка, а не молчаливый no-op.

Эквивалент через окружение — `GPS_SOURCE_REF` (список — через запятую или пробел). Если в командной строке есть хотя бы один `--sourceRef`, переменная игнорируется целиком. В сгенерированном проекте то же самое доступно как `make regenerate SOURCE_REF=release/2026.10` или `make regenerate SOURCE_REF="org/specs:release/2026.10 org/proto:main"`.

Подробнее: [Удалённые спеки → Переопределение ref](../configuration/remote-specs.md#переопределение-ref-sourceref).

## Информационные параметры

### --help, -h

Показать справку по команде.

```bash
go-project-starter --help
go-project-starter init --help
go-project-starter setup --help
```

### --version

Показать версию генератора.

```bash
go-project-starter --version
```

## Примеры использования

### Генерация нового проекта

```bash
# Создать конфигурацию через wizard
go-project-starter init --target=.

# Сгенерировать проект
go-project-starter --configDir=.project-config --target=.

# Настроить CI/CD
go-project-starter setup --configDir=.project-config --target=.
```

### Регенерация существующего проекта

```bash
# Предпросмотр изменений
go-project-starter --dry-run --configDir=.project-config --target=.

# Применить изменения
go-project-starter --configDir=.project-config --target=.
```

### Генерация во временную директорию (тестирование)

```bash
go-project-starter --config=config.yaml --target=/tmp/test-project
cd /tmp/test-project
go build ./...
```

## Переменные окружения

Генератор может использовать переменные окружения для некоторых настроек:

| Переменная | Описание |
|------------|----------|
| `GPS_SOURCE_REF` | То же, что `--sourceRef`; список записей — через запятую или пробел. Игнорируется, если флаг передан хотя бы раз |
| `GOPROXY` | Go proxy для загрузки зависимостей |
| `GOPRIVATE` | Список приватных Go модулей |

## Post-generation шаги

После генерации автоматически выполняются шаги, указанные в конфигурации:

```yaml
post_generate:
  - git_install      # Инициализировать git репозиторий
  - tools_install    # Установить инструменты (ogen, argen, golangci-lint)
  - clean_imports    # Организовать imports через goimports
  - executable_scripts  # chmod +x для скриптов
  - call_generate    # Запустить make generate
  - go_mod_tidy      # Запустить go mod tidy
```

Шаги можно отключить, убрав их из списка.
