package video

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"azoth/capture/internal/proc"
)

// The Windows recorder: ffmpeg's gfxcapture source records the New World
// window. Clips have no sound: ffmpeg cannot record one program's sound on
// Windows, and recording the whole desktop would take in other programs, such
// as voice chat.

// The ffmpeg build the app downloads when none is installed: BtbN's Windows
// build of ffmpeg 9.0.2, which has gfxcapture. Its SHA-256 is the one GitHub
// publishes for the file.
const (
	ffmpegURL = "https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-10-25-13-00/" +
		"ffmpeg-n9.0.2-36-g6882969ce9-win64-gpl-9.0.zip"
	ffmpegSHA256  = "941603e8e99cf92309a3faaf156479438c41e738d007dbef3e484204614cb156"
	ffmpegZipPath = "/bin/ffmpeg.exe"
	downloadSize  = "about 190 MB"

	queryTimeout     = 30 * time.Second
	downloadTimeout  = 30 * time.Minute
	stopWait         = 15 * time.Second
	logEveryBytes    = 20 << 20               // a download progress line every 20 MB
	gfxcaptureMarker = "gfxcapture AVOptions" // in `ffmpeg -h filter=gfxcapture` when it has the source
)

// InstallHint tells the player how to get the recorder.
var InstallHint = "Video needs ffmpeg 8.1 or newer. Press Install to download it (" + downloadSize +
	"), or put your own ffmpeg on PATH."

// hasGfxcapture reports whether this ffmpeg can capture a window.
func hasGfxcapture(ffmpeg string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	out, _ := proc.Hidden(exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-h", "filter=gfxcapture")).CombinedOutput()
	return strings.Contains(string(out), gfxcaptureMarker)
}

// findFFmpeg returns an ffmpeg that can capture a window.
func findFFmpeg(toolsDir string) (string, error) {
	p, err := FFmpegPath(toolsDir)
	if err != nil {
		return "", errors.New(InstallHint)
	}
	if !hasGfxcapture(p) {
		return "", fmt.Errorf("the ffmpeg at %s is too old to record a window. %s", p, InstallHint)
	}
	return p, nil
}

// Probe reports whether video can be recorded. Windows always records the
// game window, found by its program name.
func Probe(toolsDir string) Info {
	if _, err := findFFmpeg(toolsDir); err != nil {
		return Info{Reason: err.Error(), CanInstall: true}
	}
	return Info{Available: true, WindowCapture: true}
}

// Install downloads ffmpeg into toolsDir and checks it.
func Install(toolsDir string, logf func(string)) error {
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return err
	}
	zipPath := filepath.Join(toolsDir, "ffmpeg-download.zip")
	defer os.Remove(zipPath)
	logf("Video: downloading ffmpeg (" + downloadSize + ")…")
	if err := download(ffmpegURL, zipPath, logf); err != nil {
		return fmt.Errorf("could not download ffmpeg: %w", err)
	}
	if err := verifySHA256(zipPath, ffmpegSHA256); err != nil {
		return err
	}
	dst := filepath.Join(toolsDir, ffmpegFile())
	if err := extractFromZip(zipPath, ffmpegZipPath, dst); err != nil {
		return err
	}
	if !hasGfxcapture(dst) {
		return errors.New("the downloaded ffmpeg cannot record a window")
	}
	logf("Video: ffmpeg is installed.")
	return nil
}

