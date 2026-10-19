package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"azoth/capture/internal/capture"
	"azoth/capture/internal/config"
	"azoth/capture/internal/hotkey"
	"azoth/capture/internal/journal"
	"azoth/capture/internal/video"
)

// ---- video (opt-in; implemented on Linux) -----------------------------------
//
// Video is off until the player allows it in Settings. Then, while a session
// runs, the Video button or the video hotkey starts and stops a clip. Clips go
// into the session folder next to steps.json, never into "send", and video.json
// lists them with their times on the PC clock.

var (
	errVideoNotAllowed = errors.New("turn on video in Settings first")
	errVideoNoSession  = errors.New("start a session first; video belongs to a session")
	errVideoNoMonitor  = errors.New("choose the monitor the game is on in Settings first")
	errVideoNoWindow   = errors.New("this desktop cannot pick a single window; choose a monitor instead")
	errVideoNoCapture  = errors.New("the capture is not running; video only records alongside a running capture")
)

// portalTokenName is the file that remembers which window the player picked.
const portalTokenName = "video-portal-token"

func portalTokenPath() string { return filepath.Join(config.DataDir(), portalTokenName) }

// videoState is the video part of App, behind its own lock: stopping a clip
// waits for the recorder to finish writing, and that must not block the rest
// of the app.
type videoState struct {
	mu    sync.Mutex
	info  video.Info
	rec   *video.Recording
	clips []video.Clip
	err   string // the last failure, shown in the window

	hk    *hotkey.Listener // guarded by App.mu, like the mark hotkey
	hkErr string
}

// VideoStatus is what the window needs to draw the video controls.
type VideoStatus struct {
	Info         video.Info `json:"info"`
	Allowed      bool       `json:"allowed"`
	Recording    bool       `json:"recording"`
	Target       string     `json:"target"` // video.WindowTarget, a monitor name, or "" (not chosen)
	WindowPicked bool       `json:"window_picked"`
	Audio        string     `json:"audio"`
	Hotkey       string     `json:"hotkey"`
	HotkeyError  string     `json:"hotkey_error"`
	Clips        int        `json:"clips"`
	Error        string     `json:"error"`
}

// probeVideo finds out whether the recorder is installed and which monitors it
// sees.
func (a *App) probeVideo() {
	info := video.Probe()
	a.vid.mu.Lock()
	a.vid.info = info
	a.vid.mu.Unlock()
}

// VideoState returns the current video status.
func (a *App) VideoState() VideoStatus {
	a.mu.Lock()
	set, hkErr := a.set, a.vid.hkErr
	a.mu.Unlock()
	_, tokenErr := os.Stat(portalTokenPath())
	a.vid.mu.Lock()
	defer a.vid.mu.Unlock()
	return VideoStatus{
		Info: a.vid.info, Allowed: set.VideoAllowed, Recording: a.vid.rec != nil,
		Target: videoTarget(set, a.vid.info), WindowPicked: tokenErr == nil, Audio: set.VideoAudio,
		Hotkey: set.VideoHotkey, HotkeyError: hkErr,
		Clips: len(a.vid.clips), Error: a.vid.err,
	}
}

func (a *App) emitVideo() { a.emit("video", a.VideoState()) }

// RefreshVideo checks again for the recorder, for example after the player
// installed it.
func (a *App) RefreshVideo() VideoStatus {
	a.probeVideo()
	a.mu.Lock()
	a.registerVideoLocked()
	a.mu.Unlock()
	return a.VideoState()
}

// videoTarget is what clips record: the game window when the desktop lets the
// player pick one, otherwise the monitor the player chose. It never guesses a
// monitor: an unchosen or vanished monitor gives "".
func videoTarget(set config.Settings, info video.Info) string {
	if info.WindowCapture {
		return video.WindowTarget
	}
	if info.HasMonitor(set.VideoTarget) {
		return set.VideoTarget
	}
	return ""
}

// SetVideoAllowed turns video on or off. Turning it off stops a running clip.
func (a *App) SetVideoAllowed(on bool) (VideoStatus, error) {
	if !on {
		a.stopClip()
	}
	a.mu.Lock()
	a.set.VideoAllowed = on
	err := a.saveLocked()
	a.registerVideoLocked()
	a.mu.Unlock()
	a.emitVideo()
	return a.VideoState(), err
}

