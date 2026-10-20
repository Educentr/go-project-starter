# Карта ключей OnlineConf (`onlineconf.path`)

Карта ключей OnlineConf — **объявление всех ключей, которые сервису разрешено читать**. Это второй вход тому же механизму, что и [карта метрик](metrics-map.md): карта в контрактном репозитории, ссылка полем `project.yaml`, генерация, парный валидатор, включение по наличию карты.

Карта не указана — **ничего не включается**: ни сгенерированного пакета, ни валидатора, ни проверки на старте. Поведение проекта остаётся прежним.

## Зачем

Генератор **уже** генерирует чтение OnlineConf — для своих ключей (`pkg/app/restconfig/psg_config_oc_gen.go`, `psg_zlog_gen.go`, `psg_service_gen.go`). Форма ему принадлежит; снаружи её не было только у ключей **сервиса**.

Предмет — не «строки в коде». Путь собирается из приватных констант, объявленных у каждого читателя по-своему:

```go
onlineconf.MakePath(constant.ServiceName, ocSegmentDocker, "network")
onlineconf.MakePath(serviceName, segConfluence, keyURL)
```

Собранного пути не существует нигде целиком, поэтому любой внешний инструмент (гвард, реестр, ревью) видит не путь, а частный словарь сегментов — и расходится с картой **молча**. Типизированный аксессор плюс парный запрет прямого обращения к `onlineconf` делают «сервис читает необъявленный ключ» невозможным по построению, а не по договорённости.

## Подключение

Карта — **контрактный артефакт**, поэтому живёт в контрактном репозитории, а `project.yaml` несёт только ссылку. Форма ссылки та же, что у `rest[].path` и `metrics.path` (см. [Удалённые спеки](remote-specs.md)): локальный путь, `https://`, `git+ssh://`, `git+https://`.

```yaml
# project.yaml
onlineconf:
  path: "git+https://github.com/org/contracts.git@v1.4.0#common/onlineconf-key-map.yaml"
```

Локально — для проб и для тестов:

```yaml
onlineconf:
  path: ./onlineconf-key-map.yaml
```

Карта читается **во время генерации** и в проект не копируется: ничего ниже по цепочке её не читает, артефактом является сгенерированный пакет аксессоров. Второй копии, которая молча расходится с контрактом, не возникает. Переопределение ref (`--sourceRef` / `SOURCE_REF`) действует и на этот канал — удалённая карта приезжает из того же состояния контрактов, что и остальные спеки.

## Формат карты

Плоский список. Минимально достаточны **два** поля — `path` и `type`:

```yaml
# common/onlineconf-key-map.yaml
keys:
  - path: /myservice/supervisor/min_lifetime
    type: duration
    default: 5m
    description: >-
      How long a spawned agent is kept before the supervisor may reap it.
```

Схема **строгая**: неизвестное поле — отказ генерации. Процессные поля, которые обычно обрастает реестр ключей (`consumer`, `seeded_by`, `declared_in`, `status`, `service_tree`), в карту **не входят**. Карта — вход генератора, а «кто и где ключ читает» — вопрос о потребителях, не о контракте; владелец ключа — сам контракт. Закрытая схема — это то, что превращает опечатку в имени поля в ошибку, а не в объявление, которое молча ничего не делает.

### `path` — полный путь ключа

В `path` пишется **то, что существует в дереве**, целиком. Ведущий `/` необязателен, завершающий отбрасывается: `svc/a/b/` и `/svc/a/b` — один и тот же ключ.

Требования:

- минимум два сегмента — `/<корень дерева>/<ключ>`. OnlineConf читает **значение**, не поддерево, поэтому один корень ключа не называет;
- сегмент — `[A-Za-z0-9][A-Za-z0-9_.-]*`;
- **все записи карты лежат в одном дереве.** Карта — одна на сервис: у ключа ровно одно дерево, а общая карта смешивала бы поверхности нескольких сервисов в одном файле. Расхождение первого сегмента — ошибка;
- **шаблоны со списковым сегментом не принимаются.** `/svc/agent_env/<i>/name` — не ключ: обхода поддерева в OnlineConf нет (так задумано). Список объявляется как **один ключ со структурным значением** (`type: struct`) и один аксессор.

