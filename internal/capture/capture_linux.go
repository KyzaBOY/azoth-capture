package capture

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"azoth/capture/internal/steam"
)

//go:embed embed/install_env.sh
var installScript []byte

// helperScripts: Linux always attaches to the running game.
var helperScripts = map[string][]byte{attachHelperName: attachHelper}

const (
	installerName = "install_env.sh"

	fridaAddr       = "127.0.0.1:27943"
	fridaLogName    = "frida-server.log"
	gameWaitTimeout = 120 * time.Second
	serverWait      = 20 * time.Second
	pollInterval    = 200 * time.Millisecond
	dialTimeout     = 200 * time.Millisecond
)

func (r *Runner) pythonExe() string {
	return filepath.Join(r.rootDir, ".venv-capture", "bin", "python")
}

func (r *Runner) fridaServerExe() string {
	return filepath.Join(r.captureDir(), "frida-server.exe")
}

func (r *Runner) fridaLogPath() string {
	return filepath.Join(r.captureDir(), "logs", fridaLogName)
}

// runInstaller runs the shell installer and streams its output to the log.
func (r *Runner) runInstaller() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	scriptPath := filepath.Join(r.rootDir, installerName)
	cmd := exec.CommandContext(ctx, "bash", scriptPath, "__APP_LAUNCHED__")
	cmd.Dir = r.rootDir
	return r.streamCommand(cmd)
}

// startEngine on Linux keeps the anti-cheat active: the game runs the normal
// way through Steam, and the capture ATTACHES to it. A Windows Frida server runs
// inside the game's Proton prefix so it can see inside the game; the attach
// helper connects to it over a local socket. The game is never spawned or
// killed by this app.
//
// The Linux capture writes only the DTLS ledger, so onePerEP (an option of the
// Windows HTTPS extractor) does not apply here.
func (r *Runner) startEngine(ctx context.Context, exePath, session string, timeout int, onePerEP bool, setup hookSetup) {
	args, err := r.attachArgs(exePath, session, timeout, setup, "--host", fridaAddr)
	if err != nil {
		r.fail(err.Error())
		return
	}

	if err := r.ensureGameRunning(ctx, exePath); err != nil {
		r.fail(err.Error())
		return
	}
	running, err := steam.FindGame(exePath)
	if err != nil {
		r.fail("Setup problem: " + err.Error())
		return
	}
	launch, err := r.serverLaunch(running)
	if err != nil {
		r.fail("Setup problem: " + err.Error())
		return
	}

	if socketOpen(fridaAddr) {
		r.fail(fmt.Sprintf("Port %s is already in use. Close any other capture tool and try again.", fridaAddr))
		return
	}

	// The Frida server lives only as long as this capture. Its context is NOT
	// the capture's: on Stop the attach helper still needs the server to flush
	// and detach, and the helper stops the server itself. stopServer runs when
	// this function returns, as a backstop.
	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()

	if err := r.startFridaServer(serverCtx, launch); err != nil {
		r.fail("Could not start the in-prefix capture server: " + err.Error())
		return
	}
	if err := waitForSocket(ctx, fridaAddr, serverWait); err != nil {
		r.fail(fmt.Sprintf("The capture server did not become ready (%v). Details: %s", err, r.fridaLogPath()))
		return
	}
	r.log("Capture server ready; attaching to the running game.")

	cmd := exec.CommandContext(ctx, r.pythonExe(), args...)
	cmd.Dir = r.rootDir
	// On Stop, ask the helper to finish (flush, detach, stop the server)
	// instead of killing it; it is killed only if it does not exit in time.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = stopGrace
	r.runCaptureCommand(ctx, cmd)
}

// gameExe returns the file the hooks are cut from. The Linux capture attaches
// to the running game, so that is the file: the running game's own, or
// before it starts, the one in Steam's install (Steam starts it). The path in Settings is used only when
// neither is found. A wrong file makes the memory check refuse to hook, so
// this picks the file that is really running.
func (r *Runner) gameExe(chosen string) string {
	exe := chosen
	if g, err := steam.FindGame(chosen); err == nil {
		exe = g.Exe
	} else if game, derr := steam.Discover(); derr == nil {
		exe = game.ExePath
	}
	if exe != chosen {
		r.log("Using the game file " + exe + " (Settings has " + chosen + ").")
	}
	return exe
}

