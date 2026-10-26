package steam

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGame(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSameGame(t *testing.T) {
	dir := t.TempDir()
	picked := writeGame(t, filepath.Join(dir, "NewWorld.exe"), "MZ game")
	renamedCopy := writeGame(t, filepath.Join(dir, "client.exe"), "MZ game")
	other := writeGame(t, filepath.Join(dir, "other.exe"), "MZ else!")
	link := filepath.Join(dir, "link.exe")
	if err := os.Link(picked, link); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		picked:                            true,
		link:                              true, // the same file under another name
		renamedCopy:                       true, // a copy with the same contents
		other:                             false,
		filepath.Join(dir, "missing.exe"): false,
	} {
		if got := SameGame(path, picked); got != want {
			t.Errorf("SameGame(%s) = %v, want %v", filepath.Base(path), got, want)
		}
	}
}

func TestGameFileName(t *testing.T) {
	if got := GameFileName(""); got != GameProcess {
		t.Errorf("no picked file: %q", got)
	}
	if got := GameFileName(filepath.Join("games", "client.exe")); got != "client.exe" {
		t.Errorf("picked file: %q", got)
	}
}
