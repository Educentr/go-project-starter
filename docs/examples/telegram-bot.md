# Telegram бот

Пример Telegram бота.

## Простой Telegram бот

```yaml
main:
  name: notification-bot
  logger: zerolog

git:
  repo: github.com/myorg/notification-bot
  module_path: github.com/myorg/notification-bot

rest:
  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

workers:
  - name: telegram_bot
    generator_type: telegram

applications:
  - name: bot
    workers: [telegram_bot]
    transport: [sys]
```

## Telegram бот с базой данных

```yaml
main:
  name: support-bot
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/myorg/support-bot
  module_path: github.com/myorg/support-bot

rest:
  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

workers:
  - name: support_bot
    generator_type: telegram

drivers:
  - name: telegram
    import: pkg/drivers/telegram
    package: telegram
    obj_name: TelegramDriver

applications:
  - name: bot
    workers: [support_bot]
    transport: [sys]
    driver: [telegram]
```

## Telegram бот с REST API

Бот с API для управления:

```yaml
main:
  name: admin-bot
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/myorg/admin-bot
  module_path: github.com/myorg/admin-bot

rest:
  # API для управления ботом
  - name: api
    path: [./api/admin.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

workers:
  - name: admin_bot
    generator_type: telegram

applications:
  # Всё в одном приложении
  - name: bot
    transport: [api, sys]
    workers: [admin_bot]
```

## Генерируемые обработчики

| Событие | Handler | Назначение |
|---------|---------|------------|
| PreCheckoutQuery | `PreCheckout` | Валидация платежа |
| SuccessfulPayment | `Purchase` | Обработка платежа |
| CallbackQuery | `CallbackQuery` | Callback от inline кнопок |
| Message | `TextMessage` | Обработка текста и команд |

## Генерируемая структура

```
notification-bot/
├── cmd/bot/
│   └── main.go
├── internal/
│   ├── app/bot/
│   │   └── worker/telegram/telegram_bot/
│   │       ├── router.go
│   │       ├── handler/
│   │       │   └── handler.go        # Ваш код здесь
│   │       └── commands/
│   │           └── commands.go
│   └── pkg/
│       └── service/
├── pkg/
│   └── drivers/telegram/
├── docker-compose.yaml
└── Makefile
```

## Конфигурация через OnlineConf

| Путь | Описание |
|------|----------|
| `{service}/worker/telegram/{name}/token` | Bot token |
| `{service}/worker/telegram/{name}/webhook_url` | Webhook URL (опционально) |

## Пример handler.go

```go
package handler

import (
    "context"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ==========================================
// GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
// ==========================================

func (h *Handler) TextMessage(ctx context.Context, msg *tgbotapi.Message) error {
    // Обработка текстовых сообщений

    switch msg.Text {
    case "/start":
        return h.handleStart(ctx, msg)
    case "/help":
        return h.handleHelp(ctx, msg)
    default:
        return h.handleUnknown(ctx, msg)
    }
}

func (h *Handler) CallbackQuery(ctx context.Context, query *tgbotapi.CallbackQuery) error {
    // Обработка callback от inline кнопок
    data := query.Data

    switch data {
    case "confirm":
        return h.handleConfirm(ctx, query)
    case "cancel":
        return h.handleCancel(ctx, query)
    }

    return nil
}

func (h *Handler) handleStart(ctx context.Context, msg *tgbotapi.Message) error {
    reply := tgbotapi.NewMessage(msg.Chat.ID, "Добро пожаловать!")
    _, err := h.bot.Send(reply)
    return err
}

func (h *Handler) handleHelp(ctx context.Context, msg *tgbotapi.Message) error {
    text := `Доступные команды:
/start - Начать
/help - Справка`

    reply := tgbotapi.NewMessage(msg.Chat.ID, text)
    _, err := h.bot.Send(reply)
    return err
}
```

## Запуск

```bash
# Генерация проекта
go-project-starter --config=config.yaml

# Настройка токена (через OnlineConf или env)
export OC_notification_bot__worker__telegram__telegram_bot__token=YOUR_BOT_TOKEN

# Запуск
cd notification-bot
make docker-up
make run

# Или через dev-stand с OnlineConf UI
make dev-up
# Настройте токен в http://localhost:8888
```
