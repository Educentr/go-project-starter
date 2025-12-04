# Docker Integration Tests

Интеграционные тесты для go-project-starter, запускаемые в Docker-контейнере.

## Запуск тестов

```bash
# Все тесты
make integration-test

# С подробным выводом
make integration-test-verbose

# Отдельные тесты
make integration-test-rest
make integration-test-grpc
make integration-test-telegram
make integration-test-combined
```

## Как это работает

### Docker-образ

Тесты запускаются внутри Docker-контейнера с предустановленными инструментами:
- Go (версия задаётся в Makefile)
- buf (для proto генерации)
- protoc-gen-go, protoc-gen-go-grpc
- goimports

Образ собирается из `Dockerfile` в этой директории.

### Параметры сборки образа

Параметры задаются в `Makefile`:

```makefile
INTEGRATION_GO_VERSION := 1.24.4
INTEGRATION_BUF_VERSION := 1.47.2
INTEGRATION_IMAGE_NAME := go-project-starter-test
```

Дополнительно используется `GITHUB_TOKEN` из окружения для доступа к приватным репозиториям.

### Автоматическая пересборка образа

Образ пересобирается автоматически только при изменении параметров. Для этого используется marker файл `.image-params`, который содержит текущие значения параметров.

При запуске `make integration-test`:
1. Сравниваются текущие параметры с сохранёнными в `.image-params`
2. Если параметры изменились или файл отсутствует — образ пересобирается
3. Если параметры не изменились — используется существующий образ

Для принудительной пересборки удалите marker файл:
```bash
rm test/docker-integration/.image-params
make integration-test
```

### Структура тестов

```
test/docker-integration/
├── Dockerfile           # Dockerfile для тестового образа
├── README.md            # Этот файл
├── integration_test.go  # Код тестов
├── .image-params        # Marker файл (в .gitignore)
└── configs/             # Тестовые конфигурации
    ├── rest-only/
    ├── grpc-only/
    ├── worker-telegram/
    └── combined/
```

### Переменные окружения

| Переменная | Описание |
|------------|----------|
| `GITHUB_TOKEN` | Токен для доступа к приватным репозиториям (опционально) |
| `TEST_IMAGE` | Имя Docker-образа (устанавливается автоматически через Makefile) |
| `GOAT_DISABLE_STDOUT` | Отключить вывод генератора в тестах |

## Ручная сборка образа

```bash
# Собрать образ вручную
make build-test-image

# Собрать с кастомными параметрами
GITHUB_TOKEN=xxx make build-test-image
```
