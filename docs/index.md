# Go Project Starter — Документация

> Превращайте API спецификации в production-ready микросервисы за минуты, а не дни.

## Что это?

**Go Project Starter** — это **генератор кода**, а не работающий микросервис. Он генерирует production-ready Go микросервисы из YAML конфигурационных файлов. Генератор использует 78+ встроенных шаблонов для создания ~8000 строк production-grade кода, включая REST API, gRPC сервисы, Kafka consumers, Telegram боты и полную инфраструктуру.

## Содержание документации

### Начало работы

| Раздел | Описание |
|--------|----------|
| [Быстрый старт](quick-start.md) | Установка и создание первого сервиса за 5 минут |
| [Архитектура](architecture.md) | Трёхслойная архитектура, Applications, драйверы |
| [Конфигурация](configuration.md) | Полное руководство по YAML конфигурации |

### Возможности

| Раздел | Описание |
|--------|----------|
| [Возможности](features.md) | Все возможности генератора |
| [Примеры](examples.md) | Готовые конфигурации для разных сценариев |
| [Продвинутые темы](advanced.md) | Драйверы, OnlineConf, мониторинг, Grafana |

### Справка

| Раздел | Описание |
|--------|----------|
| [Сравнение](comparison.md) | Сравнение с Goa, go-zero, Sponge и другими |
| [Участие в разработке](contributing.md) | Как внести вклад в проект |
| [Локальная разработка](dev-start.md) | Работа с docker-compose.dev.yaml |

## Быстрый старт

```bash
# Установка
go install github.com/Educentr/go-project-starter/cmd/go-project-starter@latest

# Генерация проекта
go-project-starter --config=config.yaml

# Запуск
cd myservice && make docker-up && make run
```

## Ключевые особенности

### Сохранение пользовательского кода

Перегенерируйте проект без потери ваших изменений:

```go
// ==========================================
// GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
// ==========================================

func (h *Handler) CreateUser(...) {
    // Ваш код здесь - переживёт регенерацию!
}
```

### Application-based масштабирование

```yaml
applications:
  - name: api
    transport: [rest, grpc]
  - name: workers
    workers: [telegram_bot]
    kafka: [events]
```

### Production-ready из коробки

- Docker multi-stage builds (~50MB образы)
- GitHub Actions CI/CD
- Traefik reverse proxy
- Prometheus метрики
- Grafana dashboards
- Structured logging (zerolog)
- Graceful shutdown

## Что генерируется

```
myservice/
├── cmd/server/              # Entry point
├── internal/
│   ├── app/                 # Ваш код
│   └── pkg/                 # Бизнес-логика
├── pkg/                     # Runtime библиотеки
├── api/                     # OpenAPI/Proto specs
├── docker-compose.yaml
├── Dockerfile
├── Makefile                 # 40+ целей
└── .github/workflows/       # CI/CD
```

- ~50 файлов, готовых к запуску
- ~8000 строк production-grade кода
- Компилируется с первого раза

## Дополнительные материалы

- [Архитектурные заметки](arch.md) — детальное описание слоёв, драйверов, транспортов
- [GOAT тесты](goat-tests.md) — тестирование генератора
- [TODO](todo.md) — планы развития
