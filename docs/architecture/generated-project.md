# Архитектура сгенерированного проекта

Описание структуры и паттернов сгенерированных Go-проектов.

## Общая архитектура

Сгенерированный проект следует принципам чистой архитектуры с чётким разделением ответственности. Зависимости направлены **только сверху вниз** — верхние слои зависят от нижних, но не наоборот.

```mermaid
flowchart TB
    subgraph CMD["cmd/"]
        MAIN[main.go<br/>Точка входа]
    end

    subgraph INTERNAL_APP["internal/app/ — Проект-специфичный код"]
        direction TB
        TRANSPORT[transport/<br/>REST/gRPC handlers]
        WORKER[worker/<br/>Telegram, Daemon]
        APP_SERVICE[service/<br/>Бизнес-логика]
    end

    subgraph INTERNAL_PKG["internal/pkg/ — Переиспользуемый код"]
        direction TB
        MODEL[model/<br/>Модели данных]
        REPOSITORY[repository/<br/>Доступ к данным]
        IPK_SERVICE[service/<br/>Интерфейсы]
    end

    subgraph PKG["pkg/ — Runtime библиотеки"]
        direction TB
        PKG_APP[app/<br/>Lifecycle]
        PKG_DRIVERS[drivers/<br/>Интеграции]
        PKG_REST[rest/<br/>REST клиенты]
    end

    subgraph EXTERNAL["Внешние сервисы"]
        DB[(PostgreSQL)]
        REDIS[(Redis)]
        KAFKA[Kafka]
        TELEGRAM[Telegram API]
    end

    MAIN --> TRANSPORT
    MAIN --> WORKER
    TRANSPORT --> APP_SERVICE
    WORKER --> APP_SERVICE
    APP_SERVICE --> MODEL
    APP_SERVICE --> REPOSITORY
    APP_SERVICE --> IPK_SERVICE
    REPOSITORY --> PKG_DRIVERS
    PKG_DRIVERS --> DB
    PKG_DRIVERS --> REDIS
    PKG_DRIVERS --> KAFKA
    PKG_DRIVERS --> TELEGRAM

    style CMD fill:#e1f5fe
    style INTERNAL_APP fill:#fff3e0
    style INTERNAL_PKG fill:#e8f5e9
    style PKG fill:#f3e5f5
    style EXTERNAL fill:#fce4ec
```

## Трёхслойный дизайн

Архитектура основана на трёх слоях с различными правилами зависимостей:

```mermaid
flowchart TB
    subgraph LAYER1["<b>Слой 1: internal/app/</b><br/>Проект-специфичный код"]
        L1_DESC["✓ Зависит от конфига<br/>✓ Использует конкретный логгер<br/>✓ Знает о транспортах"]
    end

    subgraph LAYER2["<b>Слой 2: internal/pkg/</b><br/>Переиспользуемый код"]
        L2_DESC["✗ НЕ зависит от конфига<br/>✗ НЕ привязан к логгеру<br/>✓ Возвращает ошибки вверх"]
    end

    subgraph LAYER3["<b>Слой 3: pkg/</b><br/>Runtime библиотеки"]
        L3_DESC["✗ НЕ зависит от конфига<br/>✗ Максимально переиспользуем<br/>✓ Может быть вынесен в отдельный модуль"]
    end

    LAYER1 -->|импортирует| LAYER2
    LAYER2 -->|импортирует| LAYER3

    style LAYER1 fill:#fff3e0,stroke:#ff9800
    style LAYER2 fill:#e8f5e9,stroke:#4caf50
    style LAYER3 fill:#f3e5f5,stroke:#9c27b0
```

### Почему это важно?

| Проблема | Решение в архитектуре |
|----------|----------------------|
| Сложность тестирования | Нижние слои не зависят от конфига — легко мокать |
| Привязка к логгеру | `internal/pkg` возвращает ошибки, логгер — на верхнем уровне |
| Переиспользование | `pkg/` можно вынести в отдельный go module |
| Циклические зависимости | Зависимости только сверху вниз |

## Описание слоёв

