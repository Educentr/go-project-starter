# 10-03 Сессия: Архитектура Go Project Starter — слои, драйверы, REST и автогенерация метрик/Grafana
## A. Обзор сессии

Сессия проходила в формате комбинированного знания: преимущественно Domain Knowledge Sharing с элементами Process-Driven и частичной дискуссии (Presentation Style) по архитектурным принципам Go Project Starter. Участники: Speaker 1 (основной носитель знаний/архитектор), Speaker 2 (участник, уточняющий реализацию и конфигурацию), эпизодически Speaker 3. Цель — разъяснить архитектурные концепции (разделение пакетов, понятие application, драйверы, транспорты/генераторы), практики масштабирования и интеграций (Telegram, S3), а также обсудить конфигурацию REST, генерацию клиентов/серверов, и инициативы по автоматизации (валидация конфигов, авто-дашборды для Grafana).

## B. Ключевые выводы

- Разделение слоев и зависимостей:
  - pkg — runtime-библиотеки, максимально общие и переиспользуемые.
  - internal/pkg — генерируемые пакеты, не завязанные на конфиг проекта и специфичные логгеры.
  - internal/app (в речи звучало как “internal lab”/“internal app”) — проектная специфика: общие для проекта библиотеки с зависимостью от конфига, конкретных реализаций (включая логгер).

- Логирование:
  - InternalPKG не должен зависеть от конкретного логгера; функции возвращают ошибки, логирование выполняется на верхнем уровне (в проект-специфичном слое).
  - Возможны разные реализации логгера (пример: The Logo, RusLogo) через единый интерфейс в отдельном пакете.

- Концепция application:
  - Application — атомарная единица горизонтального масштабирования (контейнер).
  - Один бинарь/контейнер может включать несколько компонентов (HTTP-серверы, воркеры), которые инициализируются/запускаются параллельно (горутины).
  - Выбор сервисов (HTTP/worker и т. п.) определяется конфигурацией, каждый application масштабируется независимо.

- Драйверы:
  - Драйвер — слой интеграции между бизнес-логикой и внешним API (Telegram, S3 и т. д.) по стабильному интерфейсу.
  - Принцип: сервис вызывает абстрактный интерфейс (например, “отправь сообщение”), драйвер переводит вызов в конкретный API.
  - Драйверы реализуют интерфейс Runnable (init, run, shutdown, graceful shutdown). Запускаются до сервиса; у активных интеграций (Telegram — веб-хуки/веб-сокеты) есть фоновые процессы, у пассивных (S3) — нет.
  - Замена провайдера проста: меняется драйвер/библиотека без изменения бизнес-логики (пример: переход с Azure S3 на DigitalOcean S3).

- Транспорты и генерация:
  - Transport “sys” из template — генерация преднастроенного сервера для прометеус-метрик (prometheus metrics), стартующего в составе application.
  - Генераторы типов:
    - gen-client — генерирует клиентскую библиотеку и интегрирует её в сервис (инициализация при старте).
    - gen-server — генерирует сервер и будет запущен в будущем (в составе application).

- REST в Project Config:
  - REST — это JSON RESTful API, не gRPC.
  - Определённые REST-сервисы должны быть назначены в конкретные application; иначе генерация падает (требуется валидация).
  - REST-сервер выставляет ручки наружу; REST-клиент генерирует клиентский код и предоставляет доступ к нему через сервис.

- Долги/несоответствия:
  - Временная путаница с пакетами Telegram (pkg, internal/pkg, internal/app) из-за незавершённого рефакторинга; возможны алиасы и смешение импортов — лучше эскалировать на автора для ускорения чистки.

- Инициативы:
  - Добавить валидацию конфигов (проверка непротиворечивости: определено в REST, но не привязано к application — ошибка).
  - Автогенерация дашбордов для Grafana на основе метрик — высокая приоритетность и практическая полезность.

## C. Вопросы и уточнения
- Что означает транспорт sys и генератор type/template?
  - Ответ: transport sys из шаблонов — генерация сервера для Prometheus metrics; “generator type = template” означает генерацию по готовым шаблонам. “gen-client”/“gen-server” — разные режимы генерации (клиент vs сервер).

- Что значит REST в Project Config?
  - Ответ: REST — JSON RESTful API (не gRPC).

