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
| `generator_type` | Да | Тип генератора: `telegram` или `daemon` |
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

#### Точки расширения: `customize()`

Сгенерированный воркер правится только ниже маркера «add your code after this
message», но кое-что проекту нужно поменять внутри сгенерированного `Run()`, роутера
и реестра команд. Всё это делается из одного места — метода `customize()`, который
`Create()` зовёт на свежесконструированном воркере, до `Init` и `Run`.

По умолчанию `customize()` промотирован из встроенного `EmptyCustomizer` и ничего не
делает. Проект объявляет свой — метод на `*Worker` перекрывает промотированный:

```go
// internal/app/worker/telegrambot/dispatch.go — ваш файл в пакете воркера
func (w *Worker) customize() {
	// Свои команды — в реестр воркера
	w.AddCommands(
		telegram.Cmd{
			Name:    CmdHelp,
			Private: true,
			Handler: func(ctx context.Context, h any, params string) (telegram.Response, error) {
				return h.(*handler.Handler).Help(ctx, params)
			},
		},
	)

	// Хуки воркера — здесь же
	w.SetUpdateHandler(w.dispatchToQueues, w.queuesDrained)
	w.SetUnknownCommandHandler(func(ctx context.Context, w *Worker, rd telegram.RequestData) {
		w.clearStaleDialogState(ctx, rd.Actor.ID)
	})

	// Ответы на свои ошибки авторизации
	w.AddErrorHandler(ErrSubscriptionExpired, func(ctx context.Context) (telegram.Response, error) {
		return renewSubscriptionPrompt(), nil
	})
}
```

Файл может быть и своим файлом пакета, и секцией ниже disclaimer-маркера в
`psg_telegram_gen.go` — `regenerate` эту правку не трогает.

**Команды.** Реестр команд — поле воркера (`w.cmds`), а не package-global
`cmdMaps`, наполняемый из `init()`: порядок инициализации пакетов на него не влияет,
и два воркера в одном процессе не делят один набор команд. Команды генератора
(`generatedCommands()` в `psg_const_gen.go`) `Create()` добавляет первыми, затем
зовёт `customize()` — команда проекта с тем же `Name` перекрывает сгенерированную.

**Хуки воркера.** Тоже поля, ставятся сеттером из `customize()`. Прежде это были
package-level переменные, которым проект присваивал новое значение; у полей два
отличия, которые видно в работе: два воркера в одном процессе больше не делят один
набор хуков, и подмена читается как вызов, а не как запись в переменную чужого
пакета. В сгенерированных проектах такая переменная ещё и запрещена линтером
(`gochecknoglobals`, как `init()` — `gochecknoinits`).

| Сеттер | По умолчанию | Зачем ставить свой |
|--------|--------------|--------------------|
| `w.SetUpdateHandler(h, idle)` | `handleUpdate`: лог апдейта, `JobStarter`, разбор его ошибок — апдейты строго по одному | разложить апдейты по очередям (например, по автору), чтобы одна долгая команда не держала всех пользователей |
| `w.SetUnknownCommandHandler(h)` | неизвестная команда только логируется | сбросить состояние диалога пользователя, чтобы старая кнопка не оставила его на полушаге |
| `w.AddErrorHandler(target, h)` | ошибка авторизации разбирается только по `telegram.Err*` | свой ответ на свою ошибку `Authorization` (истёкшая подписка, бан) |

`idle` у `SetUpdateHandler` — вторая половина той же точки расширения, и параметр у
неё там же не случайно: `GracefulStop` ждёт не только пустого канала апдейтов, но и
того, что обработчик закончил начатое. Для синхронного обработчика пустой канал это
и значит — передавайте `nil`. Для асинхронного (разложил апдейты по очередям) нет, и
воркер остановился бы на полпути; передавайте функцию, которая отвечает, разобраны ли
очереди. Отдельный сеттер позволял заменить обработчик и промолчать про `idle` —
здесь про неё приходится сказать хотя бы `nil`.

Обработчик по умолчанию пишет в лог апдейт целиком; если в сообщениях бота бывают
секреты (токены, пароли), свой обработчик должен логировать только `updateID`.
`JobStarter` для неподдержанных типов апдейта пишет в лог только `updateID` и тип.

> Проекты, зарегистрированные до появления `customize()`, наполняли `cmdMaps` из
> своего `init()` и присваивали значения хукам пакета. Миграция — в
> [DEPRECATION.md](https://github.com/Educentr/go-project-starter/blob/main/DEPRECATION.md).

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

#### Таймеры цикла

Паузы цикла переопределяются через ENV; имена переменных включают имя воркера:

| Переменная | По умолчанию | Когда применяется |
|------------|--------------|-------------------|
| `WORKER_<NAME>_ERROR_TIMEOUT` | `10s` (`DefaultErrorTimeout`) | источник задач вернул ошибку |
| `WORKER_<NAME>_NEW_CYCLE_TIMEOUT` | `10s` (`DefaultNewCycleTimeout`) | источник задач вернул `nil` — начался новый цикл |

ENV читается в `Create()`, в поля воркера, а не в инициализаторах package-level
переменных: прежняя форма делала чтение окружения side-effect'ом импорта пакета, то
есть моментом, которого в `main` не видно, и значение было общим на все экземпляры
воркера в процессе. Непригодный разбор (пустая или неразбираемая строка) и ноль
дают дефолт: ноль здесь значит «не задано», а не «не ждать» — иначе цикл крутился
бы на полной скорости и долбил бы отказывающий источник задач без паузы.

> Заодно эти переменные стали работать из `.env`. Package-level инициализатор
> срабатывал до `main`, то есть до `godotenv.Load(".env")`, и значение из `.env`
> не доезжало — только из окружения процесса. `Create()` зовётся после загрузки.

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

## Конфигурация таймеров daemon worker через переменные окружения

Два таймера внутреннего цикла daemon worker'а можно переопределить через env vars
(полезно для goat-тестов и проектов с `use_envs: true`). Если переменная не задана
или не парсится, используется дефолт.

| Env var | Дефолт | Описание |
|---------|--------|----------|
| `WORKER_{NAME}_NEW_CYCLE_TIMEOUT` | `10s` | Пауза между итерациями цикла, когда `GetTasks` вернул `nil` |
| `WORKER_{NAME}_ERROR_TIMEOUT` | `10s` | Пауза после ошибки в `GetTasks` |

`{NAME}` — имя воркера в верхнем регистре. Значение должно парситься через
`time.ParseDuration` (например, `500ms`, `1m30s`).

Пример: для воркера с именем `supervisor` ускорить goat-тесты можно так:

```go
envVars["WORKER_SUPERVISOR_NEW_CYCLE_TIMEOUT"] = "200ms"
```

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
