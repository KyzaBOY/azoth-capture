package steam

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
)

// GameProcess is the usual name of New World's program file. A player may
// rename the file; the capture then finds the game by the file picked in
// Settings (see SameGame), and this name is only the fallback.
const GameProcess = "NewWorld.exe"

// GameFileName is the file name the game runs under: the name of the picked
// file, or GameProcess when no file is picked.
func GameFileName(picked string) string {
	if picked == "" {
		return GameProcess
	}
	return filepath.Base(picked)
}

// SameGame reports whether path is the picked game file: the same file, or a
// copy of it with the same contents.
func SameGame(path, picked string) bool {
	a, errA := os.Stat(path)
	b, errB := os.Stat(picked)
	if errA != nil || errB != nil || a.IsDir() || b.IsDir() {
		return false
	}
	if os.SameFile(a, b) {
		return true
	}
	if a.Size() != b.Size() {
		return false
	}
	hashA, errA := fileSHA256(path)
	hashB, errB := fileSHA256(picked)
	return errA == nil && errB == nil && bytes.Equal(hashA, hashB)
}

func fileSHA256(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
