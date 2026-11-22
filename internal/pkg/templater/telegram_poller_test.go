package templater

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// guardedTelegramFieldRx matches a direct read or write of the telegram driver
// state that token rotation changes.
//
// The driver's bot is replaced from the OnlineConf watcher goroutine while
// request goroutines use it, so every one of these fields is only reachable
// under the driver's mutex. The fields the constructor fills once and nobody
// writes again (`startUpdatePoller`, `apiEndpoint`, `updates`, `pollerStop`) are
// deliberately absent: they are not shared mutable state.
var guardedTelegramFieldRx = regexp.MustCompile(`\btg\.(bot|pollerBot|tgSHA|updateOffset|offsetBotID|pollerDone)\b`)

// telegramFieldAccessors are the functions allowed to touch those fields: the
// accessors themselves, plus the two lifecycle methods that own pollerDone.
//
// A list of functions rather than a list of files: before this guard the reads
// were spread over five templates, and "the driver may read its own field" is
// exactly the rule that let that happen.
var telegramFieldAccessors = map[string]string{
	"currentBot":         "аксессор бота",
	"applyToken":         "установка бота и ключа подписи его токена",
	"startPoller":        "старт опроса на текущем боте",
	"stopPollerBot":      "остановка опроса на боте, где он работает",
	"recordUpdateOffset": "перенос смещения апдейтов",
	"GetTgTokenSHA":      "аксессор ключа подписи",
	"Run":                "ставит pollerDone до старта помпы",
	"Shutdown":           "ждёт pollerDone",
}

// funcLineRx matches the opening line of a top-level function or method and
// captures its name.
var funcLineRx = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)\s*\(`)

// TestTelegramDriverStateOnlyBehindAccessors keeps the telegram driver's
// rotating state behind its accessors.
//
// The bot is rebuilt on every token rotation, from the OnlineConf watcher
// goroutine, while request goroutines send through it — so a direct read of the
// field is a data race, and a read repeated twice in one function (the
// `if tg.bot == nil` guard and the request right after it) can even land on two
// different bots. Nothing in the generated project can report this: a linter
// never parses a `.tmpl`, and `exclusions.generated: lax` keeps even the
// generated tree out of the project's own lint run.
func TestTelegramDriverStateOnlyBehindAccessors(t *testing.T) {
	found := map[string][]string{}

	err := walkTemplates(func(relative, body string) error {
		if !strings.HasSuffix(relative, ".go.tmpl") {
			return nil
		}

		// Тесты драйвера — вне правила: тест в его же пакете единственный
		// законный читатель pollerDone (утверждение «помпа вышла» ни одним
		// аксессором не выражается), и он выполняется не в горутине запроса.
		if strings.HasSuffix(relative, "_test.go.tmpl") {
			return nil
		}

		enclosing := ""

		for i, line := range strings.Split(body, "\n") {
			if m := funcLineRx.FindStringSubmatch(line); m != nil {
				enclosing = m[1]
			}

			field := guardedTelegramFieldRx.FindString(line)
			if field == "" {
				continue
			}

			if _, ok := telegramFieldAccessors[enclosing]; ok {
				found[enclosing] = append(found[enclosing], field)

				continue
			}

			t.Errorf("%s:%d: %s читается напрямую в %s(); состояние драйвера, которое меняет "+
				"ротация токена, доступно только через аксессоры (%s)",
				relative, i+1, field, enclosing, strings.Join(sortedKeys(telegramFieldAccessors), ", "))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walkTemplates() error: %v", err)
	}

	// Без этой проверки удалённый драйвер (или сломанный обход шаблонов) дал бы
	// зелёный прогон: нет чтений — нет и нарушений.
	for name, why := range telegramFieldAccessors {
		if len(found[name]) == 0 {
			t.Errorf("%s() не читает ни одного защищённого поля — %s больше не на месте, "+
				"и гвард перестал что-либо проверять", name, why)
		}
	}
}

// TestTelegramPollerChannelIsNotAField keeps the poller channel out of an
// exported field.
//
// `Telegram.Poller` was assigned once in `Run` and read from the worker
// goroutine through the service, so rotation had nowhere to publish a new
// poller: it replaced the bot and left the channel of the stopped one in place.
// The channel the worker reads is now the driver's own, handed out by
// GetUpdateChannel and never closed — a closed channel would hand the worker a
// zero update indistinguishable from a real one.
func TestTelegramPollerChannelIsNotAField(t *testing.T) {
	err := walkTemplates(func(relative, body string) error {
		for i, line := range strings.Split(body, "\n") {
			if strings.Contains(line, ".Poller") {
				t.Errorf("%s:%d: поле Poller больше не существует — канал апдейтов отдаёт "+
					"GetUpdateChannel", relative, i+1)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walkTemplates() error: %v", err)
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))

	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}
