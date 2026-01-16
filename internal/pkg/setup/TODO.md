# TODO: Setup Module

## Актуализация .env файлов

### Проблема
Сейчас .env файлы генерируются с неполным списком переменных. Нужно:
1. Собрать полный список всех OC (OnlineConf) переменных
2. В зависимости от используемых частей проекта собирать только нужные переменные

### Предлагаемое решение
Создать файлы со списком переменных внутри каждой модульной части проекта:

```
templater/embedded/templates/
├── transport/
│   ├── rest/
│   │   └── env_vars.yaml      # HTTP_PORT, HTTP_HOST, etc.
│   ├── grpc/
│   │   └── env_vars.yaml      # GRPC_PORT, GRPC_HOST, etc.
│   └── kafka/
│       └── env_vars.yaml      # KAFKA_BROKERS, KAFKA_GROUP_ID, etc.
├── worker/
│   ├── telegram/
│   │   └── env_vars.yaml      # TELEGRAM_BOT_TOKEN, etc.
│   └── daemon/
│       └── env_vars.yaml      # DAEMON_INTERVAL, etc.
├── driver/
│   ├── postgres/
│   │   └── env_vars.yaml      # DB_HOST, DB_PORT, DB_USER, etc.
│   ├── redis/
│   │   └── env_vars.yaml      # REDIS_HOST, REDIS_PORT, etc.
│   └── kafka/
│       └── env_vars.yaml      # KAFKA_BROKERS, KAFKA_TLS_*, etc.
└── common/
    └── env_vars.yaml          # LOG_LEVEL, APP_ENV, etc.
```

### Формат env_vars.yaml
```yaml
variables:
  - name: HTTP_PORT
    description: HTTP server port
    default: "8080"
    required: true
    onlineconf_path: /http/port

  - name: HTTP_HOST
    description: HTTP server bind address
    default: "0.0.0.0"
    required: false
    onlineconf_path: /http/host
```

### При генерации
1. Собрать все env_vars.yaml из используемых модулей
2. Объединить в один список (исключая дубликаты)
3. Сгенерировать:
   - `.env.example` - пример с описаниями
   - `.env-{app}` - файл для каждого приложения
   - OnlineConf init SQL (если используется)

### Связанные файлы
- `internal/pkg/templater/templater.go` - логика сборки шаблонов
- `internal/pkg/setup/deploy.go` - генерация .env файлов при деплое
- CI/CD workflows - создание env файлов на сервере

## SSH Key Handling (WIP)
- [x] Генерация ключа в setup ci
- [x] Предупреждение при перегенерации
- [x] Генерация public из private
- [x] Сохранение public для переиспользования
- [ ] Тестирование полного цикла CI/CD

## Документация "How to Start"
На основе `docs/how-to-start.md` создать исчерпывающую документацию:
- [ ] Пошаговое руководство по началу работы
- [ ] Описание всех команд setup
- [ ] Примеры конфигураций для разных типов проектов
- [ ] Troubleshooting частых проблем
