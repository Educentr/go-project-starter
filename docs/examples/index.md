# Примеры

Готовые примеры конфигураций для разных сценариев.

## Содержание раздела

- [REST API](rest-api.md) — простой REST API сервис
- [gRPC сервис](grpc-service.md) — gRPC с REST gateway
- [Telegram бот](telegram-bot.md) — Telegram бот
- [Multi-app](multi-app.md) — несколько приложений из одной кодовой базы

## Быстрый выбор

| Сценарий | Пример |
|----------|--------|
| Простой REST API | [REST API](rest-api.md) |
| gRPC с REST gateway | [gRPC сервис](grpc-service.md) |
| Telegram бот | [Telegram бот](telegram-bot.md) |
| E-commerce с воркерами | [Multi-app](multi-app.md) |
| Микросервис с Kafka | [Multi-app](multi-app.md) |

## Минимальная конфигурация

Простейший сервис с одним REST API:

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

## Что генерируется

Для каждого примера генерируется:

- ~50 файлов, готовых к запуску
- Docker и Docker Compose
- GitHub Actions CI/CD
- Makefile с 40+ целями
- Prometheus метрики
- Structured logging

## Следующие шаги

Выберите пример, наиболее близкий к вашему сценарию, и адаптируйте под свои нужды.
