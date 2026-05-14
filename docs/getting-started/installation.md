# Установка

## Системные требования

- **Go 1.24+** — [Установка Go](https://go.dev/doc/install)
- **Docker** — для локальной разработки и тестирования
- **Git** — для автоматической инициализации репозитория (опционально)

## Установка через Go

Рекомендуемый способ установки:

```bash
go install github.com/Educentr/go-project-starter/cmd/go-project-starter@latest
```

После установки проверьте, что команда доступна:

```bash
go-project-starter --help
```

## Установка конкретной версии

```bash
# Установка определённой версии
go install github.com/Educentr/go-project-starter/cmd/go-project-starter@v0.12.0

# Проверка версии
go-project-starter --version
```

## Сборка из исходников

```bash
# Клонирование репозитория
git clone https://github.com/Educentr/go-project-starter
cd go-project-starter

# Сборка
make build

# Или напрямую
go build -o go-project-starter ./cmd/go-project-starter

# Установка
go install ./cmd/go-project-starter
```

## Обновление

Для обновления до последней версии:

```bash
go install github.com/Educentr/go-project-starter/cmd/go-project-starter@latest
```

## Проверка установки

```bash
# Версия
go-project-starter --version

# Справка
go-project-starter --help
```

## Следующий шаг

Перейдите к [быстрому старту](quickstart.md), чтобы создать свой первый проект.
