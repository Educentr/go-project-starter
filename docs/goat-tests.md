# GOAT Integration Tests

GOAT (Go Application Testing) - фреймворк для интеграционного тестирования Go-микросервисов. go-project-starter генерирует базовую инфраструктуру для GOAT-тестов.

## Включение генерации тестов

### Простой вариант

В конфиге проекта добавьте `goat_tests: true` для нужного приложения:

```yaml
applications:
  - name: api
    goat_tests: true
    transport:
      - api
      - sys
    driver:
      - name: postgres
```

### Расширенная конфигурация

```yaml
applications:
  - name: api
    goat_tests_config:
      enabled: true
      binary_path: /tmp/my-custom-path  # опционально, по умолчанию /tmp/{app_name}
    transport:
      - api
      - sys
```

## Генерируемые файлы

После запуска генератора в директории `tests/` появятся:

| Файл | Описание |
|------|----------|
| `psg_app_gen.go` | Инициализация тестового окружения, работа с БД |
| `psg_base_suite_gen.go` | Базовый test suite с setup/teardown |
| `psg_config_gen.go` | Конфигурация сервиса (порты, пути, имена) |
| `psg_helpers_gen.go` | HTTP клиент, retry логика, парсеры |
| `psg_init_gen.go` | Интерфейс инициализации (требует реализации) |
| `psg_main_test.go` | Точка входа для тестов |

## Требования

- Go 1.23+
- Docker (для testcontainers)
- Скомпилированный бинарник приложения

## Настройка проекта

### 1. Создайте `init.go`

Это **обязательный** файл, который вы должны создать вручную. Он реализует интерфейс `TestEnvInitializer`:

```go
package tests

import (
    "context"
    "database/sql"
    "fmt"
    "os"
    "path/filepath"
    "time"

    gtt "github.com/Educentr/goat"
    "github.com/Educentr/goat-services/psql"
    "github.com/Educentr/goat/services"
    "github.com/Educentr/goat/testutil"
)

// testEnvInitializerImpl реализует интерфейс TestEnvInitializer
type testEnvInitializerImpl struct{}

// myTestConfig расширяет сгенерированный конфиг
type myTestConfig struct {
    *YourProjectConfig  // Сгенерированный конфиг (имя зависит от project_name)
}

// NewExecutor создает Executor для запуска приложения
func (c *myTestConfig) NewExecutor(env *gtt.Env, mockAddress string) *gtt.Executor {
    envVars := c.loadEnvVars()
    c.configureDB(envVars, env)

    return gtt.NewExecutorBuilder(c.BinaryPath()).
        WithEnv(envVars).
        WithOutputFile(fmt.Sprintf("/tmp/yourproject-test-%d.log", time.Now().Unix())).
        Build()
}

// loadEnvVars загружает переменные окружения
func (c *myTestConfig) loadEnvVars() map[string]string {
    // Вариант 1: Загрузка из .env файла
    envFilePath := "tests/etc/onlineconf/onlineconf.env"
    if _, err := os.Stat(envFilePath); err == nil {
        envVars, err := testutil.LoadEnvFile(envFilePath)
        if err != nil {
            panic(fmt.Errorf("failed to load %s: %w", envFilePath, err))
        }
        envVars["ONLINECONFIG_FROM_ENV"] = "true"
        return envVars
    }

    // Вариант 2: Загрузка из TREE.conf (OnlineConf формат)
    envVars, err := testutil.ParseOnlineConfFile("tests/etc/onlineconf/TREE.conf")
    if err != nil {
        panic(fmt.Errorf("failed to parse TREE.conf: %w", err))
    }
    envVars["ONLINECONFIG_FROM_ENV"] = "true"
    return envVars
}

// configureDB настраивает подключение к PostgreSQL
func (c *myTestConfig) configureDB(envVars map[string]string, env *gtt.Env) {
    pg := services.MustGetTyped[*psql.Env](env.Manager(), "postgres")
    svc := c.ServiceName()
    envVars[fmt.Sprintf("OC_%s__db__main", svc)] = fmt.Sprintf("%s:%s", pg.DBHost, pg.DBPort)
    envVars[fmt.Sprintf("OC_%s__db__main__User", svc)] = pg.DBUser
    envVars[fmt.Sprintf("OC_%s__db__main__Password", svc)] = pg.DBPass
    envVars[fmt.Sprintf("OC_%s__db__main__DB", svc)] = pg.DBName
}

// ApplyMigrations применяет миграции БД
func (c *myTestConfig) ApplyMigrations(ctx context.Context, db *sql.DB) error {
    migrationsPath, _ := filepath.Abs(filepath.Join("..", "etc/database/postgres"))

    migrationFiles := []string{
        "01_types.sql",
        "02_schema.sql",
        // ... ваши миграции
    }

    for _, file := range migrationFiles {
        content, err := os.ReadFile(filepath.Join(migrationsPath, file))
        if err != nil {
            if os.IsNotExist(err) {
                continue
            }
            return fmt.Errorf("read migration %s: %w", file, err)
        }
        if _, err := db.ExecContext(ctx, string(content)); err != nil {
            return fmt.Errorf("apply migration %s: %w", file, err)
        }
    }
    return nil
}

// CleanupTables очищает таблицы между тестами
func (c *myTestConfig) CleanupTables(ctx context.Context, db *sql.DB) error {
    tables := []string{
        "sessions",
        "app_user",
        // ... ваши таблицы в порядке, безопасном для FK
    }

    for _, table := range tables {
        if _, err := db.ExecContext(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)); err != nil {
            return fmt.Errorf("truncate %s: %w", table, err)
        }
    }
    return nil
}

// InitTestEnv инициализирует тестовое окружение
func (t *testEnvInitializerImpl) InitTestEnv() (testutil.TestAppConfig, *gtt.Env) {
    config := &myTestConfig{
        YourProjectConfig: NewYourProjectConfig(),
    }

    // Регистрируем сервисы
    services.MustRegisterServiceFuncTyped("postgres", psql.Run)

    // Создаем environment
    servicesMap := services.NewServicesMap("postgres")
    manager := services.NewManager(servicesMap, services.DefaultManagerConfig())
    env := gtt.NewEnv(gtt.EnvConfig{}, manager)

    return config, env
}

func init() {
    testEnvInit = &testEnvInitializerImpl{}
}
```

