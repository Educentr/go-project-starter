# Multi-app

Пример проекта с несколькими приложениями из одной кодовой базы.

## E-commerce API с воркерами

Платформа электронной коммерции с отдельными API для клиентов, админов и обработки событий:

```yaml
main:
  name: ecommerce-api
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/mycompany/ecommerce-api
  module_path: github.com/mycompany/ecommerce-api

rest:
  # Публичное API
  - name: public
    path: [./api/public.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  # Админское API
  - name: admin
    path: [./api/admin.yaml]
    generator_type: ogen
    port: 8081
    version: v1

  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

kafka:
  - name: order_events
    type: consumer
    client: main_kafka
    events:
      - name: order_created
      - name: order_completed

  - name: payment_events
    type: consumer
    client: main_kafka
    events:
      - name: payment_processed

workers:
  - name: notification_bot
    generator_type: telegram

applications:
  # API Gateway
  - name: api
    transport: [public, admin, sys]

  # Event processors
  - name: event_processor
    kafka: [order_events, payment_events]
    transport: [sys]

  # Notification worker
  - name: notifier
    workers: [notification_bot]
    transport: [sys]
```

**Что генерируется:**

- 3 отдельных deployable приложения
- 2 REST API (public + admin)
- Системные endpoints (health, metrics, pprof)
- Kafka event consumers
- Telegram бот для уведомлений
- Docker Compose для локальной разработки

## SaaS Analytics Platform

Высоконагруженная платформа для сбора и анализа данных:

```yaml
main:
  name: analytics-platform
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/saas/analytics
  module_path: github.com/saas/analytics

rest:
  # API для веб-дашборда
  - name: dashboard
    path: [./api/dashboard.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  # API для SDK интеграций (v1)
  - name: sdk
    path: [./api/sdk.yaml]
    generator_type: ogen
    port: 8081
    version: v1

  # Поддержка v2 API одновременно с v1
  - name: sdk
    path: [./api/sdk-v2.yaml]
    generator_type: ogen
    port: 8081
    version: v2

  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

grpc:
  - name: ingest
    path: [./proto/ingest.proto]
    port: 9000

applications:
  # REST API сервер
  - name: api
    transport: [dashboard, sdk, sys]

  # High-performance gRPC ingestion
  - name: ingest
    grpc: [ingest]
    transport: [sys]
```

**Особенности:**

- Публичное REST API для веб-дашборда
- High-performance gRPC API для SDK
- Поддержка нескольких версий API (v1, v2) одновременно

## Сервис с зависимостью от Docker образов

Сервис, которому нужны внешние Docker образы:

```yaml
main:
  name: checker-service
  logger: zerolog

git:
  repo: github.com/myorg/checker
  module_path: github.com/myorg/checker

rest:
  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

workers:
  - name: checker
    generator_type: daemon

applications:
  - name: checker
    transport: [sys]
    workers: [checker]
    depends_on_docker_images:
      - ghcr.io/some-app/tool:latest
      - postgres:15-alpine
```

**Что происходит:**

- Создаются image puller сервисы
- Образы скачиваются до запуска приложения
- Полезно для Docker-in-Docker или тестовых сценариев

## CLI утилита для администрирования

Консольная утилита для управления базой данных:

```yaml
main:
  name: admin-tools
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/myorg/admin-tools
  module_path: github.com/myorg/admin-tools

cli:
  - name: admin
    generator_type: template
    generator_template: cli

applications:
  - name: admin-cli
    cli: admin
```

**Использование:**

```bash
./admin-cli migrate up
./admin-cli user create --email admin@example.com
./admin-cli cache clear --all
```

## Масштабирование

Одна кодовая база, разные профили деплоя:

```yaml
applications:
  # Отдельные инстансы
  - name: api
    transport: [public, admin, sys]

  - name: workers
    workers: [telegram_bot]
    kafka: [events]
    transport: [sys]

  # Или монолит для небольших деплоев
  # - name: monolith
  #   transport: [public, admin, sys]
  #   workers: [telegram_bot]
  #   kafka: [events]
```

Масштабируйте каждое приложение независимо:

```bash
kubectl scale deployment api --replicas=5
kubectl scale deployment workers --replicas=2
```

## Генерируемая структура

```
ecommerce-api/
├── cmd/
│   ├── api/
│   │   └── main.go
│   ├── event_processor/
│   │   └── main.go
│   └── notifier/
│       └── main.go
├── internal/
│   ├── app/
│   │   ├── api/
│   │   │   └── transport/rest/
│   │   │       ├── public/v1/
│   │   │       └── admin/v1/
│   │   ├── event_processor/
│   │   │   └── kafka/
│   │   └── notifier/
│   │       └── worker/telegram/
│   └── pkg/
│       ├── model/
│       └── service/
├── docker-compose.yaml
├── Dockerfile-api
├── Dockerfile-event_processor
├── Dockerfile-notifier
└── Makefile
```

## Запуск

```bash
# Генерация проекта
go-project-starter --config=config.yaml

# Запуск всех приложений
cd ecommerce-api
make docker-up

# Или запуск конкретного приложения
docker compose up api
docker compose up event_processor
docker compose up notifier
```
