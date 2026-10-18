package hooks

import (
	"os"
	"testing"
)

// Runs against a real game install when AZOTH_NW_EXE points at NewWorld.exe
// (the exe is Amazon's and is not in this repo, so CI skips this).
func TestRealGameExe(t *testing.T) {
	exe := os.Getenv("AZOTH_NW_EXE")
	if exe == "" {
		t.Skip("set AZOTH_NW_EXE to the path of NewWorld.exe to run this")
	}
	b, err := Identify(exe)
	if err != nil {
		t.Fatal(err)
	}
	a, err := Find(exe)
	if err != nil {
		t.Fatalf("build %s (sha256 %s): %v", b.Version, b.SHA256, err)
	}
	t.Logf("build %s sha256 %s", b.Version, b.SHA256)
	t.Logf("SSL_read=%#X SSL_write=%#X nss_keylog=%#X", a.SSLRead, a.SSLWrite, a.Keylog)

	// Verified by hand against the Ghidra database of the previous build
	// (bodies identical except for relocated displacements).
	if b.SHA256 == "30fe6461f929ba44178376a72f0765dc5da28ccba20e5006a60a229babd74033" {
		want := Addrs{SSLRead: 0x1478F2E20, SSLWrite: 0x1478F3460, Keylog: 0x1478F39E0}
		if a != want {
			t.Fatalf("build 1.460.6033: got %+v, want %+v", a, want)
		}
	}
}