REST-сервисы должны быть назначены на конкретные application; иначе генерация валится — нужна валидация.

- Ошибка при генерации из-за REST без назначения в application:
  - Ответ: Да, это следует валидировать; лишние описания без использования — как “неиспользуемый импорт” в Go, лучше блокировать.
- Статус ToDo в README:
  - Ответ: Актуально; используется для планирования работ, можно пополнять и делегировать новичкам.

Открытые вопросы/незакрытые моменты:
- Уточнить финальную структуру и именование слоев (internal/app vs “internal lab”): стандартизировать термин.
- Определить правила размещения интеграций (например, Telegram) между pkg/internal/pkg/internal/app и провести рефакторинг для устранения дублирования.
- Подтвердить формат и источники метрик для автогенерации Grafana дашбордов (какие экспортеры и labels, схема).

## E. Предлагаемое содержание документации

### Архитектурные слои и зависимости

- pkg (runtime):
  - Назначение: общие библиотеки, не завязанные на конфиг проекта.
  - Требования: отсутствие зависимостей на конкретные реализация логгера/интеграций.
- internal/pkg (generated core):
  - Назначение: генерируемый код, переиспользуемый между проектами Go Project Starter.
  - Требования: не зависит от проекта-специфичных настроек; ошибки возвращаются вверх, логирование — выше по стеку.
- internal/app (project-specific shared):
  - Назначение: библиотеки и код, разделяемый внутри конкретного проекта; может зависеть от конфига и конкретных реализаций (логгер, драйверы).
  - Пример: подключение выбранного логгера, специфичные адаптеры.

Лучшие практики:
- Чёткое разделение бизнес-логики и интеграций.
- Интерфейсы в бизнес-слое, реализация — в драйверах.
- Минимизировать кросс-слойные зависимости, избегать специфики в pkg/internal/pkg.

### Концепция Application
- Определение: атомарная единица горизонтального масштабирования; соответствует контейнеру.
- Состав: может включать несколько компонентов (HTTP серверы, воркеры, драйверы).
- Поведение при старте:
  - Инициализация всех указанных HTTP серверов из конфига.
  - Запуск воркеров в отдельных горутинах.
  - Предварительный запуск всех драйверов (Runnable).
- Масштабирование:
  - Каждый application масштабируется независимо; одна кодовая база — несколько application профилей.

### Драйверы интеграций
- Назначение: адаптация стабильного сервисного интерфейса к внешнему API (Telegram, S3).
- Интерфейс:
  - Runnable: init, run, shutdown, graceful shutdown.
  - Сервисный интерфейс: доменные операции (например, SendMessage, PutFile, GetFile, Hash, Size).
- Жизненный цикл:
  - До старта сервисов запускаются драйверы; активные драйверы могут слушать сокеты/веб-хуки.
- Замена провайдера:
  - Реализовать новый драйвер, соблюдающий интерфейс; сервисный код не меняется.

### Транспорты и генерация
- Transport “sys” (template):
  - Автогенерация сервера для экспонирования Prometheus метрик.
  - Включение в application запускает метрик-сервер.
- Генераторы:
- gen-client: генерирует клиентскую библиотеку, интегрирует её инициализацию в сервис.
  - gen-server: генерирует серверную часть для будущего запуска.
  - generator type = template: использование шаблонов из репозитория для генерации артефактов.

### Конфигурация REST
- REST = JSON RESTful API (не gRPC).
- Правила:
  - Определённые REST-сервисы должны быть связаны с конкретными application.
  - REST-сервер — выставляет ручки наружу.
  - REST-клиент — генерирует клиентскую библиотеку и делает её доступной через сервис.
- Валидация:
  - Генерация должна падать/предупреждать, если REST описан, но не используется в application.

#### Параметры аутентификации (auth_params)
- Структура `auth_params` позволяет настроить аутентификацию для REST клиентов.
- Параметры:
  - `transport` — способ передачи аутентификационных данных (поддерживается: `header`).
  - `type` — тип аутентификации (поддерживается: `apikey`).
- Пример конфигурации в `config.yaml`:
  ```yaml
  rest:
    - name: example_api
      generator_type: ogen_client
      auth_params:
        transport: header
        type: apikey
  ```
