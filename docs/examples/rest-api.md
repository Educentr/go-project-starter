# REST API

Пример простого REST API сервиса.

## Минимальная конфигурация

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

## С базой данных

```yaml
main:
  name: user-service
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/myorg/user-service
  module_path: github.com/myorg/user-service

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

## С внешним REST клиентом

Сервис, который вызывает внешний API:

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

## С несколькими версиями API

```yaml
main:
  name: versioned-api
  logger: zerolog

git:
  repo: github.com/myorg/versioned-api
  module_path: github.com/myorg/versioned-api

rest:
  - name: api
    path: [./api/v1.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  - name: api
    path: [./api/v2.yaml]
    generator_type: ogen
    port: 8080
    version: v2

  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

applications:
  - name: server
    transport: [api, sys]  # Включает и v1, и v2
```

**Генерируемая структура:**

```
internal/app/server/transport/rest/api/
├── v1/
│   ├── handlers.go
│   └── router.go
└── v2/
    ├── handlers.go
    └── router.go
```

## С мониторингом Grafana

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

## Генерируемая структура

```
simple-api/
├── cmd/server/
│   └── main.go
├── internal/
│   ├── app/server/
│   │   └── transport/rest/api/v1/
│   │       ├── handlers.go      # Ваш код здесь
│   │       └── router.go
│   └── pkg/
│       ├── model/
│       └── service/
├── pkg/
│   ├── app/
│   └── rest/api/v1/             # Сгенерированный ogen код
├── api/
│   └── openapi.yaml
├── docker-compose.yaml
├── Dockerfile
├── Makefile
└── .github/workflows/
```

## Запуск

```bash
# Генерация проекта
go-project-starter --config=config.yaml

# Запуск
cd simple-api
make docker-up
make generate
make run

# Доступ
curl http://localhost:8080/api/v1/health
curl http://localhost:9090/metrics
```