### 2. Интерфейс `testutil.TestAppConfig`

Интерфейс `TestAppConfig` определён в пакете `github.com/Educentr/goat/testutil` и объединяет несколько интерфейсов:

```go
// ServiceConfig - информация о сервисе
type ServiceConfig interface {
    ServiceName() string
    APIPort() string
    SysPort() string
    BinaryPath() string
}

// ExecutorBuilder - создание executor
type ExecutorBuilder interface {
    NewExecutor(env *Env, mockAddress string) *Executor
}

// MigrationRunner - применение миграций
type MigrationRunner interface {
    ApplyMigrations(ctx context.Context, db *sql.DB) error
}

// TableCleaner - очистка таблиц между тестами
type TableCleaner interface {
    CleanupTables(ctx context.Context, db *sql.DB) error
}

// ActiveRecordConfig - конфигурация ActiveRecord
type ActiveRecordConfig interface {
    ConfigMap(dbHost, dbPort, dbUser, dbPass, dbName string) map[string]interface{}
}

// TestAppConfig - объединяет все интерфейсы
type TestAppConfig interface {
    ServiceConfig
    ExecutorBuilder
    MigrationRunner
    TableCleaner
    ActiveRecordConfig
}
```

### 3. Вспомогательные функции `testutil`

Пакет `testutil` предоставляет функции для загрузки конфигурации:

```go
// LoadEnvFile парсит .env файл и возвращает map переменных окружения
// Поддерживает имена переменных с дефисами, пропускает комментарии (#)
envVars, err := testutil.LoadEnvFile("path/to/file.env")

// ParseOnlineConfFile парсит OnlineConf TREE.conf файл
// Преобразует пути в переменные окружения:
// /myservice/db/host localhost -> OC_myservice__db__host=localhost
envVars, err := testutil.ParseOnlineConfFile("path/to/TREE.conf")
```

## Написание тестов

### Структура test suite

```go
package tests

import (
    "net/http"
    "testing"

    "github.com/stretchr/testify/suite"
)

// MyFeatureTestSuite тестирует функциональность
type MyFeatureTestSuite struct {
    BaseTestSuite  // Наследуем всю инфраструктуру
}

// TestMyFeature тестирует конкретную функцию
func (s *MyFeatureTestSuite) TestMyFeature() {
    // 1. Подготовка данных
    // Используйте фабрики для создания тестовых данных

    // 2. Выполнение запроса
    resp, body := s.client.Get(s.T(), "/my/endpoint")

    // 3. Проверка результата
    s.Require().Equal(http.StatusOK, resp.StatusCode)
}

// TestMyFeatureTestSuite запускает suite
func TestMyFeatureTestSuite(t *testing.T) {
    suite.Run(t, new(MyFeatureTestSuite))
}
```