- Ключ API читается из OnlineConf по пути: `{service_name}/transport/rest/{rest_name}/auth_params/apikey`
- **ВАЖНО**: Устаревший параметр `auth_type` в `generator_params` больше не поддерживается. Используйте вместо него `auth_params`.
- Генерируемый код создаёт структуру `SecuritySource` с методом `AuthHeader` для передачи API ключа в заголовках запросов.

### Kafka (Event-Driven Architecture)

#### Концепция Events

Kafka конфигурация использует концепцию **events** (ранее topics):
- `KafkaEvent` — единица публикации/потребления сообщений
- Event name используется для генерации Go методов
- Topic name по умолчанию совпадает с event name
- Topic можно переопределить per-environment через OnlineConf

#### Конфигурация

```yaml
kafka:
  - name: events_producer
    type: producer          # producer или consumer
    driver: segmentio       # segmentio (default) или custom
    client: main_kafka      # имя клиента для OnlineConf пути
    group: my_group         # consumer group (только для consumer)
    events:
      - name: user_events   # event name = default topic name
        schema: models.user # опционально: jsonschema_name.schema_id
```

#### OnlineConf пути для Kafka

| Путь | Описание |
|------|----------|
| `{service}/kafka/{client}/brokers` | Список брокеров через запятую |
| `{service}/kafka/{client}/auth_type` | none, PLAIN, SCRAM-SHA-256, SCRAM-SHA-512 |
| `{service}/kafka/{client}/username` | SASL username |
| `{service}/kafka/{client}/password` | SASL password |
| `{service}/kafka/{client}/tls_enabled` | 0 или 1 |
| `{service}/kafka/{client}/tls_skip_verify` | 0 или 1 (только для dev) |
| `{service}/kafka/{client}/tls_ca_cert` | PEM-encoded CA сертификат |
| `{service}/kafka/{client}/events/{event_name}/topic` | Override topic name |

#### TLS Configuration

Для production Kafka с TLS:
- `tls_enabled: 1` — включить TLS
- `tls_ca_cert` — PEM-encoded CA сертификат для валидации сервера
- `tls_skip_verify: 1` — пропустить валидацию сертификата (только dev!)

#### Типизированные сообщения

Events могут ссылаться на JSON Schema для типизации:

```yaml
jsonschema:
  - name: models
    schemas:
      - id: user
        path: ./user.schema.json

kafka:
  - name: producer
    type: producer
    client: main_kafka
    events:
      - name: user_events
        schema: models.user  # Генерирует типизированный метод
```

### Container Registry Types

| Тип | Registry | Требуемые секреты |
|-----|----------|-------------------|
| `github` | GitHub Container Registry (ghcr.io) | `GHCR_USER`, `GHCR_TOKEN` |
| `digitalocean` | DigitalOcean Container Registry | `REGISTRY_PASSWORD` |
| `aws` | Amazon ECR | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION` |
| `selfhosted` | Любой Docker Registry | `REGISTRY_URL`, `REGISTRY_USERNAME`, `REGISTRY_PASSWORD` |

#### AWS ECR

URL формат: `{account-id}.dkr.ecr.{region}.amazonaws.com`

#### Self-Hosted Registry

Для MinIO-backed или других self-hosted registry настройте `REGISTRY_LOGIN_SERVER`.

### Setup Command

Подкоманда для автоматизации развертывания проекта.

#### Использование

```bash
go-project-starter setup [command]

# Доступные команды:
go-project-starter setup         # Полный интерактивный wizard
go-project-starter setup ci      # Настройка CI/CD
go-project-starter setup server  # Настройка серверов
go-project-starter setup deploy  # Генерация deploy скриптов
```

#### Wizard

Интерактивный wizard собирает информацию:
1. CI/CD провайдер (GitHub Actions / GitLab CI)
2. Окружения (staging, production)
3. Серверы и SSH доступ
4. Registry credentials

#### Поддерживаемые CI провайдеры

- **GitHub Actions** — автогенерация workflows
- **GitLab CI** — автогенерация .gitlab-ci.yml

#### Файлы конфигурации

- `setup.yaml` — сохраненная конфигурация wizard
- `.github/workflows/` или `.gitlab-ci.yml` — CI/CD файлы
- `deploy.sh` — скрипт развертывания

### JSON Schema

Генерация Go структур с валидацией из JSON Schema файлов.

#### Формат конфигурации

```yaml
jsonschema:
  - name: models
    schemas:
      - id: user
        path: ./user.schema.json
        type: UserSchema        # Опционально: имя Go типа
      - id: event
        path: ./event.schema.json
    package: models             # Опционально: имя пакета
