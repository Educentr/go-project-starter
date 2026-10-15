# Карта метрик (`metrics.path`)

Карта метрик — **объявление всех метрик, которые сервису разрешено эмитить**. С ней закрываются три вещи сразу: метрики описаны в одном месте, дев-дашборд показывает данные, а «сервис эмитит незаявленную метрику» становится невозможным по построению, а не по договорённости.

Карта не указана — **ничего не включается**: ни сгенерированного пакета, ни валидатора, ни панелей. Поведение проекта остаётся прежним.

## Подключение

Карта — **контрактный артефакт**, поэтому живёт в контрактном репозитории, а `project.yaml` несёт только ссылку на неё. Форма ссылки та же, что у `rest[].path` (см. [Удалённые спеки](remote-specs.md)): локальный путь, `https://`, `git+ssh://`, `git+https://`.

```yaml
# project.yaml
metrics:
  path: "git+https://github.com/org/contracts.git@v1.4.0#common/metrics-map.yaml"
```

Локально — для проб и для тестов:

```yaml
metrics:
  path: ./metrics-map.yaml
```

Карта читается **во время генерации** и в проект не копируется: ничего ниже по цепочке её не читает, артефактом является сгенерированный пакет коллекторов. Второй копии, которая молча расходится с контрактом, не возникает.

## Формат карты

Плоский список. Минимально достаточны четыре поля:

```yaml
# common/metrics-map.yaml
metrics:
  - name: spawn_errors_total
    type: CounterVec
    labels: reason, stage
    help: >-
      Spawn attempts that failed, by reason and stage.
```

### `name` — полное имя метрики

В `name` пишется **то имя, которое увидит Prometheus**, целиком. Разбиения на `namespace` / `subsystem` / `name` (как в `prometheus.CounterOpts`) здесь нет, и это осознанно: имя, собираемое в рантайме, — это имя, которого нет в исходниках, поэтому его не находит ни поиск по коду, ни ревью, ни дашборд, ни алерт. Карта обязана нести имя, по которому метрику реально ищут.

Допустимы символы Prometheus: `[a-zA-Z_:][a-zA-Z0-9_:]*`. Дополнительно требуется, чтобы в имени была хотя бы одна буква или цифра: `_` и `:` сворачиваются при построении Go-идентификатора, и из имени вида `"_"` переменной не получится.

### `type` — конструктор

| `type` | Коллектор | Метки |
|---|---|---|
| `Counter` | `prometheus.Counter` | нет |
| `CounterVec` | `prometheus.CounterVec` | обязательны |
| `Gauge` | `prometheus.Gauge` | нет |
| `GaugeVec` | `prometheus.GaugeVec` | обязательны |
| `Histogram` | `prometheus.Histogram` | нет |
| `HistogramVec` | `prometheus.HistogramVec` | обязательны |
| `Summary` | `prometheus.Summary` | нет |
| `SummaryVec` | `prometheus.SummaryVec` | обязательны |

Метки у не-`Vec` типа и отсутствие меток у `Vec` — ошибка генерации, а не молчаливое исправление.

### `labels` — метки серии

Принимаются обе формы:

```yaml
    labels: reason, stage        # скаляр через запятую
```

```yaml
    labels:                      # YAML-список
      - reason
      - stage
```

Проверяются: грамматика Prometheus (`[a-zA-Z_][a-zA-Z0-9_]*`), отсутствие дублей, отсутствие префикса `__` (зарезервирован самим Prometheus).

### `help` — текст для Prometheus

Обязателен. Это проза, которая уезжает в чужой Prometheus и которую читает человек, не знающий ваш трекер: номерам задач и внутренним сокращениям здесь не место. Карта — заодно и то место, где такой текст **видно** и можно отревьюить; в конструкторе посреди кода его не видит никто.

### `buckets` — только для `Histogram` / `HistogramVec`

```yaml
  - name: task_duration_seconds
    type: HistogramVec
    labels: kind, outcome
    help: Wall-clock duration of a task.
    buckets: [0.05, 0.25, 1, 5, 30]
```

