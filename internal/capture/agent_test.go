package capture

import (
	"strings"
	"testing"

	"azoth/capture/internal/hooks"
)

// A stand-in with the same shape as upstream's address table, with made-up
// values (the real file is AGPL and is not copied into this repo).
const ledgerStandIn = `(function () {
    const VAS = {
        SSL_read:    '0x1111',
        SSL_write:   '0x2222',
        nss_keylog:  '0x3333',
    };

    const SOMETHING_ELSE = 42;
})();
`

func TestPatchLedgerJS(t *testing.T) {
	a := hooks.Addrs{SSLRead: 0xAAA1, SSLWrite: 0xBBB2, Keylog: 0xCCC3}
	got, err := patchLedgerJS(ledgerStandIn, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"'0xAAA1'", "'0xBBB2'", "'0xCCC3'", "SOMETHING_ELSE = 42"} {
		if !strings.Contains(got, want) {
			t.Errorf("patched script is missing %s", want)
		}
	}
	for _, gone := range []string{"0x1111", "0x2222", "0x3333"} {
		if strings.Contains(got, gone) {
			t.Errorf("patched script still has the old address %s", gone)
		}
	}
	// Patching twice must not stack banners or change the table again.
	again, err := patchLedgerJS(got, a)
	if err != nil || again != got {
		t.Errorf("patch is not idempotent (err=%v)", err)
	}
}

func TestPatchLedgerJSRefusesUnknownShape(t *testing.T) {
	if _, err := patchLedgerJS("(function(){ const OTHER = {}; })();", hooks.Addrs{}); err == nil {
		t.Fatal("expected an error when the address table is missing")
	}
	two := ledgerStandIn + ledgerStandIn
	if _, err := patchLedgerJS(two, hooks.Addrs{}); err == nil {
		t.Fatal("expected an error when the address table appears twice")
	}
}
