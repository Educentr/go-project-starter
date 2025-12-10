# Быстрый старт

## Установка

```bash
go install github.com/Educentr/go-project-starter/cmd/go-project-starter@latest
```

## Создание первого сервиса

### 1. Создайте файл конфигурации (`config.yaml`)

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

### 2. Запустите генератор

```bash
go-project-starter --config=config.yaml
```

### 3. Что вы получите

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
- Нулевые ошибки компиляции - компилируется с первого раза
- Структура тестов для вашей бизнес-логики

### 4. Начните разработку

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

**Важно:** Ваш код ниже disclaimer-маркеров будет сохранён:

```go
// ==========================================
// GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
// Changes manually made below will not be overwritten by generator.
// ==========================================

func (h *Handler) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
    // Ваша бизнес-логика здесь - она переживёт регенерацию!
}
```

## Следующие шаги

- [Архитектура](architecture.md) - понимание структуры проекта
- [Конфигурация](configuration.md) - полное руководство по настройке
- [Примеры](examples.md) - готовые примеры для разных сценариев
