# Локальная разработка с docker-compose-dev.yaml

## Обзор

При включении `dev_stand: true` в конфигурации проекта генерируется `docker-compose-dev.yaml` — полностью автономное локальное окружение для разработки. Включает:

- **OnlineConf** (MySQL + Admin UI + Updater) — локальный сервер конфигурации
- **Traefik** — reverse proxy для всех сервисов
- Все applications из конфига
- PostgreSQL (если `use_active_record: true`)
- Grafana + Prometheus + Loki (если настроены datasources)

## Требования

- Docker с поддержкой BuildKit
- SSH ключ добавлен в ssh-agent (для приватных репозиториев)
- Git (для submodule onlineconf)

## Конфигурация

Добавьте `dev_stand: true` в секцию `main`:

```yaml
main:
  name: myproject
  logger: zerolog
  dev_stand: true

post_generate:
  - git_install    # Обязательно для dev_stand
  - call_generate
  - go_mod_tidy
```

**Важно:** `dev_stand: true` требует `git_install` в `post_generate`, так как OnlineConf добавляется как git submodule.

## Быстрый старт


# 3. Собрать и запустить

- Генерация проекта
- make dev-up

После запуска дождитесь сообщения `onlineconf-updater` о создании `TREE.cdb` — это означает, что конфигурация загружена и приложения готовы к работе.

## Make-команды

| Команда | Описание |
|---------|----------|
| `make dev-up` | Собрать и запустить все сервисы в фоне |
| `make dev-down` | Остановить все сервисы |
| `make dev-restart` | Перезапустить все сервисы |
| `make dev-rebuild` | Пересобрать и перезапустить приложения |
| `make dev-drop` | Остановить и удалить все volumes (полный сброс) |

## Структура сервисов

```
┌─────────────────────────────────────────────────────────────┐
│                    docker-compose-dev.yaml                  │
├─────────────────────────────────────────────────────────────┤
│  OnlineConf Stack:                                          │
│  onlineconf-database  - MySQL для хранения конфигурации     │
│  onlineconf-admin     - Web UI для редактирования конфигов  │
│  onlineconf-updater   - синхронизация конфигов в файлы      │
├─────────────────────────────────────────────────────────────┤
│  traefik              - reverse proxy для всех сервисов     │
│  {app1}               - приложение 1                        │
│  {app2}               - приложение 2                        │
│  ...                                                        │
├─────────────────────────────────────────────────────────────┤
│  Infrastructure (опционально):                              │
│  postgres             - если use_active_record: true        │
│  grafana              - если есть grafana.datasources       │
│  prometheus           - если есть prometheus datasource     │
│  loki                 - если есть loki datasource           │
└─────────────────────────────────────────────────────────────┘
```

## Доступ к сервисам

| Сервис | URL | Credentials |
|--------|-----|-------------|
| OnlineConf Admin | http://localhost:8888 | admin / admin |
| Traefik Dashboard | http://localhost:9080 | - |
| Application API | http://localhost:{port} | - |
| Grafana | http://localhost:3000 | admin / admin |
| Prometheus | http://localhost:9090 | - |
| PostgreSQL | localhost:5432 | postgres / postgres |

## Переменные окружения

### OnlineConf (опционально)

| Переменная | Default | Описание |
|------------|---------|----------|
| `OC_USER` | admin | Пользователь OnlineConf updater |
| `OC_PASSWORD` | admin | Пароль OnlineConf updater |

### Порты (опционально)

| Переменная | Default | Описание |
|------------|---------|----------|
| `DEV_ONLINECONF_ADMIN_PORT` | 8888 | Порт OnlineConf Admin UI |
| `DEV_TRAEFIK_PORT` | 9080 | Порт Traefik dashboard |
| `DEV_PORT_{TRANSPORT}` | из конфига | Порт транспорта |
| `DEV_POSTGRES_PORT` | 5432 | Порт PostgreSQL |
| `DEV_GRAFANA_PORT` | 3000 | Порт Grafana |
| `DEV_PROMETHEUS_PORT` | 9090 | Порт Prometheus |
| `DEV_LOKI_PORT` | 3100 | Порт Loki |

