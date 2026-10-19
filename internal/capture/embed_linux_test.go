//go:build linux

package capture

import (
	"bytes"
	"testing"
)

// The shell installer must keep the same safety the batch installer has: it
// pins the upstream commit, never follows upstream HEAD, and refuses to run as
// root.
func TestEmbeddedShellInstaller(t *testing.T) {
	if len(installScript) == 0 {
		t.Fatal("embedded installer is empty")
	}
	if bytes.Contains(installScript, []byte("\r\n")) {
		t.Fatal("install_env.sh must use LF line endings")
	}
	must := [][]byte{
		[]byte("NW_CAPTURE_COMMIT_DEFAULT="),        // pinned commit kept
		[]byte("checkout --quiet --force --detach"), // forced to the pin
		[]byte("sha256sum -c -"),                    // frida-server verified
		[]byte("do not run this as root"),           // same guard as Windows
	}
	for _, m := range must {
		if !bytes.Contains(installScript, m) {
			t.Errorf("install_env.sh is missing %q", m)
		}
	}
	for _, bad := range []string{"git -C \"$CAPTURE_DIR\" pull", "pull --ff-only"} {
		if bytes.Contains(installScript, []byte(bad)) {
			t.Errorf("install_env.sh contains %q: it must never follow upstream HEAD", bad)
		}
	}
}

// The attach shim must attach, never spawn the game, and kill only its own
// Frida server (never the game).
func TestEmbeddedAttachShim(t *testing.T) {
	if !bytes.Contains(runShim, []byte("device.attach")) {
		t.Error("attach shim does not attach")
	}
	for _, bad := range []string{"spawn(", "frida.kill"} {
		if bytes.Contains(runShim, []byte(bad)) {
			t.Errorf("attach shim contains %q: it must not spawn or kill the game", bad)
		}
	}
	if n := bytes.Count(runShim, []byte("kill(")); n != 1 {
		t.Errorf("attach shim has %d kill calls, want exactly 1 (the server stop)", n)
	}
	if !bytes.Contains(runShim, []byte("process_ids(device, SERVER_PROCESS)")) {
		t.Error("the one kill call must target the Frida server process")
	}
}

// The attach shim must check the hook sites in memory before the ledger arms,
// and must not load the upstream HTTPS tap, which swaps a game callback pointer
// that would dangle once the capture stops and the game keeps running.
func TestEmbeddedAttachShimSafety(t *testing.T) {
	if bytes.Contains(runShim, []byte(`"nw_https_tap.js"`)) {
		t.Error("attach shim loads nw_https_tap.js")
	}
	build := bytes.Index(runShim, []byte("def build_agent("))
	if build < 0 {
		t.Fatal("attach shim has no build_agent")
	}
	body := runShim[build:]
	guard := bytes.Index(body, []byte("guard,"))
	ledger := bytes.Index(body, []byte("ledger_js.read_text("))
	if guard < 0 || ledger < 0 || guard > ledger {
		t.Fatal("attach shim does not put the hook-site check before the ledger")
	}
	if !bytes.Contains(runShim, []byte("if not self.sites_ok:")) {
		t.Error("attach shim does not stop when the hook-site check fails")
	}
}
