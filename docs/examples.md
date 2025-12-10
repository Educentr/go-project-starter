# Примеры

## E-commerce API с воркерами

Платформа электронной коммерции с отдельными API для клиентов, админов и обработки событий.

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

  # Системные endpoints
  - name: system
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

kafka:
  - name: order_events
    topics: [orders.created, orders.completed]

  - name: payment_events
    topics: [payments.processed]

workers:
  - name: notification_bot
    generator_type: telegram

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
```

**Что генерируется:**
- 3 отдельных deployable приложения
- 2 REST API (public + admin)
- Системные endpoints (health, metrics, pprof)
- Kafka event consumers
- Telegram бот для уведомлений
- Docker Compose для локальной разработки

---

## gRPC микросервис с REST Gateway

Высокопроизводительный gRPC сервис с REST gateway для внешних клиентов.

```yaml
main:
  name: user-service
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/mycompany/user-service
  module_path: github.com/mycompany/user-service

grpc:
  - name: users
    path: [./proto/users.proto]
    port: 9000

rest:
  # REST gateway для gRPC
  - name: gateway
    path: [./api/users-gateway.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  # Системные endpoints
  - name: system
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

applications:
  # gRPC сервер
  - name: grpc_server
    grpc: [users]
    transport: [system]

  # REST gateway
  - name: rest_gateway
    transport: [gateway, system]
```

**Варианты деплоя:**
- Вместе для небольших деплоев
- Раздельно с независимым масштабированием
- REST gateway для внешних клиентов, gRPC для внутренних сервисов

---

## Fintech платформа

Сервис обработки платежей с REST API, gRPC и event-driven архитектурой.

```yaml
main:
  name: payment-service
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/fintech/payment-service
  module_path: github.com/fintech/payment-service

rest:
  # API для мобильных клиентов
  - name: mobile
    path: [./api/mobile.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  # Внутренний API для других сервисов
  - name: internal
    path: [./api/internal.yaml]
    generator_type: ogen
    port: 8081
    version: v1

  # Системные endpoints
  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

grpc:
  - name: payments
    path: [./proto/payments.proto]
    port: 9000

kafka:
  - name: transactions
    topics: [transactions.created, transactions.completed, transactions.failed]

applications:
  # API сервер
  - name: api
    transport: [mobile, internal, sys]
    grpc: [payments]

  # Event processor
  - name: processor
    kafka: [transactions]
    transport: [sys]

grafana:
  datasources:
    - name: Prometheus
      type: prometheus
      url: http://prometheus:9090
      isDefault: true
    - name: Loki
      type: loki
      url: http://loki:3100
```

**Что получаете:**
- REST API для мобильных клиентов с OAuth2
- gRPC для внутренней коммуникации
- Kafka consumer для обработки транзакций
- PostgreSQL с миграциями
- Prometheus метрики, structured logging, distributed tracing
- Docker с Traefik reverse proxy

---

## SaaS Analytics Platform

Высоконагруженная платформа для сбора и анализа данных.

```yaml
main:
  name: analytics-platform
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/saas/analytics
  module_path: github.com/saas/analytics

rest:
  # Публичное API для веб-дашборда (rate-limited, cached)
  - name: dashboard
    path: [./api/dashboard.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  # API для SDK интеграций
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

  # Системные endpoints
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

---

## Минимальный REST сервис

Простейший сервис с одним REST API.

```yaml
main:
  name: simple-api
  logger: zerolog

git:
  repo: github.com/myorg/simple-api
  module_path: github.com/myorg/simple-api

rest:
  - name: api
    path: [./api/openapi.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

applications:
  - name: server
    transport: [api, sys]
```

---

## Сервис с внешним REST клиентом

Сервис, который вызывает внешний API.

```yaml
main:
  name: integration-service
  logger: zerolog

git:
  repo: github.com/myorg/integration
  module_path: github.com/myorg/integration

rest:
  # Внутреннее API
  - name: api
    path: [./api/api.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  # Клиент для внешнего API с аутентификацией
  - name: payment_provider
    path: [./api/external/payment.yaml]
    generator_type: ogen_client
    auth_params:
      transport: header
      type: apikey

  # Системные endpoints
  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

applications:
  - name: server
    transport: [api, payment_provider, sys]
```

**Как работает:**
- `api` — ваш REST сервер
- `payment_provider` — типобезопасный клиент для внешнего API
- API ключ читается из OnlineConf

---

## CLI утилита для администрирования

Консольная утилита для управления базой данных.

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

---

## Сервис с мониторингом Grafana

Полная настройка мониторинга.

```yaml
main:
  name: monitored-service
  logger: zerolog

git:
  repo: github.com/myorg/monitored
  module_path: github.com/myorg/monitored

rest:
  - name: api
    path: [./api/api.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

grafana:
  datasources:
    - name: Prometheus
      type: prometheus
      access: proxy
      url: http://prometheus:9090
      isDefault: true
    - name: Loki
      type: loki
      access: proxy
      url: http://loki:3100

applications:
  - name: server
    transport: [api, sys]
    grafana:
      datasources:
        - Prometheus
        - Loki
```

**Генерируется:**
- Grafana dashboard с панелями логов и метрик
- Prometheus конфигурация для scraping
- Loki конфигурация для сбора логов
- Provisioning файлы для автоматической настройки

---

## Сервис с зависимостью от Docker образов

Сервис, которому нужны внешние Docker образы.

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

## Следующие шаги

- [Продвинутые темы](advanced.md) - драйверы, OnlineConf, мониторинг
- [Конфигурация](configuration.md) - полное руководство по настройке
