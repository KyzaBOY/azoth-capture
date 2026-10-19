package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestIdentifySHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.exe")
	content := []byte("not a real game, just bytes to hash")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	b, err := Identify(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	if b.SHA256 != want {
		t.Errorf("SHA256 = %s, want %s", b.SHA256, want)
	}
	if b.Version != "unknown" {
		t.Errorf("Version = %q, want %q", b.Version, "unknown")
	}
}

func TestIdentifyMissingFile(t *testing.T) {
	if _, err := Identify(filepath.Join(t.TempDir(), "nope.exe")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
