# Локальная разработка с docker-compose-dev.yaml

## Обзор

`docker-compose-dev.yaml` - файл для локальной разработки, который автоматически генерируется на основе конфигурации проекта. Включает:

- Все applications из конфига
- Общий Traefik reverse proxy
- Общий OnlineConf updater
- PostgreSQL (если используется ActiveRecord)
- Grafana + Prometheus + Loki (если настроены datasources)

## Требования

- Docker с поддержкой BuildKit
- SSH ключ добавлен в ssh-agent (для приватных репозиториев)
- Доступ к OnlineConf серверу

## Быстрый старт

```bash
# 1. Добавить SSH ключ в agent
eval $(ssh-agent -s)
ssh-add ~/.ssh/id_rsa

# 2. Создать .env файл с настройками OnlineConf
cat > .env << EOF
OC_HOST=onlineconf.example.com
OC_PORT=443
OC_USER=your_user
OC_PASSWORD=your_password
EOF

# 3. Собрать и запустить
docker-compose -f docker-compose-dev.yaml build
docker-compose -f docker-compose-dev.yaml up
```

## Структура сервисов

```
┌─────────────────────────────────────────────────────────────┐
│                    docker-compose-dev.yaml                  │
├─────────────────────────────────────────────────────────────┤
│  traefik              - reverse proxy для всех сервисов     │
│  onlineconf-updater   - общий updater конфигурации          │
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

## Переменные окружения

### Обязательные (OnlineConf)

| Переменная | Описание |
|------------|----------|
| `OC_HOST` | Хост OnlineConf сервера |
| `OC_PORT` | Порт OnlineConf сервера |
| `OC_USER` | Пользователь OnlineConf |
| `OC_PASSWORD` | Пароль OnlineConf |

### Опциональные (порты)

| Переменная | Default | Описание |
|------------|---------|----------|
| `DEV_TRAEFIK_PORT` | 8080 | Порт Traefik dashboard |
| `DEV_PORT_{TRANSPORT}` | из конфига | Порт транспорта |
| `DEV_POSTGRES_PORT` | 5432 | Порт PostgreSQL |
| `DEV_GRAFANA_PORT` | 3000 | Порт Grafana |
| `DEV_PROMETHEUS_PORT` | 9090 | Порт Prometheus |
| `DEV_LOKI_PORT` | 3100 | Порт Loki |

### Опциональные (сборка)

| Переменная | Default | Описание |
|------------|---------|----------|
| `GOPROXY` | - | Go proxy для ускорения сборки |

## Сборка образов

### Локальная сборка (SSH)

Использует SSH agent для доступа к приватным репозиториям:

```bash
# Убедиться что SSH agent запущен и ключ добавлен
ssh-add -l

# Собрать все образы
docker-compose -f docker-compose-dev.yaml build

# Собрать конкретный сервис
docker-compose -f docker-compose-dev.yaml build {app_name}

# Собрать без кеша
docker-compose -f docker-compose-dev.yaml build --no-cache
```

### CI сборка (GitHub Token)

В CI используется GitHub token вместо SSH:

```bash
# Создать файл с токеном
echo "$GITHUB_TOKEN" > .github_token

# Собрать с секретом
docker build \
  --secret id=github_token,src=.github_token \
  -f Dockerfile-{app_name} \
  -t {project}-{app_name}:latest .
```

## Запуск

```bash
# Запустить все сервисы
docker-compose -f docker-compose-dev.yaml up

# Запустить в фоне
docker-compose -f docker-compose-dev.yaml up -d

# Запустить конкретный сервис с зависимостями
docker-compose -f docker-compose-dev.yaml up {app_name}

# Посмотреть логи
docker-compose -f docker-compose-dev.yaml logs -f {app_name}

# Остановить
docker-compose -f docker-compose-dev.yaml down
```

## Директории

```
project/
├── docker-compose-dev.yaml    # Сгенерированный compose файл
├── Dockerfile-{app}           # Dockerfile для каждого приложения
├── .env                       # Переменные окружения (создать вручную)
├── etc/
│   └── onlineconf/
│       └── dev/               # OnlineConf данные (создается автоматически)
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

## Доступ к сервисам

После запуска сервисы доступны по портам:

| Сервис | URL |
|--------|-----|
| Traefik Dashboard | http://localhost:8080 |
| Application API | http://localhost:{port} |
| Grafana | http://localhost:3000 (admin/admin) |
| Prometheus | http://localhost:9090 |

## Мониторинг

Если в конфиге настроены `grafana.datasources`, автоматически создаются:

1. **Prometheus** - сбор метрик с приложений
2. **Loki** - агрегация логов
3. **Grafana** - визуализация с предустановленными дашбордами

Prometheus автоматически настроен на scrape метрик со всех REST транспортов по пути `/metrics`.

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

### OnlineConf не подключается

```bash
# Проверить переменные
docker-compose -f docker-compose-dev.yaml config | grep OC_

# Посмотреть логи updater
docker-compose -f docker-compose-dev.yaml logs onlineconf-updater
```

### Порт уже занят

```bash
# Переопределить порт через .env
echo "DEV_GRAFANA_PORT=3001" >> .env

# Или через переменную окружения
DEV_GRAFANA_PORT=3001 docker-compose -f docker-compose-dev.yaml up
```

### Пересборка после изменений

```bash
# Пересобрать и перезапустить
docker-compose -f docker-compose-dev.yaml up --build

# Полная очистка и пересборка
docker-compose -f docker-compose-dev.yaml down -v
docker-compose -f docker-compose-dev.yaml build --no-cache
docker-compose -f docker-compose-dev.yaml up
```
