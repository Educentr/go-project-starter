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
├── default/
│   └── log_level               # info, debug, error (процессный логгер)
├── transport/
│   └── rest/
│       └── {name}_{version}/
│           ├── ip                  # 0.0.0.0
│           ├── port                # 8080
│           ├── timeout_read        # 30s (ReadTimeout + IdleTimeout)
│           ├── timeout_write       # 60s (WriteTimeout)
│           ├── timeout_read_header # 30s (ReadHeaderTimeout, default = timeout_read)
│           ├── handler/
│           │   ├── default/
│           │   │   └── timeout     # 2s (default handler timeout)
│           │   └── {path}/
│           │       └── timeout     # per-path handler timeout
│           └── mw/metrics/
│               ├── default/
│               │   └── log_level   # уровень логгера запроса
│               └── {url_path}/
│                   └── log_level   # уровень для одного endpoint-а
├── worker/
│   └── {worker_name}/
│       ├── default/
│       │   ├── timeout             # дефолтный таймаут джоба
│       │   └── log_level           # уровень логгера воркера
│       └── job/
│           ├── timeout             # таймаут джоба
│           └── log_level           # уровень логгера джоба
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

## Уровень логирования

Логгер создаётся не один на процесс: своя копия появляется на каждый HTTP-запрос и на
каждый джоб воркера. Уровень каждой копии читается **под тем поддеревом, которому эта
копия принадлежит**:

```text
<префикс>/default/log_level            # базовый уровень этого логгера
<префикс>/{endpoint|job}/log_level     # переопределение для одного endpoint-а / джоба
```

Переопределение применяется, только если значение непустое и отличается от базового;
иначе действует базовое. Нераспознанный уровень трактуется как `info`.

### Инвариант префикса

**`ocPrefix`, который вызывающий передаёт логгеру, — это абсолютный путь поддерева в
дереве конфигурации; имя сервиса в нём уже есть.** Сгенерированный конструктор логгера
(`pkg/app/logger/psg_{zlog,logrus}_gen.go`) имя сервиса **не** приписывает — он только
дописывает к префиксу узел уровня. Пустой `ocPrefix` означает процессный логгер: его
префикс — корень сервиса. Пустые компоненты пути отбрасываются, поэтому путь с `//`
не возникает.

Фактические префиксы:

| Логгер | `<префикс>` | Узел переопределения |
|---|---|---|
| процессный (создаётся на старте) | `/{svc}` | нет |
| запрос REST | `/{svc}/transport/rest/{transport}/{app}/mw/metrics` | `{urlPath}` |
| джоб воркера | `/{svc}/worker/{worker}` | `job` |

Примеры:

```text
/my-api/default/log_level                                        # весь процесс
/my-api/transport/rest/api_v1/web-app/mw/metrics/default/log_level   # все запросы транспорта
/my-api/transport/rest/api_v1/web-app/mw/metrics/users/{id}/log_level # один endpoint
/my-api/worker/telegram/default/log_level                        # воркер
/my-api/worker/telegram/job/log_level                            # джоб воркера
```

> **Поведенческое изменение конфигурации.** До исправления конструктор логгера приписывал
> имя сервиса к префиксу, в котором оно уже было, и читал уровень на один сегмент глубже
> (`/{svc}/{svc}/…`), то есть переопределение уровня не работало никогда — всегда
> применялся `info`. Проекты, в которых значение было выставлено по старому, удвоенному
> пути, после регенерации должны перенести его на путь из таблицы выше.

## Переменные окружения

OnlineConf значения могут передаваться через переменные окружения:

```bash
# Core settings
OC_{ProjectName}__devstand=0
OC_{ProjectName}__default__log_level=info

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
