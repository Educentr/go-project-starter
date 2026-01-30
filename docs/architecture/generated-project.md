# Архитектура сгенерированного проекта

Описание структуры и паттернов сгенерированных Go-проектов.

## Общая архитектура

Сгенерированный проект следует принципам слоистой архитектуры с чётким разделением ответственности. Зависимости направлены **только сверху вниз** — транспортный слой обращается к сервису, сервис — к данным.

```mermaid
flowchart TB
    NET_IN((Network)) -.-> TRANSPORT

    subgraph TRANSPORT["Transport Layer"]
        direction LR
        API["<b>API</b><br/>Authorise · Helpers<br/>Handlers"]
        CLI["<b>CLI</b><br/>Authorise<br/>Handlers"]
        WORKERS["<b>Workers</b><br/>Jobs"]
        BOTS["<b>Bots</b><br/>Authorise · Helpers<br/>Handlers"]
    end

    TRANSPORT --> SERVICE["<b>Service</b>"]

    SERVICE --> DATA

    subgraph DATA["Data Layer"]
        direction LR
        CLIENTS[Clients]
        MODELS[Models]
        DRIVERS[Drivers]
    end

    MODELS --> DB_CONN[DB connectors] -.-> DB[(DB)]
    CLIENTS & DRIVERS -.-> NET_OUT((Network))

    style TRANSPORT fill:#e3f2fd
    style DATA fill:#fff3e0
    style SERVICE fill:#e8f5e9,stroke:#4caf50
```

!!! note "Config (OnlineConf)"
    Конфигурация доступна на **всех слоях** — Transport, Service, Data.
    Реализована как синглтон через OnlineConf.

**Ключевые слои:**

| Слой | Компоненты | Назначение |
|------|------------|------------|
| **Transport** | API, CLI, Workers, Bots | Точки входа с Authorise, Helpers, Handlers/Jobs |
| **Service** | Бизнес-логика | Центральный компонент, единственное место для бизнес-логики |
| **Data** | Clients, Models, Drivers | Доступ к данным и внешним сервисам |
| **DB connectors** | Коннекторы к БД | Подключение к базам данных |
| **Config** | OnlineConf | Конфигурация, доступна на всех слоях |

**Внешние связи:**

- **Network** (вверху) → входящие запросы к API и Bots
- **Network** (внизу) ← исходящие запросы от Clients и Drivers
- **DB** ← запросы к базе данных через DB connectors

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

## Spec-First подход

Одна из ключевых особенностей сгенерированного приложения — подход **spec-first** (или contract-first):

```mermaid
flowchart LR
    SPEC[OpenAPI/Proto<br/>Спецификация] --> GEN{Генератор}
    GEN --> SERVER[Server Code<br/>Handlers]
    GEN --> CLIENT[Client Code<br/>HTTP/gRPC клиенты]

    style SPEC fill:#e3f2fd
    style SERVER fill:#e8f5e9
    style CLIENT fill:#fff3e0
```

**Принцип:** Спецификация создаётся **до** написания любого кода. По этой спецификации генерируется:

- **Серверный код** — handlers для вашего сервиса
- **Клиентский код** — типизированные клиенты для вызова других сервисов

**Преимущества:**

| Аспект | Выгода |
|--------|--------|
| **Контракт** | API описан до реализации — frontend и backend могут работать параллельно |
| **Типизация** | Сгенерированные клиенты полностью типизированы |
| **Консистентность** | Клиент и сервер всегда соответствуют одной спецификации |

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

## Service — ядро бизнес-логики

**Service** — это центральный компонент, который содержит всю бизнес-логику приложения. Независимо от количества транспортов, воркеров или ботов — все они обращаются к одному и тому же сервису.

```mermaid
flowchart TB
    subgraph TRANSPORTS["Транспортный слой"]
        REST[REST API]
        GRPC[gRPC]
        CLI[CLI]
        BOT[Telegram Bot]
        WORKER[Worker]
    end

    subgraph CORE["Ядро"]
        SERVICE[Service<br/>Бизнес-логика]
    end

    subgraph DATA["Доступ к данным"]
        MODEL[Model/ORM]
        CLIENT[REST/gRPC клиенты]
        DRIVER[Drivers]
    end

    REST --> SERVICE
    GRPC --> SERVICE
    CLI --> SERVICE
    BOT --> SERVICE
    WORKER --> SERVICE

    SERVICE --> MODEL
    SERVICE --> CLIENT
    SERVICE --> DRIVER

    style CORE fill:#e8f5e9,stroke:#4caf50
```

**Ключевые принципы:**

1. **Единственное место для бизнес-логики** — вся переиспользуемая логика лежит в сервисе
2. **Изолированность** — сервис не знает о транспортах, он просто выполняет бизнес-операции
3. **Подготовка к масштабированию** — при распиливании монорепо на микросервисы, именно сервис будет рефакториться

**Что доступно сервису:**

| Компонент | Назначение |
|-----------|------------|
| **Model** | ORM для работы с базами данных (по умолчанию go-active-record) |
| **Client** | Сгенерированные клиенты к другим сервисам (REST, gRPC) |
| **Driver** | Кастомные коннекторы к внешним системам |
| **Config** | Конфигурация через OnlineConf (синглтон) |

**Важно:** Уделяйте особое внимание структуре пакетов внутри сервиса. Каждый компонент должен быть изолирован, чтобы в будущем его было проще выделить в отдельный микросервис.

## Типы транспортов

Транспортный слой делится на 4 типа по способу получения событий:

```mermaid
flowchart TB
    subgraph INCOMING["Входящие запросы"]
        REST[REST API<br/>Принимает HTTP запросы]
        GRPC[gRPC<br/>Принимает RPC вызовы]
    end

    subgraph POLLING["Опрос/подписка"]
        BOT[Bots<br/>Подключается к серверам,<br/>читает события]
        WORKER[Workers<br/>Берёт задачи из очереди,<br/>выполняет в цикле]
    end

    subgraph UTILITY["Утилиты"]
        CLI[CLI<br/>Консольные команды<br/>для администрирования]
    end

    style INCOMING fill:#e3f2fd
    style POLLING fill:#fff3e0
    style UTILITY fill:#e8f5e9
```

| Тип | Описание | Примеры |
|-----|----------|---------|
| **API (REST/gRPC)** | Принимают внешние запросы, обрабатывают, отвечают | HTTP эндпоинты, gRPC сервисы |
| **Workers** | Демоны с бесконечным циклом, выполняют джобы пачками | Обработка очередей, cron-задачи |
| **Bots** | Подключаются к внешним серверам, ждут события | Telegram, Slack боты |
| **CLI** | Консольные утилиты для разовых операций | Миграции, админские команды |

**Workers vs Bots:**

- **Workers** — сами берут задачи (pull-модель)
- **Bots** — получают события от внешних систем (push-модель)

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

