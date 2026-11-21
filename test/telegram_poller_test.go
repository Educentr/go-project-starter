package test

import (
	"strings"
	"testing"
)

// TestGenerateTelegramPollerRotation — ротация токена бота в сгенерированном
// проекте: бот под мьютексом, опрос апдейтов переезжает на нового бота.
//
// До этого ротация (колбэк подписки OnlineConf) писала в поле `tg.bot` голым
// присваиванием, а читали его девятнадцать мест в пяти файлах драйвера — из
// горутин обработки апдейтов и запросов. Сверх гонки данных она ломала и сам
// опрос: `Run` держал long-poll на боте прежнего токена, ротация подменяла
// только поле, и `StopReceivingUpdates` на остановке приходился уже на нового
// бота — опрос прежнего не останавливался никогда (отозванный токен в бою,
// утёкшая горутина на остановке воркера).
//
// Поведение опроса проверяет тест в самом сгенерированном проекте
// (psg_telegram_poller_test.go, запускается под `-race`); здесь — что генератор
// выдаёт именно эту конструкцию: аксессор, перезапуск опроса помпой и чтение
// канала апдейтов через аксессор в сервисе и воркере.
func TestGenerateTelegramPollerRotation(t *testing.T) {
	tmpDir := generateFromConfig(t, "worker-telegram", "Generate project with a telegram worker")

	driver := readGenerated(t, tmpDir, "pkg", "drivers", "telegram", "psg_telegram_gen.go")

	for _, exp := range []string{
		// Состояние ротации — под одним мьютексом драйвера.
		"mu sync.RWMutex",
		"func (tg *Telegram) currentBot() *tgbotapi.BotAPI",
		"func (tg *Telegram) applyToken(bot *tgbotapi.BotAPI, sha []byte)",
		// Опрос поднимает и перезапускает помпа, а не Run: смещение известно
		// только тому, кто дочитал канал прежнего опроса.
		"func (tg *Telegram) pumpUpdates(ctx context.Context, done chan struct{})",
		"func (tg *Telegram) startPoller(ctx context.Context) (tgbotapi.UpdatesChannel, bool)",
		"Offset:  tg.updateOffset,",
		// Остановка бьёт по боту, на котором опрос реально работает.
		"func (tg *Telegram) stopPollerBot()",
		"bot.StopReceivingUpdates()",
		// Канал наружу живёт столько же, сколько драйвер.
		"func (tg *Telegram) GetUpdateChannel(_ context.Context) tgbotapi.UpdatesChannel",
	} {
		if !strings.Contains(driver, exp) {
			t.Errorf("psg_telegram_gen.go should contain %q", exp)
		}
	}

	for _, gone := range []string{
		// Поле канала опроса: писал Run, читал воркер через сервис, ротации
		// опубликовать новый опрос было некуда.
		".Poller",
		// Остановка по текущему боту вместо того, на котором опрос работает.
		"tg.bot.StopReceivingUpdates()",
		// Отдельный мьютекс только на ключ подписи: он закрывал гонку на одном
		// поле из трёх, которые меняет одно событие.
		"tgSHAMu",
	} {
		if strings.Contains(driver, gone) {
			t.Errorf("psg_telegram_gen.go still contains %q", gone)
		}
	}

	// Сервис отдаёт канал через аксессор драйвера, а не читает поле.
	service := readGenerated(t, tmpDir, "internal", "pkg", "service", "psg_telegram_gen.go")
	if !strings.Contains(service, "return s.telegram.GetUpdateChannel(ctx)") {
		t.Error("the service should hand out the driver's update channel through its accessor")
	}

	if strings.Contains(service, "s.telegram.Poller") {
		t.Error("the service still reads the driver's Poller field")
	}

	// Воркер не отдаёт обработчику нулевой апдейт из закрытого канала и не
	// крутится на нём: приём с проверкой ok.
	worker := readGenerated(t, tmpDir, "internal", "app", "worker", "telegrambot", "psg_telegram_gen.go")
	for _, exp := range []string{
		"case update, ok := <- w.tgSrv.GetUpdateChannel(ctx):",
		"if !ok {",
	} {
		if !strings.Contains(worker, exp) {
			t.Errorf("the worker loop should check the receive, psg_telegram_gen.go should contain %q", exp)
		}
	}

	// Сгенерированный тест опроса — часть проекта: без него ротация проверяется
	// только глазами ревьюера.
	readGenerated(t, tmpDir, "pkg", "drivers", "telegram", "psg_telegram_poller_test.go")
}