### Использование HTTP клиента

```go
// GET запрос
resp, body := s.client.Get(s.T(), "/endpoint")

// GET с заголовками
resp, body := s.client.GetWithHeaders(s.T(), "/endpoint", map[string]string{
    "X-Custom-Header": "value",
})

// POST запрос
reqBody := map[string]interface{}{
    "field": "value",
}
resp, body := s.client.Post(s.T(), "/endpoint", reqBody)

// С авторизацией
s.client.WithAuth(token)
resp, body := s.client.Get(s.T(), "/protected")
```

### Доступные методы BaseTestSuite

| Метод | Описание |
|-------|----------|
| `s.ctx` | Context для операций с БД |
| `s.env` | GOAT Environment |
| `s.client` | HTTP клиент |
| `s.GetContext()` | Получить context |
| `s.GetClient()` | Получить HTTP клиент |
| `s.GetEnv()` | Получить environment |
| `s.Mocks()` | Доступ к mock-серверам (если есть ogen_client) |

## Фабрики и фикстуры

Рекомендуется создать файлы:

### `factories.go` - Фабрики для создания тестовых данных

```go
package tests

import (
    "context"
    "testing"
)

// UserFactory создает тестовых пользователей
type UserFactory struct {
    ctx context.Context
    t   *testing.T
    // поля для настройки
}

func NewUserFactory(ctx context.Context, t *testing.T) *UserFactory {
    return &UserFactory{ctx: ctx, t: t}
}

func (f *UserFactory) WithUsername(name string) *UserFactory {
    // настройка
    return f
}

func (f *UserFactory) Create() *TestUser {
    // создание пользователя в БД
    return &TestUser{}
}
```

### `fixtures.go` - Константы и фикстуры

```go
package tests

import "time"

const (
    TestDeviceID1 = "test-device-001"
    TestDeviceID2 = "test-device-002"

    TestDuration7Days  = 7 * 24 * time.Hour
    TestDuration30Days = 30 * 24 * time.Hour
)

func GenerateTestDeviceID(suffix string) string {
    return "test-device-" + suffix
}
```

### `assertions.go` - Assertion helpers

```go
package tests

import (
    "net/http"
    "testing"

    "github.com/stretchr/testify/require"
)

func AssertHTTPStatus(t *testing.T, resp *http.Response, body []byte, expected int) {
    t.Helper()
    require.Equal(t, expected, resp.StatusCode, "Response body: %s", string(body))
}

func AssertAuthSuccess(t *testing.T, resp *http.Response, body []byte, userID int64) *AuthResponse {
    t.Helper()
    AssertHTTPStatus(t, resp, body, http.StatusOK)

    var authResp AuthResponse
    ParseJSONResponse(t, body, &authResp)
    require.Equal(t, userID, authResp.User.ID)

    return &authResp
}
```

## Запуск тестов

### Через Makefile (рекомендуется)

```bash
# Сборка и запуск тестов
make goat-tests

# Только сборка бинарника для тестов
make build_for_test-api

# Запуск с подробным выводом
make goat-tests-verbose
```

### Напрямую через go test

```bash
# Сначала соберите бинарник
CGO_ENABLED=1 go build -cover -race -o /tmp/api ./cmd/api

# Запуск всех тестов
go test -v ./tests/...

# Запуск конкретного suite
go test -v -run TestAuthTestSuite ./tests/...

# Запуск конкретного теста
go test -v -run TestAuthTestSuite/TestAuthVerifyCodeSuccessful ./tests/...

# Пропуск интеграционных тестов
go test -short ./tests/...
```

## CI/CD интеграция

При включении `goat_tests` в CI/CD pipeline добавляется job для запуска интеграционных тестов:

```yaml
goat-tests:
  stage: test
  script:
    - make goat-tests
  services:
    - docker:dind
```

## Структура файлов проекта

```
your-project/
├── cmd/
│   └── api/
│       └── main.go
├── tests/
│   ├── psg_app_gen.go        # [generated] App initialization
│   ├── psg_base_suite_gen.go # [generated] Base test suite
│   ├── psg_config_gen.go     # [generated] Service config
│   ├── psg_helpers_gen.go    # [generated] HTTP helpers
│   ├── psg_init_gen.go       # [generated] Init interface
│   ├── psg_main_test.go      # [generated] Test entry point
│   ├── init.go               # [manual] Your implementation
│   ├── factories.go          # [manual] Test data factories
│   ├── fixtures.go           # [manual] Constants & fixtures
│   ├── assertions.go         # [manual] Assertion helpers
│   ├── auth_test.go          # [manual] Your tests
│   └── etc/
│       └── onlineconf/       # Test configuration files
└── etc/
    └── database/
        └── postgres/         # SQL migrations
```

