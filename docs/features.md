# Возможности

## Многопротокольная поддержка транспортов

Генерируйте сервисы, работающие с несколькими протоколами одновременно:

### REST API (OpenAPI 3.0)

- Типобезопасные обработчики через [ogen](https://github.com/ogen-go/ogen)
- Автоматическая валидация запросов/ответов
- Интеграция Swagger UI

### gRPC сервисы (Protocol Buffers v3)

- Высокопроизводительный RPC
- Двунаправленный streaming
- Генерация клиентских библиотек

### Kafka Consumers

- Event-driven архитектура
- Управление consumer groups
- Отслеживание offset'ов

### Фоновые воркеры

- Telegram боты (webhooks/polling)
- Daemon воркеры
- Запланированные задачи

## Абстракция драйверов

Меняйте провайдеров внешних сервисов без изменения бизнес-логики:

```yaml
# Переход с AWS S3 на DigitalOcean Spaces
drivers:
  - name: storage
    type: s3
    provider: digitalocean  # Просто измените эту строку
    config:
      endpoint: nyc3.digitaloceanspaces.com
```

Все драйверы реализуют интерфейс `Runnable` (Init, Run, Shutdown, GracefulShutdown), что делает их частью жизненного цикла приложения.

## Application-Based масштабирование

Деплойте сервисы с разными профилями из одной кодовой базы:

```yaml
applications:
  # API gateway с REST и gRPC
  - name: gateway
    transport: [rest_api, grpc_users]

  # Выделенный worker instance
  - name: workers
    workers: [telegram_bot, kafka_consumer]

  # Всё-в-одном для небольших деплоев
  - name: monolith
    transport: [rest_api, grpc_users]
    workers: [telegram_bot]
```

Масштабируйте каждое приложение независимо в Kubernetes:

```bash
kubectl scale deployment gateway --replicas=5
kubectl scale deployment workers --replicas=2
```

## Production-Ready инфраструктура

Каждый сгенерированный проект включает:

| Компонент | Описание |
|-----------|----------|
| **Docker & Docker Compose** | Multi-stage сборки, оптимизированные по размеру |
| **System Packages** | deb/rpm/apk пакеты для bare-metal деплоя |
| **Traefik** | Reverse proxy с автоматическим HTTPS |
| **GitHub Actions CI/CD** | Workflows для тестов, сборки и деплоя |
| **Prometheus метрики** | RED метрики (Rate, Errors, Duration) из коробки |
| **Health Checks** | Liveness и readiness probes |
| **Distributed Tracing** | Пропагация request ID (x-req-id) |
| **Structured Logging** | Zerolog с correlation ID |
| **Graceful Shutdown** | Корректное завершение на SIGTERM |
| **OnlineConf** | Динамическая конфигурация без редеплоя |
| **Grafana Dashboards** | Автогенерируемые дашборды с Prometheus и Loki |

## Генерация Grafana Dashboard

Генерируйте готовые к использованию Grafana дашборды:

```yaml
grafana:
  datasources:
    - name: Prometheus
      type: prometheus
      url: http://prometheus:9090
      isDefault: true
    - name: Loki
      type: loki
      url: http://loki:3100

applications:
  - name: api
    transport: [api, sys]
    grafana:
      datasources: [Prometheus, Loki]
```

### Генерируемые панели

| Панель | Условие | Метрики |
|--------|---------|---------|
| **Logs** | Loki datasource | Логи с фильтрацией по уровню |
| **Go Runtime** | Prometheus | `go_goroutines`, `go_memstats_*`, `go_gc_*` |
| **HTTP Server: {name}** | Для `ogen` транспорта | `http_server_request_duration_seconds`, `http_server_requests_total` |
| **HTTP Client: {name}** | Для `ogen_client` | `http_client_request_duration_seconds`, `http_client_requests_total` |

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

## Системные пакеты (deb/rpm/apk)

Помимо Docker образов, генерируйте системные пакеты для bare-metal деплоя:

```yaml
artifacts:
  - docker    # Docker образы (по умолчанию)
  - deb       # Debian/Ubuntu
  - rpm       # CentOS/RHEL/Fedora
  - apk       # Alpine Linux

packaging:
  maintainer: "DevOps <devops@example.com>"
  description: "Order processing service"
  license: "MIT"
```

### Поддерживаемые форматы

| Формат | Дистрибутивы | Инструмент |
|--------|--------------|------------|
| **deb** | Debian, Ubuntu | dpkg, apt |
| **rpm** | CentOS, RHEL, Fedora, Rocky | yum, dnf |
| **apk** | Alpine Linux | apk |

### Что включено в пакет

- **Бинарный файл** → `/usr/bin/{app-name}`
- **Systemd unit** → `/lib/systemd/system/{project}-{app}.service`
- **Конфиг директория** → `/etc/{project}/{app}/`
- **Post-install скрипт** — создание пользователя, systemctl enable
- **Pre-remove скрипт** — systemctl stop, disable

### Сборка и установка

```bash
# Сборка пакетов
make install-nfpm     # Установить nfpm
make deb-api          # Собрать .deb для приложения api
make rpm-api          # Собрать .rpm
make packages         # Собрать все пакеты

# Установка на целевом сервере
sudo dpkg -i myservice-api_1.0.0_amd64.deb
sudo systemctl status myservice-api
```

### CI/CD интеграция

При включении системных пакетов автоматически генерируются:

- **GitLab CI**: job `build-packages` собирает пакеты и сохраняет как артефакты
- **GitHub Actions**: job `build-packages` загружает пакеты в artifacts

## Developer Experience

### Makefile с 40+ целями

```bash
make generate        # Запуск всех генераторов (ogen, argen, mock)
make test            # Запуск тестов с coverage
make lint            # golangci-lint с 40+ linters
make docker-build    # Сборка Docker образа
make docker-up       # Запуск всех зависимостей
make migrate-up      # Запуск миграций БД
make mock            # Генерация моков для тестов
make packages        # Сборка системных пакетов (deb/rpm/apk)
```

### Миграции базы данных

Сгенерированные проекты включают:

- Фреймворк миграций (go-activerecord v3+)
- Отслеживание версий в `meta.yaml`
- Скрипты up/down миграций
- Автоматические обновления схемы

### Качество кода

- golangci-lint конфигурация (v1.55.2+)
- Pre-commit hooks
- Автоматическая организация imports
- Структура для тестов

## Middleware Stack

Стандартная цепочка middleware (в порядке выполнения):

1. **Context Creation** - Request context с таймаутом
2. **Panic Recovery** - Перехват и логирование паник
3. **Request Timing** - Отслеживание времени начала запроса
4. **Tracing** - Добавление trace ID (`x-req-id` header)
5. **Metrics** - Сбор Prometheus метрик
6. **X-Server Header** - Идентификатор сервера
7. **HTTP Auth** (опционально) - Actor-based аутентификация
8. **CSRF Protection** (опционально) - Валидация токена

## Поддержка нескольких версий API

Поддерживайте несколько версий API одновременно:

```yaml
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

applications:
  - name: server
    transport: [api]  # Включает и v1, и v2
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

## Генерируемые endpoints

### System Health/Monitoring (шаблон `sys`)

| Метод | Путь | Назначение |
|-------|------|------------|
| GET | `/version` | Информация о версии |
| GET | `/ready` | Readiness probe |
| GET | `/live` | Liveness probe |
| GET | `/metrics` | Prometheus метрики |
| GET | `/debug/pprof/*` | Go pprof профилирование |

### Telegram Bot обработчики

| Событие | Handler | Назначение |
|---------|---------|------------|
| PreCheckoutQuery | `PreCheckout` | Валидация платежа |
| SuccessfulPayment | `Purchase` | Обработка платежа |
| CallbackQuery | `CallbackQuery` | Callback от кнопок |
| Message | `TextMessage` | Обработка текста/команд |

## CLI транспорт

CLI — интерактивный транспорт командной строки:

```yaml
cli:
  - name: admin
    path: [./api/cli/admin.yaml]
    generator_type: template
    generator_template: cli

applications:
  - name: admin-cli
    cli: admin
    driver: [postgres]
```

### Принцип работы

```bash
./myapp <command> [arguments...]

# Примеры:
./myapp migrate up
./myapp user create --email admin@example.com
./myapp cache clear --all
```

### Правила CLI

1. **CLI эксклюзивен** — приложение с CLI не может иметь REST/gRPC транспорты
2. **Один CLI на application**
3. **Вызывает тот же Service** — CLI handlers используют те же методы сервиса
4. **Может использовать драйверы** — подключение к БД, внешним API

## Сохранение пользовательского кода

**Уникальная возможность:** Перегенерируйте проект при изменении API без потери своего кода.

Disclaimer-маркеры автоматически разделяют сгенерированный код и ваши изменения:

```go
// ==========================================
// GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
// Changes manually made below will not be overwritten by generator.
// ==========================================

func (h *Handler) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
    // Ваш код здесь - переживёт регенерацию!
}
```

## Следующие шаги

- [Примеры](examples.md) - готовые примеры конфигураций
- [Продвинутые темы](advanced.md) - драйверы, OnlineConf, мониторинг
- [Сравнение](comparison.md) - сравнение с альтернативами