// ensureGameRunning makes sure the game of the picked file is up. If it is
// not, it asks Steam to launch it and waits for it to appear, so the player
// gets the normal anti-cheat-protected session.
func (r *Runner) ensureGameRunning(ctx context.Context, picked string) error {
	if steam.GameRunning(picked) {
		r.log("New World is already running; attaching to it.")
		return nil
	}
	if _, err := exec.LookPath("steam"); err == nil {
		r.log("Asking Steam to launch New World...")
		launchDetached("steam", "-applaunch", steam.AppID)
	} else {
		r.log("Steam command not found. Please start New World through Steam now.")
	}
	r.log("Waiting for New World to start (up to 2 minutes)...")
	deadline := time.Now().Add(gameWaitTimeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return fmt.Errorf("cancelled before the game started")
		}
		if steam.GameRunning(picked) {
			r.log("New World is running; attaching to it.")
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("New World did not start within 2 minutes; launch it through Steam, then press Start again")
}

// serverCmd is how to start a Windows program in the game's Wine setup: the
// command before the program's own path, and the environment.
type serverCmd struct {
	argv []string
	env  []string
}

// syncKeys choose how Wine programs synchronize. A program can only join the
// game's Wine session when they match the game's.
var syncKeys = []string{"WINEFSYNC", "WINEESYNC", "WINENTSYNC"}

// serverLaunch picks how to start the capture server so it runs in the same
// Wine prefix as the game; a program in another prefix cannot see the game.
//
// A game outside any container (plain Wine or Proton) is joined with its own
// Wine and environment. A game in a
// container (Steam Linux Runtime) is joined through Steam's runtime, with the
// game's own prefix and Wine when this app can see them.
func (r *Runner) serverLaunch(g steam.Running) (serverCmd, error) {
	if !g.Contained && g.Wine != "" && g.Prefix != "" {
		r.log("Game:    " + g.Exe + " (not in a container)")
		r.log("Wine:    " + g.Wine)
		r.log("Prefix:  " + g.Prefix)
		return serverCmd{argv: []string{g.Wine}, env: append(append([]string{}, g.Env...), "WINEDEBUG=-all")}, nil
	}
	game, err := steam.Discover()
	if err != nil {
		return serverCmd{}, fmt.Errorf("the game runs in a container, and Steam's runtime is needed to reach it: %w", err)
	}
	wine, prefix := game.ProtonWine, game.Prefix
	if g.Wine != "" {
		wine = g.Wine
	}
	if g.Prefix != "" {
		prefix = g.Prefix
	}
	r.log("Steam:   " + game.SteamRoot)
	r.log("Proton:  " + wine)
	r.log("Runtime: " + game.RuntimeRun)
	r.log("Prefix:  " + prefix)
	env := append(os.Environ(), "WINEPREFIX="+prefix, "WINEDEBUG=-all")
	for _, k := range syncKeys {
		for _, kv := range g.Env {
			if strings.HasPrefix(kv, k+"=") {
				env = append(env, kv)
			}
		}
	}
	return serverCmd{argv: []string{game.RuntimeRun, "--", wine}, env: env}, nil
}

// startFridaServer launches the Windows Frida server in the game's Wine setup
// (see serverLaunch). Its output goes to a log file. It runs in its own
// process group so that stopping the capture stops everything it started,
// including a runtime container and the Wine process inside it.
func (r *Runner) startFridaServer(ctx context.Context, launch serverCmd) error {
	server := r.fridaServerExe()
	if _, err := os.Stat(server); err != nil {
		return fmt.Errorf("frida-server.exe is missing; run Install again")
	}
	if err := os.MkdirAll(filepath.Dir(r.fridaLogPath()), 0o700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(r.fridaLogPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	argv := append(append([]string{}, launch.argv...), server, "--listen", fridaAddr)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = r.rootDir
	cmd.Env = launch.env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	go func() {
		cmd.Wait() // reap the process; the result is reported through the socket check
		logFile.Close()
	}()
	return nil
}

func (r *Runner) openInFileManager(dir string) error {
	return exec.Command("xdg-open", dir).Start()
}

// launchDetached starts a program in its own session, so it outlives this app
// and is not stopped with it, and reaps it when it exits.
func launchDetached(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return
	}
	go cmd.Wait()
}

// socketOpen reports whether something already accepts connections on addr.
func socketOpen(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// waitForSocket waits until addr accepts a TCP connection, the context is
// cancelled, or the timeout elapses.
func waitForSocket(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return fmt.Errorf("cancelled")
		}
		if socketOpen(addr) {
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("timed out after %s", timeout)
}
