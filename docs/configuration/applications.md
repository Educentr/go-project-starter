# Applications и Drivers

Описание секций `applications` и `drivers`.

## Секция `applications`

Конфигурация приложений — атомарных единиц деплоя.

```yaml
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

  # С зависимостью от Docker образов
  - name: checker
    transport: [sys]
    workers: [checker]
    depends_on_docker_images:
      - ghcr.io/some-app/cool-app:latest
      - postgres:15-alpine
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя приложения (= имя контейнера) |
| `transport` | Нет | REST/gRPC транспорты (string[] или object[]) |
| `kafka` | Нет | Kafka producers/consumers |
| `worker` | Нет | Фоновые воркеры |
| `driver` | Нет | Драйверы интеграций |
| `cli` | Нет | CLI транспорт (эксклюзивен) |
| `use_active_record` | Нет | Override use_active_record для приложения |
| `use_envs` | Нет | Использовать переменные окружения |
| `deploy` | Нет | Настройки деплоя (volumes) |
| `depends_on_docker_images` | Нет | Docker образы для pre-pull |
| `goat_tests` | Нет | Включить GOAT тесты (bool) |
| `goat_tests_config` | Нет | Расширенная настройка GOAT тестов |
| `grafana` | Нет | Настройки Grafana dashboard |
| `artifacts` | Нет | Per-app override artifacts (docker, deb, rpm, apk) |

### Формат транспортов

Транспорты можно указывать двумя способами:

**Простой формат (string[]):**

```yaml
applications:
  - name: api
    transport: [api, sys]  # Список имён транспортов
```

**Расширенный формат (object[]):**

```yaml
applications:
  - name: api
    transport:
      - name: api
      - name: external_api
        config:
          instantiation: dynamic  # Per-app override
```

### Концепция Application

**Application** — атомарная единица горизонтального масштабирования (контейнер).

- Один бинарь/контейнер может включать несколько компонентов
- HTTP серверы, gRPC серверы, воркеры инициализируются параллельно
- Каждый application масштабируется независимо

### Примеры конфигураций

#### API-only

```yaml
applications:
  - name: api
    transport: [api, sys]
```

#### Workers-only

```yaml
applications:
  - name: workers
    workers: [telegram_bot, kafka_consumer]
    transport: [sys]
```

#### Monolith

```yaml
applications:
  - name: monolith
    transport: [api, admin, sys]
    workers: [telegram_bot]
    kafka: [events]
```

### Зависимости от Docker образов

`depends_on_docker_images` создаёт сервисы для предварительного pull образов:

```yaml
applications:
  - name: checker
    transport: [sys]
    depends_on_docker_images:
      - ghcr.io/some-app/cool-app:latest
```

Генерируемый docker-compose:

```yaml
cool-app-image-puller:
  image: ghcr.io/some-app/cool-app:latest
  pull_policy: always
  restart: "no"

checker:
  depends_on:
    cool-app-image-puller:
      condition: service_completed_successfully
```

### GOAT тесты

Включение интеграционных тестов:

```yaml
applications:
  - name: api
    goat_tests: true
    transport: [api, sys]

# Или расширенная конфигурация
applications:
  - name: api
    goat_tests_config:
      enabled: true
      binary_path: /tmp/my-custom-path
```

### Grafana dashboard

```yaml
applications:
  - name: api
    transport: [api, sys]
    grafana:
      datasources:
        - Prometheus
        - Loki
```

## Секция `drivers`

Конфигурация драйверов интеграций.

```yaml
drivers:
  - name: telegram_bot
    import: pkg/drivers/telegram
    package: telegram
    obj_name: TelegramDriver
    service_injection: "optional custom code"

  - name: payment_gateway
    type: http
    config:
      base_url: https://api.stripe.com
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя драйвера |
| `import` | Да | Import path |
| `package` | Да | Имя пакета |
| `obj_name` | Да | Имя объекта драйвера |
| `service_injection` | Нет | Кастомный код инъекции |

### Интерфейс Runnable

Все драйверы реализуют интерфейс:

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
| **Активный** | Имеет фоновый процесс | Telegram (webhooks/websockets) |
| **Пассивный** | Run — no-op | S3, HTTP клиенты |

### Жизненный цикл драйвера

```
Init() → Run() (в горутине) → GracefulShutdown() → Shutdown()
```

1. `Init()` — инициализация соединений, валидация конфига
2. `Run()` — запуск фонового процесса (для активных драйверов)
3. `GracefulShutdown()` — мягкое завершение
4. `Shutdown()` — принудительное завершение

### Создание кастомного драйвера

1. Создайте структуру драйвера:

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

2. Добавьте в конфигурацию:

```yaml
drivers:
  - name: payment_gateway
    import: pkg/drivers/payment
    package: payment
    obj_name: PaymentDriver
```

3. Подключите к приложению:

```yaml
applications:
  - name: api
    transport: [api, sys]
    driver: [payment_gateway]
```

### Замена провайдера

Для замены провайдера без изменения бизнес-логики:

```yaml
# Переход с AWS S3 на DigitalOcean Spaces
drivers:
  - name: storage
    type: s3
    provider: digitalocean  # Просто измените эту строку
```
