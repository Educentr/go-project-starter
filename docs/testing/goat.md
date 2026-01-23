# GOAT Integration Tests

GOAT (Go Application Testing) — фреймворк для интеграционного тестирования Go-микросервисов.

## Включение генерации тестов

### Простой вариант

```yaml
applications:
  - name: api
    goat_tests: true
    transport: [api, sys]
```

### Расширенная конфигурация

```yaml
applications:
  - name: api
    goat_tests_config:
      enabled: true
      binary_path: /tmp/my-custom-path
    transport: [api, sys]
```

## Генерируемые файлы

После запуска генератора в директории `tests/` появятся:

| Файл | Описание |
|------|----------|
| `psg_app_gen.go` | Инициализация тестового окружения |
| `psg_base_suite_gen.go` | Базовый test suite |
| `psg_config_gen.go` | Конфигурация сервиса |
| `psg_helpers_gen.go` | HTTP клиент, retry логика |
| `psg_init_gen.go` | Интерфейс инициализации |
| `psg_main_test.go` | Точка входа для тестов |

## Требования

- Go 1.24+
- Docker (для testcontainers)
- Скомпилированный бинарник приложения

## Настройка проекта

### 1. Создайте `init.go`

Это **обязательный** файл, реализующий интерфейс `TestEnvInitializer`:

```go
package tests

import (
    "context"
    "database/sql"
    "fmt"
    "os"
    "path/filepath"

    gtt "github.com/Educentr/goat"
    "github.com/Educentr/goat-services/psql"
    "github.com/Educentr/goat/services"
    "github.com/Educentr/goat/testutil"
)

type testEnvInitializerImpl struct{}

type myTestConfig struct {
    *YourProjectConfig
}

func (c *myTestConfig) NewExecutor(env *gtt.Env, mockAddress string) *gtt.Executor {
    envVars := c.loadEnvVars()
    c.configureDB(envVars, env)

    return gtt.NewExecutorBuilder(c.BinaryPath()).
        WithEnv(envVars).
        Build()
}

func (c *myTestConfig) loadEnvVars() map[string]string {
    envVars, _ := testutil.LoadEnvFile("tests/etc/onlineconf/onlineconf.env")
    envVars["ONLINECONFIG_FROM_ENV"] = "true"
    return envVars
}

func (c *myTestConfig) configureDB(envVars map[string]string, env *gtt.Env) {
    pg := services.MustGetTyped[*psql.Env](env.Manager(), "postgres")
    svc := c.ServiceName()
    envVars[fmt.Sprintf("OC_%s__db__main", svc)] = fmt.Sprintf("%s:%s", pg.DBHost, pg.DBPort)
    envVars[fmt.Sprintf("OC_%s__db__main__User", svc)] = pg.DBUser
    envVars[fmt.Sprintf("OC_%s__db__main__Password", svc)] = pg.DBPass
    envVars[fmt.Sprintf("OC_%s__db__main__DB", svc)] = pg.DBName
}

func (c *myTestConfig) ApplyMigrations(ctx context.Context, db *sql.DB) error {
    migrationsPath, _ := filepath.Abs(filepath.Join("..", "etc/database/postgres"))
    migrationFiles := []string{"01_schema.sql"}

    for _, file := range migrationFiles {
        content, err := os.ReadFile(filepath.Join(migrationsPath, file))
        if err != nil {
            continue
        }
        if _, err := db.ExecContext(ctx, string(content)); err != nil {
            return fmt.Errorf("apply migration %s: %w", file, err)
        }
    }
    return nil
}

func (c *myTestConfig) CleanupTables(ctx context.Context, db *sql.DB) error {
    tables := []string{"users", "sessions"}
    for _, table := range tables {
        if _, err := db.ExecContext(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)); err != nil {
            return fmt.Errorf("truncate %s: %w", table, err)
        }
    }
    return nil
}

func (t *testEnvInitializerImpl) InitTestEnv() (testutil.TestAppConfig, *gtt.Env) {
    config := &myTestConfig{YourProjectConfig: NewYourProjectConfig()}
    services.MustRegisterServiceFuncTyped("postgres", psql.Run)
    servicesMap := services.NewServicesMap("postgres")
    manager := services.NewManager(servicesMap, services.DefaultManagerConfig())
    env := gtt.NewEnv(gtt.EnvConfig{}, manager)
    return config, env
}

func init() {
    testEnvInit = &testEnvInitializerImpl{}
}
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

type MyFeatureTestSuite struct {
    BaseTestSuite
}

func (s *MyFeatureTestSuite) TestMyFeature() {
    // 1. Подготовка данных
    // 2. Выполнение запроса
    resp, body := s.client.Get(s.T(), "/my/endpoint")
    // 3. Проверка результата
    s.Require().Equal(http.StatusOK, resp.StatusCode)
}

func TestMyFeatureTestSuite(t *testing.T) {
    suite.Run(t, new(MyFeatureTestSuite))
}
```

