# Карта ключей OnlineConf (`onlineconf.path`)

У каждого сгенерированного проекта есть **полная карта ключей OnlineConf, к которым он обращается**. Карта состоит из двух половин:

- **половина генератора** — ключи, которые читает сгенерированный код: таймауты транспортов, уровни логирования, тумблеры безопасности, настройки воркеров и Kafka, dev-stand, ACL телеграм-бота. Объявлена в самом генераторе, рядом с шаблонами, которые её читают. Есть **всегда**;
- **половина сервиса** — `onlineconf.path` в `project.yaml`: ссылка на карту в контрактном репозитории. Необязательна.

На этой карте держится всё остальное: типизированные аксессоры в одном пакете, валидатор (никто не читает OnlineConf в обход карты), проверка «все обязательные настройки сделаны» на старте и реестр всей поверхности дерева. Поэтому **`onlineconf.path` добавляет ключи, а не включает фичу**: без него карта состоит из половины генератора, и пакет, валидатор, проверка и реестр генерируются всё равно.

Механизм — тот же, что у [карты метрик](metrics-map.md): карта в контрактном репозитории, ссылка полем `project.yaml`, генерация, парный валидатор.

## Зачем

Генератор **уже** генерировал чтение OnlineConf — для своих ключей (`pkg/app/restconfig/psg_config_oc_gen.go`, `psg_zlog_gen.go`, `psg_service_gen.go`). Форма ему принадлежала; снаружи её не было только у ключей **сервиса**.

Предмет — не «строки в коде». Путь собирается из приватных констант, объявленных у каждого читателя по-своему:

```go
onlineconf.MakePath(constant.ServiceName, ocSegmentDocker, "network")
onlineconf.MakePath(serviceName, segConfluence, keyURL)
```

Собранного пути не существует нигде целиком, поэтому любой внешний инструмент (гвард, реестр, ревью) видит не путь, а частный словарь сегментов — и расходится с картой **молча**. Типизированный аксессор плюс парный запрет прямого обращения к `onlineconf` делают «проект читает необъявленный ключ» невозможным по построению, а не по договорённости.

И то же самое — причина, по которой сгенерированный код тоже читает **через аксессоры**. Пока он собирал пути сам, половина поверхности дерева существовала только внутри шаблонов: владелец контракта её не видел, валидатор обходил сгенерированные файлы целиком (а с ними — и секцию ручного кода ниже disclaimer-маркера), и проверка на старте знала только про ключи сервиса.

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
- `required` у `struct` — **нельзя**: проверить наличие значило бы его разобрать, а тип назначения принадлежит вызывающему, не карте. Читайте через признак наличия у аксессора. Для `strings` `required` возможен: наличие берётся из `GetStringIfExists` по тому же пути.

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

// strings с дефолтом: отсутствие даёт дефолт
func NotifyRecipients(ctx context.Context) ([]string, error)

