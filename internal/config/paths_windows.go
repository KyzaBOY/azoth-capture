package config

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// DefaultOutputDir returns the Documents\Azoth Captures folder.
func DefaultOutputDir() string {
	if d, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0); err == nil && d != "" {
		return filepath.Join(d, outputFolderName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Documents", outputFolderName)
}