### `type` — тип значения

| `type` | Go-тип | Читается через |
|---|---|---|
| `string` | `string` | `GetStringIfExists` |
| `int` | `int64` | `GetIntIfExists` |
| `bool` | `bool` | `GetBoolIfExists` |
| `duration` | `time.Duration` | `GetDurationIfExists` |
| `strings` | `[]string` | `GetStrings` (строка через запятую или JSON-массив) |
| `struct` | тип вызывающего | `GetStruct` (JSON в переданный `dst`) |

Используются именно `*IfExists`-формы: форма с дефолтом (`onlineconf.GetString(ctx, path, d...)`) при ошибке чтения обращается к `d[0]` и **паникует**, если дефолт не передали. Сгенерированный код не должен зависеть от того, вспомнил ли вызывающий про дефолт.

`int` — это `int64`: такова ширина в go-onlineconf, и сужать её за вас генератор не станет.

### `required` — ключ, без которого сервис не работает

```yaml
  - path: /myservice/confluence/url
    type: string
    required: true
```

Аксессор такого ключа при отсутствии значения возвращает `ErrKeyNotSet`, а не нуль. Кроме того, ключ попадает в `CheckRequired`, который сгенерированный `main` вызывает **на старте**, сразу после инициализации OnlineConf: отсутствующий обязательный ключ — это misconfiguration инсталляции, и узнавать о нём надо при старте процесса, а не на первом запросе, которому он понадобился.

Ограничения (оба — ошибка генерации):

- `required` **вместе с** `default` — противоречие: ключ с дефолтом не бывает отсутствующим;
- `required` у `strings` и `struct` — **нельзя**. go-onlineconf не сообщает для них наличие (`GetStrings` возвращает переданный fallback без признака, `GetStruct` требует тип назначения, которого в карте нет), поэтому `required` был бы обещанием, которое сгенерированный код не может выполнить. Объявляйте `default`.

### `default` — значение при отсутствии ключа

Декодируется **по объявленному типу во время генерации**, поэтому `default: 5 hours` у `duration` — ошибка генерации, а не значение, которого сервис никогда не получит.

```yaml
  - path: /myservice/supervisor/min_lifetime
    type: duration
    default: 5m          # → 5 * time.Minute

  - path: /myservice/notify/recipients
    type: strings
    default: ops@example.com, oncall@example.com   # или YAML-список
```

`struct` дефолта не принимает: форма значения принадлежит вызывающему, который передаёт `dst` и может заполнить его сам (`GetStruct` не трогает `dst`, если ключа нет).

### `name` — имя аксессора

Необязательно. По умолчанию имя выводится из сегментов пути **ниже корня дерева**: `/myservice/supervisor/min_lifetime` → `SupervisorMinLifetime`. Корень отбрасывается, потому что карта одна на сервис — повторять его в каждом идентификаторе нечего, пакет уже назван.

Разделители внутри сегмента (`_`, `-`, `.`) сворачиваются, а распространённые аббревиатуры получают принятое в Go написание (список как у `golint`): `/svc/confluence/url` → `ConfluenceURL`, `/svc/api/http_timeout` → `APIHTTPTimeout`, `/svc/user/id` → `UserID`.

`name` нужен, когда выведенное имя всё равно читается плохо (`/svc/db/dsn` → `DBDSN`) или когда путь не даёт идентификатора вовсе (`/svc/2fa` — после сворачивания идентификатор начинался бы с цифры, это ошибка генерации с указанием объявления). Значение должно быть экспортируемым Go-идентификатором: оно называет функцию.

Два объявления, дающие один идентификатор, — ошибка, а не одна функция на двоих.

### `description` — проза для godoc

Необязательно, но это единственное место, где смысл ключа **видно** и можно отревьюить; в `MakePath` посреди кода его не видит никто. Текст уезжает в doc-комментарий аксессора.

## Сгенерированный пакет аксессоров

