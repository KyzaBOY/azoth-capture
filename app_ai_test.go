package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"azoth/capture/internal/aimarks"
	"azoth/capture/internal/capture"
	"azoth/capture/internal/config"
	"azoth/capture/internal/journal"
)

func TestAIMarksNeedVideoAndHideTheKey(t *testing.T) {
	a := newTestApp(t)
	on := config.AISettings{Enabled: true, Connector: aimarks.ConnectorAnthropic, APIKey: "sk-test", Model: "m"}
	if _, err := a.SetAISettings(on, false); !errors.Is(err, errAINeedsVideo) {
		t.Fatalf("AI marks turned on without video: %v", err)
	}
	if _, err := a.SetVideoAllowed(true); err != nil {
		t.Fatal(err)
	}
	st, err := a.SetAISettings(on, false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Settings.APIKey != "" || !st.HasKey || st.Settings.SampleEvery != aimarks.MinSampleEvery {
		t.Errorf("status = %+v: the key must never reach the window; 0 s is raised to the minimum", st)
	}
	on.APIKey = "" // the window sends no key when the player did not type one
	if _, err := a.SetAISettings(on, false); err != nil || a.set.AI.APIKey != "sk-test" {
		t.Errorf("an empty key replaced the saved one: %q, %v", a.set.AI.APIKey, err)
	}
	if st, _ := a.SetAISettings(on, true); st.HasKey || a.set.AI.APIKey != "" {
		t.Error("Forget kept the key")
	}
}

// The window draws the AI panel from Boot's "ai" field.
func TestBootSendsAIStatus(t *testing.T) {
	a := newTestApp(t)
	a.cap = capture.New(t.TempDir(), func(string) {}, func(capture.Status) {})
	a.set.AI.APIKey = "sk-secret"
	b, err := json.Marshal(a.Boot())
	if err != nil {
		t.Fatal(err)
	}
	var boot struct {
		AI *AIStatus `json:"ai"`
	}
	if err := json.Unmarshal(b, &boot); err != nil {
		t.Fatal(err)
	}
	if boot.AI == nil || len(boot.AI.CLIs) == 0 || !boot.AI.HasKey {
		t.Errorf("boot = %s", b)
	}
	if strings.Contains(string(b), "sk-secret") {
		t.Error("Boot sent the API key to the window")
	}
}

func TestRunAIMarksNeedsStoppedSession(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.RunAIMarks(); !errors.Is(err, errAINoSession) {
		t.Errorf("ran without a stopped session: %v", err)
	}
}

// Accepting a draft adds an "ai" step at the frame's moment and writes
// steps.json into the send folder; the hit rate counts each draft once.
func TestDecideAIDraftWritesSteps(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.j.Start("gather-01", "Gathering"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.j.Stop(); err != nil {
		t.Fatal(err)
	}
	dir, err := a.j.Export(a.outputDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "send"), 0o755); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Minute).UnixMilli()
	res := aimarks.Result{Model: "codex: m @ low", Drafts: []aimarks.Draft{
		{ID: 1, TMs: at, Text: "Mine Iron Vein", Status: aimarks.StatusPending},
		{ID: 2, TMs: at, Text: "Dance", Status: aimarks.StatusPending},
	}}
	a.ai.result, a.ai.sessionDir = &res, dir

	// One batch, as "Accept selected" sends it, then a repeat that must not
	// change a decided draft.
	if _, err := a.DecideAIDrafts([]AIDecision{{ID: 1, Accept: true, Text: "Mine Iron Vein (Small)"}, {ID: 2}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DecideAIDrafts([]AIDecision{{ID: 1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DecideAIDrafts([]AIDecision{{ID: 9, Accept: true}}); err == nil {
		t.Error("an unknown draft was decided")
	}

	b, err := os.ReadFile(filepath.Join(dir, "send", "steps.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f journal.StepsFile
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Steps) != 1 || f.Steps[0].Via != journal.ViaAI || f.Steps[0].Text != "Mine Iron Vein (Small)" || f.Steps[0].TMs != at {
		t.Errorf("send/steps.json steps = %+v", f.Steps)
	}
	stats, err := aimarks.LoadStats(config.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Accepted != 1 || stats[0].Rejected != 1 {
		t.Errorf("stats = %+v", stats)
	}
	saved, found, err := aimarks.LoadResult(dir)
	if err != nil || !found || saved.Drafts[0].Status != aimarks.StatusAccepted || saved.Drafts[1].Status != aimarks.StatusRejected {
		t.Errorf("saved drafts = %+v, %v, %v", saved.Drafts, found, err)
	}
}
