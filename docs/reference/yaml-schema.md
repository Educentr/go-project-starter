# YAML Schema Reference

Полный справочник всех полей конфигурации Go Project Starter.

!!! note "Источник правды"
    Авторитетный источник — структуры в `internal/pkg/config/structs.go`. Эта документация синхронизируется с кодом.

## Базовая структура

```yaml
main:                      # Основные настройки проекта
git:                       # Git репозиторий
tools:                     # Версии инструментов
rest:                      # REST транспорты
grpc:                      # gRPC сервисы
kafka:                     # Kafka producers/consumers
cli:                       # CLI транспорты
worker:                    # Фоновые воркеры
driver:                    # Драйверы интеграций
applications:              # Приложения (deployment units)
grafana:                   # Grafana datasources
jsonschema:                # JSON Schema для типизации
artifacts:                 # Типы артефактов сборки
packaging:                 # Конфигурация системных пакетов
docker:                    # Docker настройки
deploy:                    # Настройки деплоя
scheduler:                 # Планировщик задач
post_generate:             # Шаги после генерации
```

---

## Секция `main`

Основные настройки проекта.

```yaml
main:
  name: string              # [required] Имя проекта (используется в путях, Docker образах)
  logger: zerolog           # [optional] Тип логгера (только zerolog)
  registry_type: github     # [required] Тип registry: github|digitalocean|aws|selfhosted
  author: string            # [optional] Автор для заголовков файлов
  use_active_record: bool   # [optional] Включить PostgreSQL ActiveRecord
  dev_stand: bool           # [optional] Генерировать docker-compose-dev.yaml с OnlineConf
  skip_service_init: bool   # [optional] Пропустить генерацию Service layer
```

---

## Секция `git`

Настройки Git репозитория.

```yaml
git:
  repo: string              # [required] URL Git репозитория
  module_path: string       # [required] Go module path
  private_repos: string     # [optional] Приватные модули для GOPRIVATE (через запятую)
```

---

## Секция `tools`

Версии инструментов, используемых при генерации и сборке.

```yaml
tools:
  golang_version: "1.24"           # [optional] Версия Go (default: 1.24)
  ogen_version: "v0.78.0"          # [optional] Версия ogen (default: v0.78.0)
  argen_version: "v1.0.0"          # [optional] Версия argen/ActiveRecord (default: v1.0.0)
  golangci_version: "1.55.2"       # [optional] Версия golangci-lint (default: 1.55.2)
  protobuf_version: "1.7.0"        # [optional] Версия protoc-gen-go (default: 1.7.0)
  go_jsonschema_version: "v0.16.0" # [optional] Версия go-jsonschema (default: v0.16.0)
  runtime_version: string          # [auto] Версия go-project-starter-runtime
  goat_version: string             # [auto] Версия GOAT test framework
  goat_services_version: string    # [auto] Версия GOAT services
```

---

## Секция `rest`

REST API транспорты.

```yaml
rest:
  - name: string                # [required] Уникальное имя транспорта
    path:                       # [required для ogen/ogen_client] Пути к OpenAPI спецификациям
      - string
    generator_type: string      # [required] Тип генератора: ogen|template|ogen_client
    generator_template: string  # [required для template] Имя шаблона (например: sys)
    generator_params:           # [optional] Дополнительные параметры генератора
      auth_handler: string      # [ogen] Кастомный auth handler
    port: int                   # [required кроме sys] HTTP порт
    version: string             # [required] Версия API (v1, v2, etc)
    api_prefix: string          # [optional] URL префикс для API
    health_check_path: string   # [optional] Путь для health check
    public_service: bool        # [optional] Публичный сервис (без авторизации)

    # Только для ogen_client:
    instantiation: string       # [optional] static (default) или dynamic
    auth_params:                # [optional] Параметры аутентификации
      transport: header         # Тип транспорта (только header)
      type: apikey              # Тип аутентификации: apikey или bearer
```

### Типы генераторов REST

| Тип | Описание |
|-----|----------|
| `ogen` | OpenAPI 3.0 сервер через ogen |
| `template` | Шаблонная генерация (sys для metrics/health) |
| `ogen_client` | REST клиент для вызова внешних API |

### Instantiation modes (ogen_client)

| Режим | Описание |
|-------|----------|
| `static` | Один экземпляр клиента на всё приложение (default) |
| `dynamic` | Новый экземпляр для каждого запроса |

---

## Секция `grpc`

gRPC сервисы.

```yaml
grpc:
  - name: string                # [required] Уникальное имя сервиса
    path: string                # [required] Путь к .proto файлу
    short: string               # [optional] Короткое имя для пакетов
    port: int                   # [required] gRPC порт
    generator_type: string      # [required] Тип: buf_client
    buf_local_plugins: bool     # [optional] Использовать локальные buf плагины

    # Только для buf_client:
    instantiation: string       # [optional] static (default) или dynamic
```

