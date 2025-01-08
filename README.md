# Go project starter

TODO name!!!

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
