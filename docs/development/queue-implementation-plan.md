# Queue Processing System — Implementation Plan

## Context

Система очередей для генерируемых Go-микросервисов. Worker (горутина) управляет job_processor-ами (горутинами), каждый из которых берёт задания из назначенной очереди, десериализует и передаёт типизированному handler-у. Без менеджера worker-ы автономно перебирают все очереди round-robin. Описание очередей — в отдельном YAML-файле (контракт), по аналогии с OpenAPI спеками.

Связанная документация:

- [Архитектура системы очередей](../architecture/queue-system.md) — детальное описание поведения
- [Queue Contract](../configuration/queue-contract.md) — формат конфигурации и генерируемый код

---

## Принятые решения

| Вопрос | Решение |
|--------|---------|
| Тип интеграции | Новый `generator_template: queue` (рядом с daemon, telegram) |
| Storage interface | В `go-project-starter-runtime` (`pkg/queue/`) |
| In-memory реализация | В `go-project-starter-runtime` (`pkg/queue/`) |
| ID очереди | `int` (числовой) |
| Лимит JobProcessor | Конфигурируемый через OnlineConf, default = 1 |
| Task payload | `[]byte`, сериализация через iproto (Tarantool msgpack) |
| Worker | Горутина в приложении (как daemon worker) |
| Таймаут цикла | 10 сек best practice, мониторинг при превышении |
| Round-robin | По `batch_size` из каждой очереди |
| Handler | Интерфейс `Handle{Name}(ctx, Storage, []*TypedTask) (HandlerStats, error)` |
| Десериализация | job_processor обёртка (handler получает типизированные данные) |
| Storage в handler | Напрямую (для DoneTask, RetryTask, PutTask) |
| Типы полей | `int`, `int64`, `string`, `bool`, `[]byte`, `[]int`, `[]int64` |
| Visibility timeout | Глобальный (default 60s) |
| Retry лимит | Без лимита (dead letter на стороне handler через Attempts) |
| Attempts | Инкрементируется Storage при возврате (RetryTask или visibility timeout) |
| PrevStartTime | Хранится в Task, обновляется при каждом возврате в очередь |
| Очереди | Из YAML контракта (фиксированы при генерации) |
| Статистика | Расширенная (per-queue: total, ready, taken, delayed) |

---

## Фазы (независимые релизы)

### Фаза 1: Storage Interface + In-Memory реализация

**Репо:** `go-project-starter-runtime`
**Зависимости:** нет

#### Интерфейс и типы

```go
// pkg/queue/storage.go
package queue

type Task struct {
    ID            int64
    QueueNum      int
    Data          []byte
    Attempts      int       // инкрементируется Storage при возврате в очередь
    CreatedAt     time.Time
    StartTime     time.Time // текущая активация
    PrevStartTime time.Time // предыдущая активация (zero при Attempts=0)
}

type RetryInfo struct {
    TaskID        int64
    NextStartTime time.Time
}

type PutTaskRequest struct {
    QueueNum  int
    Data      []byte
    StartTime *time.Time    // абсолютное (взаимоисключающее с Delay)
    Delay     time.Duration // относительная задержка
}

type QueueStat struct {
    QueueNum     int
    TotalTasks   int64
    ReadyTasks   int64
    TakenTasks   int64
    DelayedTasks int64
}

type Stats struct {
    QueueStats map[int]QueueStat
}

type HandlerStats struct {
    Processed int
    Errors    int
    Retried   int
    Duration  time.Duration
    LastError error
}

type Storage interface {
    GetTask(ctx context.Context, queueNum int, count int) ([]Task, error)
    DoneTask(ctx context.Context, taskIDs []int64) error
    RetryTask(ctx context.Context, retries []RetryInfo) error
    PutTask(ctx context.Context, req PutTaskRequest) error
    GetStat(ctx context.Context) (*Stats, error)
}
```

#### In-Memory реализация

- `sync.Mutex` для потокобезопасности
- Глобальный visibility timeout (конфигурируемый, default 60s)
- GetTask атомарно помечает задания как "taken"
- Фоновая горутина возвращает просроченные задания
- При возврате (RetryTask / visibility timeout): Attempts+1, PrevStartTime = старый StartTime
- Без лимита retry

#### Файлы

| Файл | Описание |
|------|----------|
| `pkg/queue/storage.go` | Интерфейс, типы |
| `pkg/queue/memory.go` | In-memory реализация |
| `pkg/queue/memory_test.go` | Unit-тесты |

#### Тестирование

- Атомарность GetTask (одно задание не выдаётся двум)
- Visibility timeout (задание возвращается, Attempts+1, PrevStartTime обновлён)
- RetryTask с NextStartTime (задание недоступно до NextStartTime)
- Delayed tasks (PutTask с StartTime/Delay)
- Конкурентный доступ из горутин
- GetStat корректность подсчётов

---

### Фаза 2: Worker + JobProcessor core

**Репо:** `go-project-starter-runtime`
**Зависимости:** Фаза 1

#### Компоненты

```go
// HandlerFunc — raw handler (генерируемый код предоставит типизированную обёртку)
type HandlerFunc func(ctx context.Context, storage Storage, queueNum int, tasks []Task) (HandlerStats, error)
```

- **JobProcessor** — горутина: получает очередь от worker, вызывает GetTask → handler → возвращает stats. Работает непрерывно, при остановке дорабатывает текущий handler call
- **QueueWorker** — управляет пулом JobProcessor-ов. Standalone: round-robin по очередям. НЕ реализует ds.Runnable (это делает шаблон)
- **QueueMetrics** — Prometheus метрики

#### Файлы