Не указаны — используется `prometheus.DefBuckets`. Указаны у не-гистограммы — ошибка. Значения должны строго возрастать.

### `objectives` — только для `Summary` / `SummaryVec`

Отображение «квантиль → допустимая погрешность», как в `prometheus.SummaryOpts`:

```yaml
  - name: batch_size
    type: Summary
    help: Number of records in a processed batch.
    objectives: {0.5: 0.05, 0.99: 0.001}
```

Не указаны — `nil`, то есть поведение `client_golang` по умолчанию: метрика несёт только `_sum` и `_count`, без квантилей. Выдумывать квантили за вас генератор не станет — это такое же решение, как порог алерта.

### `external` — метрика из сторонней библиотеки

```yaml
  - name: otel_sdk_exporter_spans_total
    type: CounterVec
    labels: outcome
    help: Spans handed to the OTLP exporter, by outcome.
    external: true
```

`external: true` означает «метрику эмитит библиотека, не сервис». Коллектор **не генерируется**, но запись:

- занимает своё имя в общем пространстве имён (то есть сервис не сможет его затенить);
- попадает в дашборд.

Это нужно для метрик OTEL SDK, драйверов БД и всего остального, что инструментирует себя само. Без этого поля такие метрики пришлось бы прятать от карты, и карта перестала бы быть полным списком.

## Сгенерированный пакет коллекторов

Генерируется в `internal/pkg/metrics/psg_metrics_gen.go` — по одной экспортируемой переменной на объявленную метрику:

```go
// SpawnErrorsTotal — Spawn attempts that failed, by reason and stage.
//
// Metric: spawn_errors_total (CounterVec), labels: reason, stage
var SpawnErrorsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "spawn_errors_total",
		Help: "Spawn attempts that failed, by reason and stage.",
	},
	[]string{"reason", "stage"},
)
```

Имя переменной — `name` в PascalCase: `spawn_errors_total` → `SpawnErrorsTotal`. Символ `:` свёртывается так же, как `_`; если два объявленных имени свернутся в один Go-идентификатор, генерация падает, а не создаёт одну переменную на двоих.

Использование в коде сервиса:

```go
import "github.com/org/myservice/internal/pkg/metrics"

metrics.SpawnErrorsTotal.WithLabelValues("timeout", "init").Inc()
metrics.QueueDepth.Set(float64(len(queue)))
```

### Регистрация

Пакет отдаёт три функции:

```go
func Collectors() []prometheus.Collector
func Register(reg prometheus.Registerer) error
func Unregister(reg prometheus.Registerer)
```

Registerer **явный**, а коллекторы создаются через `prometheus.New*`, не через `promauto` — то есть сами в дефолтный регистр не лезут. Это то, что нужно тестам (свой выбрасываемый registry на кейс) и нескольким приложениям в одном бинарнике.

Вызов проставляется автоматически в `cmd/<app>/psg_main_gen.go` рядом с существующим `application.InitMetrics` — для приложений, у которых есть `sys`-транспорт (то есть эндпоинт `/metrics`):

```go
err = metrics.Register(application.GetMetrics())
```

В тестах:

```go
reg := prometheus.NewRegistry()
require.NoError(t, metrics.Register(reg))
defer metrics.Unregister(reg)
```

## Валидатор

Объявление становится несущим только вместе с парным запретом: **конструкторы метрик вне сгенерированного пакета запрещены**. Иначе рядом со сгенерированными переменными всегда можно объявить ещё один `promauto.NewCounter`, и карта вырождается в договорённость.

Запрет реализован правилами `forbidigo` и генерируется в двух местах:

- `configs/golangci-metrics.yml` + цель `make metrics-validate` — **основной гейт**;
- `configs/golangci-lint.yml` — те же правила, чтобы `make lint` ловил типовой случай сразу.

```bash
make metrics-validate
```