// SetVideoTarget chooses the monitor to record.
func (a *App) SetVideoTarget(monitor string) (VideoStatus, error) {
	a.vid.mu.Lock()
	known := a.vid.info.HasMonitor(monitor)
	a.vid.mu.Unlock()
	if !known {
		return a.VideoState(), fmt.Errorf("unknown monitor %q", monitor)
	}
	a.mu.Lock()
	a.set.VideoTarget = monitor
	err := a.saveLocked()
	a.mu.Unlock()
	return a.VideoState(), err
}

// SetVideoAudio chooses between the game's own sound and no sound.
func (a *App) SetVideoAudio(mode string) (VideoStatus, error) {
	if mode != config.VideoAudioGame && mode != config.VideoAudioNone {
		return a.VideoState(), fmt.Errorf("unknown sound option %q", mode)
	}
	a.mu.Lock()
	a.set.VideoAudio = mode
	err := a.saveLocked()
	a.mu.Unlock()
	return a.VideoState(), err
}

// PickVideoWindow opens the desktop's dialog so the player can choose the game
// window once; later clips record that window without asking.
func (a *App) PickVideoWindow() (VideoStatus, error) {
	a.vid.mu.Lock()
	window := a.vid.info.WindowCapture
	a.vid.mu.Unlock()
	if !window {
		return a.VideoState(), errVideoNoWindow
	}
	err := video.PickWindow(portalTokenPath(), func(line string) { a.emit("caplog", line) })
	if err == nil {
		a.emit("caplog", "Video: game window picked. Clips will record that window.")
	}
	a.emitVideo()
	return a.VideoState(), err
}

// ToggleVideo starts a clip, or stops the running one.
func (a *App) ToggleVideo() (VideoStatus, error) {
	a.vid.mu.Lock()
	recording := a.vid.rec != nil
	a.vid.mu.Unlock()
	if recording {
		a.stopClip()
		return a.VideoState(), nil
	}
	err := a.startClip()
	return a.VideoState(), err
}

func (a *App) startClip() error {
	err := a.startClipLocked()
	a.emitVideo() // after the lock is released: emitVideo reads the state again
	return err
}

func (a *App) startClipLocked() error {
	a.mu.Lock()
	set := a.set
	a.mu.Unlock()
	if !set.VideoAllowed {
		return errVideoNotAllowed
	}
	st := a.j.State()
	if st.Phase != journal.PhaseRunning {
		return errVideoNoSession
	}

	a.vid.mu.Lock()
	defer a.vid.mu.Unlock()
	if a.vid.rec != nil {
		return nil
	}
	if !a.vid.info.Available {
		return errors.New(a.vid.info.Reason)
	}
	target := videoTarget(set, a.vid.info)
	if target == "" {
		return errVideoNoMonitor
	}
	if a.cap == nil || a.cap.Status() != capture.StatusRunning {
		return errVideoNoCapture
	}
	opt := video.Options{
		Window:      target == video.WindowTarget,
		PortalToken: portalTokenPath(),
		GameAudio:   set.VideoAudio == config.VideoAudioGame,
	}
	if !opt.Window {
		opt.Monitor = target
	}
	rec, err := video.Start(journal.SessionDir(set.OutputDir, st.Name), opt,
		func(line string) { a.emit("caplog", line) })
	if err != nil {
		a.vid.err = err.Error()
		return err
	}
	a.vid.rec, a.vid.err = rec, ""
	what := "monitor " + target
	if opt.Window {
		what = "game window"
	}
	a.emit("caplog", "Video: recording the "+what+".")
	go a.watchClip(rec)
	return nil
}

// watchClip notices a recorder that stops on its own (an error, or the monitor
// went away) and records what it saved.
func (a *App) watchClip(rec *video.Recording) {
	<-rec.Done()
	a.vid.mu.Lock()
	current := a.vid.rec == rec
	a.vid.mu.Unlock()
	if !current {
		return // stopped through stopClip, which already handled it
	}
	a.stopClip()
	a.vid.mu.Lock()
	if a.vid.err == "" {
		a.vid.err = fmt.Sprintf("the recorder stopped on its own (%v)", rec.Err())
	}
	a.vid.mu.Unlock()
	a.emit("caplog", "Video: the recorder stopped on its own; see the lines above.")
	a.emitVideo()
}

