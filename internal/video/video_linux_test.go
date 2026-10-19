package video

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRecorder is a stand-in gpu-screen-recorder: it answers the list
// commands, and when asked to record it writes a clip and a first-frame
// timestamp file, then saves and exits on SIGINT, like the real one. With a
// portal token path it writes the token, like the real one after the player
// picks a window. It records its arguments in args.txt.
//
//	FAKE_FAIL=1      exit at once with an error and no clip
//	FAKE_APPS=...    the application-audio list
//	FAKE_PORTAL=0    no window picking on this desktop
const fakeRecorder = `#!/usr/bin/env bash
here="$(dirname "$0")"
case "$1" in
  --list-monitors) printf 'DP-1|1920x1080\nHDMI-A-1|2560x1440\n'; exit 0 ;;
  --list-capture-options) printf 'DP-1|1920x1080\nHDMI-A-1|2560x1440\nregion\n'; [ "${FAKE_PORTAL:-1}" = 1 ] && echo portal; exit 0 ;;
  --list-application-audio) printf '%b' "${FAKE_APPS:-Firefox\nNew World\n}"; exit 0 ;;
  --help) echo "usage: gpu-screen-recorder -exclude-metadata yes|no -write-first-frame-ts yes|no"; exit 0 ;;
esac
echo "$@" > "$here/args.txt"
printf '%s\n' "$@" > "$here/argv.txt"
if [ "${FAKE_FAIL:-0}" = 1 ]; then echo "gsr error: failed to capture"; exit 1; fi
out=""; token=""
while [ $# -gt 0 ]; do
  [ "$1" = -o ] && out="$2"
  [ "$1" = -portal-session-token-filepath ] && token="$2"
  shift
done
[ -n "$token" ] && echo restore-token > "$token"
printf 'monotonic_microsec\trealtime_microsec\n123456\t1792415514760123\n' > "$out.ts"
trap 'printf "VIDEO" > "$out"; exit 0' INT
while :; do sleep 0.05; done
`

const fakeFirstFrameMs = 1792415514760

type fake struct{ bin string }

// installFake puts the fake recorder first on PATH (no Flatpak is found
// either).
func installFake(t *testing.T) fake {
	t.Helper()
	f := fake{bin: t.TempDir()}
	if err := os.WriteFile(filepath.Join(f.bin, recorderName), []byte(fakeRecorder), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", f.bin+":/usr/bin:/bin")
	return f
}

// argv returns the recorder's arguments exactly as it received them.
func (f fake) argv(t *testing.T) []string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(f.bin, "argv.txt"))
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func (f fake) args(t *testing.T) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(f.bin, "args.txt"))
	return string(b)
}

// record starts a clip, lets it run for d, and stops it.
func record(t *testing.T, dir string, opt Options, d time.Duration, logf func(string)) (Clip, error) {
	t.Helper()
	rec, err := Start(dir, opt, logf)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(d)
	return rec.Stop()
}

func noLog(string) {}

const settle = 300 * time.Millisecond

func TestProbeWithoutRecorder(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	info := Probe()
	if info.Available || info.Reason != InstallHint {
		t.Fatalf("Probe = %+v, want unavailable with the install hint", info)
	}
}

func TestProbe(t *testing.T) {
	installFake(t)
	info := Probe()
	want := []Monitor{{"DP-1", "1920x1080"}, {"HDMI-A-1", "2560x1440"}}
	if !info.Available || !info.WindowCapture || len(info.Monitors) != 2 || info.Monitors[0] != want[0] || info.Monitors[1] != want[1] {
		t.Fatalf("Probe = %+v", info)
	}
	t.Setenv("FAKE_PORTAL", "0")
	if Probe().WindowCapture {
		t.Error("window capture reported without the portal")
	}
}

func TestMonitorClip(t *testing.T) {
	f := installFake(t)
	dir := filepath.Join(t.TempDir(), "session")
	clip, err := record(t, dir, Options{Monitor: "HDMI-A-1", GameAudio: true}, settle, noLog)
	if err != nil {
		t.Fatal(err)
	}
	if clip.File != "gameplay-001.mkv" || clip.Target != "HDMI-A-1" || clip.Audio != "app:New World" {
		t.Errorf("clip = %+v", clip)
	}
	if clip.FirstFrameUnixMs != fakeFirstFrameMs || clip.StoppedUnixMs < clip.StartedUnixMs {
		t.Errorf("times = %+v", clip)
	}
	args := f.args(t)
	for _, want := range []string{"-w HDMI-A-1", "-c mkv", "-s 1920x1080", "-a app:New World",
		"-exclude-metadata yes", "-write-first-frame-ts yes", "-o " + filepath.Join(dir, "gameplay-001.mkv")} {
		if !strings.Contains(args, want) {
			t.Errorf("recorder args %q lack %q", args, want)
		}
	}
	for _, bad := range []string{"default_input", "default_output", "device:"} {
		if strings.Contains(args, bad) {
			t.Errorf("recorder args contain %q: only the game's own playback may be recorded", bad)
		}
	}

	clip2, err := record(t, dir, Options{Monitor: "DP-1"}, settle, noLog)
	if err != nil || clip2.File != "gameplay-002.mkv" || clip2.Audio != noAudio {
		t.Errorf("second clip = %+v, %v", clip2, err)
	}
}