func download(url, path string, logf func(string)) error {
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s (the pinned build may have been removed; install ffmpeg 8.1+ yourself)", resp.Status)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := &progressWriter{w: f, logf: logf, total: resp.ContentLength}
	_, err = io.Copy(w, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

type progressWriter struct {
	w           io.Writer
	logf        func(string)
	total, done int64
	next        int64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.done >= p.next {
		p.next = p.done + logEveryBytes
		if p.total > 0 {
			p.logf(fmt.Sprintf("Video: ffmpeg download %d%%", p.done*100/p.total))
		}
	}
	return n, err
}

var (
	encoderMu sync.Mutex
	working   = map[string]encoder{} // chosen format -> the encoder that worked
)

// pickEncoder tests the encoders for a chosen format, in order, and keeps
// the first that works on this PC for the next clips.
func pickEncoder(ffmpeg, codec string) (encoder, error) {
	encoderMu.Lock()
	defer encoderMu.Unlock()
	if e, ok := working[codec]; ok {
		return e, nil
	}
	for _, e := range encodersFor(codec) {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		err := proc.Hidden(exec.CommandContext(ctx, ffmpeg, encoderTestArgs(e)...)).Run()
		cancel()
		if err == nil {
			working[codec] = e
			return e, nil
		}
	}
	return encoder{}, errors.New("ffmpeg has no video encoder that works on this PC")
}

// Recording is one clip being recorded.
type Recording struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	path    string
	clip    Clip
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	waitErr error
}

// Start begins a new clip of the game window in dir. Lines ffmpeg prints go
// to logf.
func Start(dir string, opt Options, logf func(string)) (*Recording, error) {
	ffmpeg, err := findFFmpeg(opt.ToolsDir)
	if err != nil {
		return nil, err
	}
	chosen := codecOr(opt.Codec)
	enc, err := pickEncoder(ffmpeg, chosen)
	if err != nil {
		return nil, err
	}
	if chosen != CodecAuto && enc.codec != chosen {
		logf("Video: this PC cannot record " + chosen + "; recording " + enc.codec + " instead.")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path, err := nextClipPath(dir)
	if err != nil {
		return nil, err
	}
	if opt.GameAudio {
		logf("Video: Windows clips have no sound.")
	}

	cmd := proc.Hidden(exec.Command(ffmpeg, recordArgs(gfxcaptureSource(opt.GameExe), enc, opt.Quality, path)...))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	started := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not start ffmpeg: %w", err)
	}
	logf("Video: encoding with " + enc.name + ".")

	r := &Recording{
		cmd: cmd, stdin: stdin, path: path, done: make(chan struct{}),
		clip: Clip{File: filepath.Base(path), Target: WindowTarget, Audio: noAudio, StartedUnixMs: started.UnixMilli()},
	}
	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		readProgress(stdout, func(outTimeUs int64) {
			// The first frame was captured outTimeUs of video before now.
			r.mu.Lock()
			r.clip.FirstFrameUnixMs = time.Now().UnixMilli() - outTimeUs/usPerMs
			r.mu.Unlock()
		}, func(line string) { logf("Video: " + line) })
	}()
	go func() {
		defer readers.Done()
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			logf("Video: " + sc.Text())
		}
	}()
	go func() {
		readers.Wait()
		err := cmd.Wait()
		r.mu.Lock()
		r.waitErr = err
		r.mu.Unlock()
		close(r.done)
	}()
	return r, nil
}

// Done is closed when ffmpeg has exited, through Stop or on its own (an
// error, or the game window closed).
func (r *Recording) Done() <-chan struct{} { return r.done }

// Err is ffmpeg's exit result once Done is closed.
func (r *Recording) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.waitErr
}

// Stop asks ffmpeg to finish the clip ("q" on its input), waits for it and
// returns the clip's description. It is safe to call after ffmpeg exited.
func (r *Recording) Stop() (Clip, error) {
	r.once.Do(func() {
		_, _ = io.WriteString(r.stdin, "q")
		_ = r.stdin.Close()
		select {
		case <-r.done:
		case <-time.After(stopWait):
			_ = r.cmd.Process.Kill()
			<-r.done
		}
		r.mu.Lock()
		r.clip.StoppedUnixMs = time.Now().UnixMilli()
		r.mu.Unlock()
	})
	r.mu.Lock()
	clip := r.clip
	r.mu.Unlock()
	if fi, err := os.Stat(r.path); err != nil || fi.Size() == 0 {
		return clip, fmt.Errorf("ffmpeg saved no video (%v); is New World running? See the Activity Log", r.Err())
	}
	return clip, nil
}

// PickWindow is not needed on Windows: the recorder finds the game window
// by its program name.
func PickWindow(string, func(string)) error {
	return errors.New("Windows finds the game window by itself; there is nothing to pick")
}
