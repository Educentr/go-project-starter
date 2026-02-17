# OnlineConf

Интеграция с OnlineConf для динамической конфигурации.

## Обзор

OnlineConf позволяет изменять конфигурацию без редеплоя сервиса.

Сгенерированные проекты используют OnlineConf для:

- Настройки транспортов (порты, таймауты)
- Параметров базы данных
- Feature flags
- Секретов и credentials

## Структура путей

```
{service_name}/
├── devstand                    # 0 или 1
├── log/
│   └── level                   # info, debug, error
├── transport/
│   └── rest/
│       └── {name}_{version}/
│           ├── ip                  # 0.0.0.0
│           ├── port                # 8080
│           ├── timeout_read        # 30s (ReadTimeout + IdleTimeout)
│           ├── timeout_write       # 60s (WriteTimeout)
│           ├── timeout_read_header # 30s (ReadHeaderTimeout, default = timeout_read)
│           └── handler/
│               ├── default/
│               │   └── timeout     # 2s (default handler timeout)
│               └── {path}/
│                   └── timeout     # per-path handler timeout
├── db/
│   └── main/
│       ├── host                # localhost:5432
│       ├── User
│       ├── Password
│       └── DB
└── security/
    ├── csrf/
    │   └── enabled             # 0 или 1
    └── httpAuth/
        └── enabled             # 0 или 1
```

## Иерархия приоритетов

OnlineConf пути следуют 3-уровневой иерархии для REST транспортов:

1. **Default from code** — значения по умолчанию в коде
2. **Transport-level**: `/{serviceName}/transport/rest/{transportName}/{key}`
3. **App-specific**: `/{serviceName}/transport/rest/{transportName}/{appName}/{key}`

### Пример

Для сервиса `my-api` с application `web-app` и transport `api_v1`:

```
# Transport-level (для всех apps с этим транспортом)
/my-api/transport/rest/api_v1/timeout_read
/my-api/transport/rest/api_v1/timeout_write
/my-api/transport/rest/api_v1/port

# App-specific (override для конкретного application)
/my-api/transport/rest/api_v1/web-app/timeout_read
/my-api/transport/rest/api_v1/web-app/timeout_write
/my-api/transport/rest/api_v1/web-app/port
```

## Динамическое обновление таймаутов

Серверные таймауты (`timeout_read`, `timeout_write`, `timeout_read_header`) подписаны на изменения в OnlineConf через `RegisterSubscription`. При изменении значения в OnlineConf Admin UI:

1. Callback обновляет `http.Server` поля без рестарта
2. В логе появляется сообщение "REST server timeouts updated"
3. Новые значения применяются к следующим соединениям

## Handler Timeouts

Handler timeout — таймаут на обработку одного HTTP-запроса. Устанавливается через `context.WithTimeout` в middleware.

### 3-level fallback

1. **Per-path app-specific**: `/{svc}/transport/rest/{transport}/{app}/handler/{urlPath}/timeout`
2. **Per-path transport-level**: `/{svc}/transport/rest/{transport}/handler/{urlPath}/timeout`
3. **Default app-specific**: `/{svc}/transport/rest/{transport}/{app}/handler/default/timeout`
4. **Default transport-level**: `/{svc}/transport/rest/{transport}/handler/default/timeout`
5. **Code default**: `2s`

### Валидация handler timeout vs write timeout

Если handler timeout превышает server write timeout, он автоматически уменьшается до write timeout с предупреждением в логе. Это предотвращает ситуацию, когда handler продолжает работать после того, как сервер уже закрыл соединение.

## Переменные окружения

OnlineConf значения могут передаваться через переменные окружения:

