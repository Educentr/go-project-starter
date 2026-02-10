# Queue Contract

Описание формата файла контракта очередей и конфигурации queue worker в project.yaml.

## Конфигурация в project.yaml

Queue worker объявляется в секции `workers` с типом `queue`:

```yaml
workers:
  - name: task_processor
    generator_type: template
    generator_template: queue
    path:
      - ./contracts/queues.yaml   # путь к файлу контракта очередей
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Уникальное имя worker-а |
| `generator_type` | Да | Должно быть `template` |
| `generator_template` | Да | Должно быть `queue` |
| `path` | Да | Путь к файлу контракта очередей (один файл) |

### Подключение к приложению

```yaml
workers:
  - name: task_processor
    generator_type: template
    generator_template: queue
    path:
      - ./contracts/queues.yaml

applications:
  - name: worker
    workers: [task_processor]
    transport: [sys]   # системные endpoints для метрик
```

## Файл контракта очередей

Файл контракта описывает все очереди, которые обрабатывает worker. Хранится в директории контрактов проекта (аналогично OpenAPI спецификациям).

### Формат

```yaml
queues:
  - id: 1
    name: emails
    fields:
      - name: to
        type: string
      - name: subject
        type: string
      - name: body
        type: "[]byte"
      - name: user_id
        type: int64

  - id: 2
    name: notifications
    fields:
      - name: message
        type: string
      - name: target_ids
        type: "[]int64"
      - name: is_urgent
        type: bool

  - id: 3
    name: image_processing
    fields:
      - name: image_data
        type: "[]byte"
      - name: width
        type: int
      - name: height
        type: int
      - name: quality
        type: int
```

### Поля очереди

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `id` | Да | Числовой идентификатор очереди (int, уникальный) |
| `name` | Да | Имя очереди (уникальное, snake_case) |
| `fields` | Да | Список полей данных задания |

### Поля данных

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя поля (snake_case) |
| `type` | Да | Тип данных (см. таблицу ниже) |

### Поддерживаемые типы

| Тип в YAML | Тип в Go | Описание |
|------------|----------|----------|
| `int` | `int` | Целое число |
| `int64` | `int64` | 64-битное целое число |
| `string` | `string` | Строка |
| `bool` | `bool` | Булево значение |
| `[]byte` | `[]byte` | Массив байт |
| `[]int` | `[]int` | Слайс целых чисел |
| `[]int64` | `[]int64` | Слайс 64-битных целых |

### Правила валидации

1. **ID очередей уникальны** — два ID не могут совпадать
2. **Имена очередей уникальны** — два имени не могут совпадать
3. **Хотя бы одна очередь** — файл должен содержать минимум одну очередь
4. **Хотя бы одно поле** — каждая очередь должна иметь минимум одно поле
5. **Допустимые типы** — тип поля должен быть из таблицы выше
6. **Имена полей уникальны** внутри одной очереди

## Генерируемый код

Из контракта генерируется код в `internal/app/{app}/worker/queue/{worker_name}/`:

### Структура файлов

```
internal/app/{app}/worker/queue/{worker_name}/
├── types.go          # Go struct для каждой очереди
├── serializer.go     # iproto сериализация/десериализация
├── handler.go        # Интерфейсы handler-ов
├── dispatcher.go     # Маппинг queue_id → handler
└── queue_worker.go   # Worker (реализация Runnable)
```

### types.go — структуры данных

Для каждой очереди генерируется Go struct с полями из контракта + поле `TaskID`:

```go
// Из очереди "emails" (id: 1)
type EmailsTask struct {
    TaskID        int64     // ID задания в хранилище
    Attempts      int       // Сколько раз задание возвращалось в очередь (0 = первая попытка)
    PrevStartTime time.Time // Время предыдущей активации (zero value при Attempts=0)
    To            string
    Subject       string
    Body          []byte
    UserID        int64
}

// Из очереди "notifications" (id: 2)
type NotificationsTask struct {
    TaskID        int64
    Attempts      int
    PrevStartTime time.Time
    Message       string
    TargetIDs     []int64
    IsUrgent      bool
}
```

Правила именования:

- Имя struct: `{QueueName}Task` (PascalCase от `name`)
- Имена полей: PascalCase от `name` поля
- Служебные поля (всегда первые, не из контракта):
    - `TaskID int64` — ID задания в хранилище
    - `Attempts int` — сколько раз задание возвращалось в очередь (0 = первая попытка)
    - `PrevStartTime time.Time` — время предыдущей активации (zero value при первой попытке)

### serializer.go — сериализация

Для каждой очереди генерируется пара функций сериализации через iproto (msgpack):

```go
func SerializeEmailsTask(task *EmailsTask) ([]byte, error) { ... }
func DeserializeEmailsTask(data []byte) (*EmailsTask, error) { ... }

