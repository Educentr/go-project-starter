# Applications и Drivers

Описание секций `applications` и `drivers`.

## Секция `applications`

Конфигурация приложений — атомарных единиц деплоя.

```yaml
applications:
  # API Gateway
  - name: api
    transport: [public, admin, system]

  # Event processors
  - name: event_processor
    kafka: [order_events, payment_events]

  # Notification worker
  - name: notifier
    workers: [notification_bot]

  # С зависимостью от Docker образов
  - name: checker
    transport: [sys]
    workers: [checker]
    depends_on_docker_images:
      - ghcr.io/some-app/cool-app:latest
      - postgres:15-alpine
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя приложения (= имя контейнера) |
| `transport` | Нет | REST/gRPC транспорты (string[] или object[]) |
| `kafka` | Нет | Kafka producers/consumers |
| `worker` | Нет | Фоновые воркеры |
| `driver` | Нет | Драйверы интеграций |
| `cli` | Нет | CLI транспорт. Исключает `worker`; из транспортов допускает только клиентские (`generator_type: ogen_client`) |
| `use_active_record` | Нет | Override use_active_record для приложения |
| `use_envs` | Нет | Брать конфигурацию из переменных окружения вместо OnlineConf (только `true`; требует своей CI-джобы, см. ниже) |
| `deploy` | Нет | Настройки деплоя (volumes) |
| `depends_on_docker_images` | Нет | Docker образы для pre-pull |
| `dev_ports` | Нет | Дополнительные порты контейнера в `docker-compose-dev.yaml` (`"host:container"`, можно `"${VAR:-8102}:8102"`); порты REST-транспортов пробрасываются сами |
| `goat_tests` | Нет | Включить GOAT тесты (bool) |
| `goat_tests_config` | Нет | Расширенная настройка GOAT тестов |
| `grafana` | Нет | Настройки Grafana dashboard |
| `artifacts` | Нет | Per-app override artifacts (docker, deb, rpm, apk) |
| `metrics` | Нет | Канал вывода метрик (`export`: `pull` / `push` / `none`) |
| `docker` | Нет | Образ приложения: runtime-стадия (`base_image`, `packages`, `workdir`, `entrypoint`) и `platforms` — см. [Docker](docker.md#runtime-стадия-приложения-applicationsdocker) |

### Формат транспортов

Транспорты можно указывать двумя способами:

**Простой формат (string[]):**

```yaml
applications:
  - name: api
    transport: [api, sys]  # Список имён транспортов
```

**Расширенный формат (object[]):**

```yaml
applications:
  - name: api
    transport:
      - name: api
      - name: external_api
        config:
          instantiation: dynamic  # Per-app override
```

### Концепция Application

**Application** — атомарная единица горизонтального масштабирования (контейнер).

- Один бинарь/контейнер может включать несколько компонентов
- HTTP серверы, gRPC серверы, воркеры инициализируются параллельно
- Каждый application масштабируется независимо

### Примеры конфигураций

#### API-only

```yaml
applications:
  - name: api
    transport: [api, sys]
```

#### Workers-only

```yaml
applications:
  - name: workers
    workers: [telegram_bot, kafka_consumer]
    transport: [sys]
```

#### Monolith

```yaml
applications:
  - name: monolith
    transport: [api, admin, sys]
    workers: [telegram_bot]
    kafka: [events]
```

### Зависимости от Docker образов

`depends_on_docker_images` создаёт сервисы для предварительного pull образов:

```yaml
applications:
  - name: checker
    transport: [sys]
    depends_on_docker_images:
      - ghcr.io/some-app/cool-app:latest
```

Генерируемый docker-compose:

```yaml
cool-app-image-puller:
  image: ghcr.io/some-app/cool-app:latest
  pull_policy: always
  restart: "no"

checker:
  depends_on:
    cool-app-image-puller:
      condition: service_completed_successfully
```

### `use_envs`: конфигурация из переменных окружения

`use_envs: true` переводит приложение на переменные окружения вместо OnlineConf: в
`docker-compose` не монтируется дерево конфигурации, `run-<app>` не зависит от
`onlineconf-update`, а `goat-tests-<app>` не поднимает тестовое дерево. Значение может быть
только `true` — `false` отвергается на загрузке конфига (отсутствие ключа и есть «нет»).

```yaml
applications:
  - name: api
    use_envs: true
    transport: [api, sys]
