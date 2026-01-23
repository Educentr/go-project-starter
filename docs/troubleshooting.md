# Troubleshooting

FAQ и решение типичных проблем.

## Генерация

### Ошибка "main.name is required"

**Причина:** Отсутствует обязательное поле `main.name` в конфигурации.

**Решение:**

```yaml
main:
  name: my-service  # Добавьте это поле
```

### Ошибка "duplicate rest name"

**Причина:** Два REST транспорта с одинаковым именем и версией.

**Решение:** Используйте уникальные комбинации name + version:

```yaml
rest:
  - name: api
    version: v1
  - name: api
    version: v2  # OK - разные версии
```

### Файлы не генерируются

**Причина:** Неправильный путь к target директории.

**Решение:**

```bash
# Используйте абсолютный путь
go-project-starter --config=config.yaml --target=/home/user/my-project

# Или убедитесь, что директория существует
mkdir -p ./my-project
go-project-starter --config=config.yaml --target=./my-project
```

## Сборка проекта

### "cannot find package"

**Причина:** Зависимости не загружены.

**Решение:**

```bash
go mod download
go mod tidy
```

### "undefined: ogen"

**Причина:** Ogen код не сгенерирован.

**Решение:**

```bash
make generate
# или
make ogen
```

### Приватные модули не загружаются

**Причина:** Не настроен GOPRIVATE.

**Решение:** Добавьте в конфигурацию:

```yaml
git:
  private_repos:
    - github.com/myorg/*
```

## Docker

### "Cannot connect to Docker daemon"

**Причина:** Docker не запущен.

**Решение:**

```bash
# Linux
sudo systemctl start docker

# macOS
open -a Docker
```

### "port is already allocated"

**Причина:** Порт занят другим процессом.

**Решение:**

```bash
# Найти процесс
lsof -i :8080

# Или изменить порт в docker-compose
# Или остановить conflicting контейнер
docker compose down
```

## Dev Stand

### OnlineConf updater не создаёт TREE.cdb

**Причина:** Проблемы с MySQL или Admin UI.

**Решение:**

```bash
# Проверить логи
docker compose -f docker-compose-dev.yaml logs onlineconf-updater
docker compose -f docker-compose-dev.yaml logs onlineconf-database

# Пересоздать окружение
make dev-drop
make dev-up
```

### Изменения в init-config.sql не применяются

**Причина:** MySQL выполняет init-скрипты только при первом запуске.

**Решение:**

```bash
make dev-drop  # Удаляет volumes
make dev-up
```

### Submodule не инициализирован

**Причина:** OnlineConf submodule не загружен.

**Решение:**

```bash
git submodule update --init --recursive
```

## Тесты

### "API did not become ready"

**Причина:** Приложение не запустилось.

**Решение:**

1. Проверьте бинарник: `ls -la /tmp/api`
2. Проверьте Docker: `docker ps`
3. Посмотрите логи: `tail -f /tmp/yourproject-test-*.log`

### "Database connection refused"

**Причина:** Testcontainers не запустили PostgreSQL.

**Решение:**

1. Проверьте Docker daemon
2. Проверьте права на Docker socket
3. Увеличьте таймаут запуска

### "TestEnvInitializer not implemented"

**Причина:** Не создан файл `tests/init.go`.

**Решение:** Создайте файл с реализацией интерфейса (см. [GOAT](testing/goat.md)).

## CI/CD

### "permission denied" в GitHub Actions

**Причина:** Неправильные секреты или права.

**Решение:**

1. Проверьте секреты в Settings → Secrets
2. Убедитесь, что SSH ключ правильный
3. Проверьте права на registry

### Docker push fails

**Причина:** Неверные credentials для registry.

**Решение:**

```bash
# GitHub Container Registry
gh secret set GHCR_USER --body "username"
gh secret set GHCR_TOKEN --body "ghp_..."

# Проверьте локально
docker login ghcr.io -u username -p token
```

## Общие советы

### Предпросмотр изменений

Используйте `--dry-run` перед генерацией:

```bash
go-project-starter --dry-run --config=config.yaml --target=.
```

### Логирование

Включите debug логирование:

```bash
export LOG_LEVEL=debug
go-project-starter --config=config.yaml
```

### Версии инструментов

Убедитесь, что версии совместимы:

```bash
go version          # Должен быть 1.24+
docker --version    # Должен быть 20.10+
```

### Очистка и перегенерация

При серьёзных проблемах:

```bash
# Сохраните ваш код
cp -r internal/app/*/transport/rest/*/handler.go /tmp/backup/

# Удалите сгенерированные файлы
rm -rf cmd/ internal/ pkg/ Makefile Dockerfile docker-compose.yaml

# Перегенерируйте
go-project-starter --config=config.yaml

# Восстановите ваш код
cp /tmp/backup/*.go internal/app/*/transport/rest/*/
```

## Получение помощи

Если проблема не решена:

1. Проверьте [GitHub Issues](https://github.com/Educentr/go-project-starter/issues)
2. Создайте новый issue с:
   - Версия генератора (`go-project-starter --version`)
   - Конфигурация (без секретов)
   - Полный вывод ошибки
   - Шаги воспроизведения
