# Go project starter

## Docs

```bash
docker build -f docs/Dockerfile -t mkdocs-app . && docker run -p 8000:8000 -v $(pwd):/docs mkdocs-app
```

## Generate you project

### Flags

| Flag     | Description         | Default value |
|----------|---------------------|---------------|
| --config | Path to config file | ./config.yaml |

### Run from source

```bash
go run github.com/Nikolo/go-project-starter@latest --config=PATH_TO_MY_CONFIG.yaml
```

## Manual code support

You can place your code anywhere where there is a `CHANGES MANUALLY MADE BELOW WILL NOT BE OVERWROTE BY GENERATOR.`
splitter.

Everything below this bar does not participate in the diff and will not be overwritten by the generator.

The `CHANGES MANUALLY MADE BELOW WILL NOT BE OVERWROTE BY GENERATOR.` splitter is added automatically.

## Prepare config file

- Prepare contracts
- Select steps you want to run in config
- Select modules that you want to generate in the config file.

## Run the generator

```bash
> ls ./example
admin.proto         example.proto       example.swagger.yml kafka_example.proto
> ls config.yaml
config.yaml
>./go-project-starter
....
Generated successfully
❯ ls ../example
Dockerfile          Makefile            api                 cmd                 configs             docker-compose.yaml go.mod              go.sum              internal            pkg                 scripts
```

## Swagger

Все `path` в спеке должны содержать `default response`:

```
        default:
          description: unexpected error
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorDefault'
```

Схема ответа должна быть такой:

```
    ErrorDefault:
      required:
        - code
        - error
      properties:
        code:
          type: integer
          format: int32
        error:
          type: string
      type: object
```

## Contributing

## ToDos

- Убрать все прямые импорты `zlog "github.com/rs/zerolog"` логгер должен задаваться из конфига
- Transport может быть как серверный так и клиентский. Надо разделить генерируемые файлики для раных типов и добавить во все структуры признак транспорта
- добавить сортировку транспортов, что бы в генерируемых файлах они всегда шли в одном порядке, сейчас проблема в том, что порядок не сохраняется и после перегенерации появляются изменения в виде перестановки строк местами.
- переименовать папку public в static
- убрать под флаг в конфиге использование папки public (static)
- Хуки не запускаемые:
  hint: The '.git/hooks/pre-commit' hook was ignored because it's not set as executable.
  hint: You can disable this warning with `git config advice.ignoredHook false`.
- сделать функцию, которая позволит добираться до нужного раздела конфига из API и Handler-ов
- разделить actor-а и subject-а в системе авторизации. Actor - это инициатор запроса, subject - тот по отношению к кому выполняется действие. Для запросов от имени пользователя actor всегда == subject. Для от имени админа actor == админ, subject == пользователь по отношению к которому выполняется действие.
- добавить генерацию .gitattributes для возможности исключения сгенерированных файлов из git diff. Рецепт для пользователя как настроить исключение 
  ```
     [diff "none"]
     command = /bin/true
  ```
  Содержимое файла:
  ```
  internal/pkg/model/repository/cmpl/*/* diff=none
  *_gen.go diff=none
  ```
  Собственно содержимое должно быть разным в зависимости от подключенных генераторов.
