# Тестирование

Описание тестирования сгенерированных проектов.

## Содержание раздела

- [GOAT](goat.md) — интеграционные тесты с GOAT фреймворком

## Обзор

Сгенерированные проекты поддерживают несколько уровней тестирования:

| Уровень | Команда | Описание |
|---------|---------|----------|
| Unit тесты | `make test` | Быстрые тесты без внешних зависимостей |
| Integration тесты | `make goat-tests` | E2E тесты с реальными зависимостями |

## Unit тесты

```bash
# Запуск тестов с coverage
make test

# С race detector
make race

# Генерация HTML отчёта
make coverage
```

## GOAT Integration тесты

GOAT (Go Application Testing) — фреймворк для интеграционного тестирования.

### Включение

```yaml
applications:
  - name: api
    goat_tests: true
    transport: [api, sys]
```

### Запуск

```bash
# Сборка и запуск тестов
make goat-tests

# С подробным выводом
make goat-tests-verbose
```

### Что тестируется

- Реальные HTTP endpoints
- Реальная база данных (testcontainers)
- Реальные внешние API (mock-серверы)

## Следующие шаги

- [GOAT](goat.md) — подробное руководство по интеграционным тестам
