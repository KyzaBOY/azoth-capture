package capture

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"azoth/capture/internal/hooks"
)

//go:embed embed/config.json
var configJSON []byte

// Platform files (capture_windows.go / capture_linux.go) provide these: the
// installer script and its name, the capture shim and its name, and the
// functions that differ between Windows (spawn the game) and Linux (attach to
// the running game under Proton).
//
//   installScript   embedded installer bytes
//   installerName   file name the installer is written out as
//   runShim         embedded capture shim bytes
//   shimName        file name the shim is written out as
//   (*Runner) pythonExe() string
//   (*Runner) runInstaller() error
//   (*Runner) startEngine(ctx, exePath, session, timeout, onePerEP, setup)
//   (*Runner) openInFileManager(dir) error

type Status string

const (
	StatusNone       Status = "none"
	StatusInstalling Status = "installing"
	StatusInstalled  Status = "installed"
	StatusRunning    Status = "running"
	StatusDone       Status = "done"
	StatusError      Status = "error"
)

// stopWait bounds how long StopCapture waits for the capture process to
// finish. It covers the Linux helper's own grace period (flush, detach, stop
// the Frida server) with room to spare.
const stopWait = 30 * time.Second

type Runner struct {
	mu       sync.Mutex
	rootDir  string
	status   Status
	cancel   context.CancelFunc
	finished chan struct{} // closed when the running capture has fully ended
	onLog    func(string)
	onStatus func(Status)
}

func New(rootDir string, onLog func(string), onStatus func(Status)) *Runner {
	return &Runner{
		rootDir:  rootDir,
		status:   StatusNone,
		onLog:    onLog,
		onStatus: onStatus,
	}
}

func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *Runner) setStatus(s Status) {
	r.mu.Lock()
	r.status = s
	r.mu.Unlock()
	if r.onStatus != nil {
		r.onStatus(s)
	}
}

func (r *Runner) log(msg string) {
	if r.onLog != nil {
		r.onLog(msg)
	}
}

func (r *Runner) captureDir() string {
	return filepath.Join(r.rootDir, "Aeternum-World", "Tools", "nw_capture")
}

func (r *Runner) captureScript() string {
	return filepath.Join(r.captureDir(), "nw_capture.py")
}

// azothDir holds what this app generates at launch (the patched hook script
// and the launcher shim), kept apart from the untouched upstream checkout.
func (r *Runner) azothDir() string { return filepath.Join(r.rootDir, "azoth") }

// hookSetup is what prepareHooks hands to the platform engine: the shim to
// run, the patched hook script it loads, and the hook addresses it was patched
// with.
type hookSetup struct {
	shim    string
	patched string
	addrs   hooks.Addrs
}

// prepareHooks works out where the hooks go for THIS game build and writes the
// patched hook script plus the shim that makes the capture use it. It fails,
// before anything is launched, if the build is not one we can hook with
// certainty.
func (r *Runner) prepareHooks(exePath string) (hookSetup, error) {
	build, err := hooks.Identify(exePath)
	if err != nil {
		return hookSetup{}, fmt.Errorf("cannot read the game executable: %w", err)
	}
	r.log(fmt.Sprintf("Game build %s (sha256 %s)", build.Version, build.SHA256))

	addrs, err := hooks.Find(exePath)
	if err != nil {
		return hookSetup{}, fmt.Errorf("this game build (%s) is not supported by this version of Azoth Capture, "+
			"so nothing was launched: %v. Look for an updated Azoth Capture", build.Version, err)
	}
	r.log(fmt.Sprintf("Hooks located by signature: SSL_read 0x%X, SSL_write 0x%X, nss_keylog 0x%X",
		addrs.SSLRead, addrs.SSLWrite, addrs.Keylog))

	src, err := os.ReadFile(filepath.Join(r.captureDir(), "_dtls_ledger.js"))
	if err != nil {
		return hookSetup{}, fmt.Errorf("capture tools are incomplete, run Install again: %w", err)
	}
	out, err := patchLedgerJS(string(src), addrs)
	if err != nil {
		return hookSetup{}, err
	}

	if err := os.MkdirAll(r.azothDir(), 0o755); err != nil {
		return hookSetup{}, err
	}
	setup := hookSetup{
		shim:    filepath.Join(r.azothDir(), shimName),
		patched: filepath.Join(r.azothDir(), "_dtls_ledger.js"),
		addrs:   addrs,
	}
	if err := os.WriteFile(setup.patched, []byte(out), 0o644); err != nil {
		return hookSetup{}, err
	}
	if err := os.WriteFile(setup.shim, runShim, 0o644); err != nil {
		return hookSetup{}, err
	}
	return setup, nil
}

