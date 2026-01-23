# Быстрый старт

Создайте свой первый микросервис за 5 минут.

## Шаг 1: Создайте файл конфигурации

Создайте файл `config.yaml`:

```yaml
main:
  name: myservice
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/myorg/myservice
  module_path: github.com/myorg/myservice

rest:
  - name: api
    path: [./api/openapi.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  - name: system
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

applications:
  - name: server
    transport: [api, system]
```

## Шаг 2: Запустите генератор

```bash
go-project-starter --config=config.yaml
```

## Шаг 3: Что вы получите

```
myservice/                    # ~50 файлов, ~8000 строк production-ready кода
├── cmd/server/              # Точка входа приложения с graceful shutdown
├── internal/
│   ├── app/                 # Обработчики транспорта и воркеры (ваш код здесь)
│   └── pkg/                 # Бизнес-логика и репозитории
├── pkg/
│   ├── app/                 # Runtime библиотеки (middleware, логирование, метрики)
│   └── drivers/             # Интеграции с внешними сервисами
├── api/                     # OpenAPI/Protobuf спецификации
├── configs/                 # Конфигурационные файлы (dev, staging, prod)
├── docker-compose.yaml      # Локальное окружение (Postgres, Redis, Traefik)
├── Dockerfile               # Multi-stage сборка (образ ~50MB)
├── Makefile                 # 40+ целей (build, test, lint, deploy)
└── .github/workflows/       # CI/CD (test, build, push в registry)
```

**Что генерируется:**

- ~50 файлов, готовых к запуску
- ~8000 строк production-grade кода
- Docker образ, оптимизированный до ~50MB
- Нулевые ошибки компиляции — компилируется с первого раза
- Структура тестов для вашей бизнес-логики

## Шаг 4: Начните разработку

```bash
cd myservice
make docker-up              # Запуск зависимостей (Postgres, Redis и т.д.)
make generate               # Генерация кода из OpenAPI спецификаций
make test                   # Запуск тестов
make run                    # Запуск сервиса
```

**Доступ к сервису:**

- REST API: `http://localhost:8080`
- System endpoints: `http://localhost:9090` (health, metrics, pprof)
- Prometheus метрики: `http://localhost:9090/metrics`

## Режим dry-run

Перед генерацией можно посмотреть, какие изменения будут внесены:

```bash
go-project-starter --dry-run --config=config.yaml --target=./my-service
```

## Регенерация проекта

При изменении конфигурации или OpenAPI спецификации просто перезапустите генератор:

```bash
go-project-starter --config=config.yaml --target=./my-service
```

!!! note "Важно"
    Ваш код ниже disclaimer-маркеров будет сохранён:

    ```go
    // ==========================================
    // GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
    // Changes manually made below will not be overwritten by generator.
    // ==========================================

    func (h *Handler) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
        // Ваша бизнес-логика здесь - она переживёт регенерацию!
    }
    ```

## Интерактивный wizard

Для нового проекта можно использовать интерактивный wizard:

```bash
go-project-starter init --target=.
```

Wizard поможет:

1. Указать имя проекта
2. Выбрать тип логгера
3. Настроить Git репозиторий
4. Выбрать тип проекта (REST API, gRPC, Telegram бот)

## Следующие шаги

- [Архитектура](../architecture/index.md) — понимание структуры проекта
- [Конфигурация](../configuration/index.md) — полное руководство по настройке
- [Примеры](../examples/index.md) — готовые примеры для разных сценариев