### Instantiation modes (buf_client)

| Режим | Описание |
|-------|----------|
| `static` | Один экземпляр клиента на всё приложение (default) |
| `dynamic` | Клиент создаётся в рантайме через `NewDynamicClient(ctx, address)` |

---

## Секция `kafka`

Kafka producers и consumers.

```yaml
kafka:
  - name: string                # [required] Уникальное имя
    type: string                # [required] Тип: producer|consumer
    driver: string              # [optional] Драйвер: segmentio (default)|custom
    client: string              # [required] Имя клиента для OnlineConf путей
    group: string               # [required для consumer] Consumer group
    events:                     # [required] Список событий
      - name: string            # [required] Имя события (используется для методов)
        schema: string          # [optional] Ссылка на JSON schema: schemaset.schemaid

    # Только для custom driver:
    driver_import: string       # [required] Import path драйвера
    driver_package: string      # [required] Имя пакета
    driver_obj: string          # [required] Имя struct драйвера
```

---

## Секция `cli`

CLI транспорты для интерактивных приложений.

```yaml
cli:
  - name: string                # [required] Уникальное имя CLI
    path:                       # [optional] Пути к спецификациям
      - string
    generator_type: template    # [required] Тип генератора
    generator_template: string  # [required] Имя шаблона
    generator_params:           # [optional] Параметры генератора
      key: value
```

---

## Секция `worker`

Фоновые воркеры.

```yaml
worker:
  - name: string                # [required] Уникальное имя воркера
    path:                       # [optional] Пути к спецификациям
      - string
    version: string             # [optional] Версия
    generator_type: template    # [required] Тип генератора
    generator_template: string  # [required] Шаблон: telegram|daemon
    generator_params:           # [optional] Параметры генератора
      key: value
```

### Доступные шаблоны воркеров

| Шаблон | Описание |
|--------|----------|
| `telegram` | Telegram бот |
| `daemon` | Фоновый daemon процесс |

---

## Секция `driver`

Кастомные драйверы для интеграций.

```yaml
driver:
  - name: string                # [required] Уникальное имя драйвера
    import: string              # [required] Import path
    package: string             # [required] Имя пакета
    obj_name: string            # [required] Имя struct объекта
    service_injection: string   # [optional] Код для инъекции в Service
```

---

## Секция `applications`

Приложения — атомарные единицы деплоя (контейнеры).

```yaml
applications:
  - name: string                # [required] Имя приложения (= имя контейнера)

    # Транспорты (новый формат с config):
    transport:
      - name: string            # Имя REST транспорта
        config:                 # [optional] Переопределение настроек
          instantiation: string # static|dynamic (для ogen_client)

    # Или старый формат (deprecated, будет удалён в v0.12.0):
    transport:
      - string                  # Просто имя транспорта

    kafka:                      # [optional] Kafka producers/consumers
      - string
    worker:                     # [optional] Воркеры
      - string
    cli: string                 # [optional] CLI транспорт (исключает transport/worker)

    driver:                     # [optional] Драйверы с параметрами
      - name: string            # Имя драйвера
        params:                 # [optional] Параметры инициализации
          - string

    deploy:                     # [optional] Настройки деплоя
      volumes:                  # Docker volumes
        - path: string          # Путь к файлу/директории
          mount: string         # Точка монтирования в контейнере

    use_active_record: bool     # [optional] Переопределить глобальную настройку
    use_envs: bool              # [optional] Использовать environment variables
    depends_on_docker_images:   # [optional] Docker образы для pre-pull
      - string

    # GOAT тесты:
    goat_tests: bool            # [optional] Включить GOAT тесты (простой флаг)
    goat_tests_config:          # [optional] Расширенная конфигурация GOAT
      enabled: bool
      binary_path: string       # Путь к тестовому бинарнику

    # Grafana:
    grafana:
      datasources:              # Ссылки на datasources по имени
        - string

    # Артефакты (переопределяют глобальные):
    artifacts:
      - docker
      - deb
```

!!! warning "CLI приложения"
    Если указан `cli`, нельзя использовать `transport` и `worker`.

---

## Секция `grafana`

Глобальные настройки Grafana datasources.

```yaml
grafana:
  datasources:
    - name: string              # [required] Уникальное имя
      type: string              # [required] Тип: prometheus|loki
      access: string            # [optional] Режим: proxy (default)|direct
      url: string               # [required] URL datasource
      isDefault: bool           # [optional] Default datasource
      editable: bool            # [optional] Редактируемый в UI
```