```

#### Структура schemas

| Поле | Описание |
|------|----------|
| `id` | Уникальный идентификатор для ссылок из Kafka events |
| `path` | Путь к JSON schema файлу |
| `type` | Опционально: имя генерируемого Go типа |

#### Интеграция с Kafka

Для типизированных Kafka сообщений используйте формат `{jsonschema_name}.{schema_id}`:

```yaml
kafka:
  - name: producer
    type: producer
    events:
      - name: user_events
        schema: models.user
```

#### Генерируемые файлы

```
pkg/schema/{name}/    # Go код
api/schema/{name}/    # Копии schema файлов
```

### GOAT Integration Tests

GOAT (Go Application Testing) — фреймворк для интеграционного тестирования.

#### Включение

```yaml
applications:
  - name: api
    goat_tests: true
    # или расширенная конфигурация:
    goat_tests_config:
      enabled: true
      binary_path: /tmp/api  # опционально
```

#### Генерируемые файлы

| Файл | Описание |
|------|----------|
| `tests/psg_app_gen.go` | Инициализация тестового окружения |
| `tests/psg_base_suite_gen.go` | Базовый test suite |
| `tests/psg_config_gen.go` | Конфигурация сервиса |
| `tests/psg_helpers_gen.go` | HTTP клиент, helpers |
| `tests/psg_init_gen.go` | Интерфейс инициализации (требует реализации) |
| `tests/psg_main_test.go` | Точка входа |

#### Mock-серверы

Для `ogen_client` автоматически генерируется поддержка mock-серверов.

Подробная документация: [docs/goat-tests.md](goat-tests.md)

### Dev Stand (Локальная разработка)

При включении `dev_stand: true` генерируется `docker-compose-dev.yaml` — автономное окружение разработки.

#### Включение

```yaml
main:
  name: myproject
  dev_stand: true

post_generate:
  - git_install    # Обязательно для dev_stand
