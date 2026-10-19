package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Identify reads the file version and the SHA-256 of the game executable.
func Identify(exePath string) (Build, error) {
	var b Build
	b.Version = fileVersion(exePath)

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

func fileVersion(path string) string {
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil || size == 0 {
		return "unknown"
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return "unknown"
	}
	// The fixed numeric version has 16-bit fields and New World's last part
	// (6024067) does not fit, so read the FileVersion string instead.
	var tr *[2]uint16 // language, codepage
	var n uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&tr), &n); err != nil || tr == nil || n < 4 {
		return "unknown"
	}
	var s *uint16
	key := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileVersion`, tr[0], tr[1])
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), key, unsafe.Pointer(&s), &n); err != nil || s == nil {
		return "unknown"
	}
	return windows.UTF16PtrToString(s)
}