Интерфейс `TestAppConfig` берётся из `github.com/Educentr/goat/testutil` - создавать `pkg/tests/` не нужно.

## Mock-серверы для внешних API

Если ваше приложение использует `ogen_client` для внешних API, вам нужно создать mock-серверы для тестирования.

### 1. Создание mock-сервера

Создайте файл `mocks.go` в директории `tests/`:

```go
package tests

import (
    "context"
    "net/http"

    // Импортируйте сгенерированные ogen API
    externalapi "github.com/your-org/your-project/pkg/rest/external/v1"
    // Импортируйте сгенерированные gomock моки
    externalmock "github.com/your-org/your-project/pkg/mocks/external"

    "go.uber.org/mock/gomock"
)

// MockServers содержит ссылки на mock-серверы для использования в тестах
type MockServers struct {
    ExternalHandler  *externalmock.MockHandler
    ExternalSecurity *externalmock.MockSecurityHandler
}

// HTTPMocksSetup создает HTTP mock handlers
// Эта функция передается в gtt.NewFlow()
func HTTPMocksSetup(mocks *MockServers) func(server *http.ServeMux, ctl *gomock.Controller) {
    return func(server *http.ServeMux, ctl *gomock.Controller) {
        // Создаем моки с помощью gomock controller
        mocks.ExternalHandler = externalmock.NewMockHandler(ctl)
        mocks.ExternalSecurity = externalmock.NewMockSecurityHandler(ctl)

        // Устанавливаем дефолтные expectations
        setupExternalMockDefaults(mocks.ExternalHandler, mocks.ExternalSecurity)

        // Создаем ogen сервер с mock handlers
        externalServer, err := externalapi.NewServer(
            mocks.ExternalHandler,
            mocks.ExternalSecurity,
        )
        if err != nil {
            panic(err)
        }

        // Регистрируем сервер на определенном пути
        server.Handle("/external/", externalServer)
    }
}

// setupExternalMockDefaults устанавливает дефолтные expectations
func setupExternalMockDefaults(h *externalmock.MockHandler, s *externalmock.MockSecurityHandler) {
    // Security handler - всегда пропускаем авторизацию
    s.EXPECT().HandleAuthHeader(gomock.Any(), gomock.Any(), gomock.Any()).
        DoAndReturn(func(ctx context.Context, _ externalapi.OperationName, _ externalapi.AuthHeader) (context.Context, error) {
            return ctx, nil
        }).AnyTimes()

    // Handler - возвращаем пустой ответ по умолчанию
    h.EXPECT().GetData(gomock.Any()).
        Return(&externalapi.DataResponse{}, nil).AnyTimes()

    // Обработка ошибок
    h.EXPECT().NewError(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
}
```

### 2. Подключение mock-сервера к тестам

Mock-сервер подключается через `BaseTestSuite`. Если у вашего приложения есть `ogen_client`, генерируется код:

```go
// В psg_base_suite_gen.go (генерируется автоматически)
type BaseTestSuite struct {
    suite.Suite
    ctx    context.Context
    env    *gtt.Env
    flow   *gtt.Flow
    client *HTTPClient
    mocks  *MockServers  // Доступ к mock-серверам
}

func (s *BaseTestSuite) SetupSuite() {
    // ...

    // Инициализация mocks holder
    s.mocks = &MockServers{}

    // Создание flow с HTTP mocks
    s.flow = gtt.NewFlow(
        s.T(),
        s.env,
        NewTestApp(s.env),
        HTTPMocksSetup(s.mocks),  // Подключаем mock setup
        nil, // gRPC mocks (если нужны)
    )
    // ...
}

// Mocks возвращает mock-серверы для использования в тестах
func (s *BaseTestSuite) Mocks() *MockServers {
    return s.mocks
}
```

### 3. Настройка адреса mock-сервера в приложении

В `init.go` настройте адрес mock-сервера для внешних API:

```go
// configureExternalServices настраивает адреса mock-серверов
func (c *myTestConfig) configureExternalServices(envVars map[string]string, mockAddress string) {
    // mockAddress = "http://127.0.0.1:9898" (GOAT default)
    // Переопределяем адреса внешних сервисов на mock
    envVars[fmt.Sprintf("OC_%s__external__api__url", c.ServiceName())] = mockAddress + "/external"
    envVars[fmt.Sprintf("OC_%s__stat__api__url", c.ServiceName())] = mockAddress + "/stat"
}
```

