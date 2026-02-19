# Транспорты

Описание секций `rest`, `grpc`, `kafka` и `cli`.

## Секция `rest`

Конфигурация REST API транспортов.

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

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя транспорта |
| `path` | Для ogen/ogen_client | Пути к OpenAPI спецификациям |
| `generator_type` | Да | Тип генератора: `ogen`, `template`, `ogen_client` |
| `generator_template` | Для template | Имя шаблона (например, `sys`) |
| `generator_params` | Нет | Доп. параметры генератора (например, `auth_handler`) |
| `port` | Да (кроме sys) | HTTP порт |
| `version` | Да | Версия API (v1, v2, и т.д.) |
| `api_prefix` | Нет | Префикс URL для API |
| `health_check_path` | Нет | Путь для health check |
| `public_service` | Нет | Публичный сервис (без аутентификации) |
| `auth_params` | Нет | Параметры аутентификации (для ogen_client) |
| `instantiation` | Нет | `static` или `dynamic` (только для ogen_client) |

### Типы генераторов

| Тип | Описание | Применение |
|-----|----------|------------|
| `ogen` | Генерация сервера из OpenAPI 3.0 | Основные бизнес API |
| `template` | Шаблонная генерация | Health checks, метрики, кастомные endpoints |
| `ogen_client` | Генерация REST клиента | Вызов внешних API |

### Параметры аутентификации

Для `ogen_client` можно настроить аутентификацию:

```yaml
rest:
  - name: external_api
    generator_type: ogen_client
    auth_params:
      transport: header    # Способ передачи (пока только header)
      type: apikey         # Тип аутентификации: apikey или bearer
```

**Типы аутентификации:**

| Тип | OnlineConf путь | Описание |
|-----|-----------------|----------|
| `apikey` | `{service_name}/transport/rest/{rest_name}/auth_params/apikey` | API key в заголовке |
| `bearer` | `{service_name}/transport/rest/{rest_name}/auth_params/token` | Bearer token в заголовке Authorization |

### Динамический режим инстанцирования (ogen_client)

По умолчанию REST-клиенты создаются один раз при старте приложения (`static`).
Режим `dynamic` позволяет создавать клиенты при каждом запросе:

```yaml
rest:
  - name: external_api
    generator_type: ogen_client
    instantiation: dynamic  # Клиент создаётся при каждом вызове

applications:
  - name: server
    transport:
      - name: external_api
        config:
          instantiation: dynamic  # Override на уровне приложения
```

**Когда использовать `dynamic`:**

- URL внешнего API меняется динамически
- Нужна изоляция клиентов между запросами
- Тестирование с разными конфигурациями

**Приоритет настроек:**

1. `application.transport[].config.instantiation` (наивысший)
2. `rest[].instantiation` (default для всех приложений)

### Поддержка нескольких версий API

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
```

Генерируемая структура:

```
internal/app/server/transport/rest/api/
├── v1/
│   ├── handlers.go
│   └── router.go
└── v2/
    ├── handlers.go
    └── router.go
```

## Секция `grpc`

Конфигурация gRPC клиентов.

```yaml
grpc:
  - name: users
    path: ./proto/users.proto
    short: users
    port: 9000
    generator_type: buf_client
    buf_local_plugins: false
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя gRPC клиента |
| `path` | Да | Путь к .proto файлу |
| `short` | Нет | Короткое имя (для именования пакетов) |
| `port` | Да | gRPC порт |
| `generator_type` | Да | Тип генератора: `buf_client` |
| `buf_local_plugins` | Нет | Использовать локальные buf плагины |
| `instantiation` | Нет | `static` или `dynamic` (только для buf_client) |

!!! note "Только клиенты"
    В текущей версии поддерживается только генерация gRPC **клиентов** (`buf_client`). Генерация серверов (`buf_server`) пока не реализована.

### Динамический режим инстанцирования (buf_client)

По умолчанию gRPC-клиенты создаются один раз при старте приложения (`static`).
Режим `dynamic` позволяет создавать клиенты в рантайме через `NewDynamicClient(ctx, address)`:

```yaml
grpc:
  - name: users
    path: ./proto/users.proto
    port: 9000
    generator_type: buf_client
    instantiation: dynamic  # Клиент создаётся при каждом вызове
```

**Что меняется в `dynamic` режиме:**

- Клиент **не регистрируется** при старте (`SetClient`)
- Клиент **не добавляется** в Service struct
- Клиент **не валидируется** в `ValidateFor*`
- Вместо `NewClient()` + `Init()` генерируется `NewDynamicClient(ctx, address)`
- Адрес (host:port) передаётся как параметр, без OnlineConf

**Когда использовать `dynamic`:**

- Адрес gRPC сервера определяется в рантайме
- Нужна изоляция подключений между запросами
- Тестирование с разными конфигурациями

## Секция `kafka`

Конфигурация Kafka producers и consumers.

```yaml
kafka:
  - name: events_producer
    type: producer
    driver: segmentio
    client: main_kafka
    events:
      - name: user_events
        schema: models.user

  - name: order_consumer
    type: consumer
    driver: segmentio
    client: main_kafka
    group: my_group
    events:
      - name: order_events
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя producer/consumer |
| `type` | Да | `producer` или `consumer` |
| `driver` | Нет | `segmentio` (default) или custom |
| `client` | Да | Имя клиента для OnlineConf пути |
| `group` | Для consumer | Consumer group |
| `events` | Да | Список событий |

### Концепция Events

- `KafkaEvent` — единица публикации/потребления сообщений
- Event name используется для генерации Go методов
- Topic name по умолчанию совпадает с event name
- Topic можно переопределить через OnlineConf

### Типизированные сообщения

Events могут ссылаться на JSON Schema:

```yaml
jsonschema:
  - name: models
    schemas:
      - id: user
        path: ./user.schema.json

