# CI checks

Секция `checks:` объявляет проверки, которые сгенерированный CI запускает перед сборкой,
и на результат которых гейтятся `build` и `build-packages`.

## Зачем

До появления `checks:` единственной проверочной фазой в сгенерированном `ci_cd.yml` был
`run-goat-tests-<app>`. Ни линтер, ни юнит-тесты CI не запускал, хотя `Makefile` и
`configs/golangci-lint.yml` генератор кладёт в проект. Проверки дописывались руками
отдельными воркфлоу рядом со сгенерированным: каждая со своим триггером и ни одна — в
`needs:`, поэтому упавший линтер **не останавливал** `build` и `deploy`.

`checks:` возвращает эти проверки в один файл и в один граф зависимостей. Генератору не нужно
знать, что проверка делает: достаточно имени и команды — проект сохраняет свои собственные
гварды.

## Формат

```yaml
checks:
  - name: lint
    run: make lint
  - name: unit-tests
    run: make unit-tests
  - name: guards
    run: make lint-guards
    registry_login: true      # по умолчанию false — линтеру реестр не нужен

checks_config:
  fail_fast: false            # по умолчанию false
```

Уровень — **проектный**: линт и юнит-тесты относятся к модулю, а не к приложению.

### Поля `checks[]`

| Поле | Тип | Обязательное | Описание |
|---|---|---|---|
| `name` | string | да | Имя проверки, уникальное в пределах проекта. Становится именем ноги матрицы |
| `run` | string | да | Команда, которую проверка выполняет |
| `registry_login` | bool | нет | `true` — залогиниться в container registry перед запуском. По умолчанию `false` |

### Поля `checks_config`

| Поле | Тип | По умолчанию | Описание |
|---|---|---|---|
| `fail_fast` | bool | `false` | `true` — первая упавшая проверка отменяет остальные |

`fail_fast: false` здесь несущий, а не косметика: со значением `true` первый упавший линтер
отменяет остальные ноги, и за один push автор узнаёт про одно нарушение вместо всех.

## Что генерируется

### GitHub Actions (`.github/workflows/ci_cd.yml`)

Один job `checks` с матрицей — по ноге на объявленную проверку:

```yaml
  checks:
    runs-on: ubuntu-latest
    needs:
      - prepare-env
    if: needs.prepare-env.outputs.ENABLE_BUILD == 'true' && vars.SKIP_CHECKS != 'true'
    strategy:
      fail-fast: false
      matrix:
        include:
          - name: 'lint'
            run: 'make lint'
            registry_login: false
          - name: 'guards'
            run: 'make lint-guards'
            registry_login: true
    name: checks (${{ matrix.name }})
    steps:
      # checkout (fetch-depth: 0) / setup-go / GOPRIVATE — как в run-goat-tests-<app>
      - name: Run check ${{ matrix.name }}
        run: ${{ matrix.run }}
```

Шаги повторяют `run-goat-tests-<app>`: checkout, `setup-go` с `cache: true`, git-config для
приватных репозиториев, `GOPRIVATE`, отчёт `act10ns/slack@v2` на `failure()`. Шаг логина в
реестр генерируется, только если хотя бы одна проверка объявила `registry_login: true`, и
выполняется только в тех ногах матрицы, где он включён.

`fetch-depth: 0` — не деталь: сгенерированный `make lint` вызывается с
`--new-from-rev=origin/main`, и с дефолтным shallow-checkout ссылки `origin/main` в раннере
нет.

Гейт у `build` и `build-packages` — тем же приёмом, каким они сегодня гейтятся на goat-тестах:

```yaml
  build:
    needs:
      - prepare-env
      - checks
    if: |
      always() &&
      needs.prepare-env.outputs.ENABLE_BUILD == 'true' &&
      needs.prepare-env.result == 'success' &&
      (needs.checks.result == 'success' || needs.checks.result == 'skipped')
```

### GitLab CI (`.gitlab-ci.yml`)

Тот же набор проверок как `parallel: matrix` в стадии `test`, с `GIT_DEPTH: 0` и
`optional: true` в `needs` у build-джобов.

## Почему матрица, а не job на проверку

- `needs.checks.result` агрегирует все ноги матрицы, поэтому у `build` остаётся **одна**
  запись в `needs` независимо от числа объявленных проверок: граф перестаёт меняться при
  добавлении линтера. Job на проверку даёт тот же параллелизм, но растит и `needs`, и число
  статусов;
- ноги матрицы — отдельные параллельные job'ы, так что wall-clock не растёт с числом
  проверок. Цена — повторный checkout и `setup-go` в каждой ноге.

## Совместимость и отключение

- Нет ключа `checks:` — job не генерируется, сгенерированный CI побайтно совпадает с тем,
  что было до появления фичи. Существующие проекты не затрагиваются.
- `vars.SKIP_CHECKS` (GitHub) / `$SKIP_CHECKS` (GitLab) — лазейка для разового пропуска,
  повторяет существующую `SKIP_GOAT_TESTS`. Пропущенный job даёт `result == 'skipped'`,
  поэтому `build` не блокируется.

## Валидация

- `name` и `run` обязательны;
- `name` уникально в пределах проекта.

Нарушение — ошибка загрузки конфига (`invalid config checks section: …`,
`duplicate check name: …`), генерация не стартует.
