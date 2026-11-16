# Docker

## Зачем

Образу нельзя было штатно сообщить его собственную идентичность — версию и
ревизию (OCI `org.opencontainers.image.version` / `.revision`). `LABEL`
статичен и не умеет вызвать `git`, поэтому значения обязаны приехать через
`--build-arg`, а стадия, которая их использует, обязана объявить `ARG`. Ни
того, ни другого выразить было нечем:

- шаблон Dockerfile объявлял в builder-стадии только `ARG binary` и
  `ARG GOPROXY`;
- сгенерированный рецепт `docker-<app>` звал `docker build` без единого
  `--build-arg`;
- секция `docker:` несла единственное поле `image_prefix`.

Пользовательский маркер спасает только то, что лежит **ниже** него, то есть
финальную стадию; builder-стадия целиком генерируемая, поэтому дописанный туда
`ARG` пропадал на каждом `make regenerate` — молча, вместе со своим
комментарием. Потребителю оставался скрипт-overlay, который заново вставляет
`ARG` по якорю, плюс переопределённые копии рецептов `docker-<app>` с
дописанным `--build-arg`.

`build_args` и `labels` объявляют это в `project.yaml` — один раз на проект, с
генерацией во всех четырёх местах сразу.

## Параметры

```yaml
docker:
  image_prefix: registry.example/org   # префикс образов (REGISTRY_SERVER), по умолчанию educentr
  build_args:
    - IMAGE_VERSION                    # короткая форма: только имя
    - name: IMAGE_REVISION             # полная форма
      default: $(shell git rev-parse HEAD)
      stages: [builder, final]
  labels:
    org.opencontainers.image.version: ${IMAGE_VERSION}
    org.opencontainers.image.revision: ${IMAGE_REVISION}
    org.opencontainers.image.vendor: Educentr
```

Обе секции опциональны. Без них вывод побайтно совпадает с прежним, поэтому
регенерация существующих проектов ничего не меняет.

### `build_args`

Список объявленных build-аргументов образа. Элемент — либо имя, либо объект:

| Поле | Обязательное | Значение |
|---|---|---|
| `name` | Да | Имя аргумента. Оно же — имя `ARG` в Dockerfile, переменной в Makefile и ключа `--build-arg`. Допустимы `[A-Za-z_][A-Za-z0-9_]*` |
| `default` | Нет | Значение по умолчанию в Makefile (`<name> ?= <default>`). Может быть make-функцией, например `$(shell git rev-parse HEAD)` |
| `stages` | Нет | Стадии Dockerfile, которые объявляют `ARG`: `builder`, `final` или обе. По умолчанию `[final]` |

Из каждого аргумента генерируются:

1. `ARG <name>` в перечисленных стадиях Dockerfile;
2. `--build-arg <name>="$(<name>)"` в рецепте `docker-<app>`;
3. `<name> ?= <default>` в Makefile — локальная сборка не требует передавать
   значение руками, а `?=` оставляет приоритет за переменной окружения или
   командной строкой (так его переопределяет CI);
4. `<name>=${<name>:-}` в `build.args` приложения в `docker-compose-dev.yaml`
   (compose не умеет вычислять make-дефолт, поэтому значение берётся из
   окружения и пусто, если не задано).

**Почему по умолчанию только `final`.** `ARG` обесценивает build-кеш всех слоёв
стадии, которые идут **после** него, а версия и ревизия меняются на каждом
коммите. Лейблы живут в финальной стадии, и там `ARG`-блок генерируется в самом
конце стадии — после `COPY`/`CMD`/`EXPOSE`, так что смена значения не
перекомпилирует образ. `stages: [builder, final]` нужен только тогда, когда
значение реально требуется при сборке бинарника (например уходит в `ldflags`), и
стоит это осознанного инвалидации кеша builder-стадии.

