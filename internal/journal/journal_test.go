package journal

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// A step accepted from AI marks goes in at the moment of the video frame, in
// time order, after the session stopped; there is nothing to add it to before
// a session starts.
func TestAddReviewedStepAfterStop(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	j := New(func() time.Time { return now }, filepath.Join(t.TempDir(), "draft.json"), nil)
	if _, err := j.AddReviewedStep(now, "Mine Iron Vein"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("added a step without a session: %v", err)
	}
	if _, err := j.Start("s", "Gathering"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Mark(now.Add(10*time.Second), ViaHotkey, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Mark(now, ViaHotkey, "", nil); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Mark worked after Stop: %v", err)
	}
	st, err := j.AddReviewedStep(now.Add(5*time.Second), "  Mine   Iron Vein ")
	if err != nil {
		t.Fatal(err)
	}
	if st.Via != ViaAI || st.Text != "Mine Iron Vein" {
		t.Errorf("step = %+v", st)
	}
	steps := j.State().Steps
	if len(steps) != 2 || steps[0].ID != st.ID {
		t.Errorf("steps = %+v, want the reviewed step first by time", steps)
	}
}
