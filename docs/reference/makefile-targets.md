# Makefile Targets Reference

Полный справочник целей Makefile сгенерированного проекта.

## Разработка

| Цель | Описание |
|------|----------|
| `make run` | Запуск приложения |
| `make build` | Сборка бинарника |
| `make generate` | Все генераторы (ogen, argen, mock) |
| `make ogen` | Генерация REST из OpenAPI |
| `make argen` | Генерация ActiveRecord |
| `make mock` | Генерация моков |

## Тестирование

| Цель | Описание |
|------|----------|
| `make test` | Unit тесты с coverage |
| `make race` | Тесты с race detector |
| `make coverage` | HTML отчёт coverage |
| `make goat-tests` | GOAT интеграционные тесты |
| `make goat-tests-verbose` | GOAT тесты с подробным выводом |
| `make build_for_test-{app}` | Сборка бинарника для тестов |

## Линтинг

| Цель | Описание |
|------|----------|
| `make lint` | Проверка изменений относительно main |
| `make lint-full` | Полный lint всего кода |
| `make lint-fix` | Автоисправление lint ошибок |
| `make install-lint` | Установка golangci-lint |

## Docker

| Цель | Описание |
|------|----------|
| `make docker-build` | Сборка Docker образа |
| `make docker-build-{app}` | Сборка образа конкретного приложения |
| `make docker-push` | Push образа в registry |
| `make docker-up` | Запуск docker-compose |
| `make docker-down` | Остановка docker-compose |
| `make docker-logs` | Просмотр логов |

## Dev Stand

Доступны при `dev_stand: true`:

| Цель | Описание |
|------|----------|
| `make dev-up` | Запуск dev окружения с OnlineConf |
| `make dev-down` | Остановка dev окружения |
| `make dev-restart` | Перезапуск |
| `make dev-rebuild` | Пересборка приложений |
| `make dev-drop` | Полный сброс (удаление volumes) |

## Миграции БД

| Цель | Описание |
|------|----------|
| `make migrate-up` | Применить миграции |
| `make migrate-down` | Откатить миграции |
| `make migrate-status` | Статус миграций |
| `make migrate-create` | Создать новую миграцию |

## Системные пакеты

Доступны при `artifacts: [deb, rpm, apk]`:

| Цель | Описание |
|------|----------|
| `make packages` | Сборка всех пакетов |
| `make deb-{app}` | Сборка .deb для приложения |
| `make rpm-{app}` | Сборка .rpm |
| `make apk-{app}` | Сборка .apk |
| `make install-nfpm` | Установка nfpm |
| `make upload-packages` | Загрузка пакетов |
| `make upload-deb` | Загрузка только .deb |
| `make upload-rpm` | Загрузка только .rpm |
| `make upload-apk` | Загрузка только .apk |

## Утилиты

| Цель | Описание |
|------|----------|
| `make clean` | Очистка артефактов сборки |
| `make help` | Справка по целям |
| `make version` | Показать версию |

## Переменные

### Сборка

```makefile
GOPROXY=...              # Go proxy
GOPRIVATE=...            # Приватные модули
CGO_ENABLED=0            # Статическая сборка
```

### Docker

```makefile
DOCKER_REGISTRY=...      # URL registry
DOCKER_TAG=...           # Тег образа
```

### Dev Stand

```makefile
DEV_ONLINECONF_ADMIN_PORT=8888
DEV_TRAEFIK_PORT=9080
DEV_POSTGRES_PORT=5432
DEV_GRAFANA_PORT=3000
```

## Кастомизация

Добавьте свои цели ниже disclaimer-маркера:

```makefile
# ==========================================
# GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
# ==========================================

.PHONY: my-custom-target
my-custom-target:
    @echo "Custom target"
```