// strings без дефолта: наличие возвращается вместе со значением — «нет ключа»
// и «пустой список» это разные конфигурации
func AllowedOrigins(ctx context.Context) ([]string, bool, error)

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
func CheckRequired(ctx context.Context, app string) error
```

Читает каждый `required`-ключ **через его же аксессор** (чтобы проверка на старте и работающий сервис понимали «пригоден» одинаково) и сообщает **все** проблемы сразу. Вызов проставляется автоматически в `cmd/<app>/psg_main_gen.go` сразу после запуска watcher-а:

```go
err = occonf.CheckRequired(mainCtx, "api")
if err != nil {
	zlog.Ctx(mainCtx).Error().Err(err).Msg("required onlineconf keys are missing")
	os.Exit(ExitCodeErrorConfig)
}
```

Это ровно та граница, на которой OnlineConf и так валидируется один раз за процесс. Дальше в коде проверять наличие ключа на каждом чтении не нужно — инвариант уже установлен.

**Почему параметр — имя приложения.** Ключи сервиса проверяются всегда: у карты сервиса нет измерения приложений. А вот `required`-ключ генератора принадлежит **компоненту** — host gRPC-клиента, brokers Kafka-продюсера, токен телеграм-бота — и процесс, который этот компонент не запускает, такой ключ не читает. Проверять весь набор проекта в каждом процессе значило бы не дать стартовать REST-только приложению из-за токена воркера, которого у него нет.

По той же причине компонент, объявленный в `project.yaml` как `optional: true`, из проверки исключается: сгенерированный читатель пропускает отсутствующий ключ такого компонента вместо того, чтобы ругаться, и проверка не должна настаивать там, где код согласен обойтись.

> CLI-приложения (`cmd/<cli>`) проверку не получают: короткоживущей команде не обязательно нужна вся конфигурационная поверхность сервиса. При необходимости вызовите `occonf.CheckRequired` сами.

### Жизненный цикл OnlineConf

Пакет оборачивает и точки входа самой библиотеки — по той же причине, по которой оборачивает чтения: валидатор запрещает `onlineconf` всюду, кроме этого пакета, и сгенерированный `main`, обращающийся к библиотеке напрямую, был бы единственным исключением, которого никто не видит.

```go
func Initialize(ctx context.Context, configDir string) (context.Context, error)
func StartWatcher(ctx context.Context) error
func StopWatcher(ctx context.Context) error
```

### `ActiveRecordConfig` — известная дыра в карте

```go
func ActiveRecordConfig(ctx context.Context) *onlineconf.OnlineconfInstance
```

Генерируется **только при `use_active_record: true`**. ActiveRecord читает дерево сам, через `activerecord.ConfigInterface`, а этот интерфейс — весь набор читателей библиотеки (`GetString`, `GetStruct`, …). Значит, отдать ActiveRecord узкий интерфейс нельзя, и значение, которое возвращает эта функция, может прочитать любой путь.

Это **дыра в карте, и она была всегда**: собственные чтения ActiveRecord (`db/<shard>`, user, password, …) не объявлены и в `configs/onlineconf-tree.yaml` не попадают. Сузилась она в одном: проект без ActiveRecord такой функции не получает вовсе, то есть мимо аксессоров пройти негде. Читать ключи через неё нельзя — объявите ключ и позовите его аксессор.

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

Из проверки выведено **одно** место: `internal/pkg/occonf/` — сгенерированный пакет, где OnlineConf и **должен** читаться.

Раньше из проверки выводился ещё и `psg_*_gen.go` как класс: сгенерированный код читал ключи генератора напрямую, и иначе валидатор ругался бы на него самого. В этом была дыра — секция ручного кода ниже disclaimer-маркера **внутри** такого файла выводилась из проверки вместе с файлом, и могла читать любой ключ незаметно. Теперь сгенерированный код читает через аксессоры, исключение не нужно, и дыры нет.

Проверено на сгенерированном проекте: `make onlineconf-validate` чист на чистом выводе генератора и падает (exit 10) на дописанном ниже маркера `oc.GetBool(ctx, oc.MakePath(…))` в `psg_router_gen.go` — в том числе с переименованным импортом.

## Ключи, которые читает сгенерированный код

Половина карты принадлежит генератору, и он **объявляет свои ключи сам** — по тому же принципу, по которому у карты метрик есть внутренняя карта `internal-metrics.yaml`.

Объявление лежит **рядом с чтением**: у каждого шаблона, чей код читает ключ, есть файл `<шаблон>.ockeys.yaml` в том же каталоге. Из объявления генерируется аксессор, и шаблон вызывает именно его — так что «чтение» и «объявление» это одна и та же запись, а не два списка, которые надо держать в согласии.

```yaml
# internal/pkg/templater/embedded/templates/transport/rest/files/pkg/app/
#   restconfig/config_oc.go.tmpl.ockeys.yaml
groups:
  - scope: rest_server
    keys:
      - path: "{service}/transport/rest/{transport}/{app}/server/timeout/read"
        name: RESTServerAppTimeoutRead
        type: duration
        default: "the transport-wide value"
        description: Request read timeout of this application's HTTP server.

      - path: "{service}/transport/rest/{transport}/{app}/handler/<urlPath>/timeout"
        name: RESTAppHandlerTimeout
        type: duration
        description: Timeout of the handler of one url path of this application.

      - path: "{service}/transport/rest/{transport}/{app}/handler"
        name: RESTAppHandlerPath
        description: >-
          Per-handler configuration, keyed by the url path of the request. The
          provider hands the path to the runtime and to the logger.
