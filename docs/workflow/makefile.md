# Makefile

Описание целей Makefile сгенерированного проекта.

## Обзор

Каждый сгенерированный проект включает Makefile с 40+ целями для разработки, тестирования и деплоя.

## Основные команды

### Разработка

| Команда | Описание |
|---------|----------|
| `make run` | Запуск приложения |
| `make build` | Сборка бинарника |
| `make generate` | Запуск всех генераторов (ogen, argen, mock) |
| `make docker-up` | Запуск зависимостей в Docker |
| `make docker-down` | Остановка зависимостей |

### Тестирование

| Команда | Описание |
|---------|----------|
| `make test` | Запуск unit тестов с coverage |
| `make race` | Запуск тестов с race detector |
| `make goat-tests` | Запуск интеграционных GOAT тестов |
| `make goat-tests-verbose` | GOAT тесты с подробным выводом |

### Линтинг

| Команда | Описание |
|---------|----------|
| `make lint` | Проверка изменений относительно main |
| `make lint-full` | Полный lint всего кода |
| `make lint-fix` | Автоисправление lint ошибок |
| `make install-lint` | Установка golangci-lint |

### Docker

| Команда | Описание |
|---------|----------|
| `make docker-build` | Сборка Docker образа |
| `make docker-push` | Push образа в registry |
| `make docker-up` | Запуск docker-compose |
| `make docker-down` | Остановка docker-compose |

### Dev Stand (при `dev_stand: true`)

| Команда | Описание |
|---------|----------|
| `make dev-up` | Запуск dev окружения с OnlineConf |
| `make dev-down` | Остановка dev окружения |
| `make dev-restart` | Перезапуск dev окружения |
| `make dev-rebuild` | Пересборка и перезапуск приложений |
| `make dev-drop` | Полный сброс (удаление volumes) |

### Миграции БД

| Команда | Описание |
|---------|----------|
| `make migrate-up` | Применить миграции |
| `make migrate-down` | Откатить миграции |
| `make migrate-status` | Статус миграций |

### Генерация кода

| Команда | Описание |
|---------|----------|
| `make generate` | Все генераторы |
| `make ogen` | Генерация REST из OpenAPI |
| `make argen` | Генерация ActiveRecord |
| `make mock` | Генерация моков |

### Системные пакеты (при `artifacts: [deb, rpm, apk]`)

| Команда | Описание |
|---------|----------|
| `make packages` | Сборка всех пакетов |
| `make deb-{app}` | Сборка .deb для приложения |
| `make rpm-{app}` | Сборка .rpm для приложения |
| `make apk-{app}` | Сборка .apk для приложения |
| `make install-nfpm` | Установка nfpm |
| `make upload-packages` | Загрузка пакетов |

## Примеры использования

### Полный цикл разработки

```bash
# 1. Запуск зависимостей
make docker-up

# 2. Генерация кода
make generate

# 3. Запуск тестов
make test

# 4. Запуск приложения
make run
```

### Подготовка к коммиту

```bash
# 1. Проверка lint
make lint

# 2. Запуск тестов
make test

# 3. Commit
git add .
git commit -m "Feature: add user endpoint"
```

### Сборка и деплой

```bash
# 1. Сборка Docker образа
make docker-build

# 2. Push в registry
make docker-push

# 3. Или сборка системных пакетов
make packages
make upload-packages
```

### Dev Stand с OnlineConf

```bash
# 1. Запуск полного окружения
make dev-up

# 2. После изменений кода
make dev-rebuild

# 3. Полный сброс (при изменении SQL)
make dev-drop
make dev-up
```

## Переменные окружения

### Сборка

| Переменная | Описание |
|------------|----------|
| `GOPROXY` | Go proxy для ускорения сборки |
| `GOPRIVATE` | Приватные Go модули |

### Docker

| Переменная | Описание |
|------------|----------|
| `DOCKER_REGISTRY` | URL Docker registry |
| `DOCKER_TAG` | Тег образа |

### Dev Stand

| Переменная | Описание |
|------------|----------|
| `DEV_ONLINECONF_ADMIN_PORT` | Порт OnlineConf Admin UI (default: 8888) |
| `DEV_TRAEFIK_PORT` | Порт Traefik dashboard (default: 9080) |
| `DEV_POSTGRES_PORT` | Порт PostgreSQL (default: 5432) |
| `DEV_GRAFANA_PORT` | Порт Grafana (default: 3000) |

## Кастомизация

Makefile генерируется с disclaimer-маркером. Вы можете добавить свои цели ниже маркера:

```makefile
# ==========================================
# GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
# ==========================================

# Ваши кастомные цели
.PHONY: my-custom-target
my-custom-target:
    @echo "Custom target"
```