### Слой `pkg/` (Runtime)

```mermaid
flowchart LR
    subgraph PKG["pkg/"]
        APP["app/<br/>Lifecycle"]
        DRIVERS["drivers/<br/>DB, Redis, S3..."]
        REST["rest/<br/>HTTP клиенты"]
        MIDDLEWARE["middleware/<br/>HTTP middleware"]
    end

    style PKG fill:#f3e5f5,stroke:#9c27b0
```

**Назначение:** Runtime-библиотеки, максимально переиспользуемые между проектами.

**Требования:**

- ✗ Нет зависимостей от конфига проекта
- ✗ Нет зависимостей от конкретных реализаций логгера
- ✓ Может быть вынесен в отдельный go module

**Содержимое:**

| Директория | Назначение | Примеры |
|------------|------------|---------|
| `pkg/app/` | Жизненный цикл приложения | `Runnable` интерфейс, graceful shutdown |
| `pkg/drivers/` | Драйверы внешних сервисов | PostgreSQL, Redis, S3, Telegram |
| `pkg/rest/` | Сгенерированные REST клиенты | Ogen-клиенты для внешних API |
| `pkg/middleware/` | HTTP middleware | Metrics, tracing, auth |

### Слой `internal/pkg/` (Generated Core)

```mermaid
flowchart LR
    subgraph IPKG["internal/pkg/"]
        MODEL["model/<br/>Модели данных"]
        SERVICE["service/<br/>Интерфейсы"]
        REPO["repository/<br/>Доступ к данным"]
        CONSTANT["constant/<br/>Константы"]
    end

    style IPKG fill:#e8f5e9,stroke:#4caf50
```

**Назначение:** Генерируемый код, переиспользуемый между приложениями одного сервиса.

**Требования:**

- ✗ Не зависит от проект-специфичных настроек
- ✗ Не привязан к конкретному логгеру
- ✓ Функции возвращают ошибки вверх, логирование — на верхнем уровне

**Содержимое:**

| Директория | Назначение | Пример кода |
|------------|------------|-------------|
| `model/` | Модели данных | `type User struct {...}` |
| `service/` | Интерфейсы сервисов | `type UserService interface {...}` |
| `repository/` | Репозитории | `func (r *Repo) GetUser(ctx, id)` |
| `constant/` | Константы | `const ServiceName = "my-api"` |

### Слой `internal/app/` (Project-Specific)

```mermaid
flowchart LR
    subgraph IAPP["internal/app/{app_name}/"]
        TRANSPORT["transport/<br/>REST, gRPC, CLI"]
        WORKER["worker/<br/>Telegram, Daemon"]
        SVC["service/<br/>Бизнес-логика"]
    end

    style IAPP fill:#fff3e0,stroke:#ff9800
```

**Назначение:** Код, специфичный для конкретного приложения.

**Характеристики:**

- ✓ Может зависеть от конфига
- ✓ Может использовать конкретные реализации (логгер, драйверы)
- ✓ Содержит бизнес-логику обработчиков

**Содержимое:**

| Директория | Назначение | Пример |
|------------|------------|--------|
| `transport/rest/` | REST обработчики | `handler.go` с методами API |
| `transport/grpc/` | gRPC реализации | Реализация proto-сервисов |
| `worker/` | Фоновые воркеры | Telegram bot, Kafka consumer |
| `service/` | Бизнес-логика | Специфичная для приложения логика |

## Концепция Application

**Application** — атомарная единица горизонтального масштабирования (контейнер).

```mermaid
flowchart TB
    subgraph SERVICE["Один сервис (codebase)"]
        subgraph APP1["Application: gateway"]
            REST1[REST API v1]
            GRPC1[gRPC Users]
        end

        subgraph APP2["Application: workers"]
            TG[Telegram Bot]
            KAFKA[Kafka Consumer]
        end

        subgraph APP3["Application: admin"]
            REST2[REST Admin API]
        end
    end

    subgraph DEPLOY["Kubernetes/Docker"]
        POD1[Pod: gateway<br/>replicas: 3]
        POD2[Pod: workers<br/>replicas: 1]
        POD3[Pod: admin<br/>replicas: 1]
    end

    APP1 -.-> POD1
    APP2 -.-> POD2
    APP3 -.-> POD3

    style SERVICE fill:#e3f2fd
    style DEPLOY fill:#e8f5e9
```