```text
internal/app/svc/svc.go:9:14: use of `prometheus.NewCounter` forbidden because
  "metric constructors are not allowed here: declare the metric in the metrics
  map instead — the generated collector package is the only place metrics are
  constructed" (forbidigo)
```

### Что запрещено

| Форма | Пример |
|---|---|
| Конструкторы коллекторов | `prometheus.NewCounter`, `NewGaugeVec`, `NewHistogram`, `NewSummaryVec`, `NewCounterFunc`, … |
| Низкоуровневые конструкторы | `prometheus.NewDesc`, `NewConstMetric`, `NewConstHistogram`, `NewConstSummary`, … |
| `promauto` целиком | `promauto.NewCounter`, `promauto.With(reg).NewCounter` |

`prometheus.NewDesc` в списке не случайно: это **позиционная** форма (`NewDesc(name, help, …)`), которой пользуются кастомные коллекторы со своими `Collect`/`Describe`. Правило, знающее только `prometheus.CounterOpts{…}`, оставляло бы дыру ровно там, где пишут нестандартное.

### Что НЕ запрещено и почему

Литералы опций — `prometheus.CounterOpts{Name: …, Help: …}` — не запрещены намеренно: сами по себе они ничего не эмитят. Без вызова конструктора коллектора метрики из них не получится, поэтому запрета вызовов достаточно, чтобы закрыть дыру полностью.

### Исключения

Из проверки выведены два пути, и только они:

- `internal/pkg/metrics/` — сгенерированный пакет, где метрики и **должны** конструироваться;
- `pkg/drivers/kafka/` — сгенерированный драйвер Kafka-продюсера: имя продюсера там `ConstLabel`, поэтому каждому продюсеру нужны свои коллекторы, а не общая переменная уровня пакета.

Оба файла генерируются целиком из объявления — руками их не правят.

`configs/golangci-metrics.yml` ставит `exclusions.generated: disable`, а не `lax`, как основной конфиг. Это и есть смысл отдельного конфига: при `lax` из проверки выпадает **любой** `psg_*_gen.go`, включая секцию ручного кода ниже disclaimer-маркера, — а это ровно то место, где пишут хэндлеры. Поэтому гейт — `make metrics-validate`, а не `make lint`.

> `make metrics-validate` стоит подключить в CI-пайплайн проекта. Объявляемый список CI-проверок ведётся отдельно (TOOLS-10).

## Дашборд

В дев-дашборд (`configs/grafana/dashboards/<app>.json`) добавляется свёрнутый ряд **Service Metrics** с канонической панелью на каждую объявленную метрику:

| Тип | Запрос |
|---|---|
| `Counter` / `CounterVec` | `rate(...)`, с разбивкой по объявленным меткам |
| `Gauge` / `GaugeVec` | само значение |
| `Histogram` / `HistogramVec` | `histogram_quantile` p99 и p95 по `_bucket` |
| `Summary` / `SummaryVec` с `objectives` | серия по метке `quantile` |
| `Summary` / `SummaryVec` без `objectives` | среднее `rate(_sum) / rate(_count)` — это всё, что метрика несёт |

«Канонический» — значит «та форма, которая следует из типа метрики». Больше без знаний об инсталляции сказать нельзя, поэтому **пороги и алерт-правила не генерируются**: порог завязан на ёмкость конкретной инсталляции, и сгенерированный был бы выдуманным.

Метрики, у которых уже есть рукописный ряд — `go_*`, `process_*`, `ogen_server_*`, `ogen_client_*` — в Service Metrics не дублируются: те ряды знают о своих метриках больше, чем форма, выведенная из типа. Объявить их в карте всё равно полезно (имя занято, `help` отревьюен), просто второй, более бедной панели они не получают.

## Внутренний набор генератора

Свои метрики go-project-starter описывает **той же картой**, внутри себя (`internal/pkg/metricsmap/embedded/internal-metrics.yaml`): Go-рантайм, инструментирование ogen, метрики Kafka-продюсера. Форма объявления одна на всех, а не две.

