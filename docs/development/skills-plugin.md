# Плагин скилов для AI-агентов

Инструкции, по которым AI-агенты работают с генератором, живут **не в этом репозитории**, а в
[`Educentr/claude`](https://github.com/Educentr/claude/tree/main/plugins/go-project-starter) —
Claude-плагином `go-project-starter`. Агент читает его вместо документации и исходников: оттуда он
берёт грамматику `project.yaml`, порядок генерации, команды установки и обхода типовых ошибок.

Отсюда следует правило релиза: **плагин обновляется с каждым релизом генератора**. Плагин, оставшийся
на прошлой версии, не просто устаревает — он продолжает учить прошлой версии: выдаёт снятое
ограничение как актуальное, пропускает новую обязательную секцию конфига и ставит пользователю
`go install …@<прошлый тег>`.

## Что лежит в плагине

| Путь (от `plugins/go-project-starter/`) | Что это |
|---|---|
| `skills/go-project-starter/SKILL.md` | главный скил: версия, которую покрывают инструкции, грамматика конфига, рабочий цикл, правила именования |
| `skills/go-project-starter/references/config-schema.md` | полная схема `project.yaml`: поля, обязательность, значения по умолчанию |
| `skills/go-project-starter/references/onlineconf-paths.md` | ключи и пути OnlineConf, граница аксессоров |
| `skills/go-project-starter/references/recipes.md` | рецепты: конфигурации, которые собираются из нескольких секций |
| `skills/go-project-starter/references/troubleshooting.md` | диагностика ошибок генерации и сборки |
| `skills/go-project-starter/references/releases-vX-vY.md` | дельта релизной линии: как апгрейдиться, какие миграции сгенерированного кода нужны |
| `skills/{add,create}-*/SKILL.md` | скилы под отдельные задачи (REST, gRPC, Kafka, Telegram, worker, GOAT) — каждый читается сам по себе и несёт **свою** версию |
| `skills/go-project-starter/evals/evals.json` | проверочные прогоны скила |
| `.claude-plugin/plugin.json` | версия самого плагина (своя нумерация, не версия генератора) |
| `README.md` | описание плагина |

## Что обновлять при релизе

Обязательно, независимо от содержания релиза:

- **версия в version-statement каждого скила** — строки вида
  `These instructions cover **go-project-starter vX.Y.Z**` / `Use **go-project-starter vX.Y.Z**`;
- **пины команд** — `go install …/go-project-starter@vX.Y.Z`, `go run …@vX.Y.Z`,
  `make update-starter STARTER_VERSION=vX.Y.Z`, `GPSVERSION`;
- **ссылки на документацию по тегу** — `github.com/Educentr/go-project-starter/blob/vX.Y.Z/docs/…`;
- **`version` в `.claude-plugin/plugin.json`** и `README.md` плагина.

По содержанию релиза — то, что он изменил:

| Что изменилось в генераторе | Что правится в плагине |
|---|---|
| Секция `project.yaml`: новое поле, правило валидации, значение по умолчанию | `references/config-schema.md` |
| Поведение регенерации: манифест, конфликты, дрейф скопированных спек | `references/troubleshooting.md`, раздел рабочего цикла в `SKILL.md` |
| Ключи, пути или граница чтения OnlineConf | `references/onlineconf-paths.md` |
| Возможность, которая собирается из нескольких секций (карта метрик, свои генераторы, lint, Docker, Grafana) | `references/recipes.md` |
| Новый транспорт, воркер или драйвер | соответствующий `skills/{add,create}-*` — или новый скил, если задача новая |
| CLI: команды, флаги, их значения по умолчанию | рабочий цикл в `SKILL.md` |
| Апгрейд с прошлой версии требует действий в проекте | новый `references/releases-vX-vY.md` + ссылка на него из скилов |
| Снятое ограничение или исправленный дефект, про который написано в плагине | текст, где он описан (чаще всего `references/troubleshooting.md`) |

Удобная отправная точка — diff релиза: `git log --oneline <prev-release-commit>..HEAD` плюс release
notes. Каждая запись либо попадает в одну из строк таблицы, либо не меняет ничего из того, что видит
агент.

## Что не правится

Исторические упоминания версий остаются как есть:

- `since v0.24.0`, `Fixed in v0.30.0` — факт о том, когда что появилось;
- ссылки `releases/tag/vX.Y.Z` на прошлые релизы;
- страница `references/releases-vX-vY.md`, названная по **прошлой** релизной линии: команды в ней
  принадлежат тем релизам, которые она описывает. Для новой линии создаётся новая страница;
- `go-project-starter-workspace/` — записанные прогоны evals.

## Проверка

```bash
# версия, которая выпускается
make check-skills-plugin SKILLS_VERSION=v0.32.0

# без SKILLS_VERSION — последний тег этого репозитория
make check-skills-plugin

# локальный чекаут плагина (ветка с правками, недоступный репозиторий)
PLUGIN_SRC=~/src/claude/plugins/go-project-starter ./scripts/check-skills-plugin.sh v0.32.0
```

`scripts/check-skills-plugin.sh` скачивает плагин и сверяет его с выпускаемой версией. Ошибкой
(код выхода 1) считаются:

1. главный скил покрывает версию старее выпускаемой;
2. скил покрывает версию, отличную от главного скила (девять задачных скилов — то, что проще всего
   забыть: каждый из них агент читает отдельно);
3. пин команды или ссылка на документацию по тегу указывает не на покрываемую версию (version-statement
   обновить легко, а пользователь запускает именно команду).

Плагин, ушедший **вперёд** выпускаемой версии, — не ошибка: его обновляют и до появления тега.
Код выхода 2 — проверка не состоялась (нет версии, плагин не скачался, в главном скиле нет
version-statement); это не вердикт «плагин актуален».

Проверка **не входит в CI-гейт**: она читает другой репозиторий по сети, и недоступность GitHub не
должна красить pull request'ы — та же причина, по которой вне гейта `make lint-config-verify`.

## Процедура

1. Выпустить релиз генератора (тег + GitHub Release) — ссылки по тегу и `@vX.Y.Z` должны
   существовать.
2. `make check-skills-plugin SKILLS_VERSION=vX.Y.Z` — получить список того, что ещё указывает на
   прошлую версию.
3. В чекауте `Educentr/claude` обновить плагин: version-statement'ы, пины, ссылки, затронутые
   релизом `references/`, `version` в `.claude-plugin/plugin.json`.
4. `PLUGIN_SRC=<чекаут>/plugins/go-project-starter ./scripts/check-skills-plugin.sh vX.Y.Z` —
   проверить, что ничего не осталось.
5. PR в `Educentr/claude`. Плагин — отдельный репозиторий, поэтому это отдельный PR; задача на него
   заводится с меткой `repo:github.com/Educentr/claude`.
6. После мержа — `make check-skills-plugin SKILLS_VERSION=vX.Y.Z` без `PLUGIN_SRC`, по `main`
   плагина.
