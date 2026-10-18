package capture

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"azoth/capture/internal/hooks"
)

//go:embed embed/install_env.bat
var installScript []byte

//go:embed embed/config.json
var configJSON []byte

//go:embed embed/azoth_run.py
var runShim []byte

type Status string

const (
	StatusNone       Status = "none"
	StatusInstalling Status = "installing"
	StatusInstalled  Status = "installed"
	StatusRunning    Status = "running"
	StatusDone       Status = "done"
	StatusError      Status = "error"
)

type Runner struct {
	mu       sync.Mutex
	rootDir  string
	status   Status
	cancel   context.CancelFunc
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

func (r *Runner) pythonExe() string {
	return filepath.Join(r.rootDir, "python_embedded", "python.exe")
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

// prepareHooks works out where the hooks go for THIS game build and writes the
// patched hook script plus the shim that makes the upstream runner use it.
// It fails, before anything is launched, if the build is not one we can hook
// with certainty.
func (r *Runner) prepareHooks(exePath string) (shim, patched string, err error) {
	build, ierr := hooks.Identify(exePath)
	if ierr != nil {
		return "", "", fmt.Errorf("cannot read the game executable: %w", ierr)
	}
	r.log(fmt.Sprintf("Game build %s (sha256 %s)", build.Version, build.SHA256))

	addrs, err := hooks.Find(exePath)
	if err != nil {
		return "", "", fmt.Errorf("this game build (%s) is not supported by this version of Azoth Capture, "+
			"so nothing was launched: %v. Look for an updated Azoth Capture", build.Version, err)
	}
	r.log(fmt.Sprintf("Hooks located by signature: SSL_read 0x%X, SSL_write 0x%X, nss_keylog 0x%X",
		addrs.SSLRead, addrs.SSLWrite, addrs.Keylog))

	src, err := os.ReadFile(filepath.Join(r.captureDir(), "_dtls_ledger.js"))
	if err != nil {
		return "", "", fmt.Errorf("capture tools are incomplete, run Install again: %w", err)
	}
	out, err := patchLedgerJS(string(src), addrs)
	if err != nil {
		return "", "", err
	}

	if err := os.MkdirAll(r.azothDir(), 0o755); err != nil {
		return "", "", err
	}
	patched = filepath.Join(r.azothDir(), "_dtls_ledger.js")
	shim = filepath.Join(r.azothDir(), "azoth_run.py")
	if err := os.WriteFile(patched, []byte(out), 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(shim, runShim, 0o644); err != nil {
		return "", "", err
	}
	return shim, patched, nil
}

func (r *Runner) capturesDir(session string) string {
	return filepath.Join(r.rootDir, "Aeternum-World", "Tools", "nw_capture", "captures", session)
}

func (r *Runner) IsEnvInstalled() bool {
	_, err1 := os.Stat(r.pythonExe())
	_, err2 := os.Stat(r.captureScript())
	installed := err1 == nil && err2 == nil
	if installed {
		r.setStatus(StatusInstalled)
	}
	return installed
}

func (r *Runner) ensureScripts() error {
	// The installer is code, not user settings: always refresh it from the
	// embedded copy so fixes (like the pinned upstream commit) reach installs
	// made by an older version. config.json is the user's, written only once.
	batPath := filepath.Join(r.rootDir, "install_env.bat")
	if cur, err := os.ReadFile(batPath); err != nil || !bytes.Equal(cur, installScript) {
		if err := os.WriteFile(batPath, installScript, 0o644); err != nil {
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
		return err
	}

	batPath := filepath.Join(r.rootDir, "install_env.bat")

	r.log("Starting environment setup...")
	r.log("Working in: " + r.rootDir)
	r.log("")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cmd.exe", "/C", batPath, "__APP_LAUNCHED__")
	cmd.Dir = r.rootDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start installer: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		r.log(scanner.Text())
	}

	if err := cmd.Wait(); err != nil {
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
		return fmt.Errorf("python environment not installed, run Install first")
	}
	if _, err := os.Stat(r.captureScript()); err != nil {
		return fmt.Errorf("capture script not found, run Install first")
	}
	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("executable not found: %s", exePath)
	}

	session = strings.TrimSpace(session)
	if session == "" {
		session = "my_session"
	}

	shim, patched, err := r.prepareHooks(exePath)
	if err != nil {
		return err
	}

	args := []string{
		shim, r.captureDir(), patched,
		"--target", exePath,
		"--session", session,
		"--timeout", fmt.Sprint(timeout),
	}
	if onePerEP {
		args = append(args, "--one-per-endpoint")
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.cancel = cancel
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
			r.mu.Unlock()
		}()

		cmd := exec.CommandContext(ctx, r.pythonExe(), args...)
		cmd.Dir = r.rootDir

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			r.log("Error: " + err.Error())
			r.setStatus(StatusError)
			return
		}
		cmd.Stderr = cmd.Stdout

		if err := cmd.Start(); err != nil {
			r.log("Failed to start capture: " + err.Error())
			r.setStatus(StatusError)
			return
		}

		scanner := bufio.NewScanner(stdout)
		buf := make([]byte, 0, 256*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			r.log(scanner.Text())
		}

		if err := cmd.Wait(); err != nil {
			if ctx.Err() != nil {
				r.log("")
				r.log("Capture stopped by user.")
			} else {
				r.log("Capture ended with error: " + err.Error())
			}
		} else {
			r.log("")
			r.log("Capture finished successfully.")
		}
		r.setStatus(StatusDone)
	}()

	return nil
}

func (r *Runner) StopCapture() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
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
	return exec.Command("explorer.exe", dir).Start()
}

func (r *Runner) RootDir() string { return r.rootDir }