Генерируется в `internal/pkg/occonf/psg_occonf_gen.go`. Пакет назван `occonf`, а не `onlineconf`, намеренно: он оборачивает библиотеку с таким именем, и на месте вызова эти две вещи должны различаться.

### Константы путей

По одной на объявленный ключ:

```go
const (
	// PathSupervisorMinLifetime is the path of SupervisorMinLifetime (duration).
	PathSupervisorMinLifetime = "/myservice/supervisor/min_lifetime"
)
```

Экспортируются, чтобы путь можно было передать в `Subscribe`, не обращаясь к `onlineconf` (что валидатор запрещает).

### Аксессоры: сигнатуру задаёт объявление

Форму возврата выбирает **карта**, а не место вызова — поэтому обязательный ключ нельзя принять за отсутствующий, а опциональный за присутствующий.

```go
// default: отсутствие не сообщается, возвращается дефолт
func SupervisorMinLifetime(ctx context.Context) (time.Duration, error)

// required: отсутствие — ErrKeyNotSet, не нуль
func ConfluenceURL(ctx context.Context) (string, error)

// ни того, ни другого: наличие возвращается вместе со значением
func QueueName(ctx context.Context) (string, bool, error)

// strings: признака наличия у API нет, отсутствие даёт дефолт (или nil)
func NotifyRecipients(ctx context.Context) ([]string, error)

// struct: форма значения принадлежит вызывающему
func AgentEnv(ctx context.Context, dst any) (bool, error)
```

Использование в коде сервиса:

```go
import "github.com/org/myservice/internal/pkg/occonf"

lifetime, err := occonf.SupervisorMinLifetime(ctx)   // 5m, если ключа нет

url, err := occonf.ConfluenceURL(ctx)                // ErrKeyNotSet, если ключа нет
if errors.Is(err, occonf.ErrKeyNotSet) { … }

queue, ok, err := occonf.QueueName(ctx)              // ok == false ≠ queue == ""

var env []AgentEnvVar
if _, err := occonf.AgentEnv(ctx, &env); err != nil { … }
```

### `CheckRequired` — проверка на старте

```go
func CheckRequired(ctx context.Context) error
```

Читает каждый объявленный `required`-ключ **через его же аксессор** (чтобы проверка на старте и работающий сервис понимали «пригоден» одинаково) и сообщает **все** проблемы сразу. Вызов проставляется автоматически в `cmd/<app>/psg_main_gen.go` сразу после `onlineconf.StartWatcher`:

```go
err = occonf.CheckRequired(mainCtx)
if err != nil {
	zlog.Ctx(mainCtx).Error().Err(err).Msg("required onlineconf keys are missing")
	os.Exit(ExitCodeErrorConfig)
}
```

Это ровно та граница, на которой OnlineConf и так валидируется один раз за процесс. Дальше в коде проверять наличие ключа на каждом чтении не нужно — инвариант уже установлен.

> CLI-приложения (`cmd/<cli>`) проверку не получают: короткоживущей команде не обязательно нужна вся конфигурационная поверхность сервиса. При необходимости вызовите `occonf.CheckRequired` сами.

### `Subscribe` и `Paths`

```go
func Paths() []string
func Subscribe(ctx context.Context, paths []string, cb func() error) error
```

`Subscribe` существует именно потому, что валидатор запрещает вызывать `onlineconf` из кода сервиса: без него сервис, реагирующий на изменения конфигурации, не мог бы это сделать иначе, чем нарушив правило.

```go
err := occonf.Subscribe(ctx, []string{occonf.PathSupervisorMinLifetime}, func() error {
	reloadSupervisor()

	return nil
})
```

## Валидатор

Объявление становится несущим только вместе с парным запретом: **обращения к пакету `onlineconf` вне сгенерированного пакета запрещены**. Иначе рядом с аксессорами всегда можно написать ещё один `onlineconf.MakePath`, и карта вырождается в договорённость.

Запрет реализован правилом `forbidigo` и генерируется в двух местах:

- `configs/golangci-onlineconf.yml` + цель `make onlineconf-validate` — **основной гейт**;
- `configs/golangci-lint.yml` — то же правило, чтобы `make lint` ловил типовой случай сразу.

