# Команды

Подробное описание команд Go Project Starter.

## Генерация проекта (по умолчанию)

Основная команда для генерации проекта из YAML-конфигурации:

```bash
go-project-starter --config=config.yaml
```

### Флаги

| Флаг | Описание | По умолчанию |
|------|----------|--------------|
| `--config` | Путь к файлу конфигурации | `project.yaml` |
| `--configDir` | Директория с конфигурацией | `.` |
| `--target` | Целевая директория для генерации | `.` |
| `--dry-run` | Показать изменения без записи файлов | `false` |

### Примеры

```bash
# Генерация в текущую директорию
go-project-starter --config=config.yaml

# Генерация в указанную директорию
go-project-starter --config=config.yaml --target=./my-service

# Предпросмотр изменений (dry-run)
go-project-starter --dry-run --config=config.yaml --target=./my-service

# Конфигурация из директории
go-project-starter --configDir=.project-config --config=project.yaml --target=.
```

## init

Интерактивный wizard для создания начальной конфигурации проекта.

```bash
go-project-starter init --target=.
```

### Процесс

1. Ввод имени проекта
2. Выбор логгера (zerolog)
3. Выбор типа Docker registry (github, digitalocean, aws, selfhosted)
4. Настройка Git репозитория
5. Выбор типа проекта:
   - REST API
   - gRPC Service
   - Telegram Bot
   - Multi-app

### Вывод

```
=== Go Project Starter - Project Initialization ===

--- Basic Information ---
? Project name (lowercase, no spaces): my-service
? Logger: zerolog
? Docker registry type: github

? Git repository URL: git@github.com:myorg/my-service.git
? Go module path: github.com/myorg/my-service

--- Project Type ---
? What type of project? REST API

Configuration saved to .project-config/project.yaml

Next steps:

  1. Review and customize project.yaml
  2. Run: go-project-starter --configDir=.project-config --target=.
  3. Run: go-project-starter setup --configDir=.project-config --target=.
```

## setup

Автоматизация развёртывания проекта — настройка CI/CD, серверов, deploy-скриптов.

```bash
go-project-starter setup [command]
```

### Подкоманды

| Команда | Описание |
|---------|----------|
| (без подкоманды) | Полный интерактивный wizard |
| `ci` | Настройка CI/CD |
| `server` | Настройка серверов |
| `deploy` | Генерация deploy скриптов |

### Wizard

Интерактивный wizard собирает информацию:

1. CI/CD провайдер (GitHub Actions / GitLab CI)
2. Окружения (staging, production)
3. Серверы и SSH доступ
4. Registry credentials

### Пример

```bash
$ go-project-starter setup
=== Go Project Starter Setup Wizard ===

? Admin email (for certbot and notifications): admin@example.com
ℹ Detected CI provider: github
? Repository (owner/repo): myorg/my-service
ℹ Using registry type from project.yaml: github

--- Servers ---
? How many deployment servers? 1

--- Server 1 ---
? Server name (logical identifier): production
? Server host (IP or hostname): 192.168.1.100
? SSH port: 22
? SSH user (for initial setup): root
? Deploy user (will be created): deploy

--- Environments ---
? How many environments? 1

--- Environment 1 ---
? Environment name: production
? Git branch: main
? Use OnlineConf for configuration management? No

--- Notifications ---
? Enable Telegram notifications? No
? Enable Slack notifications? No

✓ Configuration saved to setup.yaml

--- CI/CD Setup ---
ℹ GitHub CLI (gh) detected. Will use it to configure secrets.

Configuring GitHub secrets...
✓ Set secret: SSH_PRIVATE_KEY
✓ Set secret: SSH_USER
✓ Set secret: REGISTRY_LOGIN_SERVER
✓ Set secret: GHCR_USER
✓ Set secret: GHCR_TOKEN

--- Deploy Script ---
✓ Generated deploy script: scripts/deploy.sh

=== Setup Complete ===
```

### Генерируемые файлы

- `setup.yaml` — сохранённая конфигурация wizard
- `.github/workflows/` или `.gitlab-ci.yml` — CI/CD файлы
- `scripts/deploy.sh` — скрипт развёртывания

## migrate

Миграция конфигурации на новую версию генератора.

```bash
go-project-starter migrate --configDir=.project-config
```

### Когда использовать

- После обновления генератора на новую версию
- При появлении deprecated-полей в конфигурации

### Процесс

1. Чтение текущей конфигурации
2. Применение миграций для устаревших полей
3. Сохранение обновлённой конфигурации

## Общие флаги

Флаги, доступные для всех команд:

| Флаг | Описание |
|------|----------|
| `--help`, `-h` | Показать справку |
| `--version` | Показать версию |