| Файл | Описание |
|------|----------|
| `pkg/queue/job_processor.go` | JobProcessor |
| `pkg/queue/worker.go` | QueueWorker |
| `pkg/queue/metrics.go` | Prometheus метрики |
| `pkg/queue/*_test.go` | Тесты |

#### Тестирование

- JobProcessor берёт задачи и вызывает handler
- Round-robin по очередям (batch_size из каждой)
- GracefulStop ждёт завершения текущего handler
- Динамическое изменение количества processor-ов
- Смена очереди у processor-а
- Метрики записываются корректно

---

### Фаза 3: Парсинг контрактов + кодогенерация

**Репо:** `go-project-starter`
**Зависимости:** нет (генерирует код, который импортирует runtime из Фазы 1)

#### 3a. YAML контракт очередей

```yaml
queues:
  - id: 1
    name: emails
    fields:
      - name: to
        type: string
      - name: user_id
        type: int64
```

Ссылка из `project.yaml`:

```yaml
worker:
  - name: task_processor
    generator_type: template
    generator_template: queue
    path:
      - ./queues.yaml
```

#### 3b. Config parsing

- `internal/pkg/config/structs.go` — добавить `QueueField`, `QueueDef`
- `Worker.IsValid()` — валидация для `generator_template: "queue"` (обязательный path)
- `internal/pkg/config/queue.go` — **новый**: загрузка и валидация YAML контракта

#### 3c. DS типы

Расширить `internal/pkg/ds/const.go`:

```go
type QueueField struct {
    Name   string
    GoName string // PascalCase
    Type   string // Go тип
}

type QueueDef struct {
    ID     int
    Name   string
    GoName string // PascalCase
    Fields []QueueField
}

type QueueConfig struct {
    Queues []QueueDef
}
```

Добавить `QueueConfig *QueueConfig` в `ds.Worker` (nil для non-queue workers).

#### 3d. Шаблоны кодогенерации

Директория: `templater/embedded/templates/worker/template/queue/files/`

| Шаблон | Генерирует |
|--------|-----------|
| `types.go.tmpl` | Go структуры `EmailsTask{TaskID, Attempts, PrevStartTime, To, UserID}` |
| `serializer.go.tmpl` | iproto сериализатор/десериализатор |
| `handler.go.tmpl` | Интерфейсы handler-ов |
| `dispatcher.go.tmpl` | `QueueHandlers` struct + `NewDispatcher()` |
| `queue_worker.go.tmpl` | Worker, реализующий ds.Runnable |

#### 3e. Генерируемая типизированная структура

```go
type EmailsTask struct {
    TaskID        int64     // служебное: ID задания
    Attempts      int       // служебное: счётчик возвратов в очередь
    PrevStartTime time.Time // служебное: время предыдущей активации
    To            string    // из контракта
    UserID        int64     // из контракта
}
```

#### Модифицируемые файлы

| Файл | Изменения |
|------|----------|
| `internal/pkg/config/structs.go` | +QueueField, +QueueDef, обновить Worker.IsValid() |
| `internal/pkg/config/queue.go` | **новый** — загрузка YAML контракта |
| `internal/pkg/ds/const.go` | +QueueField, +QueueDef, +QueueConfig, расширить Worker |
| `internal/pkg/templater/templater.go` | Обновить MinRuntimeVersion |
| `internal/pkg/generator/generator.go` | Загрузка контракта, передача QueueConfig в Worker |
| `templater/embedded/templates/worker/template/queue/files/*.tmpl` | **новые** — 5 шаблонов |

#### Тестирование

- Unit-тесты парсера YAML контрактов
- Тест генерации проекта с queue worker
- Тестовый конфиг `test/configs/queue/` или `test/docker-integration/configs/worker-queue/`
- Проверка компиляции сгенерированного кода

---

### Фаза 4: Метрики

**Зависимости:** Фаза 2 + Фаза 3 (может быть включена в них)

Prometheus метрики:

- `queue_job_processor_cycle_duration_seconds` — histogram, label: `queue_num`
- `queue_tasks_in_progress` — gauge, label: `queue_num`
- `queue_running_workers` — gauge
- `queue_running_job_processors` — gauge

---

### Фаза 5: Manager

**Зависимости:** Фаза 2 + Фаза 3
**Статус:** будущая работа, правила распределения TBD

- Получает статистику от Storage (GetStat) и от workers (HandlerStats)
- Распределяет job_processor-ы по очередям
- Без инструкций — workers продолжают автономный round-robin
- Коммуникация с workers через channel-ы или command interface

---

## Порядок разработки

```text
Фаза 1 (runtime: Storage) ──┐
                              ├── Фаза 2 (runtime: Worker/JobProcessor) ──┐
Фаза 3 (generator: всё)  ───┘                                            ├── Фаза 4 (метрики)
                                                                          ├── Фаза 5 (manager)
```

- Фазы 1 и 3 можно разрабатывать параллельно
- Фаза 2 зависит от Фазы 1
- При релизе Фазы 3 нужен bump MinRuntimeVersion до версии с Фазами 1+2

## Верификация

1. `make test` — unit тесты генератора
2. `make lint` — линтер
3. Ручная проверка:

    ```bash
    go install ./cmd/go-project-starter && \
      rm -rf ~/Develop/tmp/test-queue && \
      mkdir ~/Develop/tmp/test-queue && \
      go-project-starter --config=./test/configs/queue/project.yaml --target=~/Develop/tmp/test-queue
    cd ~/Develop/tmp/test-queue && go build ./...
    ```

4. Docker integration test (после всех фаз):

    ```bash
    make buildfortest
    TEST_IMAGE=go-project-starter-test:latest go test -v -count=1 -run TestIntegrationQueue ./test/docker-integration/...
    ```
