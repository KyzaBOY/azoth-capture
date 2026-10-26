package aimarks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecideCountsOnce(t *testing.T) {
	dir := t.TempDir()
	res := Result{Model: "m", Drafts: []Draft{{ID: 1, Status: StatusPending}, {ID: 2, Status: StatusPending}}}
	if _, changed, err := res.Decide(1, StatusAccepted); err != nil || !changed {
		t.Fatalf("decide = %v, %v", changed, err)
	}
	if _, changed, _ := res.Decide(1, StatusRejected); changed {
		t.Error("a decided draft changed again")
	}
	if _, _, err := res.Decide(3, StatusAccepted); err == nil {
		t.Error("an unknown draft was decided")
	}
	if _, _, err := res.Decide(2, "maybe"); err == nil {
		t.Error("an unknown decision was accepted")
	}
	if err := SaveResult(dir, res); err != nil {
		t.Fatal(err)
	}
	back, found, err := LoadResult(dir)
	if err != nil || !found || back.Drafts[0].Status != StatusAccepted {
		t.Errorf("loaded = %+v, %v, %v", back, found, err)
	}
	if _, found, err := LoadResult(t.TempDir()); found || err != nil {
		t.Errorf("a session without a run = %v, %v", found, err)
	}
}

func TestPruneFramesKeepsOnlyPendingDraftFrames(t *testing.T) {
	session := t.TempDir()
	clip := filepath.Join(session, FramesDirName, "gameplay-001")
	if err := os.MkdirAll(clip, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"f000001.jpg", "f000002.jpg", "f000003.jpg"} {
		os.WriteFile(filepath.Join(clip, n), []byte("jpg"), 0o644)
	}
	res := Result{Drafts: []Draft{
		{ID: 1, Frame: FramesDirName + "/gameplay-001/f000002.jpg", Status: StatusPending},
		{ID: 2, Frame: FramesDirName + "/gameplay-001/f000003.jpg", Status: StatusRejected},
	}}
	if err := PruneFrames(session, res); err != nil {
		t.Fatal(err)
	}
	left, _ := filepath.Glob(filepath.Join(clip, "*.jpg"))
	if len(left) != 1 || filepath.Base(left[0]) != "f000002.jpg" {
		t.Errorf("frames left = %v, want only the pending draft's frame", left)
	}
	res.Drafts[0].Status = StatusAccepted
	if err := PruneFrames(session, res); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(session, FramesDirName)); !os.IsNotExist(err) {
		t.Errorf("the frames folder is still there after every draft was decided: %v", err)
	}
	if err := PruneFrames(session, res); err != nil {
		t.Errorf("pruning again failed: %v", err)
	}
}

func TestStatsHitRateBestFirst(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []struct {
		model              string
		accepted, rejected int
	}{{"a", 0, 1}, {"a", 1, 0}, {"b", 2, 0}} {
		if err := CountDecisions(dir, d.model, d.accepted, d.rejected); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadStats(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (HitRate{"b", 2, 0}) || got[1] != (HitRate{"a", 1, 1}) {
		t.Errorf("stats = %+v", got)
	}
}