### Характеристики

| Свойство | Описание |
|----------|----------|
| **Атомарность** | Один бинарь = один контейнер = один pod |
| **Компоненты** | HTTP серверы, gRPC серверы, воркеры инициализируются параллельно |
| **Масштабирование** | Каждое application масштабируется независимо |
| **Конфигурация** | Каждое application может иметь свои настройки |

```yaml
applications:
  # API Gateway с REST и gRPC (высоконагруженный)
  - name: gateway
    transport:
      - name: rest_api
      - name: grpc_users

  # Выделенный worker instance (один экземпляр)
  - name: workers
    worker: [telegram_bot]
    kafka: [order_consumer]

  # Всё-в-одном для небольших деплоев
  - name: monolith
    transport:
      - name: rest_api
      - name: grpc_users
    worker: [telegram_bot]
```

### Жизненный цикл Application

```mermaid
stateDiagram-v2
    [*] --> Init: Запуск

    state Init {
        [*] --> LoadConfig: 1. Загрузка конфига
        LoadConfig --> CreateLogger: 2. Создание логгера
        CreateLogger --> InitDrivers: 3. Init драйверов
        InitDrivers --> [*]
    }

    Init --> Run: Успешная инициализация

    state Run {
        [*] --> RunDrivers: 1. Run драйверов
        RunDrivers --> StartHTTP: 2. Запуск HTTP серверов
        StartHTTP --> StartGRPC: 3. Запуск gRPC серверов
        StartGRPC --> StartWorkers: 4. Запуск воркеров
        StartWorkers --> Serving: Все компоненты запущены
        Serving --> [*]: SIGTERM/SIGINT
    }

    Run --> Shutdown: Сигнал завершения

    state Shutdown {
        [*] --> GracefulWorkers: 1. Graceful shutdown воркеров
        GracefulWorkers --> StopHTTP: 2. Остановка HTTP (drain)
        StopHTTP --> StopGRPC: 3. Остановка gRPC
        StopGRPC --> ShutdownDrivers: 4. Shutdown драйверов
        ShutdownDrivers --> [*]
    }

    Shutdown --> [*]: Завершение
```

## Сохранение пользовательского кода

**Главная особенность:** Перегенерируйте весь проект без потери ваших изменений.

Каждый сгенерированный файл содержит disclaimer-маркер:

```go
// ==========================================
// GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
// Changes manually made below will not be overwritten by generator.
// ==========================================

func (h *Handler) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
    // Ваша бизнес-логика здесь
    // Этот код переживёт регенерацию!

    user := &User{
        Email: req.Email,
        Name:  req.Name,
    }

    // Кастомная валидация
    if err := h.validateBusinessRules(user); err != nil {
        return nil, err
    }

    return h.repo.Create(ctx, user)
}
```

**Правила:**

1. Код выше маркера регенерируется при каждом запуске
2. Код ниже маркера сохраняется навсегда
3. Если нужно изменить сгенерированный код — переместите его ниже маркера

## Драйверы интеграций

**Драйвер** — адаптер между бизнес-логикой и внешним API.

```mermaid
flowchart LR
    subgraph APP["Application"]
        SERVICE[Service<br/>бизнес-логика]
    end

    subgraph INTERFACE["Interface"]
        IFACE["SendMessage(ctx, msg)<br/>GetFile(ctx, id)"]
    end

    subgraph DRIVERS["Drivers"]
        TG[Telegram Driver]
        SLACK[Slack Driver]
        MOCK[Mock Driver]
    end

    subgraph EXTERNAL["External APIs"]
        TG_API[Telegram API]
        SLACK_API[Slack API]
    end

    SERVICE --> IFACE
    IFACE -.-> TG
    IFACE -.-> SLACK
    IFACE -.-> MOCK
    TG --> TG_API
    SLACK --> SLACK_API

    style APP fill:#fff3e0
    style INTERFACE fill:#e8f5e9
    style DRIVERS fill:#e3f2fd
    style EXTERNAL fill:#fce4ec
```

