package capture

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"azoth/capture/internal/hooks"
)

// The same shapes the attach helper accepts (SITE_VA / SITE_BYTES there).
var (
	siteVA    = regexp.MustCompile(`^0x[0-9A-F]+$`)
	siteBytes = regexp.MustCompile(`^[0-9A-F]{2}( [0-9A-F]{2}){15}$`)
)

// The attach helper must attach, never spawn the game, and kill only its own
// Frida server (never the game).
func TestEmbeddedAttachShim(t *testing.T) {
	if !bytes.Contains(attachHelper, []byte("device.attach")) {
		t.Error("attach helper does not attach")
	}
	for _, bad := range []string{"spawn(", "frida.kill"} {
		if bytes.Contains(attachHelper, []byte(bad)) {
			t.Errorf("attach helper contains %q: it must not spawn or kill the game", bad)
		}
	}
	if n := bytes.Count(attachHelper, []byte("kill(")); n != 1 {
		t.Errorf("attach helper has %d kill calls, want exactly 1 (the server stop)", n)
	}
	if !bytes.Contains(attachHelper, []byte("process_ids(device, SERVER_PROCESS)")) {
		t.Error("the one kill call must target the Frida server process")
	}
}

// The attach helper must check the hook sites in memory before the ledger arms,
// and must not load the upstream HTTPS tap, which swaps a game callback pointer
// that would dangle once the capture stops and the game keeps running.
func TestEmbeddedAttachShimSafety(t *testing.T) {
	if bytes.Contains(attachHelper, []byte(`"nw_https_tap.js"`)) {
		t.Error("attach helper loads nw_https_tap.js")
	}
	build := bytes.Index(attachHelper, []byte("def build_agent("))
	if build < 0 {
		t.Fatal("attach helper has no build_agent")
	}
	body := attachHelper[build:]
	guard := bytes.Index(body, []byte("guard,"))
	ledger := bytes.Index(body, []byte("ledger_js.read_text("))
	if guard < 0 || ledger < 0 || guard > ledger {
		t.Fatal("attach helper does not put the hook-site check before the ledger")
	}
	if !bytes.Contains(attachHelper, []byte("if not self.sites_ok:")) {
		t.Error("attach helper does not stop when the hook-site check fails")
	}
}

// On Windows the helper attaches with no Frida server (no --host) to the
// process ID the app found, and the app's Stop closes the helper's standard
// input, because a program without a console gets no Ctrl+C. The game is
// looked for under its real file name, with the hook helpers patched to it.
func TestEmbeddedAttachHelperRunsWithoutServer(t *testing.T) {
	for _, want := range []string{
		`parser.add_argument("--process", required=True)`,
		`parser.add_argument("--pid", type=int, default=0)`,
		`parser.add_argument("--common-js", required=True)`,
		"process_ids(device, self.process)",
		`parser.add_argument("--host", default="")`,
		"frida.get_local_device()",
		`parser.add_argument("--stop-on-eof", action="store_true")`,
		"sys.stdin.buffer.read()",
	} {
		if !bytes.Contains(attachHelper, []byte(want)) {
			t.Errorf("attach helper is missing %q", want)
		}
	}
}

// A game file the hook sites cannot be read from stops the capture before
// anything attaches.
func TestAttachArgsNeedsTheGameFile(t *testing.T) {
	r := New(t.TempDir(), nil, nil)
	if _, err := r.attachArgs(filepath.Join(t.TempDir(), "NewWorld.exe"), "s", 0, hookSetup{}); err == nil {
		t.Error("a missing game file did not stop the attach")
	}
}

// Runs against a real game install when AZOTH_NW_EXE points at NewWorld.exe
// (the exe is Amazon's and is not in this repo, so CI skips this).
func TestHookSitesJSONRealGame(t *testing.T) {
	exe := os.Getenv("AZOTH_NW_EXE")
	if exe == "" {
		t.Skip("set AZOTH_NW_EXE to the path of NewWorld.exe to run this")
	}
	a, err := hooks.Find(exe)
	if err != nil {
		t.Fatal(err)
	}
	text, err := hookSitesJSON(exe, a)
	if err != nil {
		t.Fatal(err)
	}
	var sites []hookSite
	if err := json.Unmarshal([]byte(text), &sites); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
	names := []string{"SSL_read", "SSL_write", "nss_keylog"}
	if len(sites) != len(names) {
		t.Fatalf("got %d sites, want %d", len(sites), len(names))
	}
	for i, s := range sites {
		if s.Name != names[i] || !siteVA.MatchString(s.VA) || !siteBytes.MatchString(s.Bytes) {
			t.Errorf("site %d = %+v, not in the shape the attach helper accepts", i, s)
		}
	}
	t.Log(text)

	r := New(t.TempDir(), nil, nil)
	args, err := r.attachArgs(exe, "s", 0, hookSetup{addrs: a}, "--stop-on-eof")
	if err != nil {
		t.Fatal(err)
	}
	process := slices.Index(args, "--process")
	if !slices.Contains(args, text) || args[len(args)-1] != "--stop-on-eof" ||
		args[1] != r.helperPath(attachHelperName) || process < 0 || args[process+1] != filepath.Base(exe) {
		t.Errorf("attach args = %q", args)
	}
}