```bash
make onlineconf-validate
```

```text
internal/app/svc/svc.go:42:14: use of `onlineconf.MakePath` forbidden because
  "reading OnlineConf directly is not allowed here: declare the key in the
  OnlineConf key map instead — the generated accessor package is the only place
  OnlineConf is read" (forbidigo)
```

### Почему запрещён весь пакет, а не только читатели

Правило одно — `^onlineconf\.` — и это осознанно. Запрет только `Get*` оставил бы `MakePath` свободным собирать путь для какого-нибудь другого читателя; запрет только `MakePath` оставил бы свободным строковый литерал пути. Одно правило на пакет закрывает оба случая.

### Почему правило не обходится алиасом импорта

Оба конфига ставят `forbidigo: analyze-types: true`. По умолчанию `forbidigo` сравнивает паттерн с **текстом вызова, как он написан**, а паттерн называет пакет. Без резолва типов мимо запрета проходят обе переименованные формы:

```go
import oc "github.com/Educentr/go-onlineconf/pkg/onlineconf"

v, _ := oc.GetString(ctx, "/svc/undeclared", "")
```

```go
import . "github.com/Educentr/go-onlineconf/pkg/onlineconf"

v, _ := GetString(ctx, "/svc/undeclared", "")
```

С `analyze-types: true` вызов резолвится до настоящего пакета, и обе формы ловятся. Гейт, который обходится переименованием импорта, гейтом не является.

Побочное следствие: валидатору нужна информация о типах, то есть проект должен компилироваться. Для `make onlineconf-validate` это не ограничение — цель и так запускается после генерации.

### Исключения

Из проверки выведены две группы, и только они:

- `internal/pkg/occonf/` — сгенерированный пакет, где OnlineConf и **должен** читаться;
- `psg_*_gen.go` — любой сгенерированный Go-файл. Эти файлы читают **собственные поддеревья генератора** (`transport`, `security`, `kafka`, логгер, dev-stand), которые карта сервиса объявлять не может вовсе (см. ниже). Они генерируются целиком из `project.yaml` — это не рукописные лазейки.

> **Известное ограничение.** Секция ручного кода ниже disclaimer-маркера **внутри** `psg_*_gen.go` выводится из проверки вместе с файлом. Рукописные файлы — хэндлеры, сервисы, воркеры, то есть то место, где живёт код сервиса, — остаются под проверкой. В карте метрик этот выбор сделан иначе (`generated: disable` без исключения по имени файла), потому что там сгенерированные файлы метрик почти не конструируют, а здесь OnlineConf читают почти все.

## Ключи, которые генератор читает сам

Генератор читает OnlineConf для себя — конфигурацию транспортов, безопасности, Kafka, логгера, dev-stand. Карта сервиса в эти поддеревья **объявлять не может**: пути там строят сгенерированные читатели из имён транспортов, приложений и клиентов, поэтому рукописное объявление либо затеняло бы сгенерированное чтение, либо расходилось бы с ним — а смысл карты в том, что ни один ключ не читается из двух мест двумя написаниями.

Зарезервированы следующие поддеревья `/<корень>/`:

| Сегмент | Что читает генератор |
|---|---|
| `transport` | REST- и gRPC-транспорты (`restconfig`, ogen/buf-клиенты) |
| `security` | `httpAuth`, CSRF, CORS (`restconfig`) |
| `kafka` | конфигурация Kafka-клиента (драйвер Kafka) |
| `log`, `logger` | конфигурация логгера (сгенерированный логгер) |
| `devstand` | тумблер dev-stand (сгенерированный пакет `service`) |

```text
invalid onlineconf key map:
  keys[0]: reserved onlineconf subtree: path "/myservice/transport/rest/api/port"
  is under /myservice/transport, which the generator reads itself (REST and gRPC
  transports (generated restconfig and clients))
```

Это тот же инвариант, что у внутренней карты метрик («имя занято генератором»), выраженный префиксом: пути генератора параметризованы именами транспортов и приложений, поэтому перечислить их записями нельзя, а зарезервировать поддерево — можно, и гарантия получается строже.

## Чего генератор не делает