### Принцип работы

| Компонент | Ответственность |
|-----------|-----------------|
| **Service** | Вызывает абстрактный интерфейс ("отправь сообщение") |
| **Interface** | Определяет контракт (что можно делать) |
| **Driver** | Транслирует вызов в конкретный API |

**Преимущества:**

- Замена провайдера без изменения бизнес-логики
- Лёгкое тестирование через mock-драйверы
- Поддержка нескольких провайдеров одновременно

### Интерфейс Runnable

Все драйверы реализуют унифицированный интерфейс жизненного цикла:

```go
type Runnable interface {
    Init(ctx context.Context) error           // Инициализация подключений
    Run(ctx context.Context) error            // Запуск фоновых процессов
    Shutdown(ctx context.Context) error       // Немедленная остановка
    GracefulShutdown(ctx context.Context) error // Graceful остановка
}
```

### Типы драйверов

```mermaid
flowchart TB
    subgraph ACTIVE["Активные драйверы"]
        TG["Telegram<br/>webhook listener"]
        WS["WebSocket<br/>connection pool"]
    end

    subgraph PASSIVE["Пассивные драйверы"]
        S3["S3<br/>on-demand calls"]
        DB["PostgreSQL<br/>connection pool"]
    end

    ACTIVE -->|Run блокирует| GOROUTINE[Фоновая горутина]
    PASSIVE -->|Run = no-op| NOOP[Немедленный возврат]

    style ACTIVE fill:#fff3e0
    style PASSIVE fill:#e8f5e9
```

| Тип | Примеры | Метод Run |
|-----|---------|-----------|
| **Активные** | Telegram, WebSocket | Блокирует, слушает события |
| **Пассивные** | S3, PostgreSQL | No-op, вызовы по требованию |

## Middleware Stack

```mermaid
flowchart TB
    REQ[HTTP Request] --> MW1

    subgraph MIDDLEWARE["Middleware Chain"]
        MW1["1. Context Creation<br/>таймаут, cancel"]
        MW2["2. Panic Recovery<br/>перехват паник"]
        MW3["3. Request Timing<br/>время начала"]
        MW4["4. Tracing<br/>x-req-id header"]
        MW5["5. Metrics<br/>Prometheus counters"]
        MW6["6. X-Server Header<br/>идентификатор сервера"]
        MW7["7. HTTP Auth<br/>(опционально)"]
        MW8["8. CSRF Protection<br/>(опционально)"]

        MW1 --> MW2 --> MW3 --> MW4 --> MW5 --> MW6 --> MW7 --> MW8
    end

    MW8 --> HANDLER[Handler<br/>бизнес-логика]
    HANDLER --> RESP[HTTP Response]

    style MIDDLEWARE fill:#e3f2fd
```

### Описание middleware

| # | Middleware | Назначение |
|---|------------|------------|
| 1 | **Context Creation** | Создание request context с таймаутом и cancel функцией |
| 2 | **Panic Recovery** | Перехват паник, логирование stack trace, возврат 500 |
| 3 | **Request Timing** | Добавление времени начала запроса в context |
| 4 | **Tracing** | Добавление/проброс trace ID (`x-req-id` header) |
| 5 | **Metrics** | Сбор Prometheus метрик (latency, status codes, in-flight) |
| 6 | **X-Server Header** | Добавление заголовка с идентификатором сервера |
| 7 | **HTTP Auth** | Actor-based аутентификация (опционально) |
| 8 | **CSRF Protection** | Валидация CSRF токена (опционально) |

## Поток запроса (Request Flow)

Полный путь HTTP-запроса через систему:

