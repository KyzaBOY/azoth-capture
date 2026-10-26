// Package video records gameplay video next to a capture. It is opt-in: the
// player allows it in Settings, then starts and stops each clip with a button
// or a hotkey while a session runs. Each clip is a separate file, and
// video.json lists them with their times on the PC clock, the same clock as
// ledger.bin and steps.json, so a clip can be lined up with the packets and the
// marks. Linux records through GPU Screen Recorder (video_linux.go), Windows
// through ffmpeg's Windows Graphics Capture source (video_windows.go).
//
// Video never goes into the "send" folder: it can show the player's name, chat
// and anything else on screen.
package video

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// IndexName is the file that lists a session's clips.
const IndexName = "video.json"

// WindowTarget is Clip.Target for a clip of the window the player picked.
const WindowTarget = "window"

// Monitor is one screen the recorder can capture.
type Monitor struct {
	Name string `json:"name"` // e.g. DP-1
	Size string `json:"size"` // e.g. 1920x1080
}

// Info says whether video can be recorded here, and how.
type Info struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"` // why not, when Available is false
	// WindowCapture is true when the desktop lets the player pick one window to
	// record (the xdg-desktop-portal screen-cast dialog, on Wayland). Then
	// clips record only the game window. Otherwise the player must choose a
	// monitor.
	WindowCapture bool      `json:"window_capture"`
	Monitors      []Monitor `json:"monitors"`
	// NeedsPick is true when the player must pick the game window once (the
	// Linux desktop dialog). On Windows the recorder finds the game window by
	// its program name.
	NeedsPick bool `json:"needs_pick"`
	// Sound is true when clips can record the game's own sound.
	Sound bool `json:"sound"`
	// CanInstall is true when Install can download the missing recorder.
	CanInstall bool `json:"can_install"`
}

// Video formats. CodecAuto picks the most compact one the GPU can encode;
// a format the GPU cannot encode falls back to the next one.
const (
	CodecAuto = "auto"
	CodecAV1  = "av1"
	CodecHEVC = "hevc"
	CodecH264 = "h264"
)

// Codecs are the formats the player can choose, most compact first.
var Codecs = []string{CodecAuto, CodecAV1, CodecHEVC, CodecH264}

// Quality levels, named as GPU Screen Recorder names them.
const (
	QualityMedium   = "medium"
	QualityHigh     = "high"
	QualityVeryHigh = "very_high"
)

// Qualities are the levels the player can choose, lowest first.
var Qualities = []string{QualityMedium, QualityHigh, QualityVeryHigh}

// HasMonitor reports whether name is one of the monitors.
func (i Info) HasMonitor(name string) bool {
	return slices.ContainsFunc(i.Monitors, func(m Monitor) bool { return m.Name == name })
}

// Options configure one clip.
type Options struct {
	// Window records the window the player picked through the desktop's
	// dialog; PortalToken is the file that remembers that choice. Otherwise
	// Monitor names the screen to record.
	Window      bool
	PortalToken string
	Monitor     string
	// GameAudio records the game's own playback sound. A microphone is never
	// recorded; see gameAudioName in the Linux implementation.
	GameAudio bool
	// GameExe is the game file picked in Settings. Its name finds the game's
	// window (Windows) or sound (Linux) when the player renamed the file.
	GameExe string
	// ToolsDir holds the tools the app downloaded (ffmpeg on Windows).
	ToolsDir string
	// Codec is one of Codecs and Quality one of Qualities.
	Codec   string
	Quality string
}

// qualityOr returns q when it is one of Qualities, else QualityHigh.
func qualityOr(q string) string {
	if slices.Contains(Qualities, q) {
		return q
	}
	return QualityHigh
}

// codecOr returns c when it is one of Codecs, else CodecAuto.
func codecOr(c string) string {
	if slices.Contains(Codecs, c) {
		return c
	}
	return CodecAuto
}

// codecOrder is the list of formats to try for a chosen one: the chosen
// format first, then the more widely supported ones after it. CodecAuto
// tries them all, most compact first.
func codecOrder(chosen string) []string {
	all := Codecs[1:]
	for i, c := range all {
		if c == chosen {
			return all[i:]
		}
	}
	return all
}

// Clip describes one recorded file. Times are Unix milliseconds on the PC
// clock.
type Clip struct {
	File             string `json:"file"`
	Target           string `json:"target"`
	Audio            string `json:"audio"` // the recorder's audio source, or "none"
	StartedUnixMs    int64  `json:"started_unix_ms"`
	FirstFrameUnixMs int64  `json:"first_frame_unix_ms,omitempty"` // 0 when the recorder could not report it
	StoppedUnixMs    int64  `json:"stopped_unix_ms"`
}

// index is the content of video.json.
type index struct {
	Note  string `json:"note"`
	Clips []Clip `json:"clips"`
}

const indexNote = "Times are Unix milliseconds on the PC clock, the same clock as ledger.bin " +
	"and steps.json. first_frame_unix_ms is when the first video frame was captured; " +
	"use it to line a clip up with the packets and the marks."

// ReadIndex returns the clips listed in dir's video.json. A session without
// video has no video.json and no clips.
func ReadIndex(dir string) ([]Clip, error) {
	idx, err := readIndex(filepath.Join(dir, IndexName))
	return idx.Clips, err
}

func readIndex(path string) (index, error) {
	var idx index
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return idx, nil
	}
	if err != nil {
		return idx, err
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		return idx, fmt.Errorf("%s is not readable: %w", path, err)
	}
	return idx, nil
}

// AddToIndex records clips in dir's video.json. Clips already listed there
// (for example from before the app was restarted and the session recovered)
// are kept; a clip listed again under the same file name is updated.
func AddToIndex(dir string, clips []Clip) error {
	path := filepath.Join(dir, IndexName)
	idx, err := readIndex(path)
	if err != nil {
		return fmt.Errorf("%w, so it was left as it is", err)
	}
	for _, c := range clips {
		i := slices.IndexFunc(idx.Clips, func(old Clip) bool { return old.File == c.File })
		if i >= 0 {
			idx.Clips[i] = c
		} else {
			idx.Clips = append(idx.Clips, c)
		}
	}
	idx.Note = indexNote
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
