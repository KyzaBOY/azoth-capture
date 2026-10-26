package video

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFFmpegPathPrefersOwnCopy(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, ffmpegFile())
	if err := os.WriteFile(own, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := FFmpegPath(dir); err != nil || got != own {
		t.Errorf("FFmpegPath = %q, %v; want the app's own copy", got, err)
	}
}

func TestVerifyAndExtract(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ff.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{"ffmpeg-n9/bin/ffmpeg.exe": "binary", "ffmpeg-n9/bin/ffprobe.exe": "other"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, body)
	}
	zw.Close()
	f.Close()

	b, _ := os.ReadFile(zipPath)
	sum := sha256.Sum256(b)
	if err := verifySHA256(zipPath, hex.EncodeToString(sum[:])); err != nil {
		t.Errorf("a matching download was rejected: %v", err)
	}
	if err := verifySHA256(zipPath, strings.Repeat("0", 64)); err == nil {
		t.Error("a changed download was accepted")
	}
	dst := filepath.Join(dir, "ffmpeg.exe")
	if err := extractFromZip(zipPath, "/bin/ffmpeg.exe", dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "binary" {
		t.Errorf("extracted %q", got)
	}
	if err := extractFromZip(zipPath, "/bin/missing.exe", dst); err == nil {
		t.Error("a missing file was extracted")
	}
}

func TestReadProgressReportsFirstFrameOnce(t *testing.T) {
	text := "frame=0\nout_time_us=0\nprogress=continue\n" +
		"frame=12\nout_time_us=200000\nprogress=continue\n" +
		"frame=40\nout_time_us=666000\nprogress=end\nsome log line\n"
	var calls []int64
	var logged []string
	readProgress(strings.NewReader(text), func(us int64) { calls = append(calls, us) }, func(l string) { logged = append(logged, l) })
	if !slices.Equal(calls, []int64{200000}) || !slices.Equal(logged, []string{"some log line"}) {
		t.Errorf("first-frame calls %v, log %v", calls, logged)
	}
}

// Every format falls back to H.264, and H.264 ends with the CPU encoder,
// which works on any PC: there is always an encoder to try last.
func TestEncodersAlwaysFallBack(t *testing.T) {
	for _, c := range Codecs {
		list := encodersFor(c)
		if len(list) == 0 || list[len(list)-1].name != "libx264" {
			t.Errorf("%s: encoders %v do not end with libx264", c, list)
		}
	}
	if got := encodersFor(CodecHEVC)[0].name; got != "hevc_nvenc" {
		t.Errorf("HEVC starts with %s", got)
	}
	if got := encodersFor(CodecAuto)[0].codec; got != CodecAV1 {
		t.Errorf("Auto starts with %s, want the most compact format", got)
	}
	for _, e := range encoders {
		for _, q := range Qualities {
			if e.levels[q] == "" {
				t.Errorf("%s has no setting for quality %s", e.name, q)
			}
		}
	}
}

func TestRecordArgs(t *testing.T) {
	a := recordArgs(gfxcaptureSource(""), encodersFor(CodecHEVC)[0], QualityVeryHigh, "/s/gameplay-001.mkv")
	src := a[slices.Index(a, "-i")+1]
	if !strings.HasPrefix(src, "gfxcapture=window_exe=(?i)^NewWorld\\.exe$:") || !strings.HasSuffix(src, ",hwdownload,format=bgra") {
		t.Errorf("source = %q", src)
	}
	cq := slices.Index(a, "-cq")
	if a[len(a)-1] != "/s/gameplay-001.mkv" || !slices.Contains(a, "hevc_nvenc") || !slices.Contains(a, "pipe:1") ||
		cq < 0 || a[cq+1] != "22" {
		t.Errorf("args = %q", a)
	}
	if slices.ContainsFunc(a, func(s string) bool { return strings.Contains(s, "dshow") || strings.Contains(s, "wasapi") }) {
		t.Errorf("args = %q: Windows clips record no sound device", a)
	}
}

// A real recording with the same command line, from a test picture instead
// of the game window: ffmpeg reports its first frame and stops cleanly on "q".
func TestRecordStopsOnQ(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	libx264 := encoders[len(encoders)-1]
	if exec.Command(ffmpeg, encoderTestArgs(libx264)...).Run() != nil {
		t.Skip("this ffmpeg has no libx264")
	}
	out := filepath.Join(t.TempDir(), "clip.mkv")
	cmd := exec.Command(ffmpeg, recordArgs("testsrc=size=640x360:rate=60", libx264, QualityHigh, out)...)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	first := make(chan int64, 1)
	go readProgress(stdout, func(us int64) { first <- us }, func(string) {})
	select {
	case <-first:
	case <-time.After(20 * time.Second):
		t.Fatal("no first frame reported")
	}
	io.WriteString(stdin, "q")
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("ffmpeg did not stop cleanly: %v", err)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Errorf("no clip written: %v", err)
	}
}

// A renamed game file still finds its window; characters that mean something
// in a regular expression or a filter graph match any one character.
func TestGameExeMatch(t *testing.T) {
	for gameExe, want := range map[string]string{
		"":                            `(?i)^NewWorld\.exe$`,
		"D:/Games/NW/nw-client_2.exe": `(?i)^nw-client_2\.exe$`,
		"C:/x/My Game (old),v1:2.exe": `(?i)^My.Game..old..v1.2\.exe$`,
	} {
		if got := gameExeMatch(gameExe); got != want {
			t.Errorf("gameExeMatch(%q) = %q, want %q", gameExe, got, want)
		}
	}
}
