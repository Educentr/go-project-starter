# Регенерация и миграции

Описание процесса регенерации проекта и миграций между версиями.

## Сохранение пользовательского кода

**Уникальная возможность:** Перегенерируйте весь проект без потери ваших изменений.

### Как работают Disclaimer-маркеры

Каждый сгенерированный файл содержит маркер:

```go
// ==========================================
// GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
// Changes manually made below will not be overwritten by generator.
// ==========================================
```

**Правила:**

1. Код выше маркера регенерируется при каждом запуске
2. Код ниже маркера сохраняется навсегда
3. Если нужно изменить сгенерированный код — переместите его ниже маркера

### Пример

```go
package handler

import (
    "context"
    "net/http"
)

// ==========================================
// GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
// ==========================================

func (h *Handler) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
    // Ваш код здесь - переживёт регенерацию!

    user := &User{
        Email: req.Email,
        Name:  req.Name,
    }

    // Кастомная валидация
    if err := h.validateBusinessRules(user); err != nil {
        return nil, err
    }

    return h.repo.Create(ctx, user)
}

// Ваши вспомогательные функции
func (h *Handler) validateBusinessRules(user *User) error {
    // ...
}
```

## Workflow регенерации

### При изменении OpenAPI/Protobuf

Используйте `make generate`:

```bash
# 1. Изменить api/openapi.yaml
# 2. Регенерировать код
make generate

# Ваш код в handlers.go сохранится
```

### При изменении config.yaml

Запустите генератор повторно:

```bash
# 1. Изменить config.yaml
# 2. Предпросмотр изменений (опционально)
go-project-starter --dry-run --config=config.yaml --target=.

# 3. Применить изменения
go-project-starter --config=config.yaml --target=.
```

### Файлы, которые никогда не перезаписываются

- `.gitignore`
- `go.mod`, `go.sum`
- `LICENSE.txt`
- `README.md`
- `.git/` директория

## Миграции между версиями

При обновлении генератора на новую версию может потребоваться миграция конфигурации.

### Проверка версии

```bash
go-project-starter --version
```

### Запуск миграции

```bash
go-project-starter migrate --configDir=.project-config
```

### Что делает миграция

1. Читает текущую конфигурацию
2. Применяет изменения для устаревших полей:
   - Переименование полей
   - Изменение структуры
   - Удаление deprecated-полей
3. Сохраняет обновлённую конфигурацию

### Пример миграции

```yaml
# До миграции (старый формат)
rest:
  - name: api
    generator: ogen

# После миграции (новый формат)
rest:
  - name: api
    generator_type: ogen
```

## Добавление новых компонентов

### Добавление нового REST транспорта

1. Добавьте в `config.yaml`:

```yaml
rest:
  - name: admin
    path: [./api/admin.yaml]
    generator_type: ogen
    port: 8081
    version: v1
```

2. Добавьте в application:

```yaml
applications:
  - name: api
    transport: [api, admin, sys]
```

3. Регенерируйте:

```bash
go-project-starter --config=config.yaml
```

### Добавление нового воркера

1. Добавьте в `config.yaml`:

```yaml
workers:
  - name: email_sender
    generator_type: daemon
```

2. Добавьте в application:

```yaml
applications:
  - name: workers
    workers: [telegram_bot, email_sender]
```

3. Регенерируйте проект.

## Удаление компонентов

При удалении компонента из конфигурации и перегенерации проекта:

1. **Сгенерированные файлы без user code** — удаляются автоматически.
   Генератор определяет устаревшие файлы по наличию disclaimer-маркера
   и отсутствию в текущем наборе шаблонов.

2. **Сгенерированные файлы с user code** — генерация завершается ошибкой.
   Перенесите свой код в другое место и удалите файл вручную.

3. **Пользовательские файлы** (без disclaimer) — не затрагиваются.

При использовании `--dry-run` устаревшие файлы отображаются в выводе
как "Remove obsolete file".

## Best Practices

### Версионирование конфигурации

Храните `config.yaml` в Git:

```bash
git add config.yaml
git commit -m "Add admin API transport"
```

### Предпросмотр изменений

Используйте `--dry-run` перед регенерацией:

```bash
go-project-starter --dry-run --config=config.yaml --target=.
```

### Регулярная регенерация

Регенерируйте проект после обновления генератора:

```bash
go install github.com/Educentr/go-project-starter/cmd/go-project-starter@latest
go-project-starter --config=config.yaml --target=.
```

Это обеспечит актуальность инфраструктурного кода.
