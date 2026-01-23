# gRPC сервис

Пример gRPC микросервиса с REST gateway.

## Простой gRPC сервис

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
  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

applications:
  - name: server
    grpc: [users]
    transport: [sys]
```

## gRPC с REST Gateway

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

  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

applications:
  # gRPC сервер
  - name: grpc_server
    grpc: [users]
    transport: [sys]

  # REST gateway
  - name: rest_gateway
    transport: [gateway, sys]
```

**Варианты деплоя:**

- Вместе для небольших деплоев
- Раздельно с независимым масштабированием
- REST gateway для внешних клиентов, gRPC для внутренних сервисов

## Несколько gRPC сервисов

```yaml
main:
  name: platform-api
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/mycompany/platform-api
  module_path: github.com/mycompany/platform-api

grpc:
  - name: users
    path: [./proto/users.proto]
    port: 9000

  - name: orders
    path: [./proto/orders.proto]
    port: 9001

  - name: payments
    path: [./proto/payments.proto]
    port: 9002

rest:
  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

applications:
  # Один сервер со всеми gRPC сервисами
  - name: monolith
    grpc: [users, orders, payments]
    transport: [sys]

  # Или разделение по доменам
  # - name: user-service
  #   grpc: [users]
  #   transport: [sys]
  #
  # - name: order-service
  #   grpc: [orders, payments]
  #   transport: [sys]
```

## Fintech платформа

Сервис обработки платежей с REST API, gRPC и event-driven архитектурой:

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

  # Внутренний API
  - name: internal
    path: [./api/internal.yaml]
    generator_type: ogen
    port: 8081
    version: v1

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
    type: consumer
    client: main_kafka
    events:
      - name: transaction_created
      - name: transaction_completed
      - name: transaction_failed

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

- REST API для мобильных клиентов
- gRPC для внутренней коммуникации
- Kafka consumer для обработки транзакций
- PostgreSQL с миграциями
- Prometheus метрики, structured logging, distributed tracing
- Docker с Traefik reverse proxy

## Генерируемая структура

```
user-service/
├── cmd/
│   ├── grpc_server/
│   │   └── main.go
│   └── rest_gateway/
│       └── main.go
├── internal/
│   ├── app/
│   │   ├── grpc_server/
│   │   │   └── grpc/users/
│   │   │       ├── service.go
│   │   │       └── handler.go   # Ваш код здесь
│   │   └── rest_gateway/
│   │       └── transport/rest/gateway/v1/
│   │           └── handlers.go
│   └── pkg/
│       └── model/
├── pkg/
│   └── grpc/users/              # Сгенерированный protobuf код
├── proto/
│   └── users.proto
├── docker-compose.yaml
└── Makefile
```

## Запуск

```bash
# Генерация проекта
go-project-starter --config=config.yaml

# Запуск
cd user-service
make docker-up
make generate
make run

# Тестирование gRPC
grpcurl -plaintext localhost:9000 list
grpcurl -plaintext localhost:9000 users.UserService/GetUser
```
