package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"azoth/capture/internal/video"
)

const DefaultHotkey = "F8"

// Video settings. Video is opt-in: VideoAllowed is false until the player turns
// it on.
const (
	DefaultVideoHotkey = "F9"
	VideoAudioGame     = "game" // record the game's own sound
	VideoAudioNone     = "none" // record no sound
	// The video format and quality: the most compact format the GPU has, at
	// the quality clips always had.
	DefaultVideoCodec   = video.CodecAuto
	DefaultVideoQuality = video.QualityHigh
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
	VideoCodec   string `json:"video_codec"`   // one of video.Codecs; "auto" picks the most compact the GPU has
	VideoQuality string `json:"video_quality"` // one of video.Qualities

	AI AISettings `json:"ai"`
}

// AISettings configure AI marks. They are off until the player turns them on
// and picks a model. The API key is kept in settings.json, which only the
// player's account can read.
type AISettings struct {
	Enabled     bool   `json:"enabled"`
	Connector   string `json:"connector"`
	BaseURL     string `json:"base_url"`
	APIKey      string `json:"api_key"`
	CLI         string `json:"cli"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	DataSource  string `json:"data_source"` // "online" or "local"
	DataDir     string `json:"data_dir"`    // the local game data folder
	SampleEvery int    `json:"sample_every"`
}

// Defaults for AI marks.
const (
	DefaultAISampleEvery = 2 // seconds between frames
	DefaultAIBaseURL     = "http://127.0.0.1:11434/v1"
	AIDataOnline         = "online"
	AIDataLocal          = "local"
)

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
	if s.VideoCodec == "" {
		s.VideoCodec = DefaultVideoCodec
	}
	if s.VideoQuality == "" {
		s.VideoQuality = DefaultVideoQuality
	}
	if s.AI.SampleEvery == 0 {
		s.AI.SampleEvery = DefaultAISampleEvery
	}
	if s.AI.BaseURL == "" {
		s.AI.BaseURL = DefaultAIBaseURL
	}
	if s.AI.DataSource != AIDataLocal {
		s.AI.DataSource = AIDataOnline
	}
	return s
}

func Save(s Settings) error {
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil { // may hold an API key
		return err
	}
	return os.Rename(tmp, settingsPath())
}
