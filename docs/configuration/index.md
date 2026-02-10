# Конфигурация

Полное руководство по YAML-конфигурации Go Project Starter.

## Содержание раздела

- [Основные секции](main.md) — секции main, git, tools
- [Транспорты](transports.md) — REST, gRPC, Kafka, CLI
- [Workers](workers.md) — Telegram, Daemon, Queue
- [Queue Contract](queue-contract.md) — Контракт очередей
- [Applications](applications.md) — Applications, drivers
- [Инфраструктура](infrastructure.md) — Grafana, artifacts, deploy

## Базовая структура

```yaml
main:
  name: string              # Имя проекта
  logger: zerolog           # Тип логгера
  registry_type: github     # Container registry
  use_active_record: bool   # Включить ORM

git:
  repo: string              # URL Git репозитория
  module_path: string       # Go module path
  private_repos: []         # Приватные Go модули

rest:
  - name: string            # Имя транспорта
    path: []                # Пути к OpenAPI спецификациям
    generator_type: ogen    # Тип генератора
    port: int               # HTTP порт
    version: string         # Версия API

grpc:
  - name: string            # Имя сервиса
    path: []                # Пути к Protobuf файлам
    port: int               # gRPC порт

kafka:
  - name: string            # Имя producer/consumer
    type: producer          # producer или consumer
    events: []              # События

workers:
  - name: string            # Имя воркера
    generator_type: telegram # Тип: telegram/daemon/queue

applications:
  - name: string            # Имя приложения
    transport: []           # REST/gRPC транспорты
    workers: []             # Воркеры
    drivers: []             # Драйверы

grafana:
  datasources: []           # Datasources для Grafana

artifacts:
  - docker                  # Типы артефактов

packaging:
  maintainer: string        # Maintainer пакетов
  description: string       # Описание
```

## Минимальная конфигурация

```yaml
main:
  name: myservice
  logger: zerolog

git:
  repo: github.com/myorg/myservice
  module_path: github.com/myorg/myservice

rest:
  - name: api
    path: [./api/openapi.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  - name: sys
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

applications:
  - name: server
    transport: [api, sys]
```

## Правила валидации

1. **REST/gRPC сервисы должны быть назначены в applications**
2. **Драйверы, на которые есть ссылки, должны существовать**
3. **Имена не должны дублироваться** (REST, gRPC, drivers)
4. **ActiveRecord требует указания ArgenVersion**
5. **Registry type: только `github`, `digitalocean`, `aws`, или `selfhosted`**
6. **Порты обязательны для REST (кроме шаблона `sys`)**

## Следующие шаги

Перейдите к детальному описанию каждой секции:

- [Основные секции](main.md) — main, git, tools
- [Транспорты](transports.md) — REST, gRPC, Kafka, CLI
- [Workers](workers.md) — Telegram, Daemon, Queue
- [Queue Contract](queue-contract.md) — Контракт очередей
- [Applications](applications.md) — Applications, drivers
- [Инфраструктура](infrastructure.md) — Grafana, artifacts, deploy
