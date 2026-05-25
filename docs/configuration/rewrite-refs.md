# Rewrite refs

## Зачем

OpenAPI и JSON Schema специфы из контрактных репозиториев часто используют
кросс-директорные `$ref`:

```yaml
# orchestrator/manual_session.swagger.yml
allOf:
  - $ref: '../common/workspace.swagger.yml#/components/schemas/RuntimeWorkspaceConfig'
```

`go-project-starter` копирует все файлы из `path:` **плоско** в одну
директорию (`api/rest/<transport>/<version>/` для REST, `api/schema/<name>/`
для JSON Schema). После такой копии `../common/` указывает на несуществующий
путь, и ogen / go-jsonschema падают на резолве `$ref`.

Флаг `rewrite_refs: true` включает пост-обработку: после `CopySpecs` /
`CopySchemas` стартер сканирует целевую директорию и переписывает все
`$ref`, у которых `basename(<path>)` совпадает с одним из скопированных
sibling-файлов:

```yaml
# Было (в source-репо)
$ref: '../common/workspace.swagger.yml#/components/schemas/X'

# Стало (в api/rest/<svc>/v1/)
$ref: ./workspace.swagger.yml#/components/schemas/X
```

Поведение по умолчанию (`rewrite_refs: false` или поле отсутствует) — без
изменений: специфы копируются как есть.

## Включение

REST:

```yaml
rest:
  - name: api
    version: v1
    rewrite_refs: true
    path:
      - "git+ssh://git@github.com/org/contracts.git#orchestrator/manual_session.swagger.yml"
      - "git+ssh://git@github.com/org/contracts.git#common/workspace.swagger.yml"
```

JSON Schema:

```yaml
jsonschema:
  - name: events
    rewrite_refs: true
    schemas:
      - id: order
        path: ./schemas/order.json
      - id: shared
        path: ./schemas/shared.json
```

## Что переписывается

`evaluateRef` применяется к каждому `$ref` значению:

| Вход                                            | Результат                          | Почему                                    |
|------------------------------------------------|------------------------------------|-------------------------------------------|
| `#/components/schemas/X`                       | без изменений                      | внутренний указатель                      |
| `https://example.com/x.yaml#/X`                | без изменений, warn                | абсолютный URL                            |
| `foo.yaml#/X`                                  | без изменений                      | уже плоский                               |
| `./foo.yaml#/X`                                | без изменений                      | уже плоский                               |
| `../common/foo.yaml#/X` (sibling существует)   | `./foo.yaml#/X`                    | sibling найден в target-директории        |
| `../missing/foo.yaml#/X` (sibling нет)         | без изменений, warn                | sibling не найден — оставлено как есть   |
| `sub/foo.yaml#/X` (sibling существует)         | `./foo.yaml#/X`                    | sibling найден                            |

## Защита через whitelist

Стартер передаёт rewriter список ожидаемых basename-ов
(`Transport.SpecTargetFiles` / `JSONSchema.SchemaTargetFiles`). Если в
target-директории есть файл, который совпадает по basename с `$ref`, но НЕ
входит в этот список (например, пользовательский файл, оставленный там
вручную), rewriter его игнорирует и пишет warning. Это защищает от
случайной перепиcи на не-сгенерированный файл.

## Поддерживаемые форматы

- `.yaml`, `.yml` — через `gopkg.in/yaml.v3` (`yaml.Node`); комментарии и
  порядок ключей сохраняются. Известное ограничение: round-trip может
  нормализовать стиль кавычек (`'` ↔ `"`). Семантика не меняется.
- `.json` — через line-regex по `"$ref": "..."`. JSON не допускает
  multi-line строк или комментариев, поэтому regex безопасен.

## Ограничения v1

1. Только REST (ogen / ogen_client) и JSON Schema. gRPC/proto имеет другой
   механизм импортов и в эту фичу не входит.
2. Rewriting, не bundling. Если нужен один self-contained файл со всеми
   inline-схемами — это отдельная задача.
3. Сканирование non-recursive: только верхний уровень target-директории.
4. При коллизии basename выигрывает файл, который попал в whitelist
   (`SpecTargetFiles`). Если файлы из разных source-директорий с одинаковым
   именем (`common.swagger.yml` из двух разных путей) — стартер уже на
   этапе copy перезапишет один другим, так что реальной коллизии в
   target-директории не возникает.

## Миграция с `regen-cleanup.sh`

Если у consumer-сервиса есть скрипт вида:

```bash
perl -i -pe 's{\.\./common/([a-z][a-z0-9_-]*\.swagger\.yml)}{./$1}g' \
  api/rest/*/v1/*.swagger.yml
```

— после включения `rewrite_refs: true` в `project.yaml` этот блок можно
удалить. Стартер сам перепишет `$ref` сразу после copy, до запуска ogen.