```

### Переменные сегменты пути

| форма | что это | что с ней делает генератор |
|---|---|---|
| `{service}`, `{transport}`, `{app}`, `{worker}`, `{client}`, `{event}` | измерение проекта, известное на генерации | раскрывается в конкретные имена в реестре; становится **параметром** аксессора |
| `<urlPath>`, `<tgID>`, `<action>`, … | сегмент, который сгенерированный код вычисляет в рантайме | остаётся как есть в реестре; становится параметром аксессора и **отдаёт поддерево генератору** с этого сегмента |

Измерения становятся параметрами, а не подставляются в имя функции, потому что сгенерированные читатели **generic** по ним: провайдер конфигурации REST получает имя сервера и приложения от рантайма, а не от генератора. Аксессор с «впечённым» именем транспорта вызвать было бы нельзя.

Из этого же следует, что **набор аксессоров не зависит от измерений проекта** — от них зависит реестр. Шаблон, который генерируется, зовёт свои аксессоры безусловно: провайдер конфигурации REST генерируется и в проекте, у которого единственный REST-транспорт — клиент, и всё равно зовёт каждый серверный аксессор. Поэтому аксессоры генерируются все, а претензии на пути (строки реестра и проверки на старте) — только по тому, что проект действительно читает.

Побочно: у ключа без переменных сегментов путь известен на генерации, и построитель возвращает константу, а не собирает ту же строку на каждом чтении (`DevStandPath()` читается раз на запрос через логгер).

```go
// {service}/transport/rest/{transport}/{app}/handler/<urlPath>/timeout
func RESTAppHandlerTimeoutPath(transport, app, urlPath string) string
func RESTAppHandlerTimeout(ctx context.Context, transport, app, urlPath string) (time.Duration, bool, error)
```

### `scope` — измерение, по которому запись раскрывается

`service`, `rest_server`, `rest_client`, `grpc_client`, `kafka_producer`, `kafka_producer_event`, `worker_daemon`, `worker_telegram`, `telegram_driver`. Scope же определяет, какие `{плейсхолдеры}` в записи допустимы.

### Запись без `type` — построитель пути

Сгенерированному коду иногда нужно не значение, а сам путь: префикс, который он отдаёт рантайму или логгеру, читающему **ниже**. Такая запись объявляется без `type`, её имя обязано заканчиваться на `Path`, и она отдаёт генератору всё поддерево:

```go
func RESTAppHandlerPath(transport, app string) string
```

Дописать сегменты построитель не даёт. Дать ему `extra ...string` значило бы завести второй способ адресовать дерево — путь, которого нет ни в одном объявлении и ни в одной строке реестра.

### `shared` и `read_via` — одно чтение на много путей

Уровень логирования читается под префиксом, который передаёт вызывающий, а вызывающие — сгенерированные читатели, этот префикс и собирающие (middleware REST — по url запроса, воркеры — по этапу job-а). Сам конструктор логгера видит две непрозрачные строки, поэтому у него **один** аксессор:

```yaml
# logger/zlog/zlog.go.tmpl.ockeys.yaml
- path: "{service}/<prefix>/<levelKey>/log_level"
  name: LogLevel
  type: string
  shared: true
  description: …
```

`shared: true` означает «путь целиком собирает вызывающий», и такая запись **не отдаёт себе никакого поддерева** — иначе `<prefix>` сразу под корнем забрал бы всё дерево сервиса. Конкретные пути, до которых аксессор доходит, объявлены там, где находится вызывающий, и ссылаются на него полем `read_via`:

```yaml
  - scope: rest_server
    keys:
      - path: "{service}/transport/rest/{transport}/{app}/handler/default/log_level"
        type: string
        read_via: LogLevel
        default: info
        description: …
