package capture

import (
	"context"
	_ "embed"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"azoth/capture/internal/steam"
)

//go:embed embed/install_env.bat
var installScript []byte

//go:embed embed/azoth_run.py
var runShim []byte

const (
	installerName = "install_env.bat"
	runShimName   = "azoth_run.py"
	// antiCheatPrefix starts the file names of Easy Anti-Cheat's programs
	// (EasyAntiCheat.exe, EasyAntiCheat_EOS.exe).
	antiCheatPrefix = "easyanticheat"

	// The game keeps Python's stdout open after Python dies.
	outputDrainWait = 2 * time.Second
)

// helperScripts: Windows spawns the game with the run shim, or attaches to a
// game the player already started with the attach helper.
var helperScripts = map[string][]byte{runShimName: runShim, attachHelperName: attachHelper}

func (r *Runner) pythonExe() string {
	return filepath.Join(r.rootDir, "python_embedded", "python.exe")
}

// runInstaller runs the batch installer and streams its output to the log.
func (r *Runner) runInstaller() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	batPath := filepath.Join(r.rootDir, installerName)
	cmd := exec.CommandContext(ctx, "cmd.exe", "/C", batPath, "__APP_LAUNCHED__")
	cmd.Dir = r.rootDir
	return r.streamCommand(cmd)
}

// gameExe returns the file the hooks are cut from: the running game's own
// file when the player already started it, else the file the player chose,
// which the capture launches.
func (r *Runner) gameExe(chosen string) string {
	g, found, err := findGame(chosen)
	if err != nil || !found {
		return chosen
	}
	if !strings.EqualFold(g.exe, chosen) {
		r.log("Using the running game's file " + g.exe + " (Settings has " + chosen + ").")
	}
	return g.exe
}

// startEngine on Windows attaches to New World when the player already
// started it. Otherwise it launches the game itself under Frida (with the
// anti-cheat absent) and records until the timeout or Stop.
func (r *Runner) startEngine(ctx context.Context, exePath, session string, timeout int, onePerEP bool, setup hookSetup) {
	running, found, err := findGame(exePath)
	if err != nil {
		r.fail(err.Error())
		return
	}
	if found {
		r.attachToGame(ctx, running, session, timeout, setup)
		return
	}
	args := []string{
		r.helperPath(runShimName), r.captureDir(), setup.patched, setup.common,
		"--target", exePath,
		"--session", session,
		"--timeout", fmt.Sprint(timeout),
	}
	if onePerEP {
		args = append(args, "--one-per-endpoint")
	}

	cmd := exec.CommandContext(ctx, r.pythonExe(), args...)
	cmd.Dir = r.rootDir
	cmd.WaitDelay = outputDrainWait
	r.runCaptureCommand(ctx, cmd)
}

// attachToGame records a game the player started, and leaves it running
// after the capture. Like the Linux capture, it writes only the DTLS ledger.
func (r *Runner) attachToGame(ctx context.Context, game runningGame, session string, timeout int, setup hookSetup) {
	all, err := processes()
	if err != nil {
		r.fail(err.Error())
		return
	}
	if slices.ContainsFunc(all, func(p process) bool {
		return strings.HasPrefix(strings.ToLower(p.name), antiCheatPrefix)
	}) {
		r.fail("New World is running with Easy Anti-Cheat, so nothing was attached: attaching to it " +
			"could get your account flagged. Close the game and press Start; the app then starts " +
			"the game itself without the anti-cheat. A game that runs without the anti-cheat can " +
			"be started first.")
		return
	}
	r.log("New World is already running; attaching to it. The game keeps running after the capture.")
	args, err := r.attachArgs(game.exe, session, timeout, setup, "--pid", fmt.Sprint(game.pid), "--stop-on-eof")
	if err != nil {
		r.fail(err.Error())
		return
	}
	cmd := exec.CommandContext(ctx, r.pythonExe(), args...)
	cmd.Dir = r.rootDir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		r.fail(err.Error())
		return
	}
	// On Stop, closing standard input asks the helper to finish (flush,
	// detach) instead of killing it; it is killed only if it does not exit
	// in time.
	cmd.Cancel = stdin.Close
	cmd.WaitDelay = stopGrace
	r.runCaptureCommand(ctx, cmd)
}

// runningGame is a running program of the game: its process ID and file.
type runningGame struct {
	pid uint32
	exe string
}

// findGame finds the running game: the program that runs the picked file,
// or a copy of it. When none does, it is the program named
// steam.GameProcess, so a game updated since it was picked is still found.
// found is false when the game is not running.
func findGame(picked string) (game runningGame, found bool, err error) {
	all, err := processes()
	if err != nil {
		return runningGame{}, false, err
	}
	var byFile, byName []runningGame
	for _, p := range all {
		exe, err := programFile(p.pid)
		if err != nil {
			continue // a program this app may not look at; not the game
		}
		switch {
		case picked != "" && steam.SameGame(exe, picked):
			byFile = append(byFile, runningGame{p.pid, exe})
		case strings.EqualFold(p.name, steam.GameProcess):
			byName = append(byName, runningGame{p.pid, exe})
		}
	}
	games := byFile
	if len(games) == 0 {
		games = byName
	}
	switch len(games) {
	case 0:
		return runningGame{}, false, nil
	case 1:
		return games[0], true, nil
	default:
		return runningGame{}, false, fmt.Errorf("%d copies of New World are running; close all but one", len(games))
	}
}

// process is one running program: its process ID and file name.
type process struct {
	pid  uint32
	name string
}

// processes lists the running programs.
func processes() ([]process, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot list the running programs: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	var all []process
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		all = append(all, process{entry.ProcessID, windows.UTF16ToString(entry.ExeFile[:])})
	}
	return all, nil
}

// programFile is the full path of the file a running program was started from.
func programFile(pid uint32) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	name := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(name))
	if err := windows.QueryFullProcessImageName(handle, 0, &name[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(name[:size]), nil
}

func (r *Runner) openInFileManager(dir string) error {
	return exec.Command("explorer.exe", dir).Start()
}