```

#### Компоненты

- **OnlineConf Stack** (MySQL + Admin UI + Updater)
- **Traefik** — reverse proxy
- **PostgreSQL** (если `use_active_record: true`)
- **Grafana + Prometheus + Loki** (если настроены)

#### Доступ

| Сервис | URL |
|--------|-----|
| OnlineConf Admin | http://localhost:8888 |
| Traefik Dashboard | http://localhost:9080 |
| Grafana | http://localhost:3000 |

#### Команды

```bash
make dev-up      # Запустить
make dev-down    # Остановить
make dev-drop    # Полный сброс с volumes
make dev-rebuild # Пересборка приложений
```

Подробная документация: [docs/dev-start.md](dev-start.md)

### Логирование
- Интерфейс логгера:
  - Реализации взаимозаменяемы (The Logo, RusLogo и т. п.) через общий интерфейс.
- Правило слоёв:
  - internal/pkg — без привязки к конкретному логгеру (только возврат ошибок).
  - Логирование в application/internal слоях.

### Известные проблемы и рекомендации
- Telegram пакеты в трёх местах (pkg, internal/pkg, internal/app):
  - Временная мера из-за незавершённого рефакторинга; возможны алиасы и смешение импортов.
  - Рекомендация: при боли — эскалировать автору (Speaker 1) для ускоренной чистки.
- Общая валидация конфигов:
  - Проверять непротиворечивость и полноту; аналог “неиспользуемого импорта” в Go.

### План автогенерации Grafana дашбордов (черновик)
- Источники метрик:
  - Prometheus metrics из transport sys и компонент (HTTP, worker, драйверы).
- Стандартные панели:
  - HTTP: RPS, latency (p50/p90/p99), ошибки по кодам, saturation.
  - Worker: обработанные задачи, retry/fail rates, время обработки.
  - Драйверы: внешние вызовы, ошибки/таймауты, очереди/подписки (для Telegram).
- Интеграция:
  - Генератор дашбордов на основе конфигов application и метрик-лейблов.
  - Автоприменение/экспорт JSON/Mixin для Grafana.

### FAQ
- В: Почему генерация падает, если REST не привязан к application?
  - О: Описанные сущности должны быть использованы. Добавьте соответствующее назначение или удалите лишнее.
- В: Можно ли менять поставщика S3 без переписывания сервиса?
  - О: Да, замените драйвер, соблюдая интерфейс; сервисные вызовы не меняются.
- В: Где должно происходить логирование?
  - О: В проект-специфичных слоях (application/internal), не в internal/pkg.
- В: Как переопределить topic name для Kafka event?
  - О: Через OnlineConf по пути `{service}/kafka/{client}/events/{event_name}/topic`. Event name используется как default topic.
- В: Как настроить TLS для Kafka?
  - О: Установите `tls_enabled: 1` в OnlineConf и укажите `tls_ca_cert` с PEM-encoded сертификатом. Для dev-окружений можно использовать `tls_skip_verify: 1`.
- В: Как запустить GOAT тесты?
  - О: Добавьте `goat_tests: true` в application конфигурацию, соберите бинарник и запустите `make goat-tests`.
- В: Что делать если init-config.sql не применяется в dev_stand?
  - О: MySQL выполняет init-скрипты только при первом запуске. Используйте `make dev-drop && make dev-up` для полной переинициализации.
- В: Какие registry types поддерживаются?
  - О: github (ghcr.io), digitalocean, aws (ECR), selfhosted (любой Docker Registry совместимый).

### SOP: Валидация Project Config (черновик)
- Шаги:
  1. Парсинг Project Config.
  2. Проверка: каждая сущность REST (server/client) назначена хотя бы одному application.
  3. Проверка транспортов: transport sys указан — метрик-сервер должен быть сконфигурирован (порт, пути).
  4. Проверка генераторов: gen-client/gen-server соответствуют ожидаемым целям; отсутствуют “висячие” генерации без использования.
  5. Отчёт: ошибки/варнинги с указанием пути в конфиге.
- Инструменты:
  - Встроенный валидатор генератора.
  - Сбор тудушек из кода/сгенерированной документации для дальнейших задач.

### Пример жизненного цикла Application
- Инициализация:
  - Загрузка конфига, создание логгера (проект-специфический).
  - Инициализация драйверов (init).
- Запуск:
  - Запуск драйверов (run): Telegram слушает веб-сокет/веб-хуки; S3 — no-op.
  - Запуск HTTP серверов.
  - Запуск воркеров в отдельных горутинах.
- Завершение:
  - Graceful shutdown: остановка воркеров/HTTP, shutdown драйверов.

### CLI Transport

CLI (Command Line Interface) — это **транспорт** для сервиса, аналогично REST и GRPC. Ключевое отличие: CLI требует интерактивной коммуникации с пользователем.

#### CLI как транспорт

```
┌─────────────────────────────────────────────────────────────┐
│                        Service                               │
│                    (бизнес-логика)                          │
└─────────────────────────────────────────────────────────────┘
        ▲                    ▲                    ▲
        │                    │                    │
┌───────┴───────┐   ┌───────┴───────┐   ┌───────┴───────┐
│  REST Transport│   │ GRPC Transport │   │ CLI Transport  │
│  (HTTP/JSON)   │   │   (protobuf)   │   │  (shell-like)  │
└───────────────┘   └───────────────┘   └───────────────┘
        ▲                    ▲                    ▲
        │                    │                    │
   HTTP клиент          gRPC клиент         Пользователь
```

#### Принцип работы CLI

CLI работает по принципу shell:
- Первое слово — **команда** (command)
- Остальные слова — **аргументы** (arguments)

```bash
./myapp <command> [arguments...]

