# LLMS.md — Документ для AI-агентов

## Назначение

`LLMS.md` — документ-инструкция для AI-кодинг агентов (Claude Code, Codex, Cursor и др.), который генерируется в корень проекта. После прочтения агент понимает архитектуру, правила и может работать без дополнительных вопросов.

## Включение генерации

Добавьте в `project.yaml`:

```yaml
main:
  name: myproject
  generate_llms_md: true
```

По умолчанию генерация выключена (`false`).

## Поведение при регенерации

`LLMS.md` генерируется **один раз**. При повторном запуске `make regenerate` файл **не перезаписывается** (аналогично `README.md` и `LICENSE.txt`). Это позволяет добавлять в него проектно-специфичные правила.

Файл **не содержит disclaimer-маркеров** — он полностью редактируемый.

## Структура генерируемого документа

### Статические секции (всегда присутствуют)

| Секция | Описание |
|--------|----------|
| **Project Overview** | Имя проекта, module path, основные технологии |
| **Critical Rules** | Запрет редактирования `_gen` файлов, правила disclaimer-маркеров |
| **Three-layer architecture** | Описание `pkg/`, `internal/pkg/`, `internal/app/` |
| **Where to Put Code** | Таблица: что куда класть (handlers, service, models, constants) |
| **Build Commands** | `make build`, `make test`, `make lint`, `make generate`, `make regenerate` |
| **Code Generation** | Когда какую команду запускать |
| **Service Layer** | Паттерн единого сервиса для бизнес-логики |
| **OnlineConf** | Иерархия путей конфигурации, правила |
| **Applications** | Список приложений с транспортами, воркерами, драйверами |
| **Project Structure** | Дерево каталогов проекта |

### Динамические секции (по конфигу)

| Условие | Секция |
|---------|--------|
| `use_active_record: true` | **ActiveRecord** — `decl/`, `cmpl/`, `make generate-argen` |
| Есть REST транспорты | **REST API** — ogen, спецификации, паттерн хендлеров |
| Есть gRPC транспорты | **gRPC** — proto файлы, генерация |
| Есть workers | **Workers** — типы, обязательный `enabled` флаг, OnlineConf пути |
| Есть Kafka | **Kafka** — producers/consumers, конфигурация |
| `dev_stand: true` | **Dev Environment** — docker-compose-dev, OnlineConf UI |
| Есть GOAT тесты | **Integration Tests** — правила тестирования |

## Кастомизация после генерации

После первой генерации рекомендуется добавить проектно-специфичные секции:

### Что стоит добавить

1. **Бизнес-правила** — специфичные для проекта ограничения (например, «суммы в копейках», «воркеры не должны быть дольше 30 секунд»)
2. **Внешние сервисы** — описание интеграций (платёжные провайдеры, сторонние API)
3. **Тестирование** — как запускать конкретные тесты, какие моки используются
4. **Среда разработки** — дополнительные зависимости, секреты
5. **Правила контрактов** — если контракты живут в отдельном репозитории

### Пример дополнения

```markdown
## Payment System

- Providers: Pike (kopecks), Wata (rubles) — different amount formats!
- Provider queue in OnlineConf: `/myproject/payment/providers_queue`
- Webhook handlers: `internal/app/transport/rest/webhooks_*/v1/handler/`

## Running Specific Tests

```bash
make goat-tests-api-run TEST=TestPaymentTestSuite/TestPaymentLifecycle
```
```

## Язык документа

Документ генерируется на **английском** языке для максимальной совместимости с AI-агентами. При необходимости можно добавить русскоязычные секции вручную после генерации.
