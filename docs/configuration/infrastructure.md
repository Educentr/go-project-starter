# Инфраструктура

Описание секций `grafana`, `artifacts`, `packaging` и `jsonschema`.

## Секция `grafana`

Конфигурация Grafana dashboards: datasources и [записи аннотаций](#аннотации).

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
| `uid` | [optional] uid datasource. По умолчанию `ds-<name в lowercase>` |
| `isDefault` | Установить как default |
| `editable` | Разрешить редактирование в UI |

### Явный `uid` datasource

uid попадает и в `configs/grafana/provisioning/datasources/datasources.yaml`, и в каждую
панель сгенерированного дашборда `configs/grafana/dashboards/<app>.json`. Пока доска живёт
в локальной compose-Grafana проекта, вычисленного `ds-<name>` достаточно.

Если же доску забирают в **общую** Grafana, где datasource провижинится централизованно под
своим uid, панели ссылаются на несуществующий datasource («Datasource ... was not found»).
Для этого случая uid задаётся явно:

```yaml
grafana:
  datasources:
    - name: Prometheus
      uid: victoriametrics      # uid datasource в общей Grafana
      type: prometheus
      url: http://victoriametrics:8428
      isDefault: true
```

- `uid` задан → используется как есть в provisioning и во всех панелях;
- не задан → прежнее поведение `ds-<name в lowercase>` (пробелы заменяются на `-`),
  существующие проекты не меняются;
- формат: от 1 до 40 символов из набора `a-z`, `A-Z`, `0-9`, `-`, `_` (правила Grafana).
  Явный uid, в отличие от вычисленного, не приводится к lowercase — Grafana различает регистр;
- **итоговые** uid проверяются на уникальность: явный uid одного datasource не может совпасть
  с вычисленным uid другого (например `uid: ds-prometheus` при наличии datasource с
  `name: Prometheus`).

### Аннотации

Grafana держит org-level аннотации — события с тегами, которые рисуются вертикальными
линиями поперёк всех графиков доски. Сами события постит сервис в рантайме
(`POST /api/annotations`: деплой, рестарт, краш). Чтобы они **отрисовались**, в доске должна
лежать запись-запрос в `.annotations.list[]` — её объявляет секция `annotations`:

```yaml
grafana:
  datasources: [...]
  annotations:
    - name: "Deploys / Restarts / Crashes"
      icon_color: yellow
      tags: deploy, restart, crash
      match_any: true              # OR: событие с одним тегом тоже рисуется
    - name: Migrations
      tags:
        - migration
```

Записи эмитятся в `.annotations.list[]` **каждого** сгенерированного дашборда рядом со
стоковой записью builtIn «Annotations & Alerts» — аннотации объявляются на сервис целиком,
приложение не ссылается на них по имени, как на datasources.

| Поле | Описание |
|------|----------|
| `name` | [required] Подпись тумблера в шапке доски. Уникальна; имя стоковой записи «Annotations & Alerts» занято |
| `tags` | [required] Теги событий. Список YAML либо строка через запятую (`deploy, restart, crash` — пробелы срезаются) |
| `icon_color` | [optional] Цвет маркера, как его принимает Grafana (`yellow`, `rgba(0, 211, 255, 1)`). По умолчанию `yellow` |
| `datasource` | [optional] Хранилище аннотаций. Только `grafana` — встроенное (uid `-- Grafana --`), оно же по умолчанию |
| `match_any` | [optional] `true` — OR по тегам, `false` (default) — AND, как в Grafana по умолчанию |
| `hide` | [optional] Скрыть тумблер в шапке. Запрос всё равно выполняется, маркеры рисуются — просто их нельзя выключить из UI |
| `enable` | [optional] Начальное состояние тумблера. По умолчанию `true` |
| `limit` | [optional] Сколько событий вытягивает один запрос. По умолчанию `100` |

Почему это конфиг, а не правка файла: дашборд генератор владеет целиком, и дописанная
руками запись стиралась следующим `make regenerate`. Отказ при этом тихий — JSON остаётся
валидным, доска открывается, панели работают, просто события перестают рисоваться, и
замечают это обычно на инциденте, когда ищут маркер деплоя.

Прометеевские/лок-овские аннотации (запрос выражением вместо тегов) не поддерживаются:
у них принципиально другая форма `target`, это не ещё одно значение поля `datasource`.

Объявить аннотации, не объявив ни одного datasource, — ошибка конфигурации: дашборд
генерируется только для приложений, которые ссылаются на datasource, и записям было бы
некуда попасть.

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

## Deploy-time плейсхолдеры

В env файлах можно использовать специальные плейсхолдеры, которые заменяются на реальные значения во время деплоя (а не во время создания env файла в CI).

### Доступные плейсхолдеры

| Плейсхолдер | Описание |
|-------------|----------|
| `@@SSH_HOST@@` | IP адрес или hostname сервера деплоя |

### Пример использования

В docker-compose или конфигурации приложения:

```yaml
# docker-compose.yaml
services:
  myapp:
    environment:
      - ADVERTISE_HOST=@@SSH_HOST@@
      - EXTERNAL_URL=http://@@SSH_HOST@@:8080
```

При деплое `@@SSH_HOST@@` будет заменён на реальный IP/hostname целевого сервера.

### Когда использовать

Плейсхолдеры полезны когда:

- Приложению нужно знать свой внешний IP для регистрации в service discovery
- Нужно сформировать callback URL для внешних сервисов
- При деплое на несколько серверов с разными IP
