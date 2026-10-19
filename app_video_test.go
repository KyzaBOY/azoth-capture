package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"azoth/capture/internal/capture"
	"azoth/capture/internal/config"
	"azoth/capture/internal/video"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AZOTH_CAPTURE_DATA", t.TempDir())
	set := config.Load()
	set.OutputDir = t.TempDir()
	a := NewApp(set)
	a.vid.info = video.Info{Available: true, Monitors: []video.Monitor{{Name: "DP-1"}, {Name: "DP-2"}}}
	return a
}

func TestToggleVideoNeedsOptInAndSession(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.ToggleVideo(); !errors.Is(err, errVideoNotAllowed) {
		t.Fatalf("video started without being allowed: %v", err)
	}
	if _, err := a.SetVideoAllowed(true); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ToggleVideo(); !errors.Is(err, errVideoNoSession) {
		t.Fatalf("video started without a session: %v", err)
	}
	if st := a.VideoState(); st.Recording || !st.Allowed {
		t.Errorf("state = %+v", st)
	}
}

// The app never guesses a monitor: an unchosen or vanished one means "not
// chosen", and the game window wins when the desktop can pick one.
func TestVideoTargetNeverGuesses(t *testing.T) {
	info := video.Info{Monitors: []video.Monitor{{Name: "DP-1"}, {Name: "DP-2"}}}
	for saved, want := range map[string]string{"DP-2": "DP-2", "": "", "HDMI-gone": ""} {
		if got := videoTarget(config.Settings{VideoTarget: saved}, info); got != want {
			t.Errorf("saved %q: target %q, want %q", saved, got, want)
		}
	}
	info.WindowCapture = true
	if got := videoTarget(config.Settings{VideoTarget: "DP-2"}, info); got != video.WindowTarget {
		t.Errorf("with window capture: target %q, want the game window", got)
	}
}

// With a session running and video allowed, a clip still must not start
// until the player has chosen what to record.
func TestStartClipNeedsAChosenMonitor(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.SetVideoAllowed(true); err != nil {
		t.Fatal(err)
	}
	if _, err := a.j.Start("s", "Other"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ToggleVideo(); !errors.Is(err, errVideoNoMonitor) {
		t.Errorf("ToggleVideo = %v, want %v", err, errVideoNoMonitor)
	}
	if a.VideoState().Recording {
		t.Error("a clip is recording although no monitor was chosen")
	}
}

func TestSetVideoTargetAndAudioValidate(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.SetVideoTarget("DP-9"); err == nil {
		t.Error("accepted a monitor the recorder does not have")
	}
	if st, err := a.SetVideoTarget("DP-2"); err != nil || st.Target != "DP-2" {
		t.Errorf("SetVideoTarget = %+v, %v", st, err)
	}
	if _, err := a.SetVideoAudio("desktop"); err == nil {
		t.Error("accepted a sound option other than game or none")
	}
}

// A clip must not start while no capture is running (for example after the
// capture ended by its timeout but the session was not stopped yet).
func TestStartClipNeedsARunningCapture(t *testing.T) {
	a := newTestApp(t)
	a.cap = capture.New(t.TempDir(), nil, nil) // installed, but not capturing
	if _, err := a.SetVideoAllowed(true); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetVideoTarget("DP-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.j.Start("s", "Other"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ToggleVideo(); !errors.Is(err, errVideoNoCapture) {
		t.Errorf("ToggleVideo = %v, want %v", err, errVideoNoCapture)
	}
}

// stubRecorder stands in for gpu-screen-recorder: it records until SIGINT,
// then saves a clip, like the real one.
const stubRecorder = `#!/usr/bin/env bash
case "$1" in --*) exit 0 ;; esac
out=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done
trap 'printf VIDEO > "$out"; exit 0' INT
while :; do sleep 0.05; done
`

// When the capture ends for any reason, a running clip is stopped and kept.
func TestCaptureEndStopsTheClip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("video recording exists on Linux only")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gpu-screen-recorder"), []byte(stubRecorder), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")

	a := newTestApp(t)
	rec, err := video.Start(t.TempDir(), video.Options{Monitor: "DP-1"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	a.vid.rec = rec
	time.Sleep(300 * time.Millisecond)

	a.onCaptureEnded()

	if st := a.VideoState(); st.Recording || st.Clips != 1 {
		t.Fatalf("after the capture ended: recording %v, clips %d; want stopped with 1 clip", st.Recording, st.Clips)
	}
	select {
	case <-rec.Done():
	default:
		t.Error("the recorder is still running")
	}
	a.onCaptureEnded() // nothing running any more: must be a no-op
}
