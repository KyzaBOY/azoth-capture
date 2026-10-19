package capture

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"azoth/capture/internal/hooks"
	"azoth/capture/internal/steam"
)

//go:embed embed/install_env.sh
var installScript []byte

//go:embed embed/azoth_attach.py
var runShim []byte

const (
	installerName = "install_env.sh"
	shimName      = "azoth_attach.py"

	fridaAddr       = "127.0.0.1:27943"
	fridaLogName    = "frida-server.log"
	gameWaitTimeout = 120 * time.Second
	serverWait      = 20 * time.Second
	pollInterval    = 200 * time.Millisecond
	dialTimeout     = 200 * time.Millisecond
	// stopGrace is how long the attach helper gets, after Stop, to flush the
	// last data, detach from the game and stop the Frida server.
	stopGrace = 15 * time.Second
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
	sites, err := hookSitesJSON(exePath, setup.addrs)
	if err != nil {
		r.fail("Cannot read the hook sites from the game executable: " + err.Error())
		return
	}

	game, err := steam.Discover()
	if err != nil {
		r.fail("Setup problem: " + err.Error())
		return
	}
	r.log("Steam:   " + game.SteamRoot)
	r.log("Proton:  " + game.ProtonWine)
	r.log("Runtime: " + game.RuntimeRun)
	r.log("Prefix:  " + game.Prefix)

	if err := r.ensureGameRunning(ctx); err != nil {
		r.fail(err.Error())
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

	if err := r.startFridaServer(serverCtx, game); err != nil {
		r.fail("Could not start the in-prefix capture server: " + err.Error())
		return
	}
	if err := waitForSocket(ctx, fridaAddr, serverWait); err != nil {
		r.fail(fmt.Sprintf("The capture server did not become ready (%v). Details: %s", err, r.fridaLogPath()))
		return
	}
	r.log("Capture server ready; attaching to the running game.")

	args := []string{
		// -u: the helper's output goes through a pipe to the Activity Log, and
		// Python holds piped output back in a buffer; unbuffered, each line
		// shows up as soon as it is printed.
		"-u",
		setup.shim,
		"--nw-dir", r.captureDir(),
		"--ledger-js", setup.patched,
		"--sites", sites,
		"--host", fridaAddr,
		"--session", session,
		"--captures-root", filepath.Join(r.captureDir(), "captures"),
		"--timeout", fmt.Sprint(timeout),
	}
	cmd := exec.CommandContext(ctx, r.pythonExe(), args...)
	cmd.Dir = r.rootDir
	// On Stop, ask the helper to finish (flush, detach, stop the server)
	// instead of killing it; it is killed only if it does not exit in time.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = stopGrace
	r.runCaptureCommand(ctx, cmd)
}

// hookSite is one function the agent will hook, with the bytes the executable
// file has at its start. The attach helper checks the running game's memory
// against these before any hook is placed.
type hookSite struct {
	Name  string `json:"name"`
	VA    string `json:"va"`
	Bytes string `json:"bytes"`
}

// hookSitesJSON describes the three hook sites for the attach helper.
func hookSitesJSON(exePath string, a hooks.Addrs) (string, error) {
	p, err := hooks.ReadPrologues(exePath, a)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal([]hookSite{
		{"SSL_read", fmt.Sprintf("0x%X", a.SSLRead), p.SSLRead},
		{"SSL_write", fmt.Sprintf("0x%X", a.SSLWrite), p.SSLWrite},
		{"nss_keylog", fmt.Sprintf("0x%X", a.Keylog), p.Keylog},
	})
	return string(b), err
}

// fail logs a reason the capture could not run and marks it as failed.
func (r *Runner) fail(msg string) {
	r.log(msg)
	r.setStatus(StatusError)
}

// ensureGameRunning makes sure NewWorld.exe is up. If it is not, it asks Steam
// to launch it and waits for it to appear, so the player gets the normal
// anti-cheat-protected session.
func (r *Runner) ensureGameRunning(ctx context.Context) error {
	if steam.GameRunning() {
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
		if steam.GameRunning() {
			r.log("New World is running; attaching to it.")
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("New World did not start within 2 minutes; launch it through Steam, then press Start again")
}

// startFridaServer launches the Windows Frida server inside the game's Proton
// prefix, through the same runtime and Proton the game uses. Its output goes to
// a log file. It runs in its own process group so that stopping the capture
// stops the whole runtime container, including the Wine process inside it.
func (r *Runner) startFridaServer(ctx context.Context, game steam.Game) error {
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

	cmd := exec.CommandContext(ctx, game.RuntimeRun, "--",
		game.ProtonWine, server, "--listen", fridaAddr)
	cmd.Dir = r.rootDir
	cmd.Env = append(os.Environ(),
		"WINEPREFIX="+game.Prefix,
		"WINEDEBUG=-all",
	)
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
