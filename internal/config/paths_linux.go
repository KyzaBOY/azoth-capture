package config

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultOutputDir returns the Documents/Azoth Captures folder. It honours the
// XDG Documents location when the desktop sets one, and falls back to
// ~/Documents, so it fits whatever the distribution uses.
func DefaultOutputDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(xdgDocumentsDir(home), outputFolderName)
}

// xdgDocumentsDir reads the user's Documents folder from the XDG_DOCUMENTS_DIR
// variable, then the freedesktop user-dirs file, and defaults to ~/Documents.
func xdgDocumentsDir(home string) string {
	if d := expandUserDir(os.Getenv("XDG_DOCUMENTS_DIR"), home); d != "" {
		return d
	}
	if d := documentsFromUserDirsFile(home); d != "" {
		return d
	}
	if home == "" {
		return "Documents"
	}
	return filepath.Join(home, "Documents")
}

// documentsFromUserDirsFile parses XDG_DOCUMENTS_DIR out of
// ~/.config/user-dirs.dirs, the file the desktop writes for localized folder
// names. A missing or unreadable file yields an empty string.
func documentsFromUserDirsFile(home string) string {
	if home == "" {
		return ""
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	data, err := os.ReadFile(filepath.Join(cfg, "user-dirs.dirs"))
	if err != nil {
		return ""
	}
	const key = "XDG_DOCUMENTS_DIR="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, key) {
			continue
		}
		value := strings.Trim(line[len(key):], `"`)
		return expandUserDir(value, home)
	}
	return ""
}

// expandUserDir turns a user-dirs value such as "$HOME/Documents" or
// "~/Documents" into an absolute path. An empty value yields an empty string,
// which callers read as "not set".
func expandUserDir(value, home string) string {
	switch {
	case value == "":
		return ""
	case value == "$HOME":
		return home
	case strings.HasPrefix(value, "$HOME/"):
		return filepath.Join(home, value[len("$HOME/"):])
	case strings.HasPrefix(value, "~/"):
		return filepath.Join(home, value[len("~/"):])
	}
	return value
}
