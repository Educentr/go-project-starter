# Рабочий процесс

Описание рабочего процесса с Go Project Starter.

## Содержание раздела

- [Регенерация](regeneration.md) — регенерация проекта и миграции
- [Makefile](makefile.md) — цели Makefile сгенерированного проекта
- [OnlineConf](onlineconf.md) — интеграция с OnlineConf

## Типичный workflow

### 1. Создание проекта

```bash
# Интерактивный wizard
go-project-starter init --target=.

# Или генерация из готового конфига
go-project-starter --config=config.yaml
```

### 2. Разработка

```bash
# Запуск зависимостей
make docker-up

# Генерация кода из OpenAPI
make generate

# Запуск сервиса
make run
```

### 3. Изменение API

1. Обновите OpenAPI спецификацию
2. Регенерируйте код:

```bash
make generate
```

### 4. Изменение структуры проекта

1. Обновите `config.yaml`
2. Регенерируйте проект:

```bash
go-project-starter --config=config.yaml
```

Ваш код ниже disclaimer-маркеров будет сохранён.

### 5. Тестирование

```bash
make test           # Unit тесты
make goat-tests     # Интеграционные GOAT тесты
```

### 6. Деплой

```bash
make docker-build   # Сборка Docker образа
make docker-push    # Push в registry
```

## Файлы, которые никогда не перезаписываются

- `.gitignore`
- `go.mod`, `go.sum`
- `LICENSE.txt`
- `README.md`
- `.git/` директория

## Следующие шаги

- [Регенерация](regeneration.md) — подробнее о регенерации
- [Makefile](makefile.md) — все цели Makefile
- [OnlineConf](onlineconf.md) — динамическая конфигурация