---

## Секция `jsonschema`

JSON Schema для генерации Go структур с валидацией.

```yaml
jsonschema:
  - name: string                # [required] Уникальное имя группы схем
    package: string             # [optional] Имя Go пакета (default: name)

    # Новый формат (рекомендуется):
    schemas:
      - id: string              # [required] Уникальный ID для ссылок из kafka
        path: string            # [required] Путь к JSON schema файлу
        type: string            # [optional] Имя Go типа (auto-generated если пусто)

    # Legacy формат (deprecated):
    path:
      - string
```

---

## Секция `artifacts`

Типы артефактов для сборки.

```yaml
artifacts:
  - docker                      # Docker образы
  - deb                         # Debian пакеты
  - rpm                         # RPM пакеты
  - apk                         # Alpine пакеты
```

---

## Секция `packaging`

Конфигурация системных пакетов (deb/rpm/apk).

```yaml
packaging:
  maintainer: string            # [required] Email maintainer'а
  description: string           # [required] Описание пакета
  homepage: string              # [optional] URL проекта
  license: string               # [optional] Лицензия
  vendor: string                # [optional] Компания/организация
  install_dir: string           # [optional] Путь установки бинарника
  config_dir: string            # [optional] Путь установки конфигов
  upload:                       # [optional] Загрузка пакетов
    type: string                # Тип хранилища: minio|aws|rsync
```

!!! note "Credentials для upload"
    Credentials (endpoint, bucket, keys) передаются через CI/CD переменные, не через конфиг.

---

## Секция `docker`

Docker настройки.

```yaml
docker:
  image_prefix: string          # [optional] Префикс для Docker образов
```

---

## Секция `deploy`

Настройки деплоя.

```yaml
deploy:
  log_collector:                # [optional] Сборщик логов
    type: string                # Тип сборщика
    parameters:                 # Параметры
      key: value
```

---

## Секция `scheduler`

Планировщик задач.

```yaml
scheduler:
  enabled: bool                 # [optional] Включить scheduler
```

---

## Секция `post_generate`

Шаги, выполняемые после генерации.

```yaml
post_generate:
  - git_install                 # Инициализация git репозитория
  - tools_install               # Установка инструментов
  - clean_imports               # Организация импортов (goimports)
  - executable_scripts          # chmod +x для скриптов
  - call_generate               # Вызов make generate
  - go_mod_tidy                 # go mod tidy
```

---

## Правила валидации

### Обязательные проверки

1. `main.name` — обязательно
2. `main.registry_type` — обязательно, допустимые значения: `github`, `digitalocean`, `aws`, `selfhosted`
3. `git.repo` и `git.module_path` — обязательны
4. REST/gRPC транспорты должны быть назначены в `applications`
5. Драйверы, на которые есть ссылки, должны существовать в `driver`
6. Имена транспортов, воркеров, драйверов не должны дублироваться
7. Порты обязательны для REST (кроме шаблона `sys`)

### Специфичные проверки

| Условие | Требование |
|---------|------------|
| `use_active_record: true` | Требуется `tools.argen_version` |
| `artifacts` содержит `deb/rpm/apk` | Требуется секция `packaging` |
| `kafka.type: consumer` | Требуется `group` |
| `kafka.driver: custom` | Требуются `driver_import`, `driver_package`, `driver_obj` |
| `rest.generator_type: template` | Требуется `generator_template` |
| `rest.instantiation` | Только для `ogen_client` |

---

## Полный пример

```yaml
main:
  name: myservice
  logger: zerolog
  registry_type: github
  use_active_record: true
  dev_stand: true

git:
  repo: https://github.com/myorg/myservice
  module_path: github.com/myorg/myservice

tools:
  golang_version: "1.22"
  ogen_version: "v1.0.0"

rest:
  - name: api
    path: [./api/openapi.yaml]
    generator_type: ogen
    port: 8080
    version: v1
    health_check_path: /health

  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

  - name: external_api
    generator_type: ogen_client
    path: [./api/external.yaml]
    instantiation: dynamic
    auth_params:
      transport: header
      type: apikey

kafka:
  - name: events_producer
    type: producer
    client: main_kafka
    events:
      - name: user_created
        schema: models.user

jsonschema:
  - name: models
    schemas:
      - id: user
        path: ./schemas/user.json

applications:
  - name: api
    transport:
      - name: api
      - name: sys
      - name: external_api
        config:
          instantiation: dynamic
    kafka: [events_producer]
    goat_tests: true

  - name: workers
    worker: [telegram_bot]

worker:
  - name: telegram_bot
    generator_type: template
    generator_template: telegram

artifacts:
  - docker
```
