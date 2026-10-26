package config

import (
	"os"
	"runtime"
	"testing"
)

// AI marks are opt-in and default to online game data; settings.json may
// hold an API key, so only the player's account may read it.
func TestAIOffByDefaultAndSettingsPrivate(t *testing.T) {
	t.Setenv("AZOTH_CAPTURE_DATA", t.TempDir())
	s := Load()
	if s.AI.Enabled {
		t.Error("AI marks must be off until the player turns them on")
	}
	if s.AI.SampleEvery != DefaultAISampleEvery || s.AI.DataSource != AIDataOnline || s.AI.BaseURL != DefaultAIBaseURL {
		t.Errorf("AI defaults = %+v", s.AI)
	}
	s.AI.APIKey = "secret"
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 { // Windows has no Unix modes
		t.Errorf("settings.json mode = %o, want 600", perm)
	}
}

// Video is opt-in: a fresh install must not record video, and its other
// settings get safe defaults (the game's sound only, F9).
func TestVideoIsOffByDefault(t *testing.T) {
	t.Setenv("AZOTH_CAPTURE_DATA", t.TempDir())
	s := Load()
	if s.VideoAllowed {
		t.Error("video must be off until the player turns it on")
	}
	if s.VideoAudio != VideoAudioGame || s.VideoHotkey != DefaultVideoHotkey {
		t.Errorf("defaults = audio %q, hotkey %q", s.VideoAudio, s.VideoHotkey)
	}
}

// Any saved sound value other than "none" falls back to the game's sound only,
// so a stray value can never widen what is recorded.
func TestVideoAudioOnlyGameOrNone(t *testing.T) {
	t.Setenv("AZOTH_CAPTURE_DATA", t.TempDir())
	for saved, want := range map[string]string{VideoAudioNone: VideoAudioNone, "desktop": VideoAudioGame, "": VideoAudioGame} {
		s := Load()
		s.VideoAudio = saved
		if err := Save(s); err != nil {
			t.Fatal(err)
		}
		if got := Load().VideoAudio; got != want {
			t.Errorf("saved %q, loaded %q, want %q", saved, got, want)
		}
	}
}
