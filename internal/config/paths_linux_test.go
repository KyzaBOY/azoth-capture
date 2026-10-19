package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultOutputDirPlain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DOCUMENTS_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config")) // empty config dir

	got := DefaultOutputDir()
	want := filepath.Join(home, "Documents", outputFolderName)
	if got != want {
		t.Errorf("DefaultOutputDir = %q, want %q", got, want)
	}
}

func TestDefaultOutputDirFromEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DOCUMENTS_DIR", "$HOME/Papers")

	got := DefaultOutputDir()
	want := filepath.Join(home, "Papers", outputFolderName)
	if got != want {
		t.Errorf("DefaultOutputDir = %q, want %q", got, want)
	}
}

func TestDocumentsFromUserDirsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DOCUMENTS_DIR", "")
	cfg := filepath.Join(home, ".config")
	t.Setenv("XDG_CONFIG_HOME", cfg)
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `# generated
XDG_DESKTOP_DIR="$HOME/Desktop"
XDG_DOCUMENTS_DIR="$HOME/MyDocs"
`
	if err := os.WriteFile(filepath.Join(cfg, "user-dirs.dirs"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := DefaultOutputDir()
	want := filepath.Join(home, "MyDocs", outputFolderName)
	if got != want {
		t.Errorf("DefaultOutputDir = %q, want %q", got, want)
	}
}

func TestExpandUserDir(t *testing.T) {
	home := "/home/u"
	cases := map[string]string{
		"":                "",
		"$HOME":           home,
		"$HOME/Documents": home + "/Documents",
		"~/Documents":     home + "/Documents",
		"/mnt/data/Docs":  "/mnt/data/Docs",
	}
	for in, want := range cases {
		if got := expandUserDir(in, home); got != want {
			t.Errorf("expandUserDir(%q) = %q, want %q", in, got, want)
		}
	}
}