Имя не может совпадать ни с `ARG`-ами шаблона Dockerfile (`binary`, `GOPROXY`),
ни с переменными, которые объявляет сгенерированный Makefile (`DOCKER_TAG`,
`RELEASE`, `SERVICE_NAME`, …) — иначе объявление переопределило бы значение,
которым пользуется остальная сборка. Такие имена отвергаются на загрузке
конфига.

### `labels`

Карта `ключ: значение` → одна инструкция `LABEL` (один слой) в финальной стадии.
Значение может ссылаться на build-аргумент (`${IMAGE_VERSION}` или
`$IMAGE_VERSION`); литерал `$` экранируется как `$$`.

**Ссылка проверяется на загрузке конфига:** если значение упоминает аргумент,
которого нет в `build_args` или который не объявлен в стадии `final`, генерация
падает с ошибкой. В финальной стадии значение больше взять негде, поэтому иначе
лейбл молча отрендерился бы пустым — ровно та поломка, из-за которой секция и
появилась.

Ключи — в нижнем регистре (viper приводит ключи конфига к нижнему регистру, так
что ключ с большой буквой молча изменился бы по дороге), из символов
`[a-z0-9._-]`. Значения — однострочная строка без `"` и `\` (на этих символах
ломается генерируемая инструкция).

## Что получается на выходе

Для конфига из примера выше:

```dockerfile
# Dockerfile-api, builder-стадия
FROM golang:1.24-alpine AS builder
ARG binary=bin/api
ARG GOPROXY
ARG IMAGE_REVISION
```

```dockerfile
# Dockerfile-api, финальная стадия — ARG-блок и LABEL в самом конце
EXPOSE 8080

ARG IMAGE_VERSION
ARG IMAGE_REVISION

LABEL org.opencontainers.image.revision="${IMAGE_REVISION}" \
      org.opencontainers.image.vendor="Educentr" \
      org.opencontainers.image.version="${IMAGE_VERSION}"
```

```makefile
# Makefile
REGISTRY_SERVER ?= registry.example/org
DOCKER_TAG ?= $(ENVIRONMENT_PREFIX)$(RELEASE)-$(LAST_COMMIT_HASH)
IMAGE_VERSION ?=
IMAGE_REVISION ?= $(shell git rev-parse HEAD)

docker-api:
	DOCKER_BUILDKIT=1 docker build . -f ./Dockerfile-api --rm --secret id=github_token,env=GITHUB_TOKEN --build-arg IMAGE_VERSION="$(IMAGE_VERSION)" --build-arg IMAGE_REVISION="$(IMAGE_REVISION)" -t $(REGISTRY_SERVER)/$(SERVICE_NAME)-api:${DOCKER_TAG}
```

Передать значение сборке:

```bash
make docker-api IMAGE_VERSION=v1.2.3          # аргументом make
IMAGE_VERSION=v1.2.3 make docker-api          # переменной окружения (её же использует CI)
```

Аргумент без `default` и без переданного значения уедет в образ пустым — это
выбор конфига, а не ошибка. Если лейбл обязан быть заполнен всегда, задайте
`default`.

## Runtime-стадия приложения (`applications[].docker`)

Финальная стадия сгенерированного `Dockerfile-<app>` была зафиксирована:
`FROM alpine` и `CMD ["/service"]`. Приложению, которому нужен другой дистрибутив
или рантайм-зависимость (`git`, `curl`, `ca-certificates`), сказать об этом было
нечем: стадия генерируется, а пользовательский маркер спасает только то, что
лежит **ниже** него. Оставался обходной путь — отдельный
`Dockerfile-<app>.custom` плюс переопределённая цель `docker-<app>` в секции
после маркера.

```yaml
applications:
  - name: vt
    docker:
      base_image: ubuntu:24.04       # по умолчанию alpine
      packages:                      # рантайм-пакеты
        - git
        - curl
        - ca-certificates
      package_manager: apt           # опционально; выводится из base_image
      workdir: /workspace            # по умолчанию нет WORKDIR (докеровский "/")
      entrypoint: ["/service"]       # по умолчанию CMD ["/service"]