Наборы складываются в **одно пространство имён**, и совпадение имён — **ошибка генерации**, а не переопределение:

```text
metrics map: metric name collision:
  "go_goroutines" declared by both go-project-starter internal metrics[0]
  and service metrics map[3]
```

Молчаливое переопределение означало бы, что имя в `/metrics` зависит от порядка слияния, — а имя метрики это то, на что смотрят дашборды и алерты.

Внутренний набор фильтруется по тому, что в проекте реально есть: без Kafka-продюсера метрики `kafka_producer_*` не объявляются вовсе. Это работает в обе стороны — метрика, которую в этом проекте нечем эмитить, не занимает имя против сервиса и не получает панель, которая может только пустовать.

Поля `owner` и `requires`, которыми это выражено, **зарезервированы за внутренней картой** и в карте сервиса отвергаются.

## Чего генератор не делает

Намеренно вне scope:

- **Пороги и алерт-правила.** Порог завязан на ёмкость конкретной инсталляции, а обоснование порога — это решение, и его место рядом с решением.
- **Прод-дашборды.** Они живут в мониторинг-репозитории потребителя, их правят руками и осознанно. Сгенерированный, но редактируемый артефакт — это «второй дом»: его правят на месте, и он расходится с генератором молча.
- **Доставка админской части наружу** (складывать дашборды в `etc/` для инфраструктурного репозитория).

## Полный пример

```yaml
# project.yaml
metrics:
  path: git+https://github.com/org/contracts.git@v1.4.0#common/metrics-map.yaml
```

```yaml
# common/metrics-map.yaml в контрактном репозитории
metrics:
  - name: spawn_errors_total
    type: CounterVec
    labels: reason, stage
    help: >-
      Spawn attempts that failed, by reason and stage.

  - name: queue_depth
    type: Gauge
    help: Number of tasks currently waiting in the queue.

  - name: task_duration_seconds
    type: HistogramVec
    labels:
      - kind
      - outcome
    help: Wall-clock duration of a task, by kind and outcome.
    buckets: [0.05, 0.25, 1, 5, 30]

  - name: batch_size
    type: Summary
    help: Number of records in a processed batch.
    objectives: {0.5: 0.05, 0.99: 0.001}

  - name: otel_sdk_exporter_spans_total
    type: CounterVec
    labels: outcome
    help: Spans handed to the OTLP exporter, by outcome.
    external: true
```

## Диагностика

| Сообщение | Причина |
|---|---|
| `invalid metrics map: the file is empty` | карта пустая (или в ней только пробелы) |
| `invalid metrics map: metrics[2]: name "has-a-dash" is not a valid Prometheus metric name` | дефис в имени метрики |
| `invalid metrics map: metrics[0]: name "_" leaves no Go identifier once '_' and ':' are folded away` | имя из одних разделителей: Prometheus такое примет, а Go-идентификатора не остаётся |
| `invalid metrics map: metrics[1]: type CounterVec requires labels; use Counter for a metric without labels` | `Vec`-тип без `labels` |
| `invalid metrics map: metrics[0]: buckets are only valid for Histogram/HistogramVec, not Counter` | `buckets` не у гистограммы |
| `invalid metrics map: metrics[4]: help is required` | нет `help` |
| `invalid metrics map: line 7: unknown field "lables"` | опечатка в имени поля: молча проигнорированный ключ — это объявление, которое не действует |
| `metric name collision: "x" declared by both …internal metrics[n] and service metrics map[m]` | имя уже занято внутренним набором генератора |
| `metric variable name collision: "queue:depth" and "queue_depth" both generate the variable QueueDepth` | два имени сворачиваются в один Go-идентификатор |
| `invalid metrics map: metrics[0] (x): `owner` is reserved for the generator's internal map` | `owner`/`requires` в карте сервиса |

Валидация сообщает **все** проблемы сразу: починка плохой карты занимает один проход, а не по одному на запись.
