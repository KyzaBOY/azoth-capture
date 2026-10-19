package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// Identify reads the SHA-256 of the game executable. On Linux the Windows file
// version resource API is not available, so the version is reported as
// "unknown"; the SHA-256 still pins the exact build for the activity log and bug
// reports, and Find() is the real gate on whether a build can be hooked.
func Identify(exePath string) (Build, error) {
	b := Build{Version: "unknown"}

	f, err := os.Open(exePath)
	if err != nil {
		return b, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return b, err
	}
	b.SHA256 = hex.EncodeToString(h.Sum(nil))
	return b, nil
}
