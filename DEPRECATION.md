# Deprecation Policy

## Overview

This document describes the deprecation policy for go-project-starter configuration format and features.

## Deprecation Timeline

- **Deprecated features are removed every 2nd minor release**
- Example: feature deprecated in `0.10.x` will be removed in `0.12.0`

## Current Deprecations

### Подпись инвойса ключом токена бота (драйвер telegram)

`UnpackInvoiceData` принимает подпись payload инвойса ключом текущего токена бота — так
подписывались инвойсы до TOOLS-140. Подписывает теперь только секрет
`{service}/security/invoice_sign_key`; переходный ключ нужен лишь для инвойсов, выпущенных
прежней версией и ещё не оплаченных.

Удаление — через два минорных релиза после релиза с TOOLS-140. Миграция и подробности —
[«Подпись инвойса ключом токена бота в драйвере telegram»](#подпись-инвойса-ключом-токена-бота-в-драйвере-telegram-tools-140-unreleased).

## Breaking changes in config validation

Эти изменения отвергают `project.yaml`, который прежний релиз загружал. Миграция —
правка конфига, но `go-project-starter migrate` их не трогает: автоматически выбрать
замену нельзя, в отвергнутой записи нет правильного значения.

### `auth_params` валидируются на загрузке конфига (TOOLS-90, unreleased)

`Rest.IsValid` валидирует секцию `auth_params` там, где она применима, —
`generator_type: ogen_client`. До этого секция не проверялась **вообще**: валидация жила
в мёртвой ветке `case "auth_params"` switch-а по `generator_type` (значения
`auth_params` у `generator_type` не бывает), поэтому опечатка доезжала до генерации,
а то и до рантайма — клиентом без авторизации.

| Конфиг | Было | Стало | Миграция |
|--------|------|-------|----------|
| `auth_params` на `generator_type` ≠ `ogen_client` (`ogen`, `template`, `sys`) | молча игнорировались | ошибка: `auth_params are only supported for generator_type ogen_client` | удалить секцию; аутентификация сервера — `generator_params.auth_handler` / `auth_scheme` |
| `transport` ≠ `header` | игнорировалось, клиент без `SecuritySource` | ошибка: `auth_params: invalid transport: … (supported: header)` | `transport: header` |
| `type` вне {`apikey`, `bearer`, `basic`} | то же | ошибка: `auth_params: invalid type: … (supported: apikey, bearer, basic)` | один из поддерживаемых типов (таблица в [transports.md](docs/configuration/transports.md#параметры-аутентификации)) |
| задано одно поле из двух | то же | ошибка: `auth_params: both transport and type are required` | указать оба поля либо удалить секцию |

**Почему ошибка, а не игнорирование.** Молча проигнорированная настройка
аутентификации — худший из исходов: проект собирается, клиент ходит во внешний API без
креденшелов, и выясняется это по 401 в рантайме. Ошибка загрузки конфига называет
отвергнутое значение и допустимые; имя REST-транспорта в сообщение не попадает (так
устроены все ошибки секции `rest`), поэтому при нескольких `ogen_client` проверьте
`auth_params` у каждого.

**Почему не «починить ветку как есть».** Мёртвая проверка разрешала только
`type: apikey`, тогда как шаблоны поддерживают `apikey`, `bearer` и `basic` (TOOLS-85).
Перенос ветки без изменений сломал бы валидные проекты с bearer/basic, поэтому список
допустимых значений теперь один на весь генератор — `ds.SupportedAuthTypes()`, та же
функция, на которой ветвится `ds.Transport.UsesSecuritySource`.

## Breaking changes in generated code

Эти изменения касаются не формата `project.yaml`, а **кода, который получает проект**,
поэтому `go-project-starter migrate` их не трогает: он правит конфиг, а здесь менять
надо исходники проекта. Миграция ручная и механическая, компилятор показывает каждое
место.

### `func init()` в сгенерированных проектах (TOOLS-30, v0.31.0)

`gochecknoinits` включён в `configs/golangci-lint.yml` сгенерированного проекта
(и в конфиге самого генератора), **без исключения для `_test.go`**. Точки расширения
сгенерированного кода переделаны так, что `init()` для них больше не нужен.

**Почему без переходного периода.** Переходный вариант — оставить `cmdMaps`
deprecated-глобалом и вливать его в новый реестр при конструировании — внутренне
противоречив: задача ровно в том, чтобы убрать глобальный мутабельный реестр, а
единственный способ его наполнить (`init()` в своём файле) с этого же релиза
отвергает линтер, который мы включаем. Проект получил бы и глобал, и красный
merge-гейт. Поэтому ломаем сразу: `undefined: cmdMaps` указывает точно на
строку, которую надо перенести, а миграция — перенос блока в новый метод.

| Было | Стало | Миграция |
|------|-------|----------|
| `cmdMaps[CmdX] = telegram.Cmd{…}` из `init()` в пакете telegram-воркера | реестр — поле воркера (`w.cmds`), наполняется из `customize()` | объявите `func (w *Worker) customize()` в своём файле пакета (или ниже disclaimer-маркера) и перенесите регистрацию на `w.AddCommands(telegram.Cmd{…})` |
| `updateHandler` / `updateHandlerIdle` / `unknownCommandHandler` переназначались из `init()` | в v0.31.0 — те же переменные; после v0.31.0 — сеттеры воркера (TOOLS-31) | в v0.31.0 — перенесите присваивания в тот же `customize()`; после v0.31.0 — вызовы сеттеров там же, см. TOOLS-31 ниже |
| `testEnvInit = &testEnvInitializerImpl{}` из `init()` в `tests/<app>/init.go` | явный сеттер | `var _ = SetTestEnvInitializer(&testEnvInitializerImpl{})` |
| `init()` в пакете `main` (ранняя работа до чтения конфигурации: правка окружения, re-exec, проверка `os.Args`) — обычно с `//nolint:gochecknoinits` | `earlyInit()` — метод, который `main` зовёт после `.env` и до `occonf.Initialize` (TOOLS-123, unreleased) | объявите `func (u *UserEarlyInit) earlyInit() error` в своём файле пакета `main` (или ниже disclaimer-маркера) и перенесите тело `init()` туда; ошибка возвращается наружу, процесс выходит с `ExitCodeErrorEarlyInit` |

> v0.31.0 оставил хуки package-level переменными, и присваивания им из `customize()`
> были в нём правильной миграцией. Следующий релиз (TOOLS-31, раздел ниже) убирает и
> сами переменные: такие присваивания перестают компилироваться
> (`undefined: unknownCommandHandler`), и их надо заменить вызовами сеттеров.
> Таблица соответствия — в разделе ниже. Если вы обновляетесь с версии раньше v0.31.0
> сразу на более позднюю, переносите из `init()` прямо в сеттеры.

Пример для telegram-воркера (в версии после v0.31.0, с учётом TOOLS-31):

```go
// Было — internal/app/worker/telegrambot/commands.go
func init() {
	cmdMaps[CmdHelp] = telegram.Cmd{Name: CmdHelp, Private: true, Handler: helpHandler}
	unknownCommandHandler = resetDialog
}

// Стало
func (w *Worker) customize() {
	w.AddCommands(telegram.Cmd{Name: CmdHelp, Private: true, Handler: helpHandler})
	w.SetUnknownCommandHandler(resetDialog)
}
```

`customize()` по умолчанию промотирован из встроенного `EmptyCustomizer` и ничего не
делает; метод на `*Worker` его перекрывает. `Create()` зовёт его после регистрации
команд генератора, поэтому команда проекта с тем же `Name` перекрывает сгенерированную.
Подробности — [docs/configuration/workers.md](docs/configuration/workers.md) и
[docs/testing/goat.md](docs/testing/goat.md).

**Ранняя инициализация в `main`.** Для работы, которой надо случиться **до** чтения
конфигурации, точки расширения не было вовсе: первый пользовательский код сервисного
`main` выполнялся после `occonf.Initialize` (`UserSetFunc.SetFunc`), а у CLI — в обработчике
команды, поэтому проекты держали `init()` с `//nolint:gochecknoinits`. Теперь оба `main`
зовут `earlyInit()` — после загрузки `.env` и до `occonf.Initialize` (TOOLS-123):

```go
// Было — cmd/api/init.go
//nolint:gochecknoinits
func init() {
	if !hasAnyOCKey() {
		os.Unsetenv("ONLINECONFIG_FROM_ENV")
	}
}

// Стало — cmd/api/early.go
func (u *UserEarlyInit) earlyInit() error {
	if !hasAnyOCKey() {
		return os.Unsetenv("ONLINECONFIG_FROM_ENV")
	}

	return nil
}
```

Тот же приём, что у `customize()`: метод промотирован из встроенного `EmptyEarlyInit`, метод
на `*UserEarlyInit` его перекрывает. Два имени — `EmptyEarlyInit` и `UserEarlyInit` — появляются
в пакете `main` каждого сгенерированного приложения: проект, у которого уже есть свой тип с таким
именем в `main`, после регенерации получит ошибку о повторном объявлении и его надо переименовать.
Подробности —
[docs/configuration/applications.md](docs/configuration/applications.md#ранняя-инициализация-процесса-earlyinit).

**Чем это держится.** `gochecknoinits` не видит `.tmpl`-файлы, а
`exclusions.generated: lax` выводит из проверки `psg_*_gen.go` целиком — включая
секцию ручного кода ниже disclaimer-маркера. Поэтому основной гард — не линтер, а
тест по шаблонам (`TestNoInitFuncInTemplates`) плюс проверка результата генерации
(`TestNoInitFuncInGeneratedProjects`); они от конфига проекта не зависят. Если проект
забирает конфиг линтера себе (`tools.lint_config.own`), запрет `init()` в **его** коде
остаётся на нём.

### Мутабельные package-level переменные в сгенерированных проектах (TOOLS-31, unreleased)

`gochecknoglobals` включён в `configs/golangci-lint.yml` сгенерированного проекта (и в
конфиге самого генератора), **с исключением для `_test.go`** — там табличные тесты
держат наборы случаев в package-level переменных, и запрет ничего не защищает.
Продолжение TOOLS-30: там точки расширения перестали требовать `init()`, здесь — быть
переменными, которым проект присваивает новое значение.

**Почему вообще.** У подменяемого глобала три свойства, которых у поля нет: менять его
может кто угодно в пакете (а не только владелец), два экземпляра компонента в одном
процессе делят одно значение, и подмена не видна как вызов — это запись в переменную
чужого пакета. Последнее не теория: `tgSHA` в драйвере telegram писался горутиной
наблюдателя OnlineConf при ротации токена и читался горутинами запросов при подписи
инвойса — гонка, которую видно только если держать в голове обе стороны. Два бота в
одном процессе вдобавок подписывали бы инвойс ключом друг друга.

**Почему без переходного периода**, хотя v0.31.0 уже вышел и предлагал присваивать
хукам из `customize()`. Оставить старый глобал deprecated и синхронизировать его с
новым полем значит на весь переходный период сохранить ровно то, что убирается:
общее для всех экземпляров мутабельное значение, которое может переписать кто угодно в
пакете. Сверху добавилась бы синхронизация «глобал ↔ поле» с вопросом, кто из них
главный, если заданы оба. Цена разрыва при этом минимальна: компилятор показывает
каждое место точно (`undefined: updateHandler`), а исправление — замена присваивания
вызовом сеттера, построчно по таблице ниже. Решение принято на приёмке TOOLS-31.

| Было | Стало | Миграция |
|------|-------|----------|
| `CustomError[ErrX] = handler` (telegram-воркер) | реестр — поле `w.customErrors` | `w.AddErrorHandler(ErrX, handler)` из `customize()`. Реестр стал срезом: проверка идёт в порядке регистрации, первое совпадение по `errors.Is` выигрывает. Прежний обход карты был рандомизирован, то есть при двух подходящих записях ответ менялся от запроса к запросу |
| `updateHandler = …` / `updateHandlerIdle = …` | поля воркера | `w.SetUpdateHandler(h, idle)` из `customize()`; у синхронного обработчика `idle` = `nil` |
| `unknownCommandHandler = …` | поле воркера | `w.SetUnknownCommandHandler(h)` из `customize()`; сигнатура обработчика не изменилась |
| `handler.UnimplementedResponse` (значение) | функция | `handler.UnimplementedResponse()` |
| `telegram.PackInvoiceData(ctx, d)` / `telegram.UnpackInvoiceData(ctx, &d, s)` | методы драйвера | `srv.TelegramPackInvoiceData(ctx, d)` / `srv.TelegramUnpackInvoiceData(ctx, &d, s)` |
| `telegram.GetTgTokenSHA(ctx)` | метод драйвера | `srv.TelegramTokenSHA(ctx)`. Ключ нужен для проверки подписи initData мини-аппа, поэтому путь к нему снаружи сохранён — вместе с `telegram.MakeSHA256Hash`, которая осталась функцией пакета |
| `daemon.ErrorTimeout` / `NewCycleTimeout` (экспортированные, читали ENV в инициализаторе) | поля, ENV читается в `Create()` | имена ENV-переменных не изменились; константы дефолтов — `DefaultErrorTimeout` / `DefaultNewCycleTimeout`. Присваивание из Go (обычно в тесте проекта, чтобы цикл не держал тест) — `Create().SetLoopTimeouts(…)`. Побочно исправлено: инициализатор срабатывал до `godotenv.Load(".env")`, поэтому значение из `.env` не доезжало — `Create()` зовётся после загрузки |
| `TestRetryConfig.MaxAttempts = 20` (goat-хелперы) | функция + параметр | читать — `TestRetryConfig().MaxAttempts`; **настроить** бэкофф `WaitForAPI` — `WaitForAPIWithRetry(t, url, maxWait, cfg)`, потому что присваивание полю больше ни на что не влияло бы: `WaitForAPI` берёт параметры вызовом |
| `env`, `appConfig`, `testEnvInit` и два `sync.Once` в `tests/<app>/` | одна структура `testEnvState` | кода проекта не касается: читалось через `GetEnv()` / `GetAppConfig()`, подменялось через `SetTestEnvInitializer` — и то и другое осталось |

Пример для telegram-воркера:

```go
// Было — internal/app/worker/telegrambot/dispatch.go
func (w *Worker) customize() {
	updateHandler = (*Worker).dispatchToQueues
	updateHandlerIdle = queuesDrained
	unknownCommandHandler = resetDialog
	CustomError[ErrSubscriptionExpired] = renewPrompt
}

// Стало
func (w *Worker) customize() {
	w.SetUpdateHandler(w.dispatchToQueues, w.queuesDrained)
	w.SetUnknownCommandHandler(resetDialog)
	w.AddErrorHandler(ErrSubscriptionExpired, renewPrompt)
}
```

`idle` — параметр того же сеттера, а не отдельного: `GracefulStop` ждёт не только
пустого канала апдейтов, но и того, что обработчик закончил начатое. Отдельный сеттер
позволял заменить обработчик и промолчать про `idle` — и воркер останавливался на
полпути; здесь про неё приходится сказать хотя бы `nil`.

**Если имя вашего метода совпало со сгенерированным.** Прежняя документация
предлагала объявить в пакете воркера свои `dispatchUpdate` / `updatesIdle` и
присвоить их хукам. Генератор такие имена не занимает (его методы называются
`invokeUpdateHandler` / `updateHandlerIsIdle`) — ровно чтобы это не ломало сборку.
Но если имя всё же столкнулось, компилятор скажет `method Worker.X already
declared at …psg_…_gen.go`, и чинится это переименованием **вашего** метода:
сгенерированный файл перезапишется на следующем `make regenerate`.

**Что осталось package-level** — только с `//nolint:gochecknoglobals` и обоснованием:
переменные под `-ldflags` (`Version`, `BuildCommit`, …), коллекторы Prometheus в
сгенерированном пакете метрик, таблицы required-ключей в сгенерированном пакете
OnlineConf и синглтон тестового окружения goat. Sentinel-ошибки, `regexp.MustCompile`,
`//go:embed` и `var _ I = (*T)(nil)` линтер пропускает сам.

**Чем это держится.** Тем же, чем запрет `init()`, и по той же причине:
`gochecknoglobals` не видит `.tmpl`-файлы, а `exclusions.generated: lax` выводит из
проверки `psg_*_gen.go` целиком — включая секцию ручного кода ниже disclaimer-маркера.
Гарды — `TestNoUnjustifiedGlobalsInTemplates` (скан шаблонов) и
`TestNoUnjustifiedGlobalsInGeneratedProjects` (разбор результата генерации как
настоящего Go — по манифесту, то есть только файлов, которые записал сам стартер).
Второй нужен потому, что шаблон не валидный Go и объявление, появляющееся только после
подстановки, построчному скану не видно.

### Поле `Telegram.Poller` в драйвере telegram (TOOLS-79, unreleased)

Продолжение TOOLS-31 на том поле, которое та правка назвала и не закрыла. Бот драйвера
(`tg.bot`) пересобирается при каждой ротации токена — из горутины наблюдателя
OnlineConf, — а читался он напрямую из девятнадцати мест в пяти файлах драйвера, то
есть из горутин обработки апдейтов и запросов. Теперь всё состояние, которое меняет
ротация (бот, бот с запущенным опросом, смещение апдейтов, ключ подписи
WebApp-данных), живёт под одним мьютексом драйвера и доступно только через аксессоры.

Сверх гонки данных ротация ломала сам опрос, и мьютекс вокруг поля этого бы не закрыл:
`Run` держал long-poll на боте прежнего токена, ротация подменяла только поле, и
`StopReceivingUpdates` на остановке приходился уже на нового бота — опрос прежнего не
останавливался никогда (в бою опрос отозванным токеном, на остановке воркера — утёкшая
горутина). Теперь ротация останавливает опрос на прежнем боте и поднимает его на новом.

Смещение (`update_id` + 1) при этом переносится **только если бот тот же** — отзыв токена,
самый частый повод для ротации, меняет секрет, а не идентификатор бота (`getMe`). Токен
другого бота начинает опрос с нуля: `update_id` нумеруется у каждого бота свой, а
переданный offset Telegram читает как «всё до него подтверждено», поэтому смещение
прежнего бота молча съело бы апдейты нового с меньшими номерами. Обратная сторона —
накопленные новым ботом апдейты придут заново, то есть обработчик должен быть
идемпотентным; подробнее — в [docs/configuration/workers.md](docs/configuration/workers.md).

| Было | Стало | Миграция |
|------|-------|----------|
| `tg.Poller` (экспортированное поле канала апдейтов) | `tg.GetUpdateChannel(ctx)` | заменить чтение поля вызовом. Канал теперь принадлежит драйверу и при ротации **не закрывается** — проверка закрытия в читателе больше не нужна, но и не вредит. Сгенерированный сервис (`Service.GetUpdateChannel`) и воркер переведены сами |
| `telegram.Telegram` без синхронизации | `sync.RWMutex` + аксессоры (`currentBot`, `applyToken`) | кода проекта не касается: все методы драйвера публичные и уже ходят через аксессор |
| `send`: `errors.New("telegram bot is not initialized")` | `ErrNotInitialized` | если проект сверял текст ошибки — `errors.Is(err, telegram.ErrNotInitialized)`. Текст сообщения тот же |
| паника на `nil` боте в `CreateInvoice`, `AnswerPreCheckout`, `RefundPayment`, `GetPayments`, `SendAnswerCallback`, `DeleteMessage`, `EditMessageWithButton`, `GetFileDirectURL`, `CheckUserInChannel`, `StartUpdateListener` | `ErrNotInitialized` | ничего: вызов до `Init` перестал ронять процесс |

**`StartUpdateListener` остался своим, отдельным опросом** и ротацию токена не
переживает: он поднимает `GetUpdatesChan` на том боте, который был текущим в момент
вызова. Шаблоны генератора его не вызывают. Одновременно с опросом драйвера
(`WithUpdatePoller()`) его запускать нельзя — два `getUpdates` одного бота Telegram
отвергает (409 Conflict); это было верно и раньше.

**Чем держится.** `TestTelegramDriverStateOnlyBehindAccessors` и
`TestTelegramPollerChannelIsNotAField` (скан шаблонов — линтер `.tmpl` не видит),
`TestGenerateTelegramPollerRotation` (разбор результата генерации) и тесты опроса в
самом сгенерированном проекте (`psg_telegram_poller_test.go`: поддельный Bot API, смена
токена под живым long-poll, перенос смещения на новый токен того же бота и его сброс на
токене другого, остановка без утёкшей горутины) — их прогоняет под `-race` стадия
docker-integration.

### Подпись инвойса ключом токена бота в драйвере telegram (TOOLS-140, unreleased)

Продолжение TOOLS-79 на том, что та правка назвала ограничением и оставила: payload инвойса
подписывался ключом токена бота (`HMAC("WebAppData", tg_token)`) и проверялся ключом токена
**текущего**. Инвойс живёт дольше запроса, который его выпустил — лежит в чате, пока
пользователь не нажмёт «оплатить», — поэтому ротация токена в это окно обесценивала подпись
уже выпущенного инвойса: `invalid sign` на `pre_checkout_query`, а попав между
`pre_checkout_query` и `successful_payment`, это списанные Stars без выдачи товара.

Теперь ключ подписи — отдельный секрет, ключ OnlineConf `{service}/security/invoice_sign_key`,
и из токена он не выводится. Заодно закрылось узкое окно из TOOLS-79: упаковка брала ключ, а
`SendInvoice` бота двумя отдельными захватами мьютекса драйвера — согласованная пара «бот и
ключ» теперь не нужна ни одной операции с инвойсом.

**Что обязан сделать потребитель: засеять `{service}/security/invoice_sign_key` до первого
инвойса.** Ключ **не** `required` — telegram-воркер не обязан выпускать инвойсы, и проверка на
старте ломала бы каждый проект, который их не выпускает. Поэтому отказ приходит в момент
выпуска: без значения `TelegramPackInvoiceData` возвращает `telegram.ErrInvoiceSignKeyNotSet` и
инвойс не выпускает. Пустая строка — то же, что отсутствие.

| Было | Стало | Миграция |
|------|-------|----------|
| подпись инвойса ключом `tg_token`, ничего засеивать не нужно | подпись секретом `security/invoice_sign_key` | засеять ключ в дереве OnlineConf (любое стойкое случайное значение) **до** обновления; без него выпуск инвойса отказывает |
| инвойс, выпущенный до ротации токена, не проверялся после неё | проверяется | ничего: кода проекта не касается, оба вызова идут через сервис |
| — | `telegram.ErrInvoiceSignKeyNotSet` | обработать отказ выпуска инвойса как отказ, а не как пустой payload |

**Переходный ключ, и он будет убран.** Проверка принимает ещё и подпись ключом текущего токена:
так подписывались инвойсы прежней версии, и в момент обновления они могут лежать в чатах
неоплаченными. На них прежнее ограничение остаётся — ротация до оплаты по-прежнему даёт
`invalid sign`, — но окно конечно: это инвойсы, выпущенные до засева секрета. Ключ снимается
через два минорных релиза после выпуска этой правки (политика выше); к тому моменту все
инвойсы в обороте подписаны секретом. Сам секрет при этом не меняется и его ротация — отдельное
действие, которое прежние инвойсы обесценивает так же, как раньше это делала ротация токена.

**Чем держится.** `TestTelegramNoOperationNeedsBotAndSignKeyTogether` и
`TestTelegramInvoiceSignsWithItsOwnSecret` (скан шаблонов — линтер `.tmpl` не видит),
`TestGenerateTelegramInvoiceSignKey` (разбор результата генерации) и тесты в самом
сгенерированном проекте (`psg_invoice_test.go`: инвойс, выпущенный до ротации и проверенный
после неё — и для перевыпуска токена того же бота, и для токена другого; ротация между
упаковкой и `SendInvoice`, с проверкой того payload, который реально ушёл в Bot API; упаковка на
драйвере без бота; отказ выпуска без секрета; переходный ключ и его прежнее ограничение;
подделанный payload; выпуск и проверка под ротацией) — последние прогоняет под `-race` стадия
docker-integration.

### Инициализация метрик у приложений без `sys`-транспорта (TOOLS-94, unreleased)

`application.InitMetrics(mainCtx)` и регистрация коллекторов карты метрик в
`cmd/<app>/psg_main_gen.go` теперь **безусловны** — раньше они генерировались только у
приложения с `sys`-транспортом, и приложение без него получало `nil`-реестр, на котором
middleware REST-транспорта падало (`MustRegister` на `nil`) ещё до первого запроса.

Регенерация такого проекта чинит старт сама. Чинить руками нужно только одно: **ручной
воркэраунд, если он был**.

| Было в ручном коде | Что делать после регенерации |
|--------------------|------------------------------|
| в `GetMiddlewares` транспорта подставлялся `prometheus.NewRegistry()` при `nil` | уберите ветку: реестр больше не `nil` (оставленная ветка просто мертва, компиляции не ломает) |
| ниже disclaimer-маркера свой вызов `metrics.Register(application.GetMetrics())` | **уберите его** — иначе второй `Register` в тот же реестр вернёт `AlreadyRegisteredError`, и сгенерированный код выйдет с `ExitCodeErrorApp` |

Канал вывода метрик стал объявленным свойством приложения
(`applications[].metrics.export`: `pull` / `push` / `none`). Ключ необязателен и по умолчанию
выводится из транспортов, поэтому существующие `project.yaml` менять не нужно —
[docs/configuration/applications.md](docs/configuration/applications.md#метрики-приложения-metricsexport).

### Цели `test-integration-with-deps` и `test-int-up-deps` (TOOLS-96, unreleased)

Обе цели удалены из сгенерированного `Makefile`: ни одна из них не работала с момента
появления. `test-integration-with-deps` зависела от цели `test-integration`, которую
генератор никогда не создавал, а `test-int-up-deps` поднимала compose-сервис `wiremock`,
которого нет ни в одном сгенерированном compose-файле. Заодно из `go.mod.tmpl` убран
`github.com/walkerus/go-wiremock` — он был там только ради этих целей, и ни одна строка
сгенерированного кода его не импортирует (`go mod tidy` выкидывал его и раньше).

**Миграция.** Если проект определил `test-integration` в своей секции (ниже
disclaimer-маркера) и звал `test-integration-with-deps` — перенесите обёртку туда же, к
самой цели:

```make
# ниже disclaimer-маркера, рядом со своей test-integration
.PHONY: test-integration-with-deps
test-integration-with-deps: test-int-up-deps test-integration

.PHONY: test-int-up-deps
test-int-up-deps:
	docker compose up -d --build --force-recreate <ваш сервис>
```

Зависимости интеграционных тестов объявляет проект: что именно поднимать, генератор не знает.
Штатный путь для интеграционных тестов в сгенерированном проекте — GOAT
(`goat_tests`, цель `goat-tests-<app>`), см. [docs/testing/goat.md](docs/testing/goat.md).

### `GOPRIVATE` больше не включает `github.com/Educentr` и `gitlab.com` (TOOLS-108, unreleased)

`GOPRIVATE` теперь равен **ровно** `git.private_repos`, и это значение вычисляется в одном
месте (`GeneratorParams.GoPrivate`) для всех четырёх потребителей. Раньше каждый собирал его
сам, и они расходились:

| Потребитель | Было | Стало |
|---|---|---|
| `Makefile` | `github.com/Educentr[,<private_repos>]` | `<private_repos>` |
| джобы `.github/workflows/*` | `github.com/Educentr[,<private_repos>]` | `<private_repos>`, и сам блок доступа к приватным модулям рендерится **только** при `private_repos` |
| `.gitlab-ci.yml` | `gitlab.com,<private_repos>` | `<private_repos>` |
| builder-стадия `Dockerfile-<app>` | `<private_repos>` | без изменений |

Из-за расхождения проект, зависящий от приватного модуля `github.com/Educentr/*` и не
перечисливший его в `private_repos`, собирался на хосте и в CI и падал в `docker build`.
Префикс к тому же выключал проверку по `sum.golang.org` для всех модулей Educentr, а все они,
от которых зависит сгенерированный проект, публичные.

**Миграция.** Для проекта без приватных модулей — ничего, регенерация достаточна; из
workflow заодно исчезает шаг, читавший токен, так что секрет
`git.private_repos_token_secret` (по умолчанию `GH_PAT`) для CI больше не обязателен.

| Если проект… | Что сделать |
|---|---|
| зависит от приватного модуля `github.com/Educentr/<repo>` | добавить его в `git.private_repos` (`private_repos: "github.com/Educentr/<repo>"`) — раньше он работал на хосте и в CI за счёт префикса |
| зависит от приватного модуля на `gitlab.com` и собирается в GitLab CI | перечислить его в `git.private_repos`: префикс `gitlab.com` оттуда убран |
| переопределял `GOPRIVATE` в своей секции `Makefile` (ниже disclaimer-маркера) или через окружение | ничего: `GOPRIVATE ?=` по-прежнему `?=`, переопределение выигрывает |

`git.private_repos` при этом остаётся **строкой** (несколько шаблонов — через запятую, как в
самой переменной `GOPRIVATE`); формат поля не менялся, но примеры в документации были
оформлены YAML-списком, и такой конфиг генератор не загружал — они исправлены
([docs/configuration/main.md](docs/configuration/main.md#приватные-go-модули)).

### `context.Background()` / `context.TODO()` в сгенерированных проектах (TOOLS-132, unreleased)

`forbidigo` в `configs/golangci-lint.yml` сгенерированного проекта запрещает оба имени —
`^context\.(Background|TODO)$`, с `analyze-types: true`, **без исключения для `_test.go`**.
Исключены только сгенерированные файлы, и не списком: `exclusions.generated: lax` выводит из
проверки всё, что несёт заголовок `Code generated … DO NOT EDIT.`, то есть каждый `psg_*`-файл,
включая оба `psg_main_gen.go`, где единственный законный корень и живёт.

**Почему.** Корень контекста — это место, где в картину входит сервисная обвязка: инстанс
OnlineConf, логгер, реестр метрик, отмена процесса. Контекст, построенный рядом с настоящим, не
несёт ничего из этого: чтение OnlineConf разыменовывает инстанс, которого в контексте нет,
строка лога уходит в никуда, горутина переживает shutdown. Хуже паники — код, который эту панику
«обработал»: читатель, берущий дефолт при отсутствующем дереве, превращает misconfiguration в
правдоподобный ответ.

Так дефект и нашёлся: TUI-презентер потребителя перечитывал данные на
`context.WithCancel(context.Background())`, ридер настроек видел «OnlineConf нет» и деградировал
к дефолту — экран показывал не то, что произойдёт при запуске, без ошибки и без строки лога.

**Чем заменить.** Производные формы (`context.WithCancel`, `context.WithTimeout`,
`context.WithValue`) над тем контекстом, который вам передали, не запрещены — это и есть ответ.
Таблица для остальных случаев:

| Было | Стало | Почему |
|------|-------|--------|
| `context.Background()` в хендлере / методе сервиса / воркере | `ctx`, который вам передали | корень уже есть, он у вас в параметрах |
| `context.WithCancel(context.Background())` для работы, которая переживает запрос | `appctx.Detach(ctx, 0)` | сохраняет значения родителя (дерево, логгер, trace id), снимает его отмену; `cancel` возвращается всегда, `defer cancel()` корректен |
| `context.WithTimeout(context.Background(), d)` для shutdown-шага после отмены родителя | `appctx.Detach(ctx, d)` | тот же контекст плюс свой дедлайн |
| `context.Background()` в `_test.go` | `t.Context()` | тестовая обвязка; подходит, если тест не читает конфигурацию |
| `context.Background()` в тесте, который читает OnlineConf | `occonftest.Context(t, map[string]string{occonf.PathX: "…"})` | контекст с настоящим деревом; чтение отвечает из него, а не дефолтом |
| `init()`-подобная работа до того, как дерево собрано | `earlyInit()` | см. TOOLS-30 выше и [docs/configuration/applications.md](docs/configuration/applications.md#ранняя-инициализация-процесса-earlyinit) |

Оба хелпера генерируются **безусловно, в каждом проекте** (`internal/pkg/appctx`,
`internal/pkg/occonf/occonftest`), так что регенерации достаточно, чтобы они появились.

**Как найти существующие вызовы.** `make lint` проверяет только diff
(`--new-from-rev=origin/main`), поэтому краснит **новый** код. Уже написанное покажет:

```bash
make lint-full 2>&1 | grep 'new root contexts'
```

`lint-full` в проекте, который его раньше не прогонял, покажет и другие долги; на корни
контекста достаточно отфильтрованного вывода.

### `occonf.Initialized(ctx) bool` снят (TOOLS-132, в релиз не входил)

Функция появилась в TOOLS-96 (коммит `da8b2ec`, merge в `main` 21.11) и **ни в один релиз не
вошла** — поэтому удалена без периода депрекации; проекты, сгенерированные v0.31.0 и раньше, её
не видели. Если вы сгенерировали проект из unreleased `main` и успели её позвать, компилятор
покажет каждое место (`undefined: occonf.Initialized`).

**Почему снята.** Проверка «несёт ли контекст дерево» узаконивает ровно ту деградацию, которую
убирает правило выше: читатель, берущий дефолт при отсутствующем дереве, скрывает
misconfiguration вместо того, чтобы о ней сообщить. В сгенерированном приложении контекст без
дерева невозможен — `Initialize` выполняется один раз на старте, и при ошибке процесс не
стартует, — так что его появление это дефект, и чтение должно падать сразу и громко.

| Было | Стало |
|------|-------|
| `if occonf.Initialized(ctx) { … } else { /* дефолт */ }` | убрать ветку: в сервисном коде контекст дерево несёт всегда |
| `occonf.Initialized(ctx)` в тесте, чтобы решить, читать ли конфигурацию | `occonftest.Context(t, values)` — читать настоящее дерево |
| `occonf.Clone` / `occonf.Release` | без изменений; `Clone` сам сообщает `occonf.ErrNoTree`, если у источника дерева нет (раньше это делал `Initialized` внутри него) |

## Go version policy

Generated projects target a **supported** Go release. Go upstream supports the
two latest minor releases, so the generator default (`tools.golang_version`,
and `MIN_GO_VERSION` in the generated Makefile) tracks them instead of staying
on an older version.

| Change | Release | Note |
|--------|---------|------|
| Generated projects: Go 1.24 → **1.27** | v0.31.0 | `tools.golang_version` default, `MIN_GO_VERSION`, integration image and CI; `goimports` installed from `@latest` again |

**Why a policy and not a one-off bump:** staying on an EOL release only defers
the next breakage. A dependency that raises its own `go` directive (e.g.
`gofrs/uuid/v5` → `go 1.25`, TOOLS-4) breaks `go mod tidy` for everyone on an
older toolchain, and pinning the tool that broke (`goimports@v0.49.0`) moves
the problem rather than fixing it.

**Impact on existing projects:** a project generated earlier keeps the
`tools.golang_version` written in its own `project.yaml` — regeneration does
not change that value. After raising it, the project's build/CI environment
must provide the new toolchain.

**Impact on this repository's own tests:** the generator test configs target
the new version, so the configs with `post_generate: go_mod_tidy` need a Go
1.27 toolchain. With the default `GOTOOLCHAIN=auto` Go downloads it on first
use; with `GOTOOLCHAIN=local` on an older host, `go mod tidy` in the generated
project fails with `go.mod requires go >= 1.27`. The integration-test CI job
installs 1.27.2 explicitly.

## Removed Deprecations

| Feature | Deprecated In | Removed In | Migration |
|---------|---------------|------------|-----------|
| `empty_config_available` | 0.11.0 | 0.13.0 | Use `optional: true` in application transport config |
| Transport string array format | 0.10.0 | 0.12.0 | Use object format: `- name: transport_name` |

## Migration

Run the migrate command to automatically update your config:

```bash
# Check what will be changed (dry-run)
go-project-starter migrate --dry-run

# Apply migration
go-project-starter migrate
```

The migrate command will:
1. Create a backup of your config (`.bak` file)
2. Convert deprecated formats to new formats
3. Report any issues found

## Adding New Deprecations

When adding a new deprecation:

1. Add a new version constant in `internal/pkg/migrate/migrate.go`:
   ```go
   const (
       RemovalVersionTransportStringArray = "0.12.0"
       RemovalVersionNewFeature           = "0.14.0"  // NEW
   )
   ```

2. Add migration logic in `migrate.go`

3. Add warning collection in `config/config.go` `collectDeprecationWarnings()`

4. Update this table with the new deprecation

## Release Checklist

Before each release, check if any deprecations should be removed:

1. Check `internal/pkg/migrate/migrate.go` for `RemovalVersion*` constants
2. If current release version >= removal version:
   - Remove backward compatibility code
   - Remove migration logic for that feature
   - Update this document
