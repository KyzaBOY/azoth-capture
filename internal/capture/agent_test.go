package capture

import (
	"strings"
	"testing"

	"azoth/capture/internal/hooks"
)

// A stand-in with the same shape as upstream's table (the real file is AGPL
// and is not copied into this repo).
const ledgerStandIn = `(function () {
    if (NW._dtls_ledger_armed) return;

    const VAS = {
        SSL_read:    '0x1478F17E0',
        SSL_write:   '0x1478F1E20',
        nss_keylog:  '0x1478F23A0',
    };

    const REC_MAGIC       = 0x4C44574E;
})();
`

func TestPatchLedgerJS(t *testing.T) {
	got, err := patchLedgerJS(ledgerStandIn, hooks.Addrs{SSLRead: 0x1478F2E20, SSLWrite: 0x1478F3460, Keylog: 0x1478F39E0})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"'0x1478F2E20'", "'0x1478F3460'", "'0x1478F39E0'", "REC_MAGIC       = 0x4C44574E"} {
		if !strings.Contains(got, want) {
			t.Errorf("patched script is missing %s", want)
		}
	}
	for _, gone := range []string{"0x1478F17E0", "0x1478F1E20", "0x1478F23A0"} {
		if strings.Contains(got, gone) {
			t.Errorf("patched script still has the old address %s", gone)
		}
	}
	// Patching twice must not stack banners or change the table again.
	again, err := patchLedgerJS(got, hooks.Addrs{SSLRead: 0x1478F2E20, SSLWrite: 0x1478F3460, Keylog: 0x1478F39E0})
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
