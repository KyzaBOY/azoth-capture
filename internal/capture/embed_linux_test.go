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