```mermaid
sequenceDiagram
    participant Client
    participant Traefik as Traefik<br/>(reverse proxy)
    participant HTTP as HTTP Server
    participant MW as Middleware Chain
    participant Handler as Handler
    participant Service as Service
    participant Repo as Repository
    participant DB as Database

    Client->>Traefik: POST /api/v1/users
    Traefik->>HTTP: Forward request

    HTTP->>MW: Process request
    Note over MW: 1. Context + timeout<br/>2. Panic recovery<br/>3. Tracing (x-req-id)<br/>4. Metrics start

    MW->>Handler: Call handler
    Handler->>Service: CreateUser(ctx, req)
    Service->>Repo: repo.Create(ctx, user)
    Repo->>DB: INSERT INTO users...
    DB-->>Repo: OK
    Repo-->>Service: user, nil
    Service-->>Handler: user, nil
    Handler-->>MW: response

    Note over MW: 5. Metrics end<br/>6. Log request

    MW-->>HTTP: response
    HTTP-->>Traefik: HTTP 201 Created
    Traefik-->>Client: response
```

## Структура сгенерированного проекта

```
myservice/                         # ~50 файлов, ~8000 строк кода
├── cmd/
│   └── server/                    # Точка входа приложения
│       └── main.go
│
├── internal/
│   ├── app/                       # Проект-специфичный код
│   │   └── {app_name}/
│   │       ├── transport/
│   │       │   ├── rest/
│   │       │   │   └── {api_name}/
│   │       │   │       ├── handler.go     # ← ВАШ КОД ЗДЕСЬ
│   │       │   │       └── middleware.go
│   │       │   └── grpc/
│   │       │       └── {service_name}/
│   │       │           └── handler.go
│   │       ├── worker/
│   │       │   ├── telegram/
│   │       │   │   └── handler.go
│   │       │   └── daemon/
│   │       │       └── handler.go
│   │       └── service/
│   │           └── service.go            # ← ВАШ КОД ЗДЕСЬ
│   │
│   └── pkg/                       # Переиспользуемый код
│       ├── model/                 # Модели данных
│       ├── repository/            # Доступ к данным
│       ├── service/               # Интерфейсы сервисов
│       └── constant/              # Константы (ServiceName)
│
├── pkg/                           # Runtime библиотеки
│   ├── app/                       # Lifecycle
│   ├── drivers/                   # DB, Redis, S3...
│   ├── rest/                      # HTTP клиенты
│   └── middleware/                # HTTP middleware
│
├── api/                           # API спецификации
│   ├── openapi.yaml               # OpenAPI 3.0
│   └── proto/                     # Protobuf файлы
│
├── docker-compose.yaml            # Локальное окружение
├── docker-compose-dev.yaml        # Dev окружение с OnlineConf
├── Dockerfile                     # Multi-stage build (~50MB)
├── Makefile                       # 40+ targets
│
└── .github/
    └── workflows/                 # CI/CD
        ├── ci.yaml                # Test, lint, build
        └── release.yaml           # Docker push
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

## Связь компонентов

```mermaid
flowchart TB
    subgraph ENTRY["Точки входа"]
        REST[REST API<br/>:8080]
        GRPC[gRPC<br/>:9090]
        SYS[System<br/>:8085]
    end

    subgraph HANDLERS["Handlers (internal/app)"]
        REST_H[REST Handler]
        GRPC_H[gRPC Handler]
    end

    subgraph SERVICES["Services"]
        SVC[UserService<br/>OrderService]
    end

    subgraph DATA["Data Layer"]
        REPO[Repository]
        AR[ActiveRecord]
    end

    subgraph EXTERNAL["External"]
        DB[(PostgreSQL)]
        REDIS[(Redis)]
        KAFKA[Kafka]
    end

    REST --> REST_H
    GRPC --> GRPC_H
    REST_H --> SVC
    GRPC_H --> SVC
    SVC --> REPO
    SVC --> AR
    REPO --> DB
    AR --> DB
    SVC --> REDIS
    SVC --> KAFKA

    style ENTRY fill:#e3f2fd
    style HANDLERS fill:#fff3e0
    style SERVICES fill:#e8f5e9
    style DATA fill:#f3e5f5
    style EXTERNAL fill:#fce4ec
```
