# Инфраструктура

Описание секций `grafana`, `artifacts`, `packaging` и `jsonschema`.

## Секция `grafana`

Конфигурация Grafana dashboards.

```yaml
grafana:
  datasources:
    - name: Prometheus
      type: prometheus
      access: proxy
      url: http://prometheus:9090
      isDefault: true
      editable: false
    - name: Loki
      type: loki
      access: proxy
      url: http://loki:3100
      editable: false

applications:
  - name: api
    transport: [api, sys]
    grafana:
      datasources:
        - Prometheus
        - Loki
```

### Поля datasource

| Поле | Описание |
|------|----------|
| `name` | Имя datasource |
| `type` | Тип: `prometheus`, `loki` |
| `access` | Режим доступа: `proxy`, `direct` |
| `url` | URL datasource |
| `isDefault` | Установить как default |
| `editable` | Разрешить редактирование в UI |

### Генерируемые панели

| Панель | Условие | Метрики |
|--------|---------|---------|
| **Logs** | Loki datasource | Логи с фильтрацией по уровню |
| **Go Runtime** | Prometheus | `go_goroutines`, `go_memstats_*`, `go_gc_*` |
| **HTTP Server: {name}** | Для `ogen` транспорта | `http_server_request_duration_seconds` |
| **HTTP Client: {name}** | Для `ogen_client` | `http_client_request_duration_seconds` |

### Генерируемые файлы

```
grafana/
├── dashboards/
│   └── {app-name}-dashboard.json
└── provisioning/
    ├── dashboards/
    │   └── dashboards.yaml
    └── datasources/
        └── datasources.yaml
```

## Секция `artifacts`

Типы артефактов сборки.

```yaml
artifacts:
  - docker    # Docker образы (включен по умолчанию)
  - deb       # Debian пакеты (.deb)
  - rpm       # RPM пакеты (.rpm)
  - apk       # Alpine пакеты (.apk)
```

| Тип | Описание | Использование |
|-----|----------|---------------|
| `docker` | Docker образы | Kubernetes, Docker Compose |
| `deb` | Debian пакеты | Ubuntu, Debian |
| `rpm` | RPM пакеты | CentOS, RHEL, Fedora |
| `apk` | Alpine пакеты | Alpine Linux |

## Секция `packaging`

Конфигурация системных пакетов (обязательна для `deb`, `rpm`, `apk`).

```yaml
packaging:
  maintainer: "DevOps Team <devops@example.com>"
  description: "My microservice for handling orders"
  homepage: "https://github.com/myorg/myservice"
  license: "MIT"
  vendor: "My Company"
  install_dir: "/usr/bin"
  config_dir: "/etc/myservice"
  upload:
    enabled: true
    type: minio
    bucket: "packages"
    region: "us-east-1"
    endpoint: "https://minio.example.com"
    prefix: "deb/myservice"
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `maintainer` | Да | Email maintainer'а пакета |
| `description` | Да | Описание пакета |
| `homepage` | Нет | URL проекта |
| `license` | Нет | Лицензия |
| `vendor` | Нет | Название компании |
| `install_dir` | Нет | Путь установки бинарника |
| `config_dir` | Нет | Путь конфигурационных файлов |
| `upload` | Нет | Конфигурация загрузки пакетов |

### Секция `upload`

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `enabled` | Да | Включить загрузку |
| `type` | Да | `minio`, `aws`, или `rsync` |
| `bucket` | Для minio/aws | Имя S3 bucket |
| `region` | Для minio/aws | AWS регион |
| `endpoint` | Для minio | URL MinIO endpoint |
| `prefix` | Нет | Префикс пути |
| `host` | Для rsync | SSH хост |
| `path` | Для rsync | Путь на сервере |
| `user` | Для rsync | SSH пользователь |

### Примеры upload

#### MinIO

```yaml
upload:
  enabled: true
  type: minio
  bucket: "packages"
  region: "us-east-1"
  endpoint: "https://minio.example.com"
  prefix: "deb/myservice"
```

Требуемые секреты: `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`

#### AWS S3

```yaml
upload:
  enabled: true
  type: aws
  bucket: "my-packages-bucket"
  region: "eu-west-1"
  prefix: "releases/myservice"
```

Требуемые секреты: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`

#### Rsync

```yaml
upload:
  enabled: true
  type: rsync
  host: "repo.example.com"
  path: "/var/repo/packages"
  user: "deploy"
```

Требуемые секреты: `UPLOAD_SSH_PRIVATE_KEY`

### Генерируемые файлы

```
packaging/
└── {app-name}/
    ├── nfpm.yaml                    # Конфигурация nfpm
    ├── systemd/
    │   └── {project}-{app}.service  # Systemd unit
    └── scripts/
        ├── postinstall.sh           # Post-install скрипт
        └── preremove.sh             # Pre-remove скрипт
```

### Сборка пакетов

```bash
make install-nfpm     # Установить nfpm
make deb-api          # Собрать .deb для api
make rpm-api          # Собрать .rpm
make packages         # Собрать все пакеты
make upload-packages  # Загрузить пакеты
```

## Секция `jsonschema`

Генерация Go структур из JSON Schema.

```yaml
jsonschema:
  - name: models
    schemas:
      - id: user
        path: ./user.schema.json
        type: UserSchema
      - id: event
        path: ./event.schema.json
    package: models
```

### Поля

| Поле | Описание |
|------|----------|
| `name` | Имя группы схем |
| `schemas` | Список схем |
| `schemas[].id` | Уникальный идентификатор |
| `schemas[].path` | Путь к JSON schema |
| `schemas[].type` | Опционально: имя Go типа |
| `package` | Опционально: имя пакета |

### Интеграция с Kafka

```yaml
jsonschema:
  - name: models
    schemas:
      - id: user
        path: ./user.schema.json

kafka:
  - name: producer
    type: producer
    events:
      - name: user_events
        schema: models.user  # Ссылка на схему
```

### Генерируемые файлы

```
pkg/schema/{name}/    # Go код
api/schema/{name}/    # Копии schema файлов
```

## Полный пример

```yaml
main:
  name: orderservice
  logger: zerolog
  registry_type: github

artifacts:
  - docker
  - deb
  - rpm

packaging:
  maintainer: "Platform Team <platform@example.com>"
  description: "Order processing microservice"
  homepage: "https://github.com/example/orderservice"
  license: "Apache-2.0"
  vendor: "Example Inc"

grafana:
  datasources:
    - name: Prometheus
      type: prometheus
      url: http://prometheus:9090
      isDefault: true
    - name: Loki
      type: loki
      url: http://loki:3100

applications:
  - name: api
    transport: [api, sys]
    grafana:
      datasources: [Prometheus, Loki]
```