func TestWindowClip(t *testing.T) {
	f := installFake(t)
	token := filepath.Join(t.TempDir(), "token")
	opt := Options{Window: true, PortalToken: token}
	if _, err := Start(t.TempDir(), opt, noLog); err == nil {
		t.Fatal("window clip started before a window was picked")
	}
	if err := os.WriteFile(token, []byte("t"), 0o600); err != nil {
		t.Fatal(err)
	}
	clip, err := record(t, t.TempDir(), opt, settle, noLog)
	if err != nil || clip.Target != WindowTarget {
		t.Fatalf("clip = %+v, %v", clip, err)
	}
	if !strings.Contains(f.args(t), "-w portal -restore-portal-session yes -portal-session-token-filepath "+token) {
		t.Errorf("recorder args %q do not restore the picked window", f.args(t))
	}
}

func TestMonitorMustBeChosen(t *testing.T) {
	installFake(t)
	if _, err := Start(t.TempDir(), Options{}, noLog); err == nil {
		t.Fatal("a clip started without a chosen monitor")
	}
}

// The only sound source the app ever asks for is the game's own playback
// ("app:<game>"). It never asks for a microphone device or any other device,
// whatever programs are playing or recording.
func TestNeverRecordsAMicrophoneDevice(t *testing.T) {
	f := installFake(t)
	t.Setenv("FAKE_APPS", `Firefox\nDiscord\nNew World\n`)
	for _, gameAudio := range []bool{true, false} {
		if _, err := record(t, t.TempDir(), Options{Monitor: "DP-1", GameAudio: gameAudio}, settle, noLog); err != nil {
			t.Fatal(err)
		}
		args := f.argv(t)
		var sources []string
		for i, a := range args {
			if a == "-a" && i+1 < len(args) {
				sources = append(sources, args[i+1])
			}
		}
		want := 0
		if gameAudio {
			want = 1
		}
		if len(sources) != want {
			t.Fatalf("game audio %v: sound sources %q, want %d", gameAudio, sources, want)
		}
		for _, src := range sources {
			if !strings.HasPrefix(src, "app:") || !isGameName(strings.TrimPrefix(src, "app:")) {
				t.Errorf("sound source %q is not the game's own playback", src)
			}
		}
		for _, bad := range []string{"default_input", "default_output", "device:", "app-inverse:"} {
			if strings.Contains(f.args(t), bad) {
				t.Errorf("recorder args contain %q", bad)
			}
		}
	}
}

func TestClipWithoutGameSound(t *testing.T) {
	f := installFake(t)
	t.Setenv("FAKE_APPS", `Firefox\nDiscord\n`)
	clip, err := record(t, t.TempDir(), Options{Monitor: "DP-1", GameAudio: true}, settle, noLog)
	if err != nil || clip.Audio != noAudio || strings.Contains(f.args(t), "-a ") {
		t.Fatalf("clip = %+v, %v, args %q", clip, err, f.args(t))
	}
}

func TestRecorderFailingOnItsOwn(t *testing.T) {
	installFake(t)
	t.Setenv("FAKE_FAIL", "1")
	var logged []string
	rec, err := Start(t.TempDir(), Options{Monitor: "DP-1"}, func(s string) { logged = append(logged, s) })
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-rec.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done did not close when the recorder exited")
	}
	if rec.Err() == nil {
		t.Error("Err should report the failed exit")
	}
	if _, err := rec.Stop(); err == nil {
		t.Error("Stop should report that no video was saved")
	}
	if strings.Join(logged, "\n") != "Video: gsr error: failed to capture" {
		t.Errorf("recorder output not passed to the log: %q", logged)
	}
}

func TestPickWindow(t *testing.T) {
	installFake(t)
	token := filepath.Join(t.TempDir(), "sub", "token")
	if err := PickWindow(token, noLog); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(token); err != nil {
		t.Fatal("the picked window was not remembered")
	}

	// Picking again forgets the old choice first; a cancelled dialog leaves none.
	t.Setenv("FAKE_FAIL", "1")
	if err := PickWindow(token, noLog); err == nil {
		t.Error("a cancelled pick reported success")
	}
	if _, err := os.Stat(token); err == nil {
		t.Error("the old window choice survived a new pick")
	}
}

func TestParseFirstFrame(t *testing.T) {
	ms, err := parseFirstFrame("monotonic_microsec\trealtime_microsec\n1\t1792415514760999\n")
	if err != nil || ms != 1792415514760 {
		t.Errorf("got %d, %v", ms, err)
	}
	for _, bad := range []string{"", "header only", "h\n1", "h\n1\tx", "h\n1\t-5"} {
		if _, err := parseFirstFrame(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestAddToIndexMerges(t *testing.T) {
	dir := t.TempDir()
	a := Clip{File: "gameplay-001.mkv", StartedUnixMs: 1}
	b := Clip{File: "gameplay-002.mkv", StartedUnixMs: 2}
	if err := AddToIndex(dir, []Clip{a}); err != nil {
		t.Fatal(err)
	}
	// After a restart only the new clip is in memory; the old one must stay.
	b2 := b
	b2.StoppedUnixMs = 9
	if err := AddToIndex(dir, []Clip{b, b2}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, IndexName))
	var idx index
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	if len(idx.Clips) != 2 || idx.Clips[0] != a || idx.Clips[1] != b2 || idx.Note == "" {
		t.Errorf("index = %+v", idx)
	}

	if err := os.WriteFile(filepath.Join(dir, IndexName), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddToIndex(dir, []Clip{a}); err == nil {
		t.Error("an unreadable index must be left alone, with an error")
	}
}
