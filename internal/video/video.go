// Package video records gameplay video next to a capture. It is opt-in: the
// player allows it in Settings, then starts and stops each clip with a button
// or a hotkey while a session runs. Each clip is a separate file, and
// video.json lists them with their times on the PC clock, the same clock as
// ledger.bin and steps.json, so a clip can be lined up with the packets and the
// marks. Recording is implemented on Linux (video_linux.go) through GPU Screen
// Recorder; on other platforms Probe reports it as unavailable.
//
// Video never goes into the "send" folder: it can show the player's name, chat
// and anything else on screen.
package video

import (
	"encoding/json"
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
}

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

// AddToIndex records clips in dir's video.json. Clips already listed there
// (for example from before the app was restarted and the session recovered)
// are kept; a clip listed again under the same file name is updated.
func AddToIndex(dir string, clips []Clip) error {
	path := filepath.Join(dir, IndexName)
	var idx index
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &idx); err != nil {
			return fmt.Errorf("%s is not readable, so it was left as it is: %w", path, err)
		}
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
