# Продвинутые темы

## Сохранение пользовательского кода

### Как работают Disclaimer-маркеры

Каждый сгенерированный файл содержит маркер:

```go
// ==========================================
// GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
// Changes manually made below will not be overwritten by generator.
// ==========================================
```

**Правила:**
1. Код выше маркера регенерируется при каждом запуске
2. Код ниже маркера сохраняется навсегда
3. Если нужно изменить сгенерированный код - переместите его ниже маркера

### Файлы, которые никогда не перезаписываются

- `.gitignore`
- `go.mod`, `go.sum`
- `LICENSE.txt`
- `README.md`
- `.git/` директория

### Workflow регенерации

1. Измените `config.yaml` или OpenAPI спецификацию
2. Запустите генератор: `go-project-starter --config=config.yaml`
3. Ваш код ниже disclaimer-маркеров автоматически сохранится
4. Новые файлы/endpoints добавляются без перезаписи ручных изменений

## Паттерн драйверов

### Интерфейс Runnable

Все драйверы реализуют интерфейс `Runnable`:

```go
type Runnable interface {
    Init(ctx context.Context) error
    Run(ctx context.Context) error
    Shutdown(ctx context.Context) error
    GracefulShutdown(ctx context.Context) error
}
```

### Типы драйверов

| Тип | Поведение | Примеры |
|-----|-----------|---------|
| **Активный** | Имеет фоновый процесс (Run не пустой) | Telegram (webhooks/websockets) |
| **Пассивный** | Run — no-op | S3, HTTP клиенты |

### Жизненный цикл драйвера

```
Init() → Run() (в горутине) → GracefulShutdown() → Shutdown()
```

1. `Init()` — инициализация соединений, валидация конфига
2. `Run()` — запуск фонового процесса (для активных драйверов)
3. `GracefulShutdown()` — мягкое завершение (ждём текущие операции)
4. `Shutdown()` — принудительное завершение

### Создание кастомного драйвера

```yaml
drivers:
  - name: payment_gateway
    import: pkg/drivers/payment
    package: payment
    obj_name: PaymentDriver
```

**Структура драйвера:**

```go
package payment

type PaymentDriver struct {
    client *http.Client
    apiKey string
}

func (d *PaymentDriver) Init(ctx context.Context) error {
    d.client = &http.Client{Timeout: 10 * time.Second}
    d.apiKey = config.Get("payment.api_key")
    return nil
}

func (d *PaymentDriver) Run(ctx context.Context) error {
    // Пассивный драйвер - no-op
    <-ctx.Done()
    return nil
}

func (d *PaymentDriver) Shutdown(ctx context.Context) error {
    d.client.CloseIdleConnections()
    return nil
}

func (d *PaymentDriver) GracefulShutdown(ctx context.Context) error {
    return d.Shutdown(ctx)
}

// Бизнес-методы
func (d *PaymentDriver) ProcessPayment(ctx context.Context, amount int64) error {
    // ...
}
```

### Замена провайдера

Для замены провайдера без изменения бизнес-логики:

1. Определите интерфейс в сервисном слое:

```go
type StorageDriver interface {
    Upload(ctx context.Context, key string, data []byte) error
    Download(ctx context.Context, key string) ([]byte, error)
    Delete(ctx context.Context, key string) error
}
```

2. Реализуйте драйвер для каждого провайдера:

```go
// pkg/drivers/s3/aws.go
type AWSDriver struct { ... }

// pkg/drivers/s3/digitalocean.go
type DODriver struct { ... }
```

3. Переключайте через конфигурацию:

```yaml
drivers:
  - name: storage
    type: s3
    provider: digitalocean  # Просто измените эту строку
```

## OnlineConf интеграция

### Динамическая конфигурация

OnlineConf позволяет изменять конфигурацию без редеплоя:

```yaml
onlineconf:
  enabled: true
  tree: myservice
  environment: production
```

### Чтение конфигурации в runtime

```go
// Чтение значений
maxRetries := onlineconf.GetInt("myservice.api.max_retries")
timeout := onlineconf.GetDuration("myservice.api.timeout")
enabled := onlineconf.GetBool("myservice.feature.enabled")

// Автоматически перезагружается при изменении в OnlineConf
```

### Структура путей OnlineConf

```
{service_name}/
├── devstand                    # 0 или 1
├── log/
│   └── level                   # info, debug, error
├── transport/
│   └── rest/
│       └── {name}_{version}/
│           ├── ip              # 0.0.0.0
│           ├── port            # 8080
│           └── timeout         # 2s
├── db/
│   └── main/
│       ├── host                # localhost:5432
│       ├── User
│       ├── Password
│       └── DB
└── security/
    ├── csrf/
    │   └── enabled             # 0 или 1
    └── httpAuth/
        └── enabled             # 0 или 1
```

## Мониторинг и наблюдаемость

### Prometheus метрики

Сгенерированные проекты включают RED метрики из коробки:

- **Rate** — количество запросов в секунду
- **Errors** — количество ошибок
- **Duration** — время обработки

### Стандартные метрики

| Метрика | Тип | Описание |
|---------|-----|----------|
| `http_server_requests_total` | Counter | Общее количество запросов |
| `http_server_request_duration_seconds` | Histogram | Время обработки запроса |
| `go_goroutines` | Gauge | Количество горутин |
| `go_memstats_alloc_bytes` | Gauge | Выделенная память |