### Сборка (опционально)

| Переменная | Default | Описание |
|------------|---------|----------|
| `GOPROXY` | - | Go proxy для ускорения сборки |

## Начальная конфигурация OnlineConf

При первом запуске MySQL выполняет `init-config.sql`, который создаёт начальную структуру конфигурации в OnlineConf. Этот файл генерируется автоматически на основе конфигурации проекта.

**Важно:** При изменении `init-config.sql` необходимо удалить volume MySQL, чтобы init-скрипты применились повторно:

```bash
make dev-drop
make dev-up
```

## Сборка образов

### Локальная сборка (SSH)

Использует SSH agent для доступа к приватным репозиториям:

```bash
# Убедиться что SSH agent запущен и ключ добавлен
ssh-add -l

# Собрать и запустить
make dev-up

# Пересобрать приложения после изменений кода
make dev-rebuild
```

## Запуск и управление

```bash
# Запустить все сервисы в фоне
make dev-up

# Остановить все сервисы
make dev-down

# Перезапустить сервисы
make dev-restart

# Пересобрать и перезапустить приложения
make dev-rebuild

# Посмотреть логи конкретного сервиса
docker compose -f docker-compose-dev.yaml logs -f {app_name}

# Запустить в foreground (для отладки)
docker compose -f docker-compose-dev.yaml up
```

## Директории

```
project/
├── docker-compose-dev.yaml    # Сгенерированный compose файл
├── Dockerfile-{app}           # Dockerfile для каждого приложения
├── .env                       # Переменные окружения (опционально)
├── etc/
│   ├── repo-oc/               # Git submodule OnlineConf
│   └── onlineconf/
│       └── dev/
│           ├── init-config.sql    # Начальная конфигурация
│           └── TREE.cdb           # Файл конфигурации (создаётся updater)
├── configs/
│   ├── grafana/
│   │   ├── provisioning/      # Grafana datasources config
│   │   └── dashboards/        # Grafana dashboards
│   └── dev/
│       ├── prometheus/
│       │   └── prometheus.yml # Prometheus config
│       └── loki/
│           └── local-config.yaml # Loki config
└── envs/
    └── .env.{app}             # Env файлы для приложений с UseEnvs=true
```

## Работа с OnlineConf

### Редактирование конфигурации

1. Откройте http://localhost:8888
2. Войдите как admin / admin
3. Отредактируйте конфигурацию в дереве
4. Изменения автоматически синхронизируются в `TREE.cdb`

### Структура конфигурации

Генератор создаёт начальную структуру:

```
/onlineconf/
├── module/
│   └── {project_name}/
│       ├── common/          # Общие настройки
│       ├── kafka/           # Настройки Kafka (если есть)
│       ├── postgres/        # Настройки PostgreSQL (если есть)
│       └── ...
```

## Мониторинг

Если в конфиге настроены `grafana.datasources`, автоматически создаются:

1. **Prometheus** — сбор метрик с приложений
2. **Loki** — агрегация логов
3. **Grafana** — визуализация с предустановленными дашбордами

Prometheus автоматически настроен на scrape метрик со всех REST транспортов.

## Troubleshooting

### SSH agent не работает

```bash
# Проверить что agent запущен
echo $SSH_AUTH_SOCK

# Проверить добавленные ключи
ssh-add -l

# Если пусто - добавить ключ
ssh-add ~/.ssh/id_rsa
```

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

MySQL выполняет init-скрипты только при первом запуске. Для применения изменений:

```bash
make dev-drop
make dev-up
```

### Порт уже занят

```bash
# Переопределить порт через .env
echo "DEV_ONLINECONF_ADMIN_PORT=8889" >> .env

# Перезапустить
make dev-restart
```

### Пересборка после изменений

```bash
# Пересобрать приложения
make dev-rebuild

# Полная очистка и пересборка
make dev-drop
make dev-up
```

### Submodule не инициализирован

```bash
# Инициализировать submodule
git submodule update --init --recursive

# Или пересоздать проект
go-project-starter --config=project.yaml
```