```

Запись с `read_via` даёт строку в реестре и участвует в проверке совпадений, но своего аксессора не получает: у читателя он уже есть. `read_via`, называющий аксессор, которого в проекте нет, — ошибка генерации.

### `required` у ключей генератора

`required` означает ровно одно: ключ попадает в `CheckRequired` **того приложения**, которое запускает читающий его компонент (см. выше).

Помечать им можно ключ, которого **никакая настройка `project.yaml` не может сделать ненужным**, пока код компонента генерируется. Таких два:

| ключ | компонент |
|---|---|
| `/<svc>/security/tg_token` | телеграм-воркер: бот без токена не выполнит ни одного вызова |
| `/<svc>/kafka/<client>/brokers` | сгенерированный Kafka-продюсер: публиковать некуда |

host/port gRPC- и REST-клиентов `required` **не** помечены, хотя без них клиент не работает: клиент с `instantiation: dynamic` получает адрес аргументом и этот ключ не читает вовсе, а `optional: true` означает «нет значения — пропустить компонент». То есть нужность ключа здесь — настройка проекта, а не свойство ключа; об отсутствующем host сгенерированный драйвер и так сообщает на Init, называя транспорт.

Форму чтения `required` **не меняет** — аксессор по-прежнему возвращает признак наличия. Так и надо: читатель ключа компонента, объявленного `optional: true`, должен уметь увидеть отсутствие и пропустить компонент, а не упасть. Поэтому требование живёт на старте процесса, где известно, какие компоненты это приложение реально поднимает, а само чтение не трогается.

Ключи с `default` остаются необязательными: инсталляции, полагающиеся на дефолт, после регенерации стартуют как раньше. Записи с двухуровневым fallback (`…/{transport}/port` и `…/{transport}/{app}/port`) `required` не бывают: проверка смотрит по одному пути, а провайдера устраивает любая из двух форм — и об отсутствующем порте сервер и так сообщает с именем транспорта и приложения.

### Связь объявлений с кодом

Раньше вместо объявлений был список зарезервированных префиксов (`transport`, `security`, `kafka`, `log`, `logger`, `devstand`). Он велся руками, с шаблонами ничем не был связан — и поддеревья, которые сгенерированный код реально читает (`worker/<имя>`, `settings`, `bot`), в него просто не попали.

Теперь связь механическая в обе стороны, и это проверяется тестами в `internal/pkg/templater/onlineconf_keys_test.go`:

- `TestNoTemplateReadsOnlineConfDirectly` — шаблон вне `occonf`, обращающийся к библиотеке `onlineconf`, роняет тест: он породил бы код, который валидатор самого проекта отвергает;
- `TestOnlineConfDeclarationsMatchAccessorCalls` — аксессор, который шаблон вызывает, обязан быть объявлен **этим же** шаблоном, а объявленный — вызван. Чтение, добавленное без объявления, не соберётся (нет такой функции); объявление, оставшееся без чтения, роняет тест.

### Совпадение с картой сервиса — ошибка генерации

Объявить в карте сервиса ключ, который читает сгенерированный код, нельзя: иначе у ключа два читателя и два написания — то, ради чего карта и существует. Проверяются два случая.

**Точное совпадение пути:**

```text
onlineconf.path: invalid onlineconf key map: onlineconf key is read by generated code:
  keys[0]: path "/myservice/transport/rest/api_v1/port" is a key the generator reads itself
  (generated by transport/rest/files/pkg/app/restconfig/config_oc.go.tmpl): Port the HTTP
  server listens on, for every application serving this transport.
```

**Объявление внутри поддерева, которое генератор отдаёт себе** — и его корня, потому что там сгенерированный читатель владеет самим узлом:

```text
onlineconf.path: invalid onlineconf key map: onlineconf key is read by generated code:
  keys[0]: path "/myservice/settings/enabled/login" is inside "/myservice/settings/enabled",
  a subtree the generated code reads below under a segment it computes itself
  (generated by main/internal/pkg/service/service.go.tmpl): Per-action allow lists …
```

Поддерево отдаётся генератору двумя способами: записью без `type` (построитель пути — всё, что ниже, читает сгенерированный код) и записью с `<runtimeСегментом>` (с него и ниже).

Проверка идёт по **фактическому** проекту, а не по префиксам: ключ измерения, которого в проекте нет, сервис объявить может — его никто не читает, второго читателя не возникает. Это не только «нет такого компонента», но и «для такого компонента генератор ничего не генерирует»: Kafka-продюсер с `driver: custom` (драйвер пишет проект, `config.go` не генерируется), consumer, queue-воркер (не читает OnlineConf вовсе), gRPC-транспорт не-клиент.

Если корень дерева карты не совпадает с именем сервиса (`main.name`), генератор печатает предупреждение: сгенерированные читатели всегда читают под `/<main.name>`, поэтому карта в другом дереве означает, что сервис читает два корня — и совпадать двум половинам поверхности больше негде, то есть проверка выше ничего не сравнивает.

### `configs/onlineconf-tree.yaml` — вся поверхность дерева

Карта описывает ключи сервиса, но дерево сервиса больше: половина поверхности — чтения сгенерированного кода. Поэтому генерируется реестр **всей** поверхности, одним списком и с указанием, кто читает:

```yaml
tree: /myservice

