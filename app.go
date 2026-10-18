package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"azoth/capture/internal/capture"
	"azoth/capture/internal/config"
	"azoth/capture/internal/hotkey"
	"azoth/capture/internal/journal"
	"azoth/capture/internal/needed"
)

const (
	normalW, normalH = 1060, 820
)

type App struct {
	ctx context.Context
	j   *journal.Journal
	cap *capture.Runner

	mu    sync.Mutex
	set   config.Settings
	hk    *hotkey.Listener
	hkErr string
}

func NewApp(set config.Settings) *App {
	a := &App{set: set}
	a.j = journal.New(time.Now, config.DraftPath(), func(st journal.State) { a.emit("state", st) })
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	rootDir := a.appDir()
	a.cap = capture.New(rootDir,
		func(line string) { a.emit("caplog", line) },
		func(st capture.Status) { a.emit("capstatus", string(st)) },
	)
	a.mu.Lock()
	a.registerLocked()
	a.mu.Unlock()

	if !a.cap.IsEnvInstalled() {
		go func() {
			a.emit("caplog", "Preparing capture environment (first run — this takes a few minutes)...")
			if err := a.cap.InstallEnv(); err != nil {
				a.emit("caplog", "Setup failed: "+err.Error())
				a.emit("caplog", "You can retry from the Settings panel.")
			}
		}()
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.cap.StopCapture()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.unregisterLocked()
}

func (a *App) emit(name string, data ...any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data...)
	}
}

func (a *App) appDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// ---- boot -------------------------------------------------------------------

type Boot struct {
	Version     string            `json:"version"`
	Settings    config.Settings   `json:"settings"`
	Activities  []string          `json:"activities"`
	Draft       journal.DraftInfo `json:"draft"`
	State       journal.State     `json:"state"`
	HotkeyError string            `json:"hotkey_error"`
	EnvReady    bool              `json:"env_ready"`
	AppDir      string            `json:"app_dir"`
}

func (a *App) Boot() Boot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Boot{
		Version: journal.Version, Settings: a.set, Activities: journal.Activities,
		Draft: a.j.CheckDraft(), State: a.j.State(), HotkeyError: a.hkErr,
		EnvReady: a.cap.IsEnvInstalled(), AppDir: a.appDir(),
	}
}

// ---- unified session (capture + journal together) ---------------------------

func (a *App) StartSession(exePath, name, activity string, timeout int, onePerEP bool) (journal.State, error) {
	a.mu.Lock()
	a.set.ExePath = exePath
	a.set.Timeout = timeout
	a.set.OnePerEP = onePerEP
	a.saveLocked()
	a.mu.Unlock()

	if exePath == "" {
		return journal.State{}, fmt.Errorf("set the game executable path first")
	}

	capSt := a.cap.Status()
	if capSt == capture.StatusInstalling {
		return journal.State{}, fmt.Errorf("environment is still being set up — check the Activity Log")
	}
	if !a.cap.IsEnvInstalled() {
		return journal.State{}, fmt.Errorf("environment not ready — check the Activity Log")
	}

	if err := a.cap.StartCapture(exePath, name, timeout, onePerEP); err != nil {
		return journal.State{}, err
	}

	st, err := a.j.Start(name, activity)
	if err != nil {
		a.cap.StopCapture()
		return journal.State{}, err
	}

	return st, nil
}

type StopResult struct {
	State       journal.State `json:"state"`
	ExportDir   string        `json:"export_dir"`
	ExportError string        `json:"export_error"`
	SendDir     string        `json:"send_dir"`
	SizeWarning string        `json:"size_warning"`
}

func (a *App) StopSession() (StopResult, error) {
	a.cap.StopCapture()

	if _, err := a.j.Stop(); err != nil {
		return StopResult{}, err
	}
	res := StopResult{}
	if dir, err := a.j.Export(a.outputDir()); err != nil {
		res.ExportError = err.Error()
	} else {
		res.ExportDir = dir
	}
	res.State = a.j.State()

	if res.ExportDir != "" {
		sendDir := filepath.Join(res.ExportDir, "send")
		_ = os.MkdirAll(sendDir, 0o755)

		ledgerSrc := a.cap.LedgerPath(res.State.Name)
		if ledgerSrc != "" {
			dst := filepath.Join(sendDir, "ledger.bin")
			if err := copyFile(ledgerSrc, dst); err == nil {
				if fi, err := os.Stat(dst); err == nil && fi.Size() > 10*1024*1024 {
					res.SizeWarning = fmt.Sprintf("ledger.bin is %.1f MB — Discord free limit is 10 MB: record a shorter session",
						float64(fi.Size())/(1024*1024))
				}
			}
		}

		stepsSrc := filepath.Join(res.ExportDir, "steps.json")
		_ = copyFile(stepsSrc, filepath.Join(sendDir, "steps.json"))

		res.SendDir = sendDir
	}

	return res, nil
}

// ---- journal (still exposed for mark/edit/steps) ----------------------------

func (a *App) SuggestName(activity string) string {
	return journal.SuggestName(a.outputDir(), activity)
}

func (a *App) MarkButton() (journal.State, error) {
	at := time.Now()
	if _, err := a.j.Mark(at, journal.ViaButton, "", nil); err != nil {
		return journal.State{}, err
	}
	return a.j.State(), nil
}

func (a *App) MarkChecklist(text string, idx int) (journal.State, error) {
	at := time.Now()
	if _, err := a.j.Mark(at, journal.ViaChecklist, text, &idx); err != nil {
		return journal.State{}, err
	}
	return a.j.State(), nil
}

