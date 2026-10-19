package video

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	recorderName = "gpu-screen-recorder"
	flatpakID    = "com.dec05eba.gpu_screen_recorder"
	// InstallHint tells the player how to get the recorder.
	InstallHint = "Video needs GPU Screen Recorder. Install it from your distribution " +
		"(for example: sudo apt install gpu-screen-recorder) or from Flathub (" + flatpakID + ")."

	queryTimeout = 10 * time.Second
	// stopWait is how long a clip gets to finish writing after Stop.
	stopWait = 15 * time.Second
	// pickTimeout bounds how long PickWindow waits for the player to choose a
	// window in the desktop's dialog.
	pickTimeout = 3 * time.Minute
	pickPoll    = 200 * time.Millisecond

	// Clip settings: 60 fps, scaled to fit 1920x1080 (4K and ultrawide
	// monitors would otherwise make very large files), the recorder's "high"
	// constant-quality preset, Matroska so a clip stays playable if the PC
	// stops mid-recording.
	clipFPS       = "60"
	clipMaxSize   = "1920x1080"
	clipQuality   = "high"
	clipContainer = "mkv"
	noAudio       = "none"
	portalTarget  = "portal" // the recorder's -w value for the desktop's window picker
	// gameNameMatch is matched against program names in the recorder's
	// application-audio list, lower case with spaces removed ("New World").
	gameNameMatch = "newworld"
)

func isGameName(s string) bool {
	return strings.Contains(strings.ToLower(strings.ReplaceAll(s, " ", "")), gameNameMatch)
}

// recorder is how to run the GPU Screen Recorder command line: installed
// natively, or inside its Flatpak.
type recorder struct{ argv []string }

func findRecorder() (recorder, bool) {
	if p, err := exec.LookPath(recorderName); err == nil {
		return recorder{argv: []string{p}}, true
	}
	if _, err := exec.LookPath("flatpak"); err == nil && exec.Command("flatpak", "info", flatpakID).Run() == nil {
		return recorder{argv: []string{"flatpak", "run", "--command=" + recorderName, flatpakID}}, true
	}
	return recorder{}, false
}

func (r recorder) command(ctx context.Context, args ...string) *exec.Cmd {
	full := append(append([]string{}, r.argv[1:]...), args...)
	return exec.CommandContext(ctx, r.argv[0], full...)
}

// query runs a short, read-only recorder command and returns its output.
func (r recorder) query(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	out, err := r.command(ctx, args...).CombinedOutput()
	return string(out), err
}

// Probe reports whether video can be recorded, whether the player can pick
// one window, and which monitors exist.
func Probe() Info {
	rec, ok := findRecorder()
	if !ok {
		return Info{Reason: InstallHint}
	}
	out, err := rec.query("--list-monitors")
	if err != nil {
		return Info{Reason: fmt.Sprintf("GPU Screen Recorder is installed but could not list monitors: %v", err)}
	}
	var monitors []Monitor
	for _, line := range strings.Split(out, "\n") {
		name, size, _ := strings.Cut(strings.TrimSpace(line), "|")
		if name != "" {
			monitors = append(monitors, Monitor{Name: name, Size: size})
		}
	}
	options, _ := rec.query("--list-capture-options")
	window := false
	for _, line := range strings.Split(options, "\n") {
		if strings.TrimSpace(line) == portalTarget {
			window = true
		}
	}
	if len(monitors) == 0 && !window {
		return Info{Reason: "GPU Screen Recorder found nothing to record."}
	}
	return Info{Available: true, WindowCapture: window, Monitors: monitors}
}

// gameAudioName returns the name of New World in the recorder's list of
// programs playing sound, or "" when the game is not playing sound.
//
// Sound is recorded only through the recorder's "app:<name>" source, which
// links a program's PLAYBACK streams (PipeWire Stream/Output/Audio) and never
// a microphone: not the microphone device, and not any program's recording
// stream. The app never passes a device source (default_input,
// default_output, device:...). The game's playback does include what the game
// plays, other players' voice chat among it; "No sound" leaves all of it out.
func gameAudioName(rec recorder) string {
	out, err := rec.query("--list-application-audio")
	if err != nil {
		return ""
	}
	for _, name := range firstField(out) {
		if isGameName(name) {
			return name
		}
	}
	return ""
}

// targetArgs returns the recorder arguments that choose what to record.
func targetArgs(opt Options) ([]string, string, error) {
	if opt.Window {
		if _, err := os.Stat(opt.PortalToken); err != nil {
			return nil, "", errors.New("pick the game window in Settings first (Video → Pick window)")
		}
		return []string{"-w", portalTarget, "-restore-portal-session", "yes",
			"-portal-session-token-filepath", opt.PortalToken}, WindowTarget, nil
	}
	if opt.Monitor == "" {
		return nil, "", errors.New("choose the monitor the game is on in Settings first")
	}
	return []string{"-w", opt.Monitor}, opt.Monitor, nil
}

