package templater

import (
	"regexp"
	"strings"
	"testing"
)

// botReadRx matches a read of the telegram driver's current bot: the accessor is
// the only way to it, which TestTelegramDriverStateOnlyBehindAccessors keeps true.
var botReadRx = regexp.MustCompile(`\btg\.currentBot\(`)

// signKeyReadRx matches a read of the token-derived signing key — the accessor
// and, for the sake of the guard, the field it hands out.
var signKeyReadRx = regexp.MustCompile(`\b(?:tg\.GetTgTokenSHA\(|tg\.tgSHA\b)`)

// TestTelegramNoOperationNeedsBotAndSignKeyTogether keeps the driver free of an
// operation that needs the bot and the signing key to be the same token's.
//
// Both live under the driver's mutex and rotation replaces them in one go, but a
// function that reads them through two accessors takes two separate RLocks — and
// a rotation landing between the two pairs one token's key with another token's
// bot. The invoice signature no longer depends on the token at all (it has its
// own secret), so nothing in the driver needs that pair, and a function that
// asked for both would be announcing a requirement that is no longer met.
//
// This is one half of the answer, not the whole of it. The invoice took the two
// in *different* functions — PackInvoiceData the key, SendInvoice the bot — which
// a per-function rule cannot see; what rules that shape out is the signature
// having its own key, pinned by TestTelegramInvoiceSignsWithItsOwnSecret below.
//
// Rotation itself is exempt: applyToken writes both under one Lock, which is the
// point of the single mutex.
func TestTelegramNoOperationNeedsBotAndSignKeyTogether(t *testing.T) {
	bot := map[string]struct{}{}
	key := map[string]struct{}{}

	err := walkTemplates(func(relative, body string) error {
		if !strings.HasSuffix(relative, ".go.tmpl") || strings.HasSuffix(relative, "_test.go.tmpl") {
			return nil
		}

		enclosing := ""

		for _, line := range strings.Split(body, "\n") {
			if m := funcLineRx.FindStringSubmatch(line); m != nil {
				enclosing = m[1]
			}

			where := relative + " " + enclosing + "()"

			if botReadRx.MatchString(line) {
				bot[where] = struct{}{}
			}

			if signKeyReadRx.MatchString(line) {
				key[where] = struct{}{}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walkTemplates() error: %v", err)
	}

	for where := range bot {
		if _, both := key[where]; both {
			t.Errorf("%s reads both the current bot and the token-derived signing key: the two "+
				"come from separate RLocks, so a rotation in between pairs one token's key with "+
				"another token's bot. Sign with the invoice secret (invoiceSignKey) instead",
				where)
		}
	}

	// Без этих двух проверок переименованный аксессор дал бы зелёный прогон: нет
	// чтений — нет и пар.
	if len(bot) == 0 {
		t.Error("no template reads the current bot: the scan is broken, not the templates")
	}

	if len(key) == 0 {
		t.Error("no template reads the token-derived signing key: the scan is broken, not the templates")
	}
}

// TestTelegramInvoiceSignsWithItsOwnSecret pins where the invoice signature gets
// its key from.
//
// The guard above says no operation reads the bot and the token key together;
// this one says why it does not have to — the invoice reads a secret of its own.
// Dropping the secret and going back to the token key would satisfy the guard
// above in the one shape that reintroduces the defect: an invoice signed with the
// key of a token that may be rotated before the user pays.
func TestTelegramInvoiceSignsWithItsOwnSecret(t *testing.T) {
	const invoiceTemplate = "worker/template/telegram/shared/pkg/drivers/telegram/invoice.go.tmpl"

	body, err := ReadTemplateSource(invoiceTemplate)
	if err != nil {
		t.Fatalf("ReadTemplateSource(%q) error: %v", invoiceTemplate, err)
	}

	for _, want := range []string{
		// Упаковка подписывает секретом инвойса и отказывается, если его нет.
		"key, ok, err := invoiceSignKey(ctx)",
		"return \"\", ErrInvoiceSignKeyNotSet",
		// Проверка берёт ключи списком: секрет инвойса и переходный ключ токена.
		"keys, err := tg.invoiceSignKeys(ctx)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("%s should contain %q", invoiceTemplate, want)
		}
	}

	if strings.Contains(body, "key, err := tg.GetTgTokenSHA(ctx)") {
		t.Errorf("%s signs or verifies an invoice with the key of the current bot token: "+
			"an invoice outlives a rotation, so its signature must not depend on the token",
			invoiceTemplate)
	}
}
