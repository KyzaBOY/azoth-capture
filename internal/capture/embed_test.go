//go:build windows

package capture

import (
	"bytes"
	"testing"
)

// The batch installer must be CRLF: cmd.exe mishandles labels and goto in
// LF-only files. .gitattributes asks for CRLF on checkout; this catches a build
// from a copy of the source that ignored it (e.g. a plain export of the index).
func TestEmbeddedInstallerIsCRLF(t *testing.T) {
	if n := bytes.Count(installScript, []byte("\n")); n == 0 {
		t.Fatal("embedded installer is empty")
	}
	bare := bytes.Count(installScript, []byte("\n")) - bytes.Count(installScript, []byte("\r\n"))
	if bare != 0 {
		t.Fatalf("embedded install_env.bat has %d bare LF line endings; it must be CRLF", bare)
	}
}

func TestEmbeddedInstallerKeepsThePin(t *testing.T) {
	if !bytes.Contains(installScript, []byte(`set "NW_CAPTURE_COMMIT=`)) {
		t.Fatal("embedded installer lost its pinned upstream commit")
	}
	for _, bad := range []string{"pull --ff-only", "stash push"} {
		if bytes.Contains(installScript, []byte(bad)) {
			t.Fatalf("embedded installer contains %q: it must never follow upstream HEAD", bad)
		}
	}
}

// The run shim points the upstream runner at both patched scripts, and refuses
// to run when upstream renamed either setting.
func TestEmbeddedRunShimPatchesBothScripts(t *testing.T) {
	for _, want := range []string{
		"_runner.COMMON_JS = common",
		"_runner.DTLS_LEDGER_JS = patched",
		`for name in ("COMMON_JS", "DTLS_LEDGER_JS"):`,
	} {
		if !bytes.Contains(runShim, []byte(want)) {
			t.Errorf("run shim is missing %q", want)
		}
	}
}
