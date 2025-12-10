# Конфигурация

## Базовая структура

```yaml
main:
  name: string              # Имя проекта
  logger: zerolog           # Тип логгера
  registry_type: github     # Container registry (github/digitalocean)
  use_active_record: bool   # Включить ORM для базы данных

git:
  repo: string              # URL Git репозитория
  module_path: string       # Go module path
  private_repos: []         # Приватные Go модули для GOPRIVATE

rest:
  - name: string            # Имя транспорта
    path: []                # Пути к OpenAPI спецификациям
    generator_type: ogen    # Генератор: ogen/template/ogen_client
    port: int               # HTTP порт
    version: string         # Версия API (v1, v2, и т.д.)

grpc:
  - name: string            # Имя сервиса
    path: []                # Пути к Protobuf файлам
    port: int               # gRPC порт

kafka:
  - name: string            # Имя consumer'а
    topics: []              # Топики для потребления

workers:
  - name: string            # Имя воркера
    generator_type: telegram # Тип: telegram/daemon

applications:
  - name: string            # Имя приложения
    transport: []           # REST/gRPC транспорты
    workers: []             # Воркеры
    drivers: []             # Внешние драйверы
    depends_on_docker_images: []  # Docker образы для предварительного pull
    grafana:                # Конфигурация Grafana dashboard
      datasources: []       # Имена datasources для этого приложения

grafana:
  datasources:
    - name: string          # Имя datasource (например, "Prometheus")
      type: string          # Тип: prometheus, loki
      access: string        # Режим доступа: proxy, direct
      url: string           # URL datasource
      isDefault: bool       # Установить как default
      editable: bool        # Разрешить редактирование в Grafana UI
```

## Секция `main`

```yaml
main:
  name: myproject
  registry_type: github     # или digitalocean
  logger: zerolog
  author: "Your Name"
  use_active_record: true   # Включает PostgreSQL + ActiveRecord ORM
```

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя проекта (используется в путях, Docker образах) |
| `registry_type` | Нет | `github` или `digitalocean` |
| `logger` | Нет | Тип логгера (сейчас только `zerolog`) |
| `use_active_record` | Нет | Включает генерацию кода для PostgreSQL |

## Секция `git`

```yaml
git:
  repo: https://github.com/org/repo
  module_path: github.com/org/repo
  private_repos:
    - github.com/myorg/internal-pkg
    - gitlab.com/company/*
```

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `repo` | Да | URL Git репозитория |
| `module_path` | Да | Go module path |
| `private_repos` | Нет | Список приватных модулей для GOPRIVATE |

## Секция `rest`

```yaml
rest:
  # OpenAPI сервер
  - name: api
    path: [./api/openapi.yaml]
    generator_type: ogen
    port: 8080
    version: v1
    health_check_path: /health

  # Системные endpoints (метрики, health)
  - name: system
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

  # REST клиент для внешнего API
  - name: payment_api
    path: [./api/payment.yaml]
    generator_type: ogen_client
    auth_params:
      transport: header
      type: apikey
```

### Типы генераторов

| Тип | Описание | Применение |
|-----|----------|------------|
| `ogen` | Генерация сервера из OpenAPI 3.0 | Основные бизнес API |
| `template` | Шаблонная генерация | Health checks, метрики, кастомные endpoints |
| `ogen_client` | Генерация REST клиента | Вызов внешних API |

### Параметры аутентификации (`auth_params`)

Для `ogen_client` можно настроить аутентификацию:

```yaml
rest:
  - name: external_api
    generator_type: ogen_client
    auth_params:
      transport: header    # Способ передачи (пока только header)
      type: apikey         # Тип аутентификации (пока только apikey)
```

API ключ читается из OnlineConf: `{service_name}/transport/rest/{rest_name}/auth_params/apikey`

## Секция `grpc`

```yaml
grpc:
  - name: users
    path: [./proto/users.proto]
    port: 9000

  - name: admin
    path: [./proto/admin.proto]
    port: 9001
```

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя gRPC сервиса |
| `path` | Да | Пути к .proto файлам |
| `port` | Да | gRPC порт |

## Секция `kafka`

```yaml
kafka:
  - name: order_events
    topics: [orders.created, orders.completed]

  - name: payment_events
    topics: [payments.processed]
```

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя Kafka consumer'а |
| `topics` | Да | Список топиков для потребления |

## Секция `workers`

```yaml
workers:
  - name: notification_bot
    generator_type: telegram

  - name: background_processor
    generator_type: daemon
```

### Типы воркеров

| Тип | Описание |
|-----|----------|
| `telegram` | Telegram бот (webhooks/polling) |
| `daemon` | Фоновый демон |

## Секция `drivers`