keys:
  - path: /myservice/confluence/url
    origin: service
    type: string
    required: true
    accessor: ConfluenceURL
    description: "Base URL of the Confluence instance the exporter writes to."
  - path: /myservice/security/csrf/enabled
    origin: generated
    type: bool
    default: "false"
    read_by: ["transport/rest/files/pkg/app/restconfig/config_oc.go.tmpl"]
    description: "Whether the CSRF middleware is applied."
  - path: /myservice/settings/enabled/<action>
    origin: generated
    type: strings
    accessor: SettingsEnabled
    owns: /myservice/settings/enabled
    read_by: ["main/internal/pkg/service/service.go.tmpl"]
    description: "Allow list of one action …"
  - path: /myservice/transport/rest/api_v1/api/handler/default/log_level
    origin: generated
    type: string
    read_via: LogLevel
    default: info
    read_by: ["logger/logrus/logrus.go.tmpl", "logger/zlog/zlog.go.tmpl"]
    description: "Log level of requests to this application …"
```

- `origin: service` — объявлено картой сервиса, читается через аксессор `accessor`;
- `origin: generated` — объявлено генератором; `read_by` называет шаблон, то есть файл, который нужно править, если чтение пора менять. Перечислены **все** объявившие шаблоны, включая взаимоисключающие альтернативы (оба логгера читают одно и то же, поэтому названы оба). У ключа с несколькими читателями `description` собирает причины всех: `devstand` переключает и консольный вывод логгера, и детализацию ошибок, и диагностику бота — хранить только первую причину значило бы называть всех читателей и объяснять одного.

Ещё два поля: `owns` называет точку, с которой поддерево принадлежит сгенерированному коду (и внутрь которой карта сервиса объявлять не может), а `read_via` — общий аксессор, через который идёт чтение, когда своего у пути нет.

Файл никем в сервисе не читается — он для чтения человеком и для diff-а: он перегенерируется каждый прогон, поэтому изменение в нём и есть изменение конфигурационной поверхности сервиса. Генерируется в **каждом** проекте: без `onlineconf.path` в нём только строки `origin: generated`.

> **Известный дефект.** Уровни логирования (`…/handler/default/log_level`, `…/worker/<имя>/job/log_level`) объявлены по той форме, которую собирает сгенерированный код, но сам логгер приписывает к переданному ему префиксу имя сервиса второй раз, поэтому фактическое чтение сейчас уходит на уровень глубже. Дефект отслеживается отдельной задачей; объявления описывают ту поверхность, к которой приведёт починка.

## Чего генератор не делает

Намеренно вне scope:

- **Значения ключей и политика сеяния.** Значение привязано к инсталляции, а решение о нём живёт рядом с решением, а не в генераторе.
- **OnlineConf admin и доставка дерева.** Это инфраструктура потребителя.
- **Отчёты и производные страницы по ключам.** Генерируется только реестр поверхности дерева (`configs/onlineconf-tree.yaml`); остальные производные страницы строятся из той же карты, но не генератором.
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
| `keys[0]: path "…" is a key the generator reads itself (generated by …)` | ключ уже читает сгенерированный код |
| `keys[0]: path "…" is inside "…", a subtree the generated code reads below under a segment it computes itself` | объявление внутри поддерева, которое генератор отдаёт себе |
| `generator onlineconf key declaration defect: …` | дефект объявлений самого генератора (не карты сервиса) — баг go-project-starter |
| `keys[0]: `required` and `default` contradict each other` | и `required`, и `default` |
| `keys[0]: type struct cannot be `required`: verifying it would mean unmarshaling the value` | `required` у `struct` |
| `keys[0]: type struct takes no `default`` | `default` у `struct` |
| `keys[0]: invalid default value: default "5 hours" is not a duration` | дефолт не парсится как объявленный тип |
| `keys[0]: name "my_key" is not an exported Go identifier` | `name` не называет функцию |
| `keys[0]: path "/svc/2fa" generates the accessor 2fa, which does not start with a letter` | из пути не выходит идентификатора — задайте `name` |
| `onlineconf accessor name collision: "/svc/max-size" and "/svc/max_size" both generate the accessor MaxSize` | два объявления сворачиваются в один идентификатор |

Валидация сообщает **все** проблемы сразу: починка плохой карты занимает один проход, а не по одному на запись.

## Миграция существующего проекта

При первом `make regenerate` после обновления генератора в проекте появятся:

- `internal/pkg/occonf/psg_occonf_gen.go` — пакет доступа к OnlineConf. Даже если `onlineconf.path` не задан: половину карты приносит генератор;
- `configs/golangci-onlineconf.yml` и цель `make onlineconf-validate`;
- `configs/onlineconf-tree.yaml` — реестр всей поверхности дерева;
- вызов `occonf.CheckRequired(mainCtx, "<app>")` в `cmd/<app>/psg_main_gen.go`.

**Фактические пути, типы и значения по умолчанию не меняются.** Сгенерированный код читает те же ключи, что и раньше, в том же порядке fallback-а — поменялось только то, через что он их читает. Инсталляциям перенастраиваться не нужно.

Что ломается — и так и задумано:

**1. Рукописный код, читающий OnlineConf напрямую, начнёт падать на `make onlineconf-validate`.**

```text
internal/app/svc/svc.go:42:14: use of `onlineconf.MakePath` forbidden …
```

Перенос: объявите ключ в карте сервиса (`onlineconf.path`), перегенерируйте и замените чтение на аксессор.

```go
// было
v, err := onlineconf.GetString(ctx, onlineconf.MakePath(constant.ServiceName, segConfluence, keyURL))

