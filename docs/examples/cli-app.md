# CLI приложение

Пример CLI приложения с генерацией команд из YAML-спецификации.

## Конфигурация

### project.yaml

```yaml
main:
  name: myctl
  logger: zerolog
  registry_type: github

git:
  repo: github.com/myorg/myctl
  module_path: github.com/myorg/myctl

tools:
  golang_version: "1.27"

cli:
  - name: admin
    path:
      - ./commands.yaml
    generator_type: template
    generator_template: cli

applications:
  - name: admin-cli
    cli: admin
```

### commands.yaml

```yaml
commands:
  - name: user
    description: "User management"
    subcommands:
      - name: create
        description: "Create a new user"
        flags:
          - name: email
            type: string
            required: true
            description: "User email"
          - name: name
            type: string
            description: "User name"
      - name: list
        description: "List all users"
        flags:
          - name: limit
            type: int
            default: "100"
            description: "Max results"

  - name: ping
    description: "Check connectivity"

  - name: migrate
    description: "Database migrations"
    flags:
      - name: dir
        type: string
        default: "up"
        description: "Direction: up or down"
      - name: steps
        type: int
        description: "Number of steps"
```

## Что генерируется

```
internal/app/transport/cli/admin/
├── psg_handler_gen.go     # Сгенерированный handler (не редактировать)
└── user.go                # Пользовательский код (создаётся вручную)
```

### Сгенерированный код (`psg_handler_gen.go`)

Содержит:

- **Params structs** — `UserCreateParams`, `UserListParams`, `MigrateParams`
- **UnimplementedCLI** — default-реализация всех команд
- **Handler** — struct с embedding `UnimplementedCLI` и ссылкой на `Service`
- **registerCommands()** — регистрация команд с flag parsing
- **Execute()** — диспетчеризация команд и подкоманд
- **PrintHelp()** — список команд с кратким описанием
- **PrintCommandHelp() / PrintSubcommandHelp()** — детальная справка (полное
  описание + флаги)

## Справка (help)

Два уровня вывода:

| Что | Где | Что печатается |
|-----|-----|----------------|
| Список команд | запуск без аргументов, `help` | по одной строке на команду — **первая строка** описания (длинная обрезается по границе слова) |
| Детальная справка | `<command> --help`, `<command> <subcommand> --help`, `help <command>`, `help <command> <subcommand>` | **полное** описание (включая многострочное), usage и список флагов |

```bash
./myctl                        # usage + список команд
./myctl user --help            # описание + список подкоманд
./myctl user create --help     # полное описание подкоманды + её флаги
./myctl help user create       # то же самое
```

Поддерживаются `--help`, `-help` и `-h`. Справка пишется в writer хендлера
(по умолчанию `os.Stdout`, меняется через `Handler.SetOutput`), что позволяет
проверять её в тестах.

### Описания — свободный текст, имена — нет

- `description` (команд, подкоманд, флагов) — свободный текст: кавычки,
  обратные слэши и многострочные блоки (`description: |`) допустимы, генератор
  экранирует их при вставке в Go-литерал.
- `name` (команд, подкоманд, флагов) — только `[a-zA-Z][a-zA-Z0-9_-]*`: имя
  становится частью Go-идентификатора (`RunUserCreate`, `UserCreateParams`).
  Невалидное имя — ошибка на этапе загрузки конфига, а не сломанная генерация.
- `default` проверяется по типу флага (`int`, `bool`, `float64`, `duration`),
  для `duration` принимается формат Go (`5s`, `1m30s`).

### Повторённый флаг — ошибка

Все типы флагов в спецификации скалярные: списочного типа нет, список передаётся одним
значением (CSV). Поэтому второе вхождение флага — не добавление, и генератор его отвергает:

```console
$ ./myctl migrate --dir up --dir down
flag --dir is specified more than once
```

Стандартный `flag.FlagSet` в этом случае оставляет **последнее** значение и молча теряет
остальные — для CLI это худший из возможных ответов. Проверка идёт до `fs.Parse` и повторяет
его же правила: один или два дефиса, `--` и первый не-флаговый аргумент заканчивают флаги,
`--name=value` несёт значение в себе, любой другой небулев флаг забирает следующий аргумент
(поэтому `--steps -1` — это значение, а не флаг). Неизвестное имя остаётся на `fs.Parse` — он
сообщит о нём своими словами.

### Пользовательский код

Создайте файлы в том же пакете для реализации логики:

```go
// user.go
package admin

import (
    "context"
    "fmt"
)

func (h *Handler) RunUserCreate(ctx context.Context, params UserCreateParams) error {
    fmt.Printf("Creating user: email=%s, name=%s\n", params.Email, params.Name)
    // user, err := h.GetService().CreateUser(ctx, params.Email, params.Name)
    return nil
}

func (h *Handler) RunUserList(ctx context.Context, params UserListParams) error {
    fmt.Printf("Listing users (limit=%d)\n", params.Limit)
    return nil
}
```