Намеренно вне scope:

- **Значения ключей и политика сеяния.** Значение привязано к инсталляции, а решение о нём живёт рядом с решением, а не в генераторе.
- **OnlineConf admin и доставка дерева.** Это инфраструктура потребителя.
- **Реестры и отчёты по ключам.** Производные страницы строятся из той же карты, но не генератором.
- **Аксессоры поддеревьев.** Обхода поддерева в OnlineConf нет; список — это один ключ со структурным значением.

## Полный пример

```yaml
# project.yaml
onlineconf:
  path: git+https://github.com/org/contracts.git@v1.4.0#common/onlineconf-key-map.yaml
```

```yaml
# common/onlineconf-key-map.yaml в контрактном репозитории
keys:
  - path: /myservice/supervisor/min_lifetime
    type: duration
    default: 5m
    description: >-
      How long a spawned agent is kept before the supervisor may reap it.

  - path: /myservice/supervisor/max_agents
    type: int
    required: true
    description: Upper bound on concurrently running agents.

  - path: /myservice/feature/new_pipeline_enabled
    type: bool
    default: false
    description: Route tasks through the new pipeline.

  - path: /myservice/confluence/url
    type: string
    required: true
    description: Base URL of the Confluence instance the exporter writes to.

  - path: /myservice/notify/recipients
    type: strings
    default: ops@example.com, oncall@example.com
    description: Addresses that receive escalation mail.

  - path: /myservice/queue/name
    type: string
    description: Queue to consume from; unset means the built-in default queue.

  # Список — ОДИН ключ со структурным значением: обхода поддерева нет.
  - path: /myservice/agent_env
    type: struct
    description: >-
      Environment variables handed to every spawned agent, as a JSON array of
      {name, value} objects.

  - path: /myservice/db/dsn
    type: string
    name: DatabaseDSN
    required: true
    description: Data source name of the primary database.
```

## Диагностика

| Сообщение | Причина |
|---|---|
| `invalid onlineconf key map: the file is empty` | карта пустая (или в ней только пробелы) |
| `invalid onlineconf key map: no keys declared` | `keys: []` |
| `malformed onlineconf key map: line 4: unknown field "consumer"` | поле вне схемы: молча проигнорированный ключ — это объявление, которое не действует |
| `keys[0]: path is required` | нет `path` |
| `keys[0]: type "float" is unknown, expected one of bool, duration, int, string, strings, struct` | неизвестный `type` |
| `keys[0]: path "/svc" has no key segment` | один корень — это поддерево, а не ключ |
| `keys[0]: path "/svc/agent_env/<i>/name" uses the template segment "<i>"` | шаблон со списковым сегментом: объявляйте `type: struct` |
| `keys[0]: path "/svc/a b" has an invalid segment "a b"` | сегмент вне грамматики |
| `keys[1]: duplicate onlineconf key path: "/svc/a" (first declared at keys[0])` | один ключ объявлен дважды |
| `keys[1]: onlineconf key map spans several service trees: "/other/b" is in tree "other", keys[0] is in tree "svc"` | карта не одна на сервис |
| `keys[0]: reserved onlineconf subtree: … which the generator reads itself` | объявление в поддереве генератора |
| `keys[0]: `required` and `default` contradict each other` | и `required`, и `default` |
| `keys[0]: type strings cannot be `required`: go-onlineconf reports no presence for it` | `required` у типа без признака наличия |
| `keys[0]: type struct takes no `default`` | `default` у `struct` |
| `keys[0]: invalid default value: default "5 hours" is not a duration` | дефолт не парсится как объявленный тип |
| `keys[0]: name "my_key" is not an exported Go identifier` | `name` не называет функцию |
| `keys[0]: path "/svc/2fa" generates the accessor 2fa, which does not start with a letter` | из пути не выходит идентификатора — задайте `name` |
| `onlineconf accessor name collision: "/svc/max-size" and "/svc/max_size" both generate the accessor MaxSize` | два объявления сворачиваются в один идентификатор |

Валидация сообщает **все** проблемы сразу: починка плохой карты занимает один проход, а не по одному на запись.