func (r *Runner) capturesDir(session string) string {
	return filepath.Join(r.captureDir(), "captures", session)
}

func (r *Runner) IsEnvInstalled() bool {
	_, err1 := os.Stat(r.pythonExe())
	_, err2 := os.Stat(r.captureScript())
	installed := err1 == nil && err2 == nil
	if installed {
		// Report "installed" only when nothing is in progress. Overwriting a
		// running status would hide the capture and let a second one start.
		r.mu.Lock()
		busy := r.status == StatusRunning || r.status == StatusInstalling
		r.mu.Unlock()
		if !busy {
			r.setStatus(StatusInstalled)
		}
	}
	return installed
}

func (r *Runner) ensureScripts() error {
	// The installer is code, not user settings: always refresh it from the
	// embedded copy so fixes (like the pinned upstream commit) reach installs
	// made by an older version. config.json is the user's, written only once.
	scriptPath := filepath.Join(r.rootDir, installerName)
	if cur, err := os.ReadFile(scriptPath); err != nil || !bytes.Equal(cur, installScript) {
		if err := os.WriteFile(scriptPath, installScript, 0o755); err != nil {
			return fmt.Errorf("failed to extract install script: %w", err)
		}
	}
	cfgPath := filepath.Join(r.rootDir, "config.json")
	if _, err := os.Stat(cfgPath); err != nil {
		if err := os.WriteFile(cfgPath, configJSON, 0o644); err != nil {
			return fmt.Errorf("failed to extract config: %w", err)
		}
	}
	return nil
}

func (r *Runner) InstallEnv() error {
	r.mu.Lock()
	if r.status == StatusRunning || r.status == StatusInstalling {
		r.mu.Unlock()
		return fmt.Errorf("busy")
	}
	r.mu.Unlock()

	r.setStatus(StatusInstalling)

	if err := r.ensureScripts(); err != nil {
		r.setStatus(StatusError)
		return err
	}

	r.log("Starting environment setup...")
	r.log("Working in: " + r.rootDir)
	r.log("")

	if err := r.runInstaller(); err != nil {
		r.log("Install failed: " + err.Error())
		r.setStatus(StatusError)
		return fmt.Errorf("install failed: %w", err)
	}

	r.log("")
	r.log("Environment setup complete!")
	r.setStatus(StatusInstalled)
	return nil
}

func (r *Runner) StartCapture(exePath, session string, timeout int, onePerEP bool) error {
	r.mu.Lock()
	if r.status == StatusRunning {
		r.mu.Unlock()
		return fmt.Errorf("a capture is already running")
	}
	r.mu.Unlock()

	if _, err := os.Stat(r.pythonExe()); err != nil {
		return fmt.Errorf("capture environment not installed, run Install first")
	}
	if _, err := os.Stat(r.captureScript()); err != nil {
		return fmt.Errorf("capture tools not found, run Install first")
	}
	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("game executable not found: %s", exePath)
	}

	session = strings.TrimSpace(session)
	if session == "" {
		session = "my_session"
	}

	setup, err := r.prepareHooks(exePath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	r.mu.Lock()
	r.cancel = cancel
	r.finished = finished
	r.mu.Unlock()

	r.setStatus(StatusRunning)
	r.log(fmt.Sprintf("Starting capture: %s", session))
	r.log(fmt.Sprintf("Target: %s", exePath))
	r.log(fmt.Sprintf("Timeout: %ds | One per endpoint: %v", timeout, onePerEP))
	r.log("")

	go func() {
		defer func() {
			r.mu.Lock()
			r.cancel = nil
			r.finished = nil
			r.mu.Unlock()
			close(finished)
		}()
		r.startEngine(ctx, exePath, session, timeout, onePerEP, setup)
	}()

	return nil
}

// StopCapture ends the running capture and waits until it has fully finished
// writing, so callers can copy the ledger right after it returns. Copying any
// earlier would miss the final flush.
func (r *Runner) StopCapture() {
	r.mu.Lock()
	cancel, finished := r.cancel, r.finished
	r.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(stopWait):
		r.log("The capture did not finish within " + stopWait.String() + "; the ledger may miss its last records.")
	}
}

func (r *Runner) LedgerPath(session string) string {
	p := filepath.Join(r.capturesDir(session), "dtls", "ledger.bin")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

func (r *Runner) OpenCaptureDir(session string) error {
	dir := r.capturesDir(session)
	if _, err := os.Stat(dir); err != nil {
		dir = filepath.Dir(dir)
	}
	os.MkdirAll(dir, 0o755)
	return r.openInFileManager(dir)
}

func (r *Runner) RootDir() string { return r.rootDir }
