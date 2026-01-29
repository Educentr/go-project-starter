# Naming Conventions

Соглашения об именовании в сгенерированных проектах.

## Иерархия имён

| Имя | Источник | Пример | Доступ |
|-----|----------|--------|--------|
| **ServiceName** | `Main.Name` | `"my-api"` | `constant.ServiceName` |
| **AppName** | `Application.Name` | `"web-app"` | `ds.AppInfo.AppName` |
| **TransportName** | `Transport.Name` | `"api_v1"` | Параметр функции |
| **WorkerName** | `Worker.Name` | `"telegram"` | В шаблонах воркеров |

## Описание

### ServiceName (Main.Name)

Верхнеуровневое имя проекта из `main.name`.

- **Доступ:** `constant.ServiceName`
- **Использование:** namespace метрик, root path OnlineConf, логирование
- **Пример:** `"my-api"`, `"payment-service"`

### AppName (Application.Name)

Имя конкретного приложения. Один сервис может иметь несколько приложений.

- **Доступ:** `ds.AppInfo.AppName`, параметр функции
- **Использование:** X-Server header, app-specific OnlineConf пути
- **Пример:** `"web-app"`, `"worker-app"`

### TransportName (Transport.Name)

Имя REST/gRPC транспорта.

- **Доступ:** Параметр функции в middleware
- **Использование:** OnlineConf пути для транспорта
- **Пример:** `"api_v1"`, `"internal-api"`

### WorkerName (Worker.Name)

Имя фонового воркера.

- **Доступ:** В шаблонах воркеров
- **Пример:** `"telegram"`, `"kafka-consumer"`

## OnlineConf пути

### Приоритеты (3 уровня)

1. **Default** — значения по умолчанию в коде
2. **Transport-level:** `/{serviceName}/transport/rest/{transportName}/{key}`
3. **App-specific:** `/{serviceName}/transport/rest/{transportName}/{appName}/{key}`

### Примеры путей

Для `my-api` с application `web-app` и transport `api_v1`:

```
# Transport-level
/my-api/transport/rest/api_v1/timeout
/my-api/transport/rest/api_v1/port

# App-specific (override)
/my-api/transport/rest/api_v1/web-app/timeout
/my-api/transport/rest/api_v1/web-app/port
```

## Использование в коде

### В main.go

```go
// ServiceName глобально доступен
constant.ServiceName  // "my-api"

// AppInfo содержит ApplicationName
info := getAppInfo()
info.AppName          // "web-app"
info.Version          // "v1.0.0"
```

### В Middleware

```go
func (dmw *DefaultMiddlewares) GetMiddlewares(
    ctx context.Context,
    appName string,           // "web-app"
    metrics *prometheus.Registry,
    srv ds.IService,
    serviceName string,       // "my-api"
    transportName string,     // "api_v1"
    errHdl rest.RestErrorHandler,
) ([]func(next http.Handler) http.Handler, error)
```

### Prometheus метрики

```go
// Namespace использует serviceName (sanitized)
prometheus.NewHistogramVec(prometheus.HistogramOpts{
    Namespace: strings.ReplaceAll(serviceName, "-", "_"), // "my_api"
    Name:      "request_duration_seconds",
})
```

### Build Info

```go
prometheus.Labels{
    "service_name": serviceName,  // "my-api"
    "app_name":     info.AppName, // "web-app"
    "version":      info.Version,
}
```

### Логирование

```go
fmt.Sprintf("Started: %s/%s v%s", constant.ServiceName, info.AppName, info.Version)
// "Started: my-api/web-app v1.0.0"
```

## Миграция с ранних версий

### До v0.11.0

```go
info := ds.NewAppInfo(constant.ServiceName)  // AppName = "my-api"
```

### После v0.11.0

```go
info := ds.NewAppInfo("web-app")             // AppName = "web-app"
// Используйте constant.ServiceName для service-level name
```

**Изменения:**

- `ds.NewAppInfo()` принимает `appName` вместо `serviceName`
- `AppInfo.AppName` содержит Application.Name
- Для ServiceName используйте `constant.ServiceName`
- OnlineConf пути изменены на 3-уровневую иерархию
