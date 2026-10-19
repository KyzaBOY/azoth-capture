package capture

import (
	"os"
	"strings"
	"testing"
)

// Runs prepareHooks against a real game install. Needs AZOTH_NW_EXE (path of
// NewWorld.exe) and AZOTH_CAPTURE_ROOT (a folder where the app already ran its
// first-time setup, i.e. holding Aeternum-World/). Skipped otherwise.
func TestPrepareHooksRealGame(t *testing.T) {
	exe, root := os.Getenv("AZOTH_NW_EXE"), os.Getenv("AZOTH_CAPTURE_ROOT")
	if exe == "" || root == "" {
		t.Skip("set AZOTH_NW_EXE and AZOTH_CAPTURE_ROOT to run this")
	}
	r := New(root, func(s string) { t.Log(s) }, nil)
	setup, err := r.prepareHooks(exe)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(setup.patched)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "const VAS = {") || strings.Contains(string(b), "0x1478F17E0") {
		t.Fatalf("patched script looks wrong:\n%s", b[:600])
	}
	t.Logf("shim=%s patched=%s", setup.shim, setup.patched)
}