### 4. Использование mock-сервера в тестах

```go
func (s *MyTestSuite) TestExternalAPICall() {
    // Переопределяем дефолтное поведение для конкретного теста
    s.Mocks().ExternalHandler.EXPECT().
        GetData(gomock.Any()).
        Return(&externalapi.DataResponse{
            Status: "success",
            Items:  []externalapi.Item{{ID: 1, Name: "Test"}},
        }, nil).
        Times(1)  // Ожидаем ровно 1 вызов

    // Выполняем запрос к нашему API, который внутри вызывает внешний сервис
    resp, body := s.client.Get(s.T(), "/my-endpoint")

    s.Require().Equal(http.StatusOK, resp.StatusCode)
}

func (s *MyTestSuite) TestExternalAPIError() {
    // Симулируем ошибку внешнего сервиса
    s.Mocks().ExternalHandler.EXPECT().
        GetData(gomock.Any()).
        Return(nil, errors.New("service unavailable")).
        Times(1)

    resp, body := s.client.Get(s.T(), "/my-endpoint")

    // Проверяем, что наш сервис корректно обрабатывает ошибку
    s.Require().Equal(http.StatusServiceUnavailable, resp.StatusCode)
}

func (s *MyTestSuite) TestExternalAPIWithSpecificInput() {
    // Проверяем конкретные параметры вызова
    s.Mocks().ExternalHandler.EXPECT().
        CreateItem(gomock.Any(), gomock.Eq(&externalapi.CreateItemRequest{
            Name: "Expected Name",
            Type: "Expected Type",
        })).
        Return(&externalapi.CreateItemResponse{ID: 123}, nil).
        Times(1)

    reqBody := map[string]interface{}{
        "name": "Expected Name",
        "type": "Expected Type",
    }
    resp, body := s.client.Post(s.T(), "/items", reqBody)

    s.Require().Equal(http.StatusCreated, resp.StatusCode)
}
```

### 5. Генерация gomock моков

Для генерации моков из ogen интерфейсов добавьте в `Makefile`:

```makefile
.PHONY: generate-mocks
generate-mocks:
    @echo "Generating mocks..."
    mockgen -source=pkg/rest/external/v1/oas_server_gen.go \
        -destination=pkg/mocks/external/mock_handler.go \
        -package=externalmock
```

Или используйте `go:generate` директиву:

```go
//go:generate mockgen -source=oas_server_gen.go -destination=../../mocks/external/mock_handler.go -package=externalmock
```

### Полезные паттерны gomock

```go
// Любые аргументы
gomock.Any()

// Конкретное значение
gomock.Eq(expectedValue)

// Количество вызовов
.Times(1)      // ровно 1 раз
.AnyTimes()    // любое количество
.MinTimes(1)   // минимум 1 раз
.MaxTimes(3)   // максимум 3 раза

// Кастомная логика
.DoAndReturn(func(ctx context.Context, req *Request) (*Response, error) {
    // Кастомная логика
    return &Response{}, nil
})

// Последовательные вызовы с разными результатами
gomock.InOrder(
    mock.EXPECT().Method().Return(result1).Times(1),
    mock.EXPECT().Method().Return(result2).Times(1),
)
```

## Best Practices

1. **Используйте фабрики** для создания тестовых данных - это делает тесты читаемыми
2. **Используйте fixtures** для констант - избегайте magic strings
3. **Используйте assertion helpers** - стандартизируют проверки
4. **Наследуйте BaseTestSuite** - не дублируйте setup/teardown
5. **Очищайте данные между тестами** - реализуйте `CleanupTables` правильно
6. **Используйте понятные имена тестов** - `TestFeature_WhenCondition_ExpectedResult`

## Troubleshooting

### "API did not become ready"

- Проверьте, что бинарник собран: `ls -la /tmp/api`
- Docker запущен
- Порты не заняты
- Посмотрите логи: `tail -f /tmp/yourproject-test-*.log`

### "Database connection refused"

- Docker запущен
- Testcontainers работает
- Миграции валидны

### "TestEnvInitializer not implemented"

- Создайте файл `tests/init.go`
- Реализуйте интерфейс `TestEnvInitializer`
- Присвойте реализацию в `init()`: `testEnvInit = &testEnvInitializerImpl{}`

## Ресурсы

- [GOAT Framework](https://github.com/Educentr/goat)
- [Testcontainers Go](https://golang.testcontainers.org/)
- [testify/suite](https://github.com/stretchr/testify)
