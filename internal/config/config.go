package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const DefaultHotkey = "F8"

// Video settings. Video is opt-in: VideoAllowed is false until the player turns
// it on.
const (
	DefaultVideoHotkey = "F9"
	VideoAudioGame     = "game" // record the game's own sound
	VideoAudioNone     = "none" // record no sound
)

type Settings struct {
	Hotkey      string `json:"hotkey"`
	AlwaysOnTop bool   `json:"always_on_top"`
	OutputDir   string `json:"output_dir"`
	ExePath     string `json:"exe_path"`
	Timeout     int    `json:"timeout"`
	OnePerEP    bool   `json:"one_per_endpoint"`

	VideoAllowed bool   `json:"video_allowed"`
	VideoTarget  string `json:"video_target"` // monitor name; empty means the first one
	VideoAudio   string `json:"video_audio"`  // VideoAudioGame or VideoAudioNone
	VideoHotkey  string `json:"video_hotkey"`
}

func DataDir() string {
	if d := os.Getenv("AZOTH_CAPTURE_DATA"); d != "" {
		return d
	}
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "AzothCapture")
	}
	d, _ := os.UserConfigDir()
	return filepath.Join(d, "AzothCapture")
}

func DraftPath() string { return filepath.Join(DataDir(), "draft.json") }

// DefaultOutputDir is where exported sessions go by default. The folder name is
// shared; the parent folder is chosen per platform (see paths_*.go).
const outputFolderName = "Azoth Captures"

func settingsPath() string { return filepath.Join(DataDir(), "settings.json") }

func Load() Settings {
	s := Settings{}
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, &s)
	}
	if s.Hotkey == "" {
		s.Hotkey = DefaultHotkey
	}
	if s.OutputDir == "" {
		s.OutputDir = DefaultOutputDir()
	}
	if s.Timeout == 0 {
		s.Timeout = 600
	}
	if s.VideoAudio != VideoAudioNone {
		s.VideoAudio = VideoAudioGame
	}
	if s.VideoHotkey == "" {
		s.VideoHotkey = DefaultVideoHotkey
	}
	return s
}

func Save(s Settings) error {
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, settingsPath())
}
