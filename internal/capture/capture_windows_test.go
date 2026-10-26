package capture

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The test program itself is running, so the process list holds it, and its
// file can be read.
func TestProcessesFindThisProgram(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	all, err := processes()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(p process) bool {
		return p.pid == uint32(os.Getpid())
	})
	if i < 0 || !strings.EqualFold(all[i].name, filepath.Base(exe)) {
		t.Fatalf("this program is not in the process list")
	}
	file, err := programFile(all[i].pid)
	if err != nil || !strings.EqualFold(filepath.Base(file), filepath.Base(exe)) {
		t.Errorf("programFile = %q, %v; want %s", file, err, exe)
	}
}

// A running program is found by its file: here the test program itself
// stands in for the game.
func TestFindGameByPickedFile(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	g, found, err := findGame(exe)
	if err != nil || !found || g.pid != uint32(os.Getpid()) {
		t.Errorf("findGame = %+v, %v, %v; want this program", g, found, err)
	}
}

// With no game running, the capture uses the file from Settings.
func TestGameExeWithoutRunningGame(t *testing.T) {
	picked := filepath.Join(t.TempDir(), "NewWorld.exe")
	if _, found, err := findGame(picked); err != nil || found {
		t.Skip("New World is running on this PC")
	}
	r := New(t.TempDir(), nil, nil)
	if got := r.gameExe(picked); got != picked {
		t.Errorf("gameExe = %q", got)
	}
}
