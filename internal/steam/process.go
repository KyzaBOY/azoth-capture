//go:build linux

package steam

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// commLen is how much of a program's name Linux keeps in /proc/<pid>/comm.
const commLen = 15

// Running is the running game as the capture needs it: which file it was
// started from and the Wine setup it runs in. The capture helper has to run
// in this same setup, or it cannot see the game.
type Running struct {
	PID    int
	Exe    string   // the Linux path of the game file
	Prefix string   // the game's Wine prefix, or "" when this app cannot see it
	Wine   string   // the Wine program the game runs with, or "" when this app cannot see it
	Env    []string // the game's environment
	// Contained is true when the game runs in a container with its own view of
	// the file system (Steam Linux Runtime, or a launcher that uses it).
	Contained bool
}

// FindGame describes the running game: the program that runs the picked file
// (see gamePIDs).
func FindGame(picked string) (Running, error) { return findGame("/proc", picked) }

// GameRunning reports whether the game of the picked file is running.
func GameRunning(picked string) bool { return len(gamePIDs("/proc", picked)) > 0 }

func findGame(procRoot, picked string) (Running, error) {
	pids := gamePIDs(procRoot, picked)
	switch len(pids) {
	case 0:
		return Running{}, errors.New("New World is not running")
	case 1:
		return inspect(filepath.Join(procRoot, strconv.Itoa(pids[0])), pids[0])
	default:
		return Running{}, fmt.Errorf("%d copies of New World are running; close all but one", len(pids))
	}
}

// gamePIDs lists the Wine programs that run the picked file, or a copy of
// it. When none does, it lists the ones that run a file named
// GameProcess, so a game updated since it was picked is still found.
func gamePIDs(procRoot, picked string) []int {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}
	var byFile, byName []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		exe, ok := programFile(filepath.Join(procRoot, e.Name()))
		switch {
		case !ok:
		case picked != "" && SameGame(exe, picked):
			byFile = append(byFile, pid)
		case strings.EqualFold(filepath.Base(exe), GameProcess):
			byName = append(byName, pid)
		}
	}
	if len(byFile) > 0 {
		return byFile
	}
	return byName
}

// programFile is the Linux path of the Windows program a Wine process runs.
// Wine names the process after that program, so a process whose name differs
// (a launcher that only passes the path on) is not taken for it.
func programFile(procDir string) (string, bool) {
	cmdline, err := os.ReadFile(filepath.Join(procDir, "cmdline"))
	if err != nil {
		return "", false
	}
	first, _, _ := strings.Cut(string(cmdline), "\x00")
	exe, ok := linuxPath(first)
	if !ok {
		return "", false
	}
	comm, err := os.ReadFile(filepath.Join(procDir, "comm"))
	if err != nil {
		return "", false
	}
	name := filepath.Base(exe)
	if len(name) > commLen {
		name = name[:commLen]
	}
	return exe, strings.TrimSpace(string(comm)) == name
}

// inspect reads a game process from its /proc folder.
func inspect(procDir string, pid int) (Running, error) {
	g := Running{PID: pid}
	exe, ok := programFile(procDir)
	if !ok || !fileExists(exe) {
		return g, fmt.Errorf("cannot find the running game's file (process %d)", pid)
	}
	g.Exe = exe

	environ, err := os.ReadFile(filepath.Join(procDir, "environ"))
	if err != nil {
		return g, fmt.Errorf("cannot read the game's environment: %w", err)
	}
	for _, kv := range strings.Split(string(environ), "\x00") {
		if kv != "" {
			g.Env = append(g.Env, kv)
		}
	}
	g.Prefix = envValue(g.Env, "WINEPREFIX")
	if g.Prefix == "" {
		g.Prefix = filepath.Join(envValue(g.Env, "HOME"), ".wine") // Wine's default
	}
	if !dirExists(g.Prefix) { // a path only a container sees
		g.Prefix = ""
	}
	g.Wine = wineProgram(g.Env, filepath.Join(procDir, "exe"))
	g.Contained = !sameMounts(procDir)
	return g, nil
}

// wineProgram finds the Wine the game runs with: WINELOADER, which Wine sets
// for the programs it starts, or the "wine" next to the game's loader.
func wineProgram(env []string, exeLink string) string {
	if w := envValue(env, "WINELOADER"); w != "" && fileExists(w) {
		return w
	}
	loader, err := os.Readlink(exeLink)
	if err != nil {
		return ""
	}
	for _, name := range []string{"wine", "wine64"} {
		if w := filepath.Join(filepath.Dir(loader), name); fileExists(w) {
			return w
		}
	}
	return ""
}

// sameMounts reports whether the process sees the same file system as this
// app (it is not in a container).
func sameMounts(procDir string) bool {
	theirs, err1 := os.Readlink(filepath.Join(procDir, "ns", "mnt"))
	ours, err2 := os.Readlink("/proc/self/ns/mnt")
	return err1 == nil && err2 == nil && theirs == ours
}

func envValue(env []string, key string) string {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

// linuxPath turns a Wine "Z:\..." path into a Linux path.
func linuxPath(winPath string) (string, bool) {
	if len(winPath) < 3 || !strings.EqualFold(winPath[:3], `Z:\`) {
		return "", false
	}
	return "/" + strings.ReplaceAll(winPath[3:], `\`, "/"), true
}
