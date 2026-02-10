# Workers

Описание секции `workers` для фоновых воркеров.

## Секция `workers`

Конфигурация фоновых воркеров.

```yaml
workers:
  - name: notification_bot
    generator_type: telegram

  - name: background_processor
    generator_type: daemon
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя воркера |
| `generator_type` | Да | Тип генератора: `telegram`, `daemon` или `queue` |
| `generator_template` | Нет | Имя кастомного шаблона |

## Типы воркеров

### Telegram Bot

Telegram бот с поддержкой webhooks и polling.

```yaml
workers:
  - name: notification_bot
    generator_type: telegram
```

#### Генерируемые обработчики

| Событие | Handler | Назначение |
|---------|---------|------------|
| PreCheckoutQuery | `PreCheckout` | Валидация платежа |
| SuccessfulPayment | `Purchase` | Обработка платежа |
| CallbackQuery | `CallbackQuery` | Callback от кнопок |
| Message | `TextMessage` | Обработка текста/команд |

#### Генерируемая структура

```
internal/app/{app}/worker/telegram/{worker_name}/
├── router.go         # Маршрутизация событий
├── handler/
│   └── handler.go    # Обработчики (ваш код)
└── commands/
    └── commands.go   # Команды бота
```

### Queue

Обработчик очередей с гарантированной доставкой заданий.

```yaml
workers:
  - name: order_processor
    generator_type: template
    generator_template: queue
    path:
      - ./contracts/queues.yaml
```

Очереди описываются в отдельном YAML-файле (контракт), аналогично OpenAPI спецификациям для REST. Генератор создаёт типизированные структуры, сериализаторы и интерфейсы handler-ов для каждой очереди.

Подробное описание: [Queue Contract](queue-contract.md) | [Архитектура системы очередей](../architecture/queue-system.md)

#### Генерируемая структура

```
internal/app/{app}/worker/queue/{worker_name}/
├── types.go          # Go struct для каждой очереди
├── serializer.go     # iproto сериализация/десериализация
├── handler.go        # Интерфейсы handler-ов
├── dispatcher.go     # Маппинг queue_id → handler
└── queue_worker.go   # Worker (реализация Runnable)
```

### Daemon

Фоновый демон для периодических задач.

```yaml
workers:
  - name: background_processor
    generator_type: daemon
```

#### Генерируемая структура

```
internal/app/{app}/worker/daemon/{worker_name}/
├── worker.go         # Основной воркер
└── handler/
    └── handler.go    # Логика обработки (ваш код)
```

## Подключение воркеров к приложению

Воркеры подключаются через секцию `applications`:

```yaml
workers:
  - name: notification_bot
    generator_type: telegram

  - name: background_processor
    generator_type: daemon

applications:
  # Выделенный воркер instance
  - name: worker
    workers: [notification_bot, background_processor]
    transport: [sys]  # Системные endpoints для метрик

  # Или комбинированное приложение
  - name: monolith
    transport: [api, sys]
    workers: [notification_bot]
```

## Жизненный цикл воркера

```
┌────────────────────────────────────────────┐
│                ИНИЦИАЛИЗАЦИЯ                │
│  1. Создание воркера                        │
│  2. Инициализация зависимостей              │
└────────────────────────────────────────────┘
                      │
                      ▼
┌────────────────────────────────────────────┐
│                   ЗАПУСК                    │
│  1. Запуск в отдельной горутине            │
│  2. Telegram: подключение к API            │
│  3. Daemon: запуск цикла обработки         │
└────────────────────────────────────────────┘
                      │
                      ▼
┌────────────────────────────────────────────┐
│                  РАБОТА                     │
│  - Telegram: обработка событий от бота     │
│  - Daemon: выполнение периодических задач  │
└────────────────────────────────────────────┘
                      │
                      ▼
┌────────────────────────────────────────────┐
│                ЗАВЕРШЕНИЕ                   │
│  1. Graceful shutdown                       │
│  2. Завершение текущих операций            │
│  3. Освобождение ресурсов                  │
└────────────────────────────────────────────┘
```

## Конфигурация через OnlineConf

### Telegram Bot

| Путь | Описание |
|------|----------|
| `{service}/worker/telegram/{name}/token` | Bot token |
| `{service}/worker/telegram/{name}/webhook_url` | Webhook URL (опционально) |

### Daemon

| Путь | Описание |
|------|----------|
| `{service}/worker/daemon/{name}/interval` | Интервал выполнения |
| `{service}/worker/daemon/{name}/enabled` | Включен/выключен |

## Пример полной конфигурации

```yaml
main:
  name: notification-service
  logger: zerolog

git:
  repo: github.com/myorg/notification-service
  module_path: github.com/myorg/notification-service

rest:
  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

workers:
  - name: telegram_bot
    generator_type: telegram

  - name: email_sender
    generator_type: daemon

drivers:
  - name: telegram
    import: pkg/drivers/telegram
    package: telegram
    obj_name: TelegramDriver

applications:
  - name: notifier
    workers: [telegram_bot, email_sender]
    transport: [sys]
    driver: [telegram]
```
