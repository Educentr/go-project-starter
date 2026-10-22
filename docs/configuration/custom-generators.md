# Свои генераторы: `generator` + `contract`

Объявить спеку, которую стартер **заберёт** из удалённого репозитория, раньше можно было
только там, где он умеет генерировать сам: `rest[]` → ogen, `jsonschema[]` → go-jsonschema,
`cli` → template. Контракт, под который нужен другой генератор, объявить было нечем, а попытка
положить его в существующую секцию ломала `make generate` целиком: запись `jsonschema:`
дописывает в Makefile строку `go-jsonschema …`, запись `rest:` — вызов ogen.

Две секции закрывают это: `generator` объявляет генератор проекта (как поставить, как
запустить, чем линтовать), `contract` объявляет спеку и ссылается на генератор. Стартер
владеет всей цепочкой — забрать объявленное, положить по `copy`, подключить install/generate/lint
в Makefile. Чем именно генерировать, ему знать не нужно.

## Пример

```yaml
generator:
  - name: runtime_limits_ts
    install: npm ci --prefix web                   # опционально
    cmd: node web/scripts/generate-runtime-limits.mjs {{ .Spec }} --out {{ .Target }}
    lint: npx --prefix web spec-lint {{ .Spec }}   # опционально

contract:
  - path: git+ssh://git@github.com/org/contracts.git#limits/runtime_limits.yaml
    copy: api/contract/runtime_limits.yaml
    target: web/src/generated/runtimeLimits.ts
    generator_name: runtime_limits_ts
```

## Секция `generator`

| Поле | Обяз. | Описание |
|------|-------|----------|
| `name` | да | Имя генератора. `^[a-z0-9_-]+$`, не должно совпадать со встроенной make-целью. |
| `install` | нет | Команда установки. Пусто — инструмент тянет себя сам (`go run tool@v1`, `npx`). |
| `cmd` | да | Команда генерации, вызывается по одному разу на каждую спеку генератора. |
| `lint` | нет | Линтер, приезжающий вместе с генератором; вызывается так же, по спеке. |

**Версии и способ установки — внутри команды, отдельного поля нет.** `go install …@v1`,
`npm ci` по lockfile, `pip install -r` не сводятся к одному набору полей, поэтому пин живёт
там, где ему место — в экосистеме генератора.

### Переменные в командах

В `cmd` и `lint` доступны ровно две переменные Go-шаблона:

- `{{ .Spec }}` — путь скопированной спеки от корня проекта (поле `copy`);
- `{{ .Target }}` — поле `target` записи `contract`.

У `install` переменных нет: он выполняется один раз на генератор, а не на спеку.

Шаблоны проверяются при загрузке конфига: неизвестная переменная, незакрытое `{{`, ссылка на
`.Target` в записи **без** `target` — ошибка загрузки, а не пустая строка в команде.
Отрендеренная команда должна быть одной строкой — это одна строка make-рецепта.

## Секция `contract`

| Поле | Обяз. | Описание |
|------|-------|----------|
| `path` | да | Источник спеки. Грамматика та же, что у `rest[].path` — см. [Удалённые спеки](remote-specs.md). |
| `copy` | да | Куда положить спеку. Путь от корня проекта, относительный, без `..`. |
| `target` | нет | Путь вывода генератора, значение `{{ .Target }}`. |
| `generator_name` | да | Ссылка на запись `generator`. |

`generator_name` обязателен: это **не** режим «положить файл как есть» — вход, из которого
ничего не генерируется, контрактом не является.

Поля `version` нет. Что забирать, задаётся как у остальных каналов: `@<ref>` в `path` — пин,
либо `--sourceRef` / `make regenerate SOURCE_REF=…` — см.
[Переопределение ref](remote-specs.md#переопределение-ref-sourceref). Что **забрано** — пишет
стартер в манифест (ниже).

## Make-цели: одна на генератор

- `install-<name>` — если задан `install`; добавляется в зависимости `install-tools`.
- `generate-<name>` — вызов `cmd` на каждую спеку, ссылающуюся на генератор; добавляется в
  `generate` (и через него в `regenerate`) **после** встроенных целей.
- `lint-<name>` — если задан `lint`; добавляется в зависимости `lint`.

`generate-<name>` от `install-<name>` не зависит — как и встроенные цели.

Для примера выше:

```make
.PHONY: install-runtime_limits_ts
install-runtime_limits_ts:
	npm ci --prefix web

.PHONY: generate-runtime_limits_ts
generate-runtime_limits_ts:
	node web/scripts/generate-runtime-limits.mjs api/contract/runtime_limits.yaml --out web/src/generated/runtimeLimits.ts

.PHONY: lint-runtime_limits_ts
lint-runtime_limits_ts:
	npx --prefix web spec-lint api/contract/runtime_limits.yaml
```

Если ни одного генератора не объявлено, Makefile не меняется — ни одной лишней строки.

## Манифест и правка спек

Скопированная спека попадает в `.project-config/meta.yaml` как `kind: copied` с провенансом:

```yaml
manifest:
    - {path: api/contract/runtime_limits.yaml, kind: copied, hash: f7ec67…,
       source: 'ssh://git@github.com/org/contracts.git@main#limits/runtime_limits.yaml',
       ref: main, commit: 4f1c2a…}
```

- `source` — источник с вырезанным токеном;
- `ref` — **эффективный** ref (учитывает `--sourceRef`), только для git-источников;
- `commit` — SHA, на котором спека забрана; для https/local провенанс — `source` + `hash`.

Вывод `cmd` в манифест не попадает: его пишет не стартер.

**Спеки внутри проекта не правятся** — только в контрактном репозитории. Если хеш скопированной
спеки на диске не совпал с манифестом, генерация **падает с ошибкой** (не предупреждением):

```text
contract spec was changed inside the project: api/contract/runtime_limits.yaml
(source: ssh://git@github.com/org/contracts.git@main#limits/runtime_limits.yaml):
edit the contract in its own repository, not the copy inside the project
```

Это строже, чем правка выше disclaimer-маркера в сгенерированном файле (там предупреждение):
там артефакт — сам файл, и перегенерация его и есть исправление. Здесь правка означает, что
проект и контрактный репозиторий расходятся в том, что такое контракт, — и сгенерированный код
больше не соответствует ни одной опубликованной версии.

## Валидация при загрузке конфига

Ошибкой загрузки являются:

- запись `contract` без `generator_name` или со ссылкой на несуществующий генератор;
- дубль имён в `generator`;
- имя генератора не по формату `^[a-z0-9_-]+$` или совпадающее со встроенной make-целью
  (`ogen`, `argen`, `jsonschema`, `lint`, `tools`, …; плюс зарезервированный префикс
  `test-env-`);
- непарсящийся шаблон `cmd`/`lint`, неизвестная переменная, ссылка на `.Target` в записи без
  `target`;
- переменные в `install`;
- пустой `cmd`;
- `copy`/`target` абсолютный, неочищенный (`./a/b.yaml`) или выходящий за корень проекта;
- две записи `contract` с одинаковым `copy`.

## Реализация

- `internal/pkg/config/contract.go` — типы секций, валидация, разбор и рендер шаблонов команд.
- `internal/pkg/ds/contract.go` — то, что видит шаблон Makefile: генераторы с уже
  отрендеренными командами.
- `internal/pkg/generator/contract.go` — `processGenerators` (рендер), `resolveContracts`
  (забор, провенанс), `checkContractDrift`, `copyContracts`.
- `internal/pkg/templater/embedded/templates/main/Makefile.tmpl` — цели.