kafka:
  - name: producer
    type: producer
    client: main_kafka
    events:
      - name: user_events
        schema: models.user  # Генерирует типизированный метод
```

### OnlineConf пути для Kafka

| Путь | Описание |
|------|----------|
| `{service}/kafka/{client}/brokers` | Список брокеров через запятую |
| `{service}/kafka/{client}/auth_type` | none, PLAIN, SCRAM-SHA-256, SCRAM-SHA-512 |
| `{service}/kafka/{client}/username` | SASL username |
| `{service}/kafka/{client}/password` | SASL password |
| `{service}/kafka/{client}/tls_enabled` | 0 или 1 |
| `{service}/kafka/{client}/events/{event_name}/topic` | Override topic name |

## Секция `cli`

Конфигурация CLI транспорта. CLI генерирует обработчики команд из YAML-спецификации — по аналогии с тем, как ogen генерирует REST handlers из OpenAPI.

```yaml
cli:
  - name: admin
    path:
      - ./commands.yaml           # Спецификация CLI команд
    generator_type: template
    generator_template: cli

applications:
  - name: admin-cli
    cli: admin
    driver: [postgres]
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя CLI транспорта |
| `path` | Нет | Путь к YAML-спецификации команд (`commands.yaml`) |
| `generator_type` | Да | `template` |
| `generator_template` | Да | `cli` |

### Особенности CLI

- **CLI эксклюзивен** — приложение с CLI не может иметь REST/gRPC транспорты
- **Один CLI на application**
- **Вызывает тот же Service** — CLI handlers используют те же методы сервиса
- **Может использовать драйверы**

### Принцип работы

```bash
./myapp <command> [arguments...]

# Примеры:
./myapp migrate up
./myapp user create --email admin@example.com
./myapp cache clear --all
```

### Спецификация команд (`commands.yaml`)

Если указан `path`, генератор читает YAML-спецификацию и генерирует:

- **Params structs** — типизированные параметры для каждой команды
- **UnimplementedCLI** — стаб с default-реализацией (по аналогии с ogen/gRPC)
- **registerCommands()** — регистрация команд с flag parsing и валидацией
- **Command/Subcommand structs** — диспетчеризация команд и подкоманд

```yaml
# commands.yaml
commands:
  - name: user
    description: "User management"
    subcommands:
      - name: create
        description: "Create a new user"
        flags:
          - name: email
            type: string
            required: true
            description: "User email"
          - name: name
            type: string
            description: "User name"
      - name: list
        description: "List all users"
        flags:
          - name: limit
            type: int
            default: "100"
            description: "Max results"

  - name: ping
    description: "Check connectivity"

  - name: migrate
    description: "Database migrations"
    flags:
      - name: dir
        type: string
        default: "up"
        description: "Direction: up or down"
      - name: steps
        type: int
        description: "Number of steps"
```

**Правила спецификации:**

- Команда может иметь `subcommands` ИЛИ быть leaf-командой (без подкоманд)
- Флаги могут быть у leaf-команд и у подкоманд
- Типы флагов: `string`, `int`, `bool`, `float64`, `duration`
- `required: true` — генерируется валидация после парсинга
- `default` — значение по умолчанию (строка, парсится в нужный тип)

### Генерируемый код

Из спецификации генерируется файл `psg_handler_gen.go`:

#### Params structs

Для каждой команды/подкоманды с флагами генерируется struct с типизированными параметрами:

```go
type UserCreateParams struct {
    Email string
    Name  string
}

type MigrateParams struct {
    Dir   string
    Steps int
}
```

Имя: `{Command}{Subcommand}Params` (PascalCase). Если у команды нет флагов, Params struct не создаётся.

#### UnimplementedCLI

Стаб с default-реализацией — можно сразу компилировать проект без написания логики:

```go
type UnimplementedCLI struct{}

func (UnimplementedCLI) RunUserCreate(ctx context.Context, params UserCreateParams) error {
    return fmt.Errorf("command 'user create' is not implemented")
}

func (UnimplementedCLI) RunPing(ctx context.Context) error {
    return fmt.Errorf("command 'ping' is not implemented")
}
```

Имя метода: `Run{Command}{Subcommand}` (PascalCase).

#### Пользовательский код

Пользователь создаёт файлы в том же пакете и переопределяет методы:

```go
// user.go
package admin

func (h *Handler) RunUserCreate(ctx context.Context, params UserCreateParams) error {
    user, err := h.GetService().CreateUser(ctx, params.Email, params.Name)
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    fmt.Printf("User created: ID=%d\n", user.ID)
    return nil
}
```

!!! tip "Аналогия с ogen"
    Паттерн идентичен ogen: spec → `UnimplementedHandler` → пользователь переопределяет нужные методы. Нереализованные команды возвращают ошибку "not implemented".

### Без спецификации

Если `path` не указан, генерируется минимальный handler с закомментированным примером для ручной регистрации команд.

### Сравнение CLI и Worker

| CLI | Worker |
|-----|--------|
| Запускается пользователем | Запускается с приложением |
| Выполняет одну команду | Работает непрерывно |
| Интерактивный | Автономный |
| Завершается после команды | Работает до shutdown |
