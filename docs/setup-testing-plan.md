# План тестирования Setup команды

## Цель
End-to-end тестирование `go-project-starter setup` на реальной инфраструктуре.

## Параметры тестирования
- **Репозиторий:** `Educentr/example-telegram-bot`
- **Module path:** `github.com/Educentr/example-telegram-bot`
- **Сервер:** `128.140.2.112` (Hetzner)
- **Registry:** `ghcr.io` (GitHub Container Registry)
- **CI/CD:** GitHub Actions
- **Тип проекта:** Telegram бот (worker + sys metrics)

---

## Шаг 1: Подготовка

### 1.1 Создать Telegram бота
```
1. Открыть @BotFather в Telegram
2. /newbot
3. Указать имя и username
4. Сохранить токен
```

### 1.2 Создать репозиторий на GitHub
```bash
gh repo create Educentr/example-telegram-bot --private --clone
```

### 1.3 Адаптировать конфиг проекта
Создать директорию `.project-config/` с:
- `project.yaml` - адаптированный конфиг телеграм бота

**Изменения в project.yaml:**
- `git.repo` → `git@github.com:Educentr/example-telegram-bot.git`
- `git.module_path` → `github.com/Educentr/example-telegram-bot`
- `main.name` → `examplebot`
- `driver.import` → адаптировать под module_path

---

## Шаг 2: Генерация проекта

```bash
# Установить актуальную версию генератора
go install ./cmd/go-project-starter

# Сгенерировать проект
cd ~/path/to/example-telegram-bot
go-project-starter --configDir=.project-config --target=.
```

**Что проверить:**
- [ ] Сгенерированы все файлы (cmd/, internal/, pkg/, .github/workflows/)
- [ ] go.mod корректный
- [ ] CI/CD workflow сгенерирован

---

## Шаг 3: Setup Wizard

```bash
go-project-starter setup --configDir=.project-config --target=.
```

### 3.1 Тестируемые вопросы wizard'а:
1. Admin email
2. CI provider → GitHub
3. Repository → Educentr/example-telegram-bot
4. Registry type → GitHub
5. Registry server → ghcr.io
6. Container name
7. Environments (production на main branch)
8. Server host → 128.140.2.112
9. SSH user/port
10. Internal subnet
11. Telegram notifications (опционально)

**Что проверить:**
- [ ] Все вопросы задаются корректно
- [ ] setup.yaml сохраняется
- [ ] Нет ошибок/паник

---

## Шаг 4: Настройка GitHub Actions (setup ci)

```bash
go-project-starter setup ci --configDir=.project-config --target=.
```

**Проверяемые секреты:**
- [ ] `SSH_PRIVATE_KEY`
- [ ] `SSH_USER`
- [ ] `GHCR_USER`
- [ ] `GHCR_TOKEN`
- [ ] `GH_PAT` (для private repos)

**Проверяемые переменные:**
- [ ] `REGISTRY_CONTAINER`
- [ ] `MAIN_ENABLED`
- [ ] `MAIN_SSH_HOST`
- [ ] `MAIN_PORT_PREFIX_SYS`

**Способы проверки:**
```bash
# Через gh CLI
gh secret list -R Educentr/example-telegram-bot
gh variable list -R Educentr/example-telegram-bot
```

---

## Шаг 5: Настройка сервера (setup server)

```bash
go-project-starter setup server --configDir=.project-config --target=.
```

### 5.1 Шаги настройки сервера:
1. [ ] Генерация SSH ключей для deploy user
2. [ ] Создание deploy пользователя
3. [ ] Установка Docker + buildx + compose
4. [ ] Установка docker-rollout плагина
5. [ ] Docker registry login (ghcr.io)
6. [ ] Создание директорий `/opt/examplebot-cd/`

**Проверка на сервере:**
```bash
ssh root@128.140.2.112
# Проверить наличие deploy пользователя
id deploy
# Проверить Docker
docker --version
docker compose version
# Проверить директории
ls -la /opt/examplebot-cd/
```

---

## Шаг 6: Deploy скрипт (setup deploy)

```bash
go-project-starter setup deploy --configDir=.project-config --target=.
```

**Что проверить:**
- [ ] `scripts/deploy.sh` создан
- [ ] Скрипт исполняемый
- [ ] Корректные имена приложений

---

## Шаг 7: Полный цикл CI/CD

### 7.1 Initial push
```bash
git add .
git commit -m "Initial commit: generated telegram bot"
git push -u origin main
```

### 7.2 Мониторинг pipeline
```bash
gh run watch
```

**Проверяемые этапы:**
- [ ] `prepare-env` - переменные подготовлены
- [ ] `create_env_files` - .env файлы созданы
- [ ] `build` - Docker образ собран и запушен в ghcr.io
- [ ] `deploy` - деплой на сервер успешен

### 7.3 Проверка на сервере
```bash
ssh deploy@128.140.2.112
docker ps  # контейнер запущен
curl localhost:8085/health  # sys endpoint отвечает
```

---

## Ожидаемые проблемы и доработки

### Известные ограничения:
1. **DNS** - только инструкции, нет автоматизации (не критично для теста)
2. **Nginx/certbot** - пропустим, т.к. метрики только по IP внутри

### Что может потребовать исправления:
1. Telegram worker специфика в CI/CD workflow
2. Генерация env файлов для бота
3. OnlineConf конфигурация (если используется)

---

## Файлы для мониторинга

- `internal/pkg/setup/setup.go` - основная логика
- `internal/pkg/setup/wizard.go` - интерактивный режим
- `internal/pkg/setup/ci_github.go` - GitHub интеграция
- `internal/pkg/setup/server.go` - настройка сервера
- `internal/pkg/setup/deploy.go` - deploy скрипт
- `internal/pkg/templater/embedded/templates/main/.github/workflows/ci_cd.yml.tmpl` - CI workflow

---

## Верификация

Тестирование считается успешным когда:
1. Wizard проходит без ошибок
2. Все секреты и переменные установлены в GitHub
3. Сервер настроен (Docker, deploy user)
4. CI/CD pipeline проходит полностью
5. Контейнер запущен на сервере
6. Sys endpoint отвечает на health check

---

## Итог: Документация

После успешного тестирования на основе этого плана создать:

**Файл:** `docs/deployment-guide.md`

**Содержание:** Пошаговая инструкция для начинающих по развертыванию Telegram бота:
1. Требования (GitHub аккаунт, сервер, домен)
2. Создание бота через @BotFather
3. Генерация проекта
4. Настройка CI/CD через setup wizard
5. Деплой на сервер
6. Проверка работоспособности
7. Troubleshooting частых проблем