// stopClip stops the running clip, if any, and keeps its description.
func (a *App) stopClip() {
	a.vid.mu.Lock()
	rec := a.vid.rec
	a.vid.rec = nil
	a.vid.mu.Unlock()
	if rec == nil {
		return
	}
	clip, err := rec.Stop()
	a.vid.mu.Lock()
	if err != nil {
		a.vid.err = err.Error()
	} else {
		a.vid.clips = append(a.vid.clips, clip)
	}
	a.vid.mu.Unlock()
	if err == nil {
		a.emit("caplog", fmt.Sprintf("Video: saved %s.", clip.File))
	}
	a.emitVideo()
}

// onCaptureEnded stops a running clip whenever the capture ends, for any
// reason: Stop Session, the timeout, the game closing, or an error. Video never
// keeps recording without a capture.
func (a *App) onCaptureEnded() {
	a.vid.mu.Lock()
	running := a.vid.rec != nil
	a.vid.mu.Unlock()
	if !running {
		return
	}
	a.emit("caplog", "Video: the capture ended, so the running clip was stopped.")
	a.stopClip()
}

// resetVideo forgets the clips of the previous session.
func (a *App) resetVideo() {
	a.vid.mu.Lock()
	a.vid.clips, a.vid.err = nil, ""
	a.vid.mu.Unlock()
	a.emitVideo()
}

// finishVideo stops a running clip and records the clips in the session
// folder's video.json. It returns how many clips the session has.
func (a *App) finishVideo(sessionDir string) (int, error) {
	a.stopClip()
	a.vid.mu.Lock()
	clips := slices.Clone(a.vid.clips)
	a.vid.mu.Unlock()
	if len(clips) == 0 {
		return 0, nil
	}
	return len(clips), video.AddToIndex(sessionDir, clips)
}

// ---- video hotkey -------------------------------------------------------------

func (a *App) unregisterVideoLocked() {
	if a.vid.hk != nil {
		a.vid.hk.Close()
		a.vid.hk = nil
	}
}

// registerVideoLocked listens for the video hotkey while video is allowed and
// the recorder is available. Call with a.mu held.
func (a *App) registerVideoLocked() {
	a.unregisterVideoLocked()
	a.vid.hkErr = ""
	a.vid.mu.Lock()
	available := a.vid.info.Available
	a.vid.mu.Unlock()
	if !a.set.VideoAllowed || !available {
		return
	}
	sp, err := hotkey.Parse(a.set.VideoHotkey)
	if err == nil {
		a.vid.hk, err = hotkey.Register(sp, a.onVideoHotkey)
	}
	if err != nil {
		a.vid.hkErr = err.Error()
	}
}

// onVideoHotkey runs on the hotkey listener, which must return quickly, so
// the toggle (which may wait for the recorder) runs on its own goroutine.
func (a *App) onVideoHotkey(time.Time) {
	go func() {
		if _, err := a.ToggleVideo(); err != nil {
			a.emit("notice", "Video: "+err.Error())
		}
	}()
}

// SetVideoHotkey changes the video hotkey. It must differ from the mark hotkey.
func (a *App) SetVideoHotkey(spec string) (VideoStatus, error) {
	sp, err := hotkey.Parse(spec)
	if err != nil {
		return a.VideoState(), err
	}
	a.mu.Lock()
	if mark, perr := hotkey.Parse(a.set.Hotkey); perr == nil && mark == sp {
		a.registerVideoLocked()
		a.registerLocked()
		a.mu.Unlock()
		return a.VideoState(), fmt.Errorf("%s is already the mark hotkey", sp)
	}
	a.set.VideoHotkey = sp.String()
	err = a.saveLocked()
	a.registerVideoLocked()
	a.registerLocked() // BeginHotkeyCapture stopped the mark hotkey too
	a.mu.Unlock()
	return a.VideoState(), err
}
