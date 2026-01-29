# Go Project Starter

> **Превращайте API-спецификации в production-ready микросервисы за минуты, а не дни.**

Go Project Starter — мощный генератор микросервисов, который создаёт полностью готовые к production Go-сервисы из YAML-конфигурации. Инструмент использует 78+ встроенных шаблонов для генерации ~8000 строк production-grade кода, включая REST API, gRPC сервисы, Kafka consumers, Telegram боты и полную инфраструктуру.

## Почему Go Project Starter?

Создание production-ready микросервиса с нуля обычно занимает **2-3 недели** написания boilerplate-кода. Go Project Starter **сокращает это до 5 минут**, генерируя 100% инфраструктурного кода из единого YAML-файла.

**Главное преимущество:** В отличие от традиционных scaffolding-инструментов, которые генерируют код один раз и оставляют вас наедине с ним, Go Project Starter использует интеллектуальные disclaimer-маркеры для разделения сгенерированного кода и вашей бизнес-логики. Это означает, что вы можете **перегенерировать весь проект** при эволюции API без потери ручных изменений.

## Ключевые возможности

### Сохранение пользовательского кода

Перегенерируйте проект при изменении API без потери вашего кода:

```go
// ==========================================
// GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
// ==========================================

func (h *Handler) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
    // Ваша бизнес-логика здесь - переживёт регенерацию!
}
```

### Многопротокольная поддержка

- **REST APIs** (OpenAPI 3.0 через [ogen](https://github.com/ogen-go/ogen))
- **gRPC сервисы** (Protocol Buffers v3)
- **Kafka Consumers** (event-driven архитектура)
- **Background Workers** (Telegram боты, daemons)

### Application-Based масштабирование

Одна кодовая база, разные профили деплоя:

```yaml
applications:
  - name: gateway
    transport: [rest_api, grpc_users]

  - name: workers
    workers: [telegram_bot, kafka_consumer]
```

### Production-Ready инфраструктура

Каждый сгенерированный проект включает:

- Docker & Docker Compose (multi-stage builds, ~50MB образы)
- GitHub Actions CI/CD
- Traefik reverse proxy
- Prometheus метрики & Grafana dashboards
- Health checks & graceful shutdown
- Structured logging (zerolog)

## Что вы получите

```
myservice/                    # ~50 файлов, ~8000 строк production-ready кода
├── cmd/server/              # Точка входа с graceful shutdown
├── internal/
│   ├── app/                 # Обработчики транспорта и воркеры (ваш код)
│   └── pkg/                 # Бизнес-логика и репозитории
├── pkg/                     # Runtime библиотеки (middleware, logging, metrics)
├── docker-compose.yaml      # Локальное окружение (Postgres, Redis, Traefik)
├── Dockerfile               # Multi-stage build (образ ~50MB)
├── Makefile                 # 40+ targets (build, test, lint, deploy)
└── .github/workflows/       # CI/CD (test, build, push to registry)
```

## Быстрый старт

```bash
# Установка
go install github.com/Educentr/go-project-starter/cmd/go-project-starter@latest

# Генерация проекта
go-project-starter --config=config.yaml

# Запуск
cd myservice
make docker-up    # Запуск зависимостей
make generate     # Генерация кода из OpenAPI
make run          # Запуск сервиса
```

[Подробное руководство по началу работы](getting-started/index.md)

## Навигация по документации

| Раздел | Описание |
|--------|----------|
| [Начало работы](getting-started/index.md) | Установка и первый проект |
| [Архитектура](architecture/index.md) | Архитектура генератора и сгенерированных проектов |
| [CLI](cli/index.md) | Команды и параметры запуска |
| [Конфигурация](configuration/index.md) | Полное руководство по YAML-конфигурации |
| [Рабочий процесс](workflow/index.md) | Регенерация, Makefile, OnlineConf |
| [Примеры](examples/index.md) | Готовые примеры конфигураций |
| [Тестирование](testing/index.md) | GOAT интеграционные тесты |
| [Справочник](reference/yaml-schema.md) | YAML Schema, Makefile targets |
| [Troubleshooting](troubleshooting.md) | FAQ и решение проблем |

## Поддержка

- [GitHub Issues](https://github.com/Educentr/go-project-starter/issues) — баг репорты и feature requests
- [GitHub Discussions](https://github.com/Educentr/go-project-starter/discussions) — вопросы и обсуждения