// Recording is one clip being recorded.
type Recording struct {
	cmd     *exec.Cmd
	path    string
	clip    Clip
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	waitErr error
}

// Start begins a new clip in dir. Lines the recorder prints go to logf.
func Start(dir string, opt Options, logf func(string)) (*Recording, error) {
	rec, ok := findRecorder()
	if !ok {
		return nil, errors.New(InstallHint)
	}
	target, targetName, err := targetArgs(opt)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path, err := nextClipPath(dir)
	if err != nil {
		return nil, err
	}

	help, _ := rec.query("--help")
	args := append(target,
		"-c", clipContainer,
		"-f", clipFPS,
		"-s", clipMaxSize,
		"-q", clipQuality,
		"-v", "no",
	)
	// Both options exist in current versions; older ones simply go without.
	if strings.Contains(help, "-exclude-metadata") {
		args = append(args, "-exclude-metadata", "yes") // keep device names out of the file
	}
	if strings.Contains(help, "-write-first-frame-ts") {
		args = append(args, "-write-first-frame-ts", "yes")
	}
	audio := noAudio
	if opt.GameAudio {
		if name := gameAudioName(rec); name != "" {
			audio = "app:" + name
			args = append(args, "-a", audio)
		} else {
			logf("Video: the game is not playing sound right now, so this clip has no sound.")
		}
	}
	args = append(args, "-o", path)

	cmd := rec.command(context.Background(), args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	started := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not start GPU Screen Recorder: %w", err)
	}

	r := &Recording{
		cmd:  cmd,
		path: path,
		done: make(chan struct{}),
		clip: Clip{File: filepath.Base(path), Target: targetName, Audio: audio, StartedUnixMs: started.UnixMilli()},
	}
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			logf("Video: " + scanner.Text())
		}
		err := cmd.Wait()
		r.mu.Lock()
		r.waitErr = err
		r.mu.Unlock()
		close(r.done)
	}()
	return r, nil
}

// Done is closed when the recorder process has exited, whether through Stop
// or on its own (an error, or the window closed).
func (r *Recording) Done() <-chan struct{} { return r.done }

// Err is the recorder's exit result once Done is closed.
func (r *Recording) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.waitErr
}

// Stop asks the recorder to finish and save the clip, waits for it, and
// returns the clip's description. It is safe to call after the recorder has
// already exited.
func (r *Recording) Stop() (Clip, error) {
	r.once.Do(func() {
		_ = r.cmd.Process.Signal(os.Interrupt) // the recorder saves and exits on SIGINT
		select {
		case <-r.done:
		case <-time.After(stopWait):
			_ = r.cmd.Process.Kill()
			<-r.done
		}
		r.clip.StoppedUnixMs = time.Now().UnixMilli()
		if data, err := os.ReadFile(r.path + timestampExt); err == nil {
			if ms, err := parseFirstFrame(string(data)); err == nil {
				r.clip.FirstFrameUnixMs = ms
			}
		}
	})
	if fi, err := os.Stat(r.path); err != nil || fi.Size() == 0 {
		return r.clip, fmt.Errorf("the recorder saved no video (%v); see the Activity Log", r.Err())
	}
	return r.clip, nil
}

// PickWindow opens the desktop's "choose what to share" dialog so the player
// can pick the game window once. The choice is remembered in tokenPath, so
// later clips record that window without asking again. It forgets any earlier
// choice first, so it also serves to pick a different window.
func PickWindow(tokenPath string, logf func(string)) error {
	rec, ok := findRecorder()
	if !ok {
		return errors.New(InstallHint)
	}
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o700); err != nil {
		return err
	}
	if err := os.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.MkdirTemp("", "azoth-pick-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "pick.mkv")

	cmd := rec.command(context.Background(),
		"-w", portalTarget, "-restore-portal-session", "yes", "-portal-session-token-filepath", tokenPath,
		"-c", clipContainer, "-v", "no", "-write-first-frame-ts", "yes", "-o", out)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start GPU Screen Recorder: %w", err)
	}
	exited := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			logf("Video: " + scanner.Text())
		}
		_ = cmd.Wait()
		close(exited)
	}()

	// Recording starts once the player has chosen; that is when the
	// first-frame file appears. Then the throwaway recording is stopped.
	deadline := time.After(pickTimeout)
wait:
	for {
		select {
		case <-exited:
			break wait
		case <-deadline:
			break wait
		case <-time.After(pickPoll):
			if _, err := os.Stat(out + timestampExt); err == nil {
				break wait
			}
		}
	}
	_ = cmd.Process.Signal(os.Interrupt)
	select {
	case <-exited:
	case <-time.After(stopWait):
		_ = cmd.Process.Kill()
		<-exited
	}
	if _, err := os.Stat(tokenPath); err != nil {
		return errors.New("no window was picked; press Pick window and choose the New World window")
	}
	return nil
}