func (a *App) EditStep(id int, text string) (journal.State, error) { return a.j.EditStep(id, text) }
func (a *App) DeleteStep(id int) (journal.State, error)            { return a.j.DeleteStep(id) }
func (a *App) SetTargets(ids []int) journal.State                  { return a.j.SetTargets(ids) }
func (a *App) NewSession() journal.State                           { return a.j.Reset() }
func (a *App) RecoverDraft() (journal.State, error)                { return a.j.Recover() }
func (a *App) DiscardDraft()                                       { a.j.DiscardDraft() }

type ExportInfo struct {
	Dir   string        `json:"dir"`
	State journal.State `json:"state"`
}

func (a *App) Export() (ExportInfo, error) {
	dir, err := a.j.Export(a.outputDir())
	if err != nil {
		return ExportInfo{}, err
	}
	return ExportInfo{Dir: dir, State: a.j.State()}, nil
}

func (a *App) OpenFolder() error {
	dir := a.j.State().ExportDir
	if dir == "" {
		dir = a.outputDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return exec.Command("explorer.exe", dir).Start()
}

// ---- capture extras ---------------------------------------------------------

func (a *App) CaptureStatus() string {
	return string(a.cap.Status())
}

func (a *App) OpenCaptureDir(session string) error {
	return a.cap.OpenCaptureDir(session)
}

func (a *App) OpenSendDir() error {
	dir := a.j.State().ExportDir
	if dir == "" {
		return fmt.Errorf("no session exported yet")
	}
	sendDir := filepath.Join(dir, "send")
	if _, err := os.Stat(sendDir); err != nil {
		return fmt.Errorf("send folder not found — stop a session first")
	}
	return exec.Command("explorer.exe", sendDir).Start()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (a *App) BrowseExe() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select the game executable",
		Filters: []runtime.FileFilter{
			{DisplayName: "Executables", Pattern: "*.exe"},
			{DisplayName: "All Files", Pattern: "*.*"},
		},
	})
	if err != nil || path == "" {
		return "", err
	}
	a.mu.Lock()
	a.set.ExePath = path
	a.saveLocked()
	a.mu.Unlock()
	return path, nil
}

// ---- missions ---------------------------------------------------------------

type NeededResult struct {
	Online      bool              `json:"online"`
	Error       string            `json:"error"`
	GeneratedAt string            `json:"generated_at"`
	Activities  []needed.Activity `json:"activities"`
}

func (a *App) FetchNeeded() NeededResult {
	url := needed.URL
	if u := os.Getenv("AZOTH_JOURNAL_NEEDED_URL"); u != "" {
		url = u
	}
	c, err := needed.Fetch(a.ctx, nil, url, journal.AppName+"/"+journal.Version)
	if err != nil {
		return NeededResult{Error: err.Error(), Activities: []needed.Activity{}}
	}
	return NeededResult{Online: true, GeneratedAt: c.GeneratedAt, Activities: c.Activities}
}

// ---- settings ---------------------------------------------------------------

func (a *App) outputDir() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.set.OutputDir
}

func (a *App) saveLocked() error { return config.Save(a.set) }

func (a *App) PickOutputDir() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Where to save captures", DefaultDirectory: a.outputDir(), CanCreateDirectories: true,
	})
	if err != nil || dir == "" {
		return "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.set.OutputDir = dir
	return dir, a.saveLocked()
}

func (a *App) SetAlwaysOnTop(on bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.set.AlwaysOnTop = on
	runtime.WindowSetAlwaysOnTop(a.ctx, on)
	return a.saveLocked()
}

// ---- hotkey -----------------------------------------------------------------

type HotkeyStatus struct {
	Hotkey string `json:"hotkey"`
	Error  string `json:"error"`
}

func (a *App) hotkeyStatusLocked() HotkeyStatus {
	return HotkeyStatus{Hotkey: a.set.Hotkey, Error: a.hkErr}
}

func (a *App) unregisterLocked() {
	if a.hk != nil {
		a.hk.Close()
		a.hk = nil
	}
}

func (a *App) registerLocked() {
	a.unregisterLocked()
	a.hkErr = ""
	sp, err := hotkey.Parse(a.set.Hotkey)
	if err == nil {
		a.hk, err = hotkey.Register(sp, a.onHotkey)
	}
	if err != nil {
		a.hkErr = err.Error()
	}
}

func (a *App) onHotkey(at time.Time) {
	if _, err := a.j.Mark(at, journal.ViaHotkey, "", nil); err != nil {
		if errors.Is(err, journal.ErrNotRunning) {
			a.emit("notice", "Hotkey pressed, but no session is running.")
		}
		return
	}
	a.emit("marked", "hotkey")
}

func (a *App) BeginHotkeyCapture() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.unregisterLocked()
}

func (a *App) EndHotkeyCapture() HotkeyStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.registerLocked()
	return a.hotkeyStatusLocked()
}

func (a *App) SetHotkey(spec string) (HotkeyStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	sp, err := hotkey.Parse(spec)
	if err == nil {
		a.unregisterLocked()
		var l *hotkey.Listener
		if l, err = hotkey.Register(sp, a.onHotkey); err == nil {
			a.hk, a.hkErr = l, ""
			a.set.Hotkey = sp.String()
			return a.hotkeyStatusLocked(), a.saveLocked()
		}
	}
	a.registerLocked()
	return a.hotkeyStatusLocked(), err
}

// ---- env retry --------------------------------------------------------------

func (a *App) RetryInstall() {
	go func() {
		a.emit("caplog", "Retrying environment setup...")
		if err := a.cap.InstallEnv(); err != nil {
			a.emit("caplog", "Setup failed: "+err.Error())
		}
	}()
}