```bash
# Core settings
OC_{ProjectName}__devstand=0
OC_{ProjectName}__log__level=info

# REST API settings
OC_{ProjectName}__transport__rest__{name}_{version}__ip=0.0.0.0
OC_{ProjectName}__transport__rest__{name}_{version}__port=8081
OC_{ProjectName}__transport__rest__{name}_{version}__timeout_read=30s
OC_{ProjectName}__transport__rest__{name}_{version}__timeout_write=60s

# Database settings
OC_{ProjectName}__db__main=127.0.0.1:5432
OC_{ProjectName}__db__main__User=myproject
OC_{ProjectName}__db__main__Password=password
OC_{ProjectName}__db__main__DB=myproject

# Security
OC_{ProjectName}__security__csrf__enabled=0
OC_{ProjectName}__security__httpAuth__enabled=0
```

## Dev Stand

При включении `dev_stand: true` генерируется полное локальное окружение с OnlineConf.

### Конфигурация

```yaml
main:
  name: myproject
  dev_stand: true

post_generate:
  - git_install    # Обязательно для dev_stand
  - call_generate
  - go_mod_tidy
```

### Запуск

```bash
make dev-up
```

### Доступ

| Сервис | URL | Credentials |
|--------|-----|-------------|
| OnlineConf Admin | http://localhost:8888 | admin / admin |
| Traefik Dashboard | http://localhost:9080 | - |
| Application API | http://localhost:{port} | - |

### Структура dev-окружения

```
┌─────────────────────────────────────────────────────────────┐
│                docker-compose-dev.yaml                       │
├─────────────────────────────────────────────────────────────┤
│  OnlineConf Stack:                                          │
│  onlineconf-database  - MySQL для хранения конфигурации     │
│  onlineconf-admin     - Web UI для редактирования           │
│  onlineconf-updater   - синхронизация в TREE.cdb файлы      │
├─────────────────────────────────────────────────────────────┤
│  traefik              - reverse proxy                        │
│  {app1}               - приложение 1                         │
│  {app2}               - приложение 2                         │
├─────────────────────────────────────────────────────────────┤
│  Infrastructure (опционально):                               │
│  postgres             - если use_active_record: true         │
│  grafana              - если есть grafana.datasources        │
│  prometheus           - если есть prometheus datasource      │
│  loki                 - если есть loki datasource            │
└─────────────────────────────────────────────────────────────┘
```

### Редактирование конфигурации

1. Откройте http://localhost:8888
2. Войдите как admin / admin
3. Отредактируйте конфигурацию в дереве
4. Изменения автоматически синхронизируются в `TREE.cdb`

### Сброс конфигурации

При изменении SQL-шаблонов нужно удалить MySQL volume:

```bash
make dev-drop
make dev-up
```

## Чтение конфигурации в коде

```go
// Чтение значений
maxRetries := onlineconf.GetInt("myservice.api.max_retries")
timeout := onlineconf.GetDuration("myservice.api.timeout")
enabled := onlineconf.GetBool("myservice.feature.enabled")

// Автоматически перезагружается при изменении в OnlineConf
```

## Kafka конфигурация

| Путь | Описание |
|------|----------|
| `{service}/kafka/{client}/brokers` | Список брокеров |
| `{service}/kafka/{client}/auth_type` | none, PLAIN, SCRAM-SHA-* |
| `{service}/kafka/{client}/username` | SASL username |
| `{service}/kafka/{client}/password` | SASL password |
| `{service}/kafka/{client}/tls_enabled` | 0 или 1 |
| `{service}/kafka/{client}/events/{event}/topic` | Topic name |

## Troubleshooting

### OnlineConf updater не создаёт TREE.cdb

```bash
# Проверить логи updater
docker compose -f docker-compose-dev.yaml logs onlineconf-updater

# Проверить что admin UI доступен
curl http://localhost:8888

# Проверить что MySQL готов
docker compose -f docker-compose-dev.yaml logs onlineconf-database
```

### Изменения в init-config.sql не применяются

MySQL выполняет init-скрипты только при первом запуске:

```bash
make dev-drop
make dev-up
```
