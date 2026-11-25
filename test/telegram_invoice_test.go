package test

import (
	"strings"
	"testing"
)

// TestGenerateTelegramInvoiceSignKey — подпись инвойса в сгенерированном
// проекте не зависит от токена бота.
//
// До этого ключ подписи выводился из токена (`HMAC("WebAppData", token)`), а
// проверялся ключом токена **текущего**. Инвойс при этом живёт в чате до оплаты,
// то есть минуты и часы: ротация токена в это окно означала «invalid sign» на
// pre_checkout_query, а попав между pre_checkout_query и successful_payment —
// списанные Stars без выдачи товара. Сверх этого упаковка и отправка брали ключ
// и бота двумя отдельными захватами мьютекса.
//
// Поведение проверяют тесты в самом сгенерированном проекте
// (psg_invoice_test.go, запускается под `-race` стадией docker-integration);
// здесь — что генератор выдаёт именно эту конструкцию: объявленный ключ,
// аксессор, отказ без секрета и отсутствие подписи ключом токена.
func TestGenerateTelegramInvoiceSignKey(t *testing.T) {
	tmpDir := generateFromConfig(t, "worker-telegram", "Generate project with a telegram worker")

	occonf := readGenerated(t, tmpDir, "internal", "pkg", "occonf", "psg_occonf_gen.go")

	for _, exp := range []string{
		// Ключ объявлен, поэтому у него есть аксессор и построитель пути —
		// второй нужен сгенерированному тесту драйвера, чтобы засеять дерево.
		"func TelegramInvoiceSignKey(ctx context.Context) (string, bool, error)",
		"func TelegramInvoiceSignKeyPath() string",
		"security/invoice_sign_key",
	} {
		if !strings.Contains(occonf, exp) {
			t.Errorf("psg_occonf_gen.go should contain %q", exp)
		}
	}

	// Реестр поверхности дерева называет ключ и его читателя: иначе владелец
	// контракта не узнает, что сервису нужно засеять новый секрет.
	registry := readGenerated(t, tmpDir, "configs", "onlineconf-tree.yaml")
	for _, exp := range []string{
		"/telegramtest/security/invoice_sign_key",
		"accessor: TelegramInvoiceSignKey",
	} {
		if !strings.Contains(registry, exp) {
			t.Errorf("configs/onlineconf-tree.yaml should contain %q", exp)
		}
	}

	driver := readGenerated(t, tmpDir, "pkg", "drivers", "telegram", "psg_invoice_gen.go")

	for _, exp := range []string{
		// Упаковка подписывает секретом инвойса.
		"key, ok, err := invoiceSignKey(ctx)",
		// И отказывает, если его нет: инвойс, который при оплате нечем
		// проверить, — это списанные Stars без выдачи товара.
		"return \"\", ErrInvoiceSignKeyNotSet",
		// Проверка берёт ключи списком — секрет инвойса и переходный ключ
		// прежних инвойсов.
		"func (tg *Telegram) invoiceSignKeys(ctx context.Context) ([][]byte, error)",
		"keys, err := tg.invoiceSignKeys(ctx)",
		// Сравнение подписи — за постоянное время.
		"hmac.Equal(sign, signCheck[0:invoiceSignLen])",
	} {
		if !strings.Contains(driver, exp) {
			t.Errorf("psg_invoice_gen.go should contain %q", exp)
		}
	}

	// Подпись ключом текущего токена: именно её не переживал инвойс.
	if strings.Contains(driver, "key, err := tg.GetTgTokenSHA(ctx)") {
		t.Error("psg_invoice_gen.go still signs an invoice with the key of the current bot token")
	}

	auth := readGenerated(t, tmpDir, "pkg", "drivers", "telegram", "psg_auth_gen.go")
	if !strings.Contains(auth, "occonf.TelegramInvoiceSignKey(ctx)") {
		t.Error("the driver should read the invoice signing secret through the accessor package")
	}

	// Сгенерированный тест подписи — часть проекта: без него ротация между
	// выпуском инвойса и его оплатой проверяется только глазами ревьюера.
	readGenerated(t, tmpDir, "pkg", "drivers", "telegram", "psg_invoice_test.go")
}