```

**Для приложения с `use_envs: true` проект обязан сам объявить CI-джобу
`create_<app>_env_files`** — в пользовательской секции `.github/workflows/ci_cd.yml` (ниже
disclaimer-маркера, иначе она пропадёт при следующем `make regenerate`). Генератор её **не
создаёт**: откуда брать значения переменных — секреты, S3, vault — знает только проект.
Генератор добавляет её в `needs` джобы `deploy`, и джоба, которую никто не объявил, делает
**весь** workflow невалидным.

Имя джобы — имя приложения с дефисами, заменёнными на подчёркивания (`api-gateway` →
`create_api_gateway_env_files`). Джоба должна выкладывать env-файлы артефактом `envs-*`:
именно их `deploy` скачивает и rsync'ит на сервер.

Запись в `needs` появляется только у приложений, которые реально едут в `deploy`, то есть у
собираемых в docker-образ. Приложение с `artifacts: [deb]` (или любым набором без `docker`)
env-файл в `deploy` не отдаёт, джоба ему не нужна и в `needs` оно не попадает.

### GOAT тесты

Включение интеграционных тестов:

```yaml
applications:
  - name: api
    goat_tests: true
    transport: [api, sys]

# Или расширенная конфигурация
applications:
  - name: api
    goat_tests_config:
      enabled: true
      binary_path: /tmp/my-custom-path
```

### Grafana dashboard

```yaml
applications:
  - name: api
    transport: [api, sys]
    grafana:
      datasources:
        - Prometheus
        - Loki
```

### Метрики приложения (`metrics.export`)

**Реестр метрик инициализируется у каждого приложения, всегда.** Код, который пишет метрики
(middleware REST-транспорта, сгенерированные коллекторы из [карты метрик](metrics-map.md),
ActiveRecord), получает реестр при инициализации и регистрируется в нём безусловно — поэтому реестр
обязан существовать даже там, где метрики никто не читает.

Что настраивается — **канал вывода**: как метрики покидают процесс.

```yaml
applications:
  - name: api
    transport: [api, sys]
    metrics:
      export: pull   # /metrics на sys-транспорте

  - name: localUI
    transport: [ui]  # нет sys — и не нужен
    metrics:
      export: none   # метрики собираются и никуда не отдаются
```

| Значение | Что делает |
|----------|------------|
| `pull` | `/metrics` на `sys`-транспорте приложения, его скрейпит Prometheus |
| `push` | Отправка во внешний приёмник. **Зарезервировано, пока не реализовано** — генерация падает с явной ошибкой |
| `none` | Метрики собираются и не отдаются никому |

**Ключ можно не указывать.** Тогда канал выводится из транспортов приложения: есть
`sys`-транспорт → `pull` (он и есть тот сервер, который отдаёт `/metrics`), нет → `none`.
Существующие `project.yaml` менять не нужно, и ни один из них не меняет смысла.

`none` — рабочая конфигурация, а не деградация: у процесса на машине пользователя (локальный UI,
CLI с HTTP-интерфейсом) нет скрейпера, который до него дотянется, а лишний слушающий порт — лишняя
поверхность без читателя.

Объявленное значение проверяется против транспортов приложения, потому что канал, которого
приложение не может предоставить, — это ошибка конфигурации, а не предпочтение:

- `export: pull` без `sys`-транспорта — просит эндпоинт, который никто не отдаёт;
- `export: none` при наличии `sys`-транспорта — просит оставить метрики внутри процесса, который
  их уже отдаёт на `/metrics`.

Резолвленный канал видно в сгенерированном `cmd/<app>/psg_main_gen.go` рядом с `InitMetrics`:

```go
// Export channel of this application: none — collected and never handed out; nothing reads this registry.
err = application.InitMetrics(mainCtx)
```

### Ранняя инициализация процесса: `earlyInit()`

Первый пользовательский код в сгенерированном `main` — метод `earlyInit()`. Он вызывается
**после загрузки `.env` и до `occonf.Initialize`**, то есть до того, как собрано дерево
конфигурации, поднят логгер и (у CLI) разобрана команда. Точка одна и стоит в одном и том же
месте в обоих `main` — и у сервисного приложения (`cmd/<app>/psg_main_gen.go`), и у
[CLI-приложения](transports.md#секция-cli) (`cmd/<app>/psg_main_gen.go`, функция `run()`).

По умолчанию `earlyInit()` промотирован из встроенного `EmptyEarlyInit` и ничего не делает.
Проект объявляет свой — метод на `*UserEarlyInit` перекрывает промотированный:

```go
// cmd/api/early.go — ваш файл в пакете main (или секция ниже disclaimer-маркера
// в psg_main_gen.go: regenerate эту правку не трогает)
func (u *UserEarlyInit) earlyInit() error {
	// Порядок ранних действий объявлен здесь явно — телом одной функции,
	// а не порядком инициализации пакетов.

	// 1. Окружение, из которого occonf.Initialize построит дерево
	if !hasAnyOCKey() {
		// Нет ни одного OC_*-ключа — пусть Initialize откроет существующую
		// TREE.cdb, а не перестроит её пустой из окружения.
		if err := os.Unsetenv("ONLINECONFIG_FROM_ENV"); err != nil {
			return fmt.Errorf("unset ONLINECONFIG_FROM_ENV: %w", err)
		}
	}

	// 2. Идентичность процесса — до любого re-exec и до запуска дочерних процессов
	if err := os.Setenv("APP_VERSION", Version); err != nil {
		return fmt.Errorf("export APP_VERSION: %w", err)
	}

	// 3. Проверка os.Args до разбора команды
	return rejectRepeatedFlags(os.Args[1:])
}
```

Что здесь уместно: изменить окружение, из которого `occonf.Initialize` строит дерево; выполнить
фазу подготовки окружения с re-exec процесса, пока CDB ещё не собрана; проверить `os.Args` до
разбора команды и перенаправить `stdout` команд, чей вывод — машиночитаемый контракт; зафиксировать
идентичность процесса (имя приложения, версия, корень хранилища) и экспортировать её в окружение.

**Ошибка останавливает процесс.** Логгера на этом шаге ещё нет, поэтому об ошибке сообщает `stderr`
через `log` — как и о неудачном `occonf.Initialize`, — а процесс выходит с кодом
`ExitCodeErrorEarlyInit` (сервис — `os.Exit`, CLI — `return` из `run()`, команда не выполняется).
Код добавлен в конец блока `iota`: значения кодов выше — наблюдаемый контракт процесса, вставка в
середину перенумеровала бы их.

**Почему точка после `.env`, а не до.** `godotenv.Load` не перезаписывает переменные, которые уже
есть в окружении, но добавляет те, которых нет. Хук, вызванный раньше, не увидел бы `OC_*`-ключи,
которые принесёт `.env`, и снял бы `ONLINECONFIG_FROM_ENV` по ошибке; а снятая им переменная
вернулась бы, если она записана в самом `.env`. Остальным сценариям (`os.Args`, `stdout`, re-exec,
экспорт идентичности) подходят обе точки, поэтому точка одна — после `.env`.

**Почему это не `init()`.** В сгенерированных проектах `init()` запрещён линтером
(`gochecknoinits`), и дело не только в линтере: `init()` — неявный side-effect при импорте, его
порядок задаётся порядком инициализации пакетов, а не кодом проекта. Инициализатор переменной
(`var _ = f()`) — тот же неявный порядок в обход линтера. `earlyInit()` называет порядок явно.
Подробности — [DEPRECATION.md](https://github.com/Educentr/go-project-starter/blob/main/DEPRECATION.md).

## Секция `drivers`

Конфигурация драйверов интеграций.

```yaml
drivers:
  - name: telegram_bot
    import: pkg/drivers/telegram
    package: telegram
    obj_name: TelegramDriver
    service_injection: "optional custom code"

  - name: payment_gateway
    type: http
    config:
      base_url: https://api.stripe.com