```go
// ping.go
package admin

import (
    "context"
    "fmt"
)

func (h *Handler) RunPing(ctx context.Context) error {
    fmt.Println("pong")
    return nil
}
```

## Использование

```bash
# Компиляция
go build -o myctl ./cmd/admin-cli

# Команды
./myctl help
./myctl user create --help
./myctl user create --email admin@example.com --name "Admin"
./myctl user list --limit 50
./myctl ping
./myctl migrate --dir up --steps 3
```

## С драйверами

CLI приложение может использовать драйверы (PostgreSQL, Redis и др.):

```yaml
driver:
  - name: postgres
    import: github.com/myorg/myctl/pkg/postgres
    package: postgres
    obj_name: Client

applications:
  - name: admin-cli
    cli: admin
    driver:
      - name: postgres
```

## С API-клиентом (`ogen_client`)

CLI-инструмент, который вызывает HTTP-API (ops-скрипт, бот, обёртка над сервисом), объявляет
клиента так же, как любое другое приложение — ссылкой в `transport`:

```yaml
rest:
  - name: billing
    path:
      - ./billing.swagger.yml
    version: v1
    generator_type: ogen_client

cli:
  - name: admin
    path:
      - ./commands.yaml
    generator_type: template
    generator_template: cli

applications:
  - name: admin-cli
    cli: admin
    transport:
      - name: billing   # только клиентские транспорты (generator_type: ogen_client)
```

Что это даёт:

- Клиент инициализируется в `main` самой CLI и попадает в `Service` — handlers обращаются к нему
  через `h.GetService()`, как в сервисном приложении. CLI не проходит через `app.Init` рантайма
  (нет ни HTTP-листенера, ни реестра метрик), поэтому клиент ей передаёт сгенерированный
  `EmptyService.InitClients`.
- Приложению с CLI разрешены **только** клиентские транспорты. Ссылка на REST/gRPC-сервер — ошибка
  загрузки конфига: слушать в CLI нечему.

Адрес клиента читается из OnlineConf (`/<service>/transport/rest/<client>/host`). Если он не задан,
команда падает на старте с `<client> client URL not configured` — это та же проверка, что у
сервисного приложения. Для CLI, которой клиент нужен не в каждой команде, объявите транспорт
опциональным — тогда незаданный адрес даёт `nil`-клиент вместо ошибки старта:

```yaml
applications:
  - name: admin-cli
    cli: admin
    transport:
      - name: billing
        config:
          optional: true
```

Клиент с `instantiation: dynamic` в `main` не регистрируется: его создаёт место вызова, как и в
сервисном приложении.

При `main.skip_service_init: true` Service-слой пишет пользователь, и `EmptyService.InitClients`
не генерируется. Если CLI хостит статический клиент, `main` CLI всё равно вызывает
`(*service.EmptyService).InitClients(ctx, []ds.ClientInitializer) error`: этот метод (как и сам
тип `EmptyService`, на который опирается `main` сервисного приложения) нужно объявить самому.

## Обязательные ключи OnlineConf (`CheckRequired`)

`main` CLI-приложения инициализирует OnlineConf (`occonf.Initialize` — дерево доступно всем
командам), но `occonf.CheckRequired` **не вызывает, намеренно**. Ключи карты сервиса
(`onlineconf.path`) проверяются в каждом приложении, потому что у карты нет измерения
приложений, — и CLI не стартовала бы из-за `required`-ключа, который читает только сервис. Плюс
короткоживущей команде не обязательно нужна вся конфигурационная поверхность сервиса.

Это **не** та проверка, что `<client> client URL not configured` выше: ту делает `InitClients`,
когда адрес статического клиента не задан, и к `CheckRequired` она отношения не имеет — как и
`optional: true`, который настраивает именно инициализацию клиента.

Нужна проверка обязательных ключей — вызовите её сами в обработчике команды или в общем коде
команд, то есть после `occonf.Initialize`:

```go
if err := occonf.CheckRequired(ctx, "admin-cli"); err != nil {
	return err
}
```

Подробнее — [«`CheckRequired` — проверка на старте»](../configuration/onlineconf-keys.md#checkrequired--проверка-на-старте).

## Сравнение подходов

| Без спецификации | Со спецификацией (`commands.yaml`) |
|------------------|------------------------------------|
| Ручная регистрация команд | Автоматическая из YAML |
| Нет типизированных параметров | Params structs для каждой команды |
| Нет UnimplementedCLI | Компилируется сразу без user code |
| Нет валидации флагов | required-валидация из спецификации |
