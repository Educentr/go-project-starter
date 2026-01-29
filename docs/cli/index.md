# CLI

Описание командного интерфейса Go Project Starter.

## Содержание раздела

- [Команды](commands.md) — доступные команды (init, setup, migrate)
- [Параметры](options.md) — параметры запуска генератора

## Обзор

Go Project Starter предоставляет CLI для генерации и настройки проектов:

```bash
go-project-starter [command] [flags]
```

## Основные команды

| Команда | Описание |
|---------|----------|
| (без команды) | Генерация проекта из конфигурации |
| `init` | Интерактивный wizard для создания конфигурации |
| `setup` | Настройка CI/CD, серверов, deploy скриптов |
| `migrate` | Миграция конфигурации на новую версию |

## Быстрый старт

```bash
# Установка
go install github.com/Educentr/go-project-starter/cmd/go-project-starter@latest

# Справка
go-project-starter --help

# Генерация проекта
go-project-starter --config=config.yaml

# Интерактивный wizard
go-project-starter init --target=.
```

## Следующие шаги

- [Команды](commands.md) — подробное описание каждой команды
- [Параметры](options.md) — все доступные флаги
