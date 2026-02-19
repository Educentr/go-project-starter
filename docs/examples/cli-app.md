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
  golang_version: "1.24"

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
- **PrintHelp()** — вывод справки

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

## Сравнение подходов

| Без спецификации | Со спецификацией (`commands.yaml`) |
|------------------|------------------------------------|
| Ручная регистрация команд | Автоматическая из YAML |
| Нет типизированных параметров | Params structs для каждой команды |
| Нет UnimplementedCLI | Компилируется сразу без user code |
| Нет валидации флагов | required-валидация из спецификации |