```yaml
drivers:
  - name: telegram_bot
    import: pkg/drivers/telegram
    package: telegram
    obj_name: TelegramDriver
    service_injection: "optional custom code"

  - name: payment_gateway
    type: http
    config:
      base_url: https://api.stripe.com
      auth_token: ${STRIPE_API_KEY}
```

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя драйвера |
| `import` | Да | Import path |
| `package` | Да | Имя пакета |
| `obj_name` | Да | Имя объекта драйвера |

## Секция `applications`

```yaml
applications:
  # API Gateway
  - name: api
    transport: [public, admin, system]

  # Event processors
  - name: event_processor
    kafka: [order_events, payment_events]

  # Notification worker
  - name: notifier
    workers: [notification_bot]

  # С зависимостью от Docker образов
  - name: checker
    transport: [sys]
    workers: [checker]
    depends_on_docker_images:
      - ghcr.io/some-app/cool-app:latest
      - postgres:15-alpine
```

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя приложения (= имя контейнера) |
| `transport` | Нет | REST/gRPC транспорты |
| `kafka` | Нет | Kafka consumers |
| `workers` | Нет | Фоновые воркеры |
| `drivers` | Нет | Драйверы интеграций |
| `depends_on_docker_images` | Нет | Docker образы для pre-pull |

### Зависимости от Docker образов

`depends_on_docker_images` создаёт сервисы для предварительного pull образов:

```yaml
# docker-compose.yaml (генерируется)
cool-app-image-puller:
  image: ghcr.io/some-app/cool-app:latest
  pull_policy: always
  restart: "no"

checker:
  depends_on:
    cool-app-image-puller:
      condition: service_completed_successfully
```

## Секция `grafana`

```yaml
grafana:
  datasources:
    - name: Prometheus
      type: prometheus
      access: proxy
      url: http://prometheus:9090
      isDefault: true
      editable: false
    - name: Loki
      type: loki
      access: proxy
      url: http://loki:3100
      editable: false

applications:
  - name: api
    transport: [api, sys]
    grafana:
      datasources:
        - Prometheus
        - Loki
```

### Генерируемые панели

| Панель | Условие | Метрики |
|--------|---------|---------|
| **Logs** | Loki datasource | Логи с фильтрацией по уровню |
| **Go Runtime** | Prometheus | `go_goroutines`, `go_memstats_*`, `go_gc_*` |
| **HTTP Server: {name}** | Для каждого `ogen` транспорта | `http_server_request_duration_seconds` |
| **HTTP Client: {name}** | Для каждого `ogen_client` | `http_client_request_duration_seconds` |

### Генерируемые файлы

```
grafana/
├── dashboards/
│   └── {app-name}-dashboard.json
└── provisioning/
    ├── dashboards/
    │   └── dashboards.yaml
    └── datasources/
        └── datasources.yaml
```

## Секция `steps` (постгенерация)

```yaml
steps:
  git_install: true          # Инициализировать git репозиторий
  tools_install: true        # Установить dev инструменты (ogen, argen, golangci-lint)
  clean_imports: true        # Организовать imports через goimports
  executable_scripts: true   # chmod +x для скриптов
  call_generate: true        # Запустить make generate
  go_mod_tidy: true          # Запустить go mod tidy
```

## Версии инструментов

```yaml
protobuf_version: 1.7.0
golang_version: "1.24"
ogen_version: v0.78.0
golangci_version: 1.55.2
argen_version: v1.0.0
```

## Переменные окружения (OnlineConf)

Сгенерированные проекты используют OnlineConf для конфигурации:

```bash
# Core settings
OC_{ProjectName}__devstand=0
OC_{ProjectName}__log__level=info

# REST API settings
OC_{ProjectName}__transport__rest__{name}_{version}__ip=0.0.0.0
OC_{ProjectName}__transport__rest__{name}_{version}__port=8081
OC_{ProjectName}__transport__rest__{name}_{version}__timeout=2s

# Database settings (при use_active_record: true)
OC_{ProjectName}__db__main=127.0.0.1:5432
OC_{ProjectName}__db__main__User=myproject
OC_{ProjectName}__db__main__Password=password
OC_{ProjectName}__db__main__DB=myproject

# Security
OC_{ProjectName}__security__csrf__enabled=0
OC_{ProjectName}__security__httpAuth__enabled=0
```

## Правила валидации

1. **REST/gRPC сервисы должны быть назначены в applications**
2. **Драйверы, на которые есть ссылки, должны существовать**
3. **Имена не должны дублироваться** (REST, gRPC, drivers)
4. **ActiveRecord требует указания ArgenVersion**
5. **Registry type: только `github` или `digitalocean`**
6. **Порты обязательны для REST (кроме шаблона `sys`)**

## Следующие шаги

- [Возможности](features.md) - все возможности генератора
- [Примеры](examples.md) - готовые примеры конфигураций
- [Продвинутые темы](advanced.md) - драйверы, OnlineConf, мониторинг