func SerializeNotificationsTask(task *NotificationsTask) ([]byte, error) { ... }
func DeserializeNotificationsTask(data []byte) (*NotificationsTask, error) { ... }
```

### handler.go — интерфейсы обработчиков

Для каждой очереди генерируется интерфейс handler-а:

```go
// EmailsHandler обрабатывает задания из очереди emails (id: 1)
type EmailsHandler interface {
    HandleEmails(ctx context.Context, storage queue.Storage, tasks []*EmailsTask) (queue.HandlerStats, error)
}

// NotificationsHandler обрабатывает задания из очереди notifications (id: 2)
type NotificationsHandler interface {
    HandleNotifications(ctx context.Context, storage queue.Storage, tasks []*NotificationsTask) (queue.HandlerStats, error)
}
```

Сигнатура handler-а:
- **ctx** — контекст выполнения
- **storage** — прямой доступ к Storage (для DoneTask, RetryTask, PutTask)
- **tasks** — типизированные задания, уже десериализованные из `[]byte`
- **return** — статистика обработки и ошибка

### dispatcher.go — диспетчер

Диспетчер связывает номер очереди с типизированным handler-ом:

```go
// QueueHandlers содержит handler-ы для всех очередей
type QueueHandlers struct {
    Emails        EmailsHandler
    Notifications NotificationsHandler
}

// NewDispatcher создаёт функцию диспетчеризации для QueueWorker
func NewDispatcher(h QueueHandlers) queue.HandlerFunc {
    return func(ctx context.Context, s queue.Storage, queueNum int, tasks []queue.Task) (queue.HandlerStats, error) {
        switch queueNum {
        case 1:
            typed, err := deserializeEmailsBatch(tasks)
            if err != nil {
                return queue.HandlerStats{}, err
            }
            return h.Emails.HandleEmails(ctx, s, typed)
        case 2:
            typed, err := deserializeNotificationsBatch(tasks)
            if err != nil {
                return queue.HandlerStats{}, err
            }
            return h.Notifications.HandleNotifications(ctx, s, typed)
        default:
            return queue.HandlerStats{}, fmt.Errorf("unknown queue: %d", queueNum)
        }
    }
}
```

### queue_worker.go — worker

Сгенерированный worker, реализующий `Runnable` интерфейс:

```go
type Worker struct {
    daemon.EmptyWorker
    Srv         ds.IService
    queueWorker *queue.QueueWorker
}

const WorkerName = "task_processor"

func Create() *Worker { return &Worker{} }

func (w *Worker) Name() string { return WorkerName }

func (w *Worker) Init(ctx context.Context, serviceName, appName string,
    metrics *prometheus.Registry, srv ds.IService) error {
    w.Srv = srv

    storage := queue.NewMemoryStorage(
        queue.WithVisibilityTimeout(60 * time.Second),
    )

    handlers := QueueHandlers{
        // *** ваш код ниже disclaimer marker ***
    }

    w.queueWorker = queue.NewQueueWorker(
        storage,
        []int{1, 2},  // ID очередей из контракта
        NewDispatcher(handlers),
        queue.WithMetrics(metrics),
    )

    return nil
}

func (w *Worker) Run(ctx context.Context, errGr *errgroup.Group) {
    w.queueWorker.Run(ctx, errGr)
}

func (w *Worker) Shutdown(ctx context.Context) error {
    return w.queueWorker.Shutdown(ctx)
}

func (w *Worker) GracefulStop(ctx context.Context) (<-chan struct{}, error) {
    return w.queueWorker.GracefulStop(ctx)
}
```

## Конфигурация через OnlineConf

Параметры обработки очередей настраиваются через OnlineConf (не определяют сами очереди):

| Путь | Описание | Default |
|------|----------|---------|
| `{service}/worker/queue/{name}/max_processors` | Максимум JobProcessor-ов | 1 |
| `{service}/worker/queue/{name}/visibility_timeout` | Visibility timeout (сек) | 60 |
| `{service}/worker/queue/{name}/queue/{id}/batch_size` | Размер batch для очереди | 10 |

## Полный пример конфигурации

### project.yaml

```yaml
main:
  name: order-service
  logger: zerolog

git:
  repo: github.com/myorg/order-service
  module_path: github.com/myorg/order-service

rest:
  - name: api
    path: [./contracts/api.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

workers:
  - name: order_processor
    generator_type: template
    generator_template: queue
    path:
      - ./contracts/queues.yaml

applications:
  - name: api
    transport: [api, sys]

  - name: worker
    workers: [order_processor]
    transport: [sys]
```

### contracts/queues.yaml

```yaml
queues:
  - id: 1
    name: order_confirmation
    fields:
      - name: order_id
        type: int64
      - name: user_id
        type: int64
      - name: email
        type: string

  - id: 2
    name: order_export
    fields:
      - name: order_ids
        type: "[]int64"
      - name: format
        type: string

  - id: 3
    name: cleanup
    fields:
      - name: older_than_days
        type: int
```