```

Секция опциональна и объявляется **у приложения**, а не у проекта: образы
приложений одного сервиса собираются из разных наборов зависимостей. Без неё
вывод побайтно совпадает с прежним — регенерация существующих проектов ничего не
меняет.

Настраивается только финальная (runtime) стадия. Builder-стадия компилирует
статический Go-бинарник, и её базовый образ определяет `tools.golang_version`.

| Поле | Обязательное | Значение |
|---|---|---|
| `base_image` | Нет | Базовый образ runtime-стадии (`FROM`). По умолчанию `alpine`. Ссылка без пробелов, кавычек и `#` — допустимы registry с портом, тег и digest |
| `packages` | Нет | Рантайм-пакеты. Ставятся **одной** инструкцией `RUN` до копирования бинарника, поэтому слой переиспользуется, пока не изменится сам список. Порядок — как в конфиге. Допустим пин версии (`curl=8.5.0-2`) |
| `package_manager` | Нет | Чем ставить: `apk`, `apt` или `dnf`. Выводится из `base_image` для `alpine`, `debian`, `ubuntu`, `fedora`; для любого другого образа с `packages` — объявить явно |
| `workdir` | Нет | `WORKDIR` runtime-стадии, абсолютный путь. По умолчанию инструкция не генерируется (докеровский дефолт `/`) |
| `entrypoint` | Нет | `ENTRYPOINT` в exec-форме. **Заменяет** `CMD`, а не дополняет его: Docker передаёт `CMD` энтрипойнту аргументами, поэтому дефолтный `/service` приехал бы энтрипойнту параметром |

**Почему `package_manager` не угадывается для произвольного образа.** Менеджер
выводится из имени репозитория образа (`registry.example:5000/mirror/ubuntu:24.04`
→ `ubuntu`) и только для дистрибутивов, у которых он однозначен. Для
`gcr.io/distroless/...` или собственного базового образа угаданный менеджер дал
бы Dockerfile, который падает на сборке, — поэтому генерация останавливается с
требованием объявить менеджер. Объявленный `package_manager` без `packages` —
тоже ошибка: ставить нечего.

Значения проверяются на загрузке конфига: образ и рабочий каталог — без пробелов
и кавычек, имена пакетов — из `[A-Za-z0-9._+-=:~/]`, элементы `entrypoint` —
непустые однострочные строки без `"` и `\`. Всё это попадает в инструкции
Dockerfile как есть, поэтому строка с пробелом или кавычкой ломала бы файл (а в
`RUN` — дописывала бы команду).

### Что получается на выходе

Для примера выше:

```dockerfile
# Dockerfile-vt, финальная стадия
FROM ubuntu:24.04

# Runtime packages, declared in the docker section of the application in
# project.yaml. Installed before the binary is copied, so the layer is reused
# until the package list itself changes.
RUN set -eux && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends git curl ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /service /service
COPY --from=builder /public /public
RUN mkdir -p /etc/onlineconf
WORKDIR /workspace
ENTRYPOINT ["/service"]
```

Индекс/кеш пакетного менеджера удаляется в той же инструкции (`rm -rf
/var/lib/apt/lists/*` для apt, `dnf clean all` для dnf, `--no-cache` для apk):
отдельный `RUN` оставил бы скачанные метаданные в слое ниже.

Приложение без секции `docker` получает прежнюю стадию — `FROM alpine` и
`CMD ["/service"]`.

## Реализация

- парсинг и валидация секций — `internal/pkg/config/docker.go` (`Docker.Parse` и
  `AppDocker.Parse`), вызывается из `GetConfig` через `Docker.IsValid` и
  `AppDocker.IsValid`;
- нормализованный вид и хелперы шаблонов — `internal/pkg/ds/docker.go`
  (`ds.DockerConfig`, `ds.AppDockerConfig`);
- шаблоны — `app/files/Dockerfile.tmpl`, `main/Makefile.tmpl`,
  `main/docker-compose-dev.yaml.tmpl`.

