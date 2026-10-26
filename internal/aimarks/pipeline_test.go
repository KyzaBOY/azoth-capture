package aimarks

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"azoth/capture/internal/video"
)

// fakeModel answers the find question with one gathering action on frame 2
// and keeps it in the check pass.
type fakeModel struct {
	mu      sync.Mutex
	prompts []string
	images  [][]string
}

func (f *fakeModel) Ask(_ context.Context, prompt string, images []string) (string, error) {
	f.mu.Lock()
	f.prompts = append(f.prompts, prompt)
	f.images = append(f.images, images)
	f.mu.Unlock()
	if strings.HasPrefix(prompt, "You are labelling") {
		return "Here you go:\n```json\n" +
			`{"events":[{"frame":2,"kind":"gather","screen_text":"Iron Vein","summary":"mining","position":[1015,2000],"confidence":0.9},` +
			`{"frame":99,"kind":"gather","screen_text":"","summary":"out of range","position":null,"confidence":1}]}` + "\n```", nil
	}
	return `{"keep":true,"text":"Mine Iron Vein (Small)","confidence":0.5}`, nil
}

// makeClip writes a short moving test video, or skips without ffmpeg.
func makeClip(t *testing.T, dir string) string {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", "testsrc=duration=8:size=320x180:rate=10", filepath.Join(dir, "clip-1.mkv")).CombinedOutput()
	if err != nil {
		t.Fatalf("make the test clip: %v: %s", err, out)
	}
	return ffmpeg
}

func TestRunDraftsCheckedMarks(t *testing.T) {
	session := t.TempDir()
	ffmpeg := makeClip(t, session)
	data, err := NewLocal(writeGameData(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	model := &fakeModel{}
	const first = int64(1_700_000_000_000)
	res, err := Run(context.Background(), Job{
		SessionDir: session, FFmpeg: ffmpeg, Clips: []video.Clip{{File: "clip-1.mkv", FirstFrameUnixMs: first}},
		Model: model, Label: "fake", Data: data, SampleEvery: 2, Workers: 2,
		Now: time.Now, Progress: func(string) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Frames != 4 || res.Parts != 1 || res.Failed != 0 {
		t.Errorf("frames %d, parts %d, failed %d", res.Frames, res.Parts, res.Failed)
	}
	if len(res.Drafts) != 1 {
		t.Fatalf("drafts = %+v", res.Drafts)
	}
	d := res.Drafts[0]
	if d.TMs != first+2000 || d.Text != "Mine Iron Vein (Small)" || !d.Unsure || d.Status != StatusPending ||
		d.Frame != "ai-frames/clip-1/f000002.jpg" {
		t.Errorf("draft = %+v", d)
	}
	if !slices.Equal(d.Candidates, []string{"Iron Vein (Small) (gatherable)", "Iron Vein (Small) (Expedition) (gatherable)", "Turkey (creature)"}) {
		t.Errorf("candidates = %q", d.Candidates)
	}
	// The check pass saw the frames around the action and the game names.
	check := model.prompts[len(model.prompts)-1]
	if len(model.images[len(model.images)-1]) != 3 || !strings.Contains(check, "Image 2 is the moment") ||
		!strings.Contains(check, "Turkey (creature)") {
		t.Errorf("check question = %q with %d images", check, len(model.images[len(model.images)-1]))
	}
}

func TestRunNeedsClips(t *testing.T) {
	_, err := Run(context.Background(), Job{SessionDir: t.TempDir(), Now: time.Now, Progress: func(string) {}})
	if err == nil {
		t.Error("a session without clips ran")
	}
}

type failingModel struct{}

func (failingModel) Ask(context.Context, string, []string) (string, error) {
	return "", errors.New("quota used up")
}

func TestRunFailsWhenEveryQuestionFails(t *testing.T) {
	session := t.TempDir()
	ffmpeg := makeClip(t, session)
	_, err := Run(context.Background(), Job{
		SessionDir: session, FFmpeg: ffmpeg, Clips: []video.Clip{{File: "clip-1.mkv", StartedUnixMs: 1}},
		Model: failingModel{}, Data: NewOnline("http://127.0.0.1:1", "t", nil), SampleEvery: 2,
		Now: time.Now, Progress: func(string) {},
	})
	if err == nil {
		t.Error("a run where every question failed reported success")
	}
}

func TestMergeEventsJoinsRepeats(t *testing.T) {
	ev := func(ms int64, kind, name string, conf float64) event {
		return event{seen: seen{Kind: kind, ScreenText: name, Confidence: conf}, frame: Frame{UnixMs: ms}}
	}
	got := mergeEvents([]event{
		ev(4000, "gather", "iron vein", 0.9),
		ev(0, "gather", "Iron Vein", 0.4),
		ev(20000, "gather", "Iron Vein", 0.5),
		ev(21000, "dance", "", 0.5),
	})
	if len(got) != 3 || got[0].Confidence != 0.9 || got[0].frame.UnixMs != 0 || got[2].Kind != "other" {
		t.Errorf("merged = %+v", got)
	}
}

func TestDecodeJSON(t *testing.T) {
	var v verdict
	if err := decodeJSON("Sure! {\"keep\":true,\"text\":\"Open chest\",\"confidence\":0.8} Done.", &v); err != nil || !v.Keep || v.Text != "Open chest" {
		t.Errorf("verdict = %+v, %v", v, err)
	}
	if err := decodeJSON("I cannot see anything.", &v); err == nil {
		t.Error("an answer without JSON was accepted")
	}
}

func TestWindowsSkipIdleParts(t *testing.T) {
	frames := make([]Frame, 25)
	frames[12].Change = idleChange // something moved in the second window only
	got := Windows(frames)
	if len(got) != 1 || len(got[0].Frames) != windowFrames || got[0].Frames[0].Index != frames[10].Index {
		t.Errorf("windows = %d", len(got))
	}
}