# Примеры:
./myapp migrate up
./myapp user create --email admin@example.com
./myapp cache clear --all
```

#### Сравнение транспортов

| Транспорт | Протокол | Жизненный цикл | Интерактивность |
|-----------|----------|----------------|-----------------|
| **REST** | HTTP/JSON | Слушает порт → обрабатывает запросы | Нет |
| **GRPC** | HTTP2/protobuf | Слушает порт → обрабатывает запросы | Нет |
| **CLI** | stdin/stdout | Получает команду → выполняет → завершается | Да |

#### Архитектура CLI Handler

CLI handler генерируется аналогично REST/GRPC handlers:

```
internal/app/transport/cli/{name}/
├── handler.go      # CLI router - разбор команд
├── commands/       # Реализация команд
│   ├── migrate.go
│   └── user.go
└── handler/        # Handler implementations
    └── handler.go
```

#### Структура конфигурации

```yaml
cli:
  - name: admin
    path:
      - ./api/cli/admin.yaml   # Спецификация команд (опционально)
    generator_type: template
    generator_template: cli

applications:
  - name: admin-cli
    cli: admin      # CLI транспорт (эксклюзивен с transport/worker)
    driver:
      - postgres
```

#### Правила использования CLI

1. **CLI эксклюзивен** — приложение с CLI не может иметь REST/GRPC транспорты или workers
2. **Один CLI на application** — только один CLI транспорт на приложение
3. **Вызывает тот же Service** — CLI handlers вызывают те же методы сервиса, что и REST/GRPC
4. **Может использовать драйверы** — подключение к БД, внешним API

#### Отличие от Worker

| CLI | Worker |
|-----|--------|
| Запускается пользователем | Запускается с приложением |
| Выполняет одну команду | Работает непрерывно |
| Интерактивный (stdin/stdout) | Автономный (без взаимодействия) |
| Завершается после команды | Работает до shutdown |

#### Примеры команд

```bash
# Миграции
./migrate-cli migrate up
./migrate-cli migrate down --steps 1
./migrate-cli migrate status

# Администрирование
./admin-cli user create --email admin@example.com
./admin-cli user list --role admin
./admin-cli cache clear --prefix "session:*"

# Утилиты
./tools-cli report generate --date 2024-01-01
./tools-cli backup create --output /backups/
```

### Термины
- Application — контейнеризуемая единица, атомарно масштабируемая.
- Driver — адаптер для внешних API с Runnable.
- Runnable — интерфейс жизненного цикла (init/run/shutdown/graceful).
- Transport — слой доставки запросов к сервису (REST, GRPC, CLI).
- Transport sys — метрик-сервер (Prometheus) из шаблона.
- gen-client/gen-server — режимы генерации клиента/сервера.
- REST — JSON RESTful API транспорт.
- GRPC — HTTP2/protobuf транспорт.
- CLI — интерактивный транспорт командной строки (shell-like: command args...).
- Worker — фоновая горутина, работающая непрерывно без интерактивной сессии.
- KafkaEvent — единица Kafka сообщения (бывший KafkaTopic), используется для генерации методов публикации/потребления.
- dev_stand — локальное окружение разработки с OnlineConf, Traefik и опциональными Grafana/Prometheus/Loki.
- GOAT — Go Application Testing framework для интеграционного тестирования сгенерированных сервисов.
- Setup Command — интерактивный wizard для автоматизации развертывания (CI/CD, серверы, deploy скрипты).
- JSON Schema — генерация Go структур из JSON Schema файлов с валидацией.

### Важные правила (IMPORTANT)

#### Версионирование Runtime

**При каждой сборке/релизе go-project-starter необходимо обновить `MinRuntimeVersion` до последней версии runtime.**

Файл: `internal/pkg/templater/templater.go`

```go
const MinRuntimeVersion = "v0.9.0"  // актуальная версия на момент написания
```

Правила:
1. Перед релизом go-project-starter — проверить последний тег в go-project-starter-runtime
2. Обновить MinRuntimeVersion до этого тега
3. Обновление go.mod зависимости и MinRuntimeVersion делаются в одном коммите

### Риски и подводные камни
- Смешение пакетов интеграций в разных слоях → сложные алиасы, повышенная связность.
- Отсутствие валидации конфига → падения генерации, "мертвые" сущности.
- Нарушение принципа независимости internal/pkg → утечки зависимостей (логгер/конфиг).

### Ресурсы и инструменты
- Репозиторий Go Project Starter: шаблоны для транспортов и генераторов.
- README/ToDo: актуальные задачи, подходящие “good first issues”.
- Prometheus/Grafana: стандартные экспортеры и JSON-модели дашбордов.
