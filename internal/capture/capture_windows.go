package capture

import (
	"context"
	_ "embed"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

//go:embed embed/install_env.bat
var installScript []byte

//go:embed embed/azoth_run.py
var runShim []byte

const (
	installerName = "install_env.bat"
	shimName      = "azoth_run.py"

	// The game keeps Python's stdout open after Python dies.
	outputDrainWait = 2 * time.Second
)

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

// startEngine on Windows launches the game itself under Frida (with the
// anti-cheat absent) and records until the timeout or Stop.
func (r *Runner) startEngine(ctx context.Context, exePath, session string, timeout int, onePerEP bool, setup hookSetup) {
	args := []string{
		setup.shim, r.captureDir(), setup.patched,
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

func (r *Runner) openInFileManager(dir string) error {
	return exec.Command("explorer.exe", dir).Start()
}