### Labels для метрик

- `server_name` — идентификатор HTTP сервера
- `client_name` — идентификатор HTTP клиента
- `method` — HTTP метод
- `path` — путь запроса
- `status` — код ответа

### Grafana Dashboard

Настройка автогенерации дашбордов:

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
      datasources:
        - Prometheus
        - Loki
```

### Генерируемые панели

| Панель | Datasource | Метрики |
|--------|------------|---------|
| Logs | Loki | Логи с фильтрацией по уровню |
| Go Runtime | Prometheus | goroutines, memory, GC |
| HTTP Server | Prometheus | RPS, latency, errors |
| HTTP Client | Prometheus | request duration, errors |

### Использование дашбордов

1. Скопируйте `grafana/` директорию в Grafana instance
2. Примонтируйте provisioning configs в docker-compose:

```yaml
grafana:
  image: grafana/grafana:latest
  volumes:
    - ./grafana/provisioning:/etc/grafana/provisioning
    - ./grafana/dashboards:/var/lib/grafana/dashboards
```

3. Дашборды и datasources автоматически провизионятся при старте

## Distributed Tracing

### Request ID пропагация

Каждый запрос получает уникальный trace ID через заголовок `x-req-id`:

```go
func (m *Middleware) Tracing(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        reqID := r.Header.Get("x-req-id")
        if reqID == "" {
            reqID = uuid.New().String()
        }
        ctx := context.WithValue(r.Context(), "req_id", reqID)
        w.Header().Set("x-req-id", reqID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Логирование с correlation ID

```go
logger.Info().
    Str("req_id", ctx.Value("req_id").(string)).
    Msg("Processing request")
```

## Версионирование API

### Поддержка нескольких версий

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

### Генерируемая структура

```
internal/app/server/transport/rest/api/
├── v1/
│   ├── handlers.go
│   └── router.go
└── v2/
    ├── handlers.go
    └── router.go
```

### Миграция между версиями

1. Добавьте новую версию API в конфиг
2. Регенерируйте проект
3. Реализуйте handlers для новой версии
4. Постепенно переводите клиентов на новую версию
5. Удалите старую версию когда все мигрируют

## Docker Image Dependencies

### Предварительный pull образов

Для сервисов, требующих внешние Docker образы:

```yaml
applications:
  - name: checker
    transport: [sys]
    workers: [checker]
    depends_on_docker_images:
      - ghcr.io/some-app/tool:latest
      - postgres:15-alpine
```

### Генерируемый docker-compose

```yaml
# Image pullers
tool-image-puller:
  image: ghcr.io/some-app/tool:latest
  pull_policy: always
  restart: "no"

postgres-image-puller:
  image: postgres:15-alpine
  pull_policy: always
  restart: "no"

# Application
checker:
  build: .
  depends_on:
    tool-image-puller:
      condition: service_completed_successfully
    postgres-image-puller:
      condition: service_completed_successfully
```

### Применения

- Docker-in-Docker сценарии
- Тестовые окружения с внешними инструментами
- Гарантия наличия образов перед стартом

## Кастомные шаблоны

### Добавление нового шаблона

Шаблоны находятся в `templater/embedded/templates/`:

```
templates/
├── main/           # Makefile, Dockerfile, configs
├── transport/
│   ├── rest/       # REST транспорты
│   │   ├── ogen/   # OpenAPI генератор
│   │   └── template/
│   │       └── sys/ # Системный шаблон
│   ├── grpc/       # gRPC
│   └── kafka/      # Kafka consumers
├── worker/
│   └── template/
│       ├── telegram/ # Telegram бот
│       └── daemon/   # Daemon воркер
├── app/            # Application layer
└── logger/         # Logger implementations
```

### Использование кастомного шаблона

```yaml
rest:
  - name: custom
    generator_type: template
    generator_template: my_custom_template  # Используется templates/transport/rest/template/my_custom_template/
    port: 8082
    version: v1
```

## Приватные Go модули

### Настройка GOPRIVATE

```yaml
git:
  repo: github.com/myorg/service
  module_path: github.com/myorg/service
  private_repos:
    - github.com/myorg/internal-pkg
    - gitlab.com/company/*
```

### Генерируемые настройки

В Makefile и Dockerfile автоматически добавляется:

```bash
GOPRIVATE=github.com/myorg/internal-pkg,gitlab.com/company/*
```

## Логирование

### Интерфейс логгера

Логгер абстрагирован через интерфейс:

```go
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
}
```

### Правила слоёв

| Слой | Логирование |
|------|-------------|
| `pkg/` | Нет (возвращает ошибки) |
| `internal/pkg/` | Нет (возвращает ошибки) |
| `internal/app/` | Да (использует конкретный логгер) |

### Zerolog

Сейчас поддерживается только zerolog:

```yaml
main:
  logger: zerolog
```

**Пример использования:**

```go
logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
logger.Info().
    Str("user_id", userID).
    Int("count", count).
    Msg("Processing request")
```

## Следующие шаги

- [Сравнение](comparison.md) - сравнение с альтернативами
- [Участие в разработке](contributing.md) - как внести вклад