### HTTP клиент

```go
// GET запрос
resp, body := s.client.Get(s.T(), "/endpoint")

// POST запрос
reqBody := map[string]interface{}{"field": "value"}
resp, body := s.client.Post(s.T(), "/endpoint", reqBody)

// С авторизацией
s.client.WithAuth(token)
resp, body := s.client.Get(s.T(), "/protected")
```

### Методы BaseTestSuite

| Метод | Описание |
|-------|----------|
| `s.ctx` | Context для операций |
| `s.env` | GOAT Environment |
| `s.client` | HTTP клиент |
| `s.Mocks()` | Mock-серверы |

## Mock-серверы для внешних API

Если приложение использует `ogen_client`:

```go
package tests

import (
    "context"
    "net/http"

    externalapi "github.com/your-org/your-project/pkg/rest/external/v1"
    externalmock "github.com/your-org/your-project/pkg/mocks/external"
    "go.uber.org/mock/gomock"
)

type MockServers struct {
    ExternalHandler *externalmock.MockHandler
}

func HTTPMocksSetup(mocks *MockServers) func(server *http.ServeMux, ctl *gomock.Controller) {
    return func(server *http.ServeMux, ctl *gomock.Controller) {
        mocks.ExternalHandler = externalmock.NewMockHandler(ctl)

        externalServer, _ := externalapi.NewServer(mocks.ExternalHandler, nil)
        server.Handle("/external/", externalServer)
    }
}
```

### Использование в тестах

```go
func (s *MyTestSuite) TestExternalAPICall() {
    s.Mocks().ExternalHandler.EXPECT().
        GetData(gomock.Any()).
        Return(&externalapi.DataResponse{Status: "success"}, nil).
        Times(1)

    resp, body := s.client.Get(s.T(), "/my-endpoint")
    s.Require().Equal(http.StatusOK, resp.StatusCode)
}
```

## Запуск тестов

### Через Makefile

```bash
make goat-tests           # Сборка и запуск
make goat-tests-verbose   # С подробным выводом
make build_for_test-api   # Только сборка бинарника
```

### Напрямую

```bash
# Сборка бинарника
CGO_ENABLED=1 go build -cover -race -o /tmp/api ./cmd/api

# Запуск тестов
go test -v ./tests/...

# Конкретный suite
go test -v -run TestAuthTestSuite ./tests/...

# Конкретный тест
go test -v -run TestAuthTestSuite/TestLogin ./tests/...
```

## Структура файлов

```
your-project/
├── cmd/api/
│   └── main.go
├── tests/
│   ├── psg_*.go           # [generated]
│   ├── init.go            # [manual] Ваша реализация
│   ├── factories.go       # [manual] Фабрики данных
│   ├── fixtures.go        # [manual] Константы
│   ├── assertions.go      # [manual] Assertion helpers
│   ├── auth_test.go       # [manual] Ваши тесты
│   └── etc/onlineconf/    # Тестовая конфигурация
└── etc/database/postgres/ # SQL миграции
```

## Best Practices

1. **Используйте фабрики** для создания тестовых данных
2. **Используйте fixtures** для констант
3. **Наследуйте BaseTestSuite**
4. **Очищайте данные между тестами** (CleanupTables)
5. **Понятные имена тестов**: `TestFeature_WhenCondition_ExpectedResult`

## Troubleshooting

### "API did not become ready"

- Проверьте бинарник: `ls -la /tmp/api`
- Docker запущен
- Порты не заняты
- Логи: `tail -f /tmp/yourproject-test-*.log`

### "TestEnvInitializer not implemented"

- Создайте `tests/init.go`
- Реализуйте интерфейс
- Присвойте в `init()`: `testEnvInit = &testEnvInitializerImpl{}`

## Ресурсы

- [GOAT Framework](https://github.com/Educentr/goat)
- [Testcontainers Go](https://golang.testcontainers.org/)
- [testify/suite](https://github.com/stretchr/testify)
