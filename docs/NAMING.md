# Naming Conventions

This document describes the naming hierarchy used in generated Go projects.

## Naming Hierarchy

| Name | Source | Example | Availability |
|------|--------|---------|--------------|
| **ServiceName** | `Main.Name` | `"my-api"` | `constant.ServiceName` (everywhere via import) |
| **AppName** | `Application.Name` | `"web-app"` | `ds.AppInfo.AppName`, function parameter |
| **TransportName** | `Transport.Name` | `"api_v1"` | Function parameter in middlewares |
| **WorkerName** | `Worker.Name` | `"telegram"` | In worker templates |

## Detailed Descriptions

### ServiceName (Main.Name)

The top-level project name, defined in the YAML configuration as `main.name`.

- **Access**: `constant.ServiceName` (generated in `internal/app/constant/constant.go`)
- **Usage**: Prometheus metrics namespace, OnlineConf root path, logging
- **Example**: `"my-api"`, `"payment-service"`

### AppName (Application.Name)

The name of a specific application within the service. One service can have multiple applications (e.g., web frontend, worker backend).

- **Access**: `ds.AppInfo.AppName`, passed as function parameter
- **Usage**: X-Server header, app-specific OnlineConf paths, build info metrics
- **Example**: `"web-app"`, `"worker-app"`, `"cron-app"`

### TransportName (Transport.Name)

The name of a specific REST/gRPC transport within an application.

- **Access**: Passed as function parameter in middleware functions
- **Usage**: OnlineConf transport-specific paths, per-transport configuration
- **Example**: `"api_v1"`, `"internal-api"`, `"sys"`

### WorkerName (Worker.Name)

The name of a background worker (Kafka consumer, Telegram bot, etc.).

- **Access**: Available in worker templates
- **Example**: `"telegram"`, `"kafka-consumer"`

## OnlineConf Path Structure

OnlineConf paths follow a 3-level priority hierarchy for REST transports:

1. **Default from code** - hardcoded values
2. **Transport-level**: `/{serviceName}/transport/rest/{transportName}/{key}`
3. **App-specific**: `/{serviceName}/transport/rest/{transportName}/{appName}/{key}`

### Example Paths

For a service `my-api` with application `web-app` and transport `api_v1`:

```
# Transport-level (applies to all apps using this transport)
/my-api/transport/rest/api_v1/timeout
/my-api/transport/rest/api_v1/port

# App-specific (overrides for specific application)
/my-api/transport/rest/api_v1/web-app/timeout
/my-api/transport/rest/api_v1/web-app/port
```

## Code Usage Examples

### In main.go

```go
// ServiceName is available globally
constant.ServiceName  // "my-api"

// AppInfo contains ApplicationName
info := getAppInfo()
info.AppName          // "web-app" (Application.Name)
info.Version          // "v1.0.0"
```

### In Middleware

```go
// GetMiddlewares receives all naming parameters
func (dmw *DefaultMiddlewares) GetMiddlewares(
    ctx context.Context,
    appName string,           // Application.Name (e.g., "web-app")
    metrics *prometheus.Registry,
    srv ds.IService,
    serviceName string,       // Main.Name (e.g., "my-api")
    transportName string,     // Transport.Name (e.g., "api_v1")
    errHdl rest.RestErrorHandler,
) ([]func(next http.Handler) http.Handler, error)
```

### Prometheus Metrics

```go
// Namespace uses serviceName (sanitized)
prometheus.NewHistogramVec(prometheus.HistogramOpts{
    Namespace: strings.ReplaceAll(serviceName, "-", "_"), // "my_api"
    Name:      "request_duration_seconds",
    ...
})
```

### Build Info Metrics

```go
// Build info includes both service_name and app_name
prometheus.Labels{
    "service_name": serviceName,  // "my-api" (Main.Name)
    "app_name":     info.AppName, // "web-app" (Application.Name)
    "version":      info.Version,
    ...
}
```

### Notifications/Logging

```go
// Use both for clarity
fmt.Sprintf("Started: %s/%s v%s", constant.ServiceName, info.AppName, info.Version)
// "Started: my-api/web-app v1.0.0"
```

## Breaking Changes in v0.11.0

- `ds.NewAppInfo()` now takes `appName` (Application.Name) instead of `serviceName`
- `AppInfo.AppName` now contains Application.Name, not ServiceName
- Use `constant.ServiceName` for the service-level name
- OnlineConf paths changed to 3-level hierarchy

## Migration Guide

### Before v0.11.0

```go
// Old: AppInfo.AppName was ServiceName
info := ds.NewAppInfo(constant.ServiceName)  // AppName = "my-api"
```

### After v0.11.0

```go
// New: AppInfo.AppName is ApplicationName
info := ds.NewAppInfo("web-app")             // AppName = "web-app"
// Use constant.ServiceName for service-level name
```