// стало: /myservice/confluence/url в карте, required: true
v, err := occonf.ConfluenceURL(ctx)
```

Ключ, который читает **сгенерированный** код, в карту сервиса объявлять не надо и нельзя — у него уже есть аксессор, см. `configs/onlineconf-tree.yaml` (`accessor` в строке `origin: generated`).

Подписка на изменения — `occonf.Subscribe(ctx, []string{occonf.PathX}, cb)`; путь ключа с параметрами даёт построитель `occonf.XPath(args…)`.

**2. Секция ручного кода ниже disclaimer-маркера в `psg_*_gen.go` больше не выведена из проверки.** Если там было прямое чтение — переносится так же.

**3. `GetConfigPath` у воркеров больше не принимает сегменты.** Было `GetConfigPath(key ...string)`, стало `GetConfigPath()` — она возвращает поддерево воркера, то есть префикс, который воркер отдаёт логгеру. Путь, собранный из сегментов на месте вызова, адресует узел, которого нет ни в одном объявлении и ни в одной строке реестра, — ровно то, что карта убирает. Если рукописный код звал `GetConfigPath("foo")`, объявите ключ в карте сервиса и читайте его аксессором.

`GetTgTokenPath` телеграм-драйвера сигнатуру сохранила (оба параметра теперь игнорируются) и возвращает `occonf.TelegramTokenPath()`.

**4. Два ключа генератора помечены `required`, и приложение без них теперь не стартует:** `/<svc>/security/tg_token` (телеграм-воркер) и `/<svc>/kafka/<client>/brokers` (Kafka-продюсер). Оба и раньше были фатальны — процесс поднимался и падал позже, при инициализации компонента; поменялось только **когда**: теперь на старте, с указанием пути. Проверяются они только в тех приложениях, которые соответствующий компонент запускают, и только если компонент не объявлен `optional: true`. Критерий и список — в разделе «`required` у ключей генератора» выше.

Остальные ключи генератора `required` не помечены: у них либо есть значение по умолчанию, либо их нужность зависит от настройки `project.yaml`.

**5. Dev-stand сеял токен телеграм-бота не туда.** `etc/onlineconf/dev/init-config.sql` создавал `tg_token`, `buffer_size` и `tg_api_endpoint` под `/<svc>/worker/<имя>/`, а драйвер читает их под `/<svc>/security/` (`configPath = "security"`). Сеялись мёртвые узлы; теперь они создаются там, где их читают. Если вы правили эти значения в своём dev-дереве — перенесите их в `security/` (или пересоздайте том MySQL, см. CLAUDE.md «Testing dev_stand Feature»).