```

### Поля

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Имя драйвера |
| `import` | Да | Import path |
| `package` | Да | Имя пакета |
| `obj_name` | Да | Имя объекта драйвера |
| `service_injection` | Нет | Кастомный код инъекции |

### Интерфейс Runnable

Все драйверы реализуют интерфейс:

```go
type Runnable interface {
    Init(ctx context.Context) error
    Run(ctx context.Context) error
    Shutdown(ctx context.Context) error
    GracefulShutdown(ctx context.Context) error
}
```

### Типы драйверов

| Тип | Поведение | Примеры |
|-----|-----------|---------|
| **Активный** | Имеет фоновый процесс | Telegram (webhooks/websockets) |
| **Пассивный** | Run — no-op | S3, HTTP клиенты |

### Жизненный цикл драйвера

```
Init() → Run() (в горутине) → GracefulShutdown() → Shutdown()
```

1. `Init()` — инициализация соединений, валидация конфига
2. `Run()` — запуск фонового процесса (для активных драйверов)
3. `GracefulShutdown()` — мягкое завершение
4. `Shutdown()` — принудительное завершение

### Создание кастомного драйвера

1. Создайте структуру драйвера:

```go
package payment

type PaymentDriver struct {
    client *http.Client
    apiKey string
}

func (d *PaymentDriver) Init(ctx context.Context) error {
    d.client = &http.Client{Timeout: 10 * time.Second}
    d.apiKey = config.Get("payment.api_key")
    return nil
}

func (d *PaymentDriver) Run(ctx context.Context) error {
    <-ctx.Done()
    return nil
}

func (d *PaymentDriver) Shutdown(ctx context.Context) error {
    d.client.CloseIdleConnections()
    return nil
}

func (d *PaymentDriver) GracefulShutdown(ctx context.Context) error {
    return d.Shutdown(ctx)
}

// Бизнес-методы
func (d *PaymentDriver) ProcessPayment(ctx context.Context, amount int64) error {
    // ...
}
```

2. Добавьте в конфигурацию:

```yaml
drivers:
  - name: payment_gateway
    import: pkg/drivers/payment
    package: payment
    obj_name: PaymentDriver
```

3. Подключите к приложению:

```yaml
applications:
  - name: api
    transport: [api, sys]
    driver: [payment_gateway]
```

### Замена провайдера

Для замены провайдера без изменения бизнес-логики:

```yaml
# Переход с AWS S3 на DigitalOcean Spaces
drivers:
  - name: storage
    type: s3
    provider: digitalocean  # Просто измените эту строку
```
