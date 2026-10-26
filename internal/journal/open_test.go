package journal

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// A stopped, exported session can be opened again after a restart, with its
// steps, and a reviewed step still goes into it.
func TestOpenExportedRoundTrip(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	out := t.TempDir()
	j := New(func() time.Time { return now }, filepath.Join(t.TempDir(), "draft.json"), nil)
	if _, err := j.Start("fishing-01", "Fishing"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Mark(now.Add(time.Second), ViaHotkey, "Cast", nil); err != nil {
		t.Fatal(err)
	}
	j.Stop()
	dir, err := j.Export(out)
	if err != nil {
		t.Fatal(err)
	}

	again := New(func() time.Time { return now }, filepath.Join(t.TempDir(), "draft.json"), nil)
	st, err := again.OpenExported(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Phase != PhaseStopped || st.Name != "fishing-01" || st.ExportDir != dir || len(st.Steps) != 1 || st.Steps[0].Text != "Cast" {
		t.Errorf("opened state = %+v", st)
	}
	if _, err := again.AddReviewedStep(now.Add(2*time.Second), "Reel in"); err != nil {
		t.Fatal(err)
	}
	if got := again.State().Steps; len(got) != 2 || got[1].ID == got[0].ID {
		t.Errorf("steps after review = %+v", got)
	}

	running := New(func() time.Time { return now }, "", nil)
	running.Start("other", "Other")
	if _, err := running.OpenExported(dir); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("opened a session over a running one: %v", err)
	}
}
