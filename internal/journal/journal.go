// Package journal is the core of the Azoth Capture Journal: the session, the
// marked steps and the draft on disk. It knows nothing about the window or the
// hotkey; the caller (app.go) injects the clock and is told about every change.
//
// The clock is the PC's (Unix ms), the same one the capture writes into
// ledger.bin, so the step times line up with the ledger's.
package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// App version, written into steps.json ("azoth-capture 0.1.0").
const (
	AppName = "azoth-capture"
	Version = "0.2.0"
)

// What produced the step.
const (
	ViaButton    = "button"
	ViaHotkey    = "hotkey"
	ViaChecklist = "checklist"
)

// Session phases.
const (
	PhaseIdle    = "idle"
	PhaseRunning = "running"
	PhaseStopped = "stopped"
)

const maxTextRunes = 500

var (
	ErrNotRunning     = errors.New("start a session first")
	ErrAlreadyRunning = errors.New("a session is already running")
	ErrNoStep         = errors.New("step not found")
)

// Activities is the list from section 7 of CONTRATOS.md, without "Developer-only" (a
// test command the normal game never sends, so there is nothing to record).
var Activities = []string{
	"Fishing", "Combat", "Inventory & gear", "Chat", "Trading post", "Housing",
	"Expeditions", "Wars & PvP", "Crafting", "Gathering", "Quests",
	"Login & world entry", "Movement", "Social & groups", "Companies",
	"Progression", "Store & cosmetics", "Camping", "Other",
}

// Step is one marked step. ID exists only so the screen can edit or delete it; it is not written to steps.json.
type Step struct {
	ID     int    `json:"id"`
	TMs    int64  `json:"t_ms"`
	Text   string `json:"text"`
	Target *int   `json:"target,omitempty"`
	Via    string `json:"via"`
}

// Session is the full session state, and also the draft format.
type Session struct {
	Schema    int    `json:"schema"`
	Phase     string `json:"phase"`
	Name      string `json:"name"`
	Activity  string `json:"activity"`
	StartedMs int64  `json:"started_ms"`
	StoppedMs int64  `json:"stopped_ms"`
	Targets   []int  `json:"targets"`
	Steps     []Step `json:"steps"`
	NextID    int    `json:"next_id"`
	// Exported: the current steps.json is already on disk; any edit clears it.
	Exported bool  `json:"exported"`
	SavedMs  int64 `json:"saved_ms"`
}

// State is what the screen receives: the session plus what exists only in memory.
type State struct {
	Session
	ExportDir  string `json:"export_dir"`
	DraftError string `json:"draft_error"`
}

// Journal holds the current session and saves the draft on every change.
type Journal struct {
	mu        sync.Mutex
	now       func() time.Time
	draftPath string
	s         Session
	exportDir string
	draftErr  string
	onChange  func(State)
}

// New creates an empty Journal. An empty draftPath turns the draft off (tests).
func New(now func() time.Time, draftPath string, onChange func(State)) *Journal {
	if now == nil {
		now = time.Now
	}
	j := &Journal{now: now, draftPath: draftPath, onChange: onChange}
	j.s = emptySession()
	return j
}

func emptySession() Session {
	return Session{Schema: 1, Phase: PhaseIdle, Targets: []int{}, Steps: []Step{}, NextID: 1}
}

// State returns a copy of the current state.
func (j *Journal) State() State {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.stateLocked()
}

func (j *Journal) stateLocked() State {
	s := j.s
	s.Targets = append([]int{}, j.s.Targets...)
	s.Steps = make([]Step, len(j.s.Steps))
	for i, st := range j.s.Steps {
		if st.Target != nil {
			v := *st.Target
			st.Target = &v
		}
		s.Steps[i] = st
	}
	return State{Session: s, ExportDir: j.exportDir, DraftError: j.draftErr}
}

// commit saves the draft, releases the lock and notifies the screen. Call it with the lock held.
func (j *Journal) commit() State {
	j.s.Exported = false
	j.persistLocked()
	st := j.stateLocked()
	j.mu.Unlock()
	if j.onChange != nil {
		j.onChange(st)
	}
	return st
}

// Start opens a new session. An empty name or activity becomes "session" / "Other".
func (j *Journal) Start(name, activity string) (State, error) {
	j.mu.Lock()
	if j.s.Phase == PhaseRunning {
		j.mu.Unlock()
		return State{}, ErrAlreadyRunning
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "session"
	}
	if activity = strings.TrimSpace(activity); activity == "" {
		activity = "Other"
	}
	targets := j.s.Targets // targets chosen before Start apply to the session
	j.s = emptySession()
	j.s.Targets = targets
	j.s.Phase = PhaseRunning
	j.s.Name = name
	j.s.Activity = activity
	j.s.StartedMs = j.now().UnixMilli()
	j.exportDir = ""
	return j.commit(), nil
}

// Stop ends the session. The draft stays until the export succeeds.
func (j *Journal) Stop() (State, error) {
	j.mu.Lock()
	if j.s.Phase != PhaseRunning {
		j.mu.Unlock()
		return State{}, ErrNotRunning
	}
	j.s.Phase = PhaseStopped
	j.s.StoppedMs = j.now().UnixMilli()
	return j.commit(), nil
}

// Mark records a step at time `at` (the moment of the click or key press,
// taken by the caller BEFORE any other work). Empty text becomes "Step N".
func (j *Journal) Mark(at time.Time, via, text string, target *int) (Step, error) {
	j.mu.Lock()
	if j.s.Phase != PhaseRunning {
		j.mu.Unlock()
		return Step{}, ErrNotRunning
	}
	text = cleanText(text)
	if text == "" {
		text = fmt.Sprintf("Step %d", len(j.s.Steps)+1)
	}
	st := Step{ID: j.s.NextID, TMs: at.UnixMilli(), Text: text, Via: via}
	if target != nil {
		v := *target
		st.Target = &v
	}
	j.s.NextID++
	j.s.Steps = append(j.s.Steps, st)
	sort.SliceStable(j.s.Steps, func(a, b int) bool { return j.s.Steps[a].TMs < j.s.Steps[b].TMs })
	j.commit()
	return st, nil
}

// EditStep changes the text of a step (its time never changes).
func (j *Journal) EditStep(id int, text string) (State, error) {
	j.mu.Lock()
	i := j.indexLocked(id)
	if i < 0 {
		j.mu.Unlock()
		return State{}, ErrNoStep
	}
	text = cleanText(text)
	if text == "" {
		text = fmt.Sprintf("Step %d", i+1)
	}
	j.s.Steps[i].Text = text
	return j.commit(), nil
}

// DeleteStep removes a step.
func (j *Journal) DeleteStep(id int) (State, error) {
	j.mu.Lock()
	i := j.indexLocked(id)
	if i < 0 {
		j.mu.Unlock()
		return State{}, ErrNoStep
	}
	j.s.Steps = append(j.s.Steps[:i], j.s.Steps[i+1:]...)
	return j.commit(), nil
}

// SetTargets stores the packets chosen for recording (no repeats, in order).
func (j *Journal) SetTargets(ids []int) State {
	j.mu.Lock()
	seen := map[int]bool{}
	out := []int{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	j.s.Targets = out
	return j.commit()
}

// Reset discards the session (and the draft) and returns to the start screen.
func (j *Journal) Reset() State {
	j.mu.Lock()
	targets := j.s.Targets
	j.s = emptySession()
	j.s.Targets = targets
	j.exportDir = ""
	j.removeDraftLocked()
	st := j.stateLocked()
	j.mu.Unlock()
	if j.onChange != nil {
		j.onChange(st)
	}
	return st
}

func (j *Journal) indexLocked(id int) int {
	for i, st := range j.s.Steps {
		if st.ID == id {
			return i
		}
	}
	return -1
}

// cleanText removes line breaks (steps.txt has one line per step) and limits the length.
func cleanText(s string) string {
	s = strings.Join(strings.Fields(strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(s)), " ")
	if r := []rune(s); len(r) > maxTextRunes {
		s = string(r[:maxTextRunes])
	}
	return s
}

// ---- draft ------------------------------------------------------------------

// persistLocked writes the draft atomically (tmp + rename). A failure does not
// stop the session; it becomes a notice on screen.
func (j *Journal) persistLocked() {
	if j.draftPath == "" || j.s.Phase == PhaseIdle {
		return
	}
	j.s.SavedMs = j.now().UnixMilli()
	if err := writeFileAtomic(j.draftPath, mustJSON(j.s)); err != nil {
		j.draftErr = "Could not save the draft: " + err.Error()
		return
	}
	j.draftErr = ""
}

func (j *Journal) removeDraftLocked() {
	if j.draftPath != "" {
		os.Remove(j.draftPath)
	}
	j.draftErr = ""
}

// markExported records in the draft that the current steps.json is already on disk.
func (j *Journal) markExported(dir string) State {
	j.mu.Lock()
	j.s.Exported = true
	j.exportDir = dir
	if j.draftPath != "" {
		j.s.SavedMs = j.now().UnixMilli()
		if err := writeFileAtomic(j.draftPath, mustJSON(j.s)); err != nil {
			j.draftErr = "Could not save the draft: " + err.Error()
		}
	}
	st := j.stateLocked()
	j.mu.Unlock()
	if j.onChange != nil {
		j.onChange(st)
	}
	return st
}

// DraftInfo summarizes a draft found on disk.
type DraftInfo struct {
	Found     bool   `json:"found"`
	Name      string `json:"name"`
	Activity  string `json:"activity"`
	Phase     string `json:"phase"`
	Steps     int    `json:"steps"`
	StartedMs int64  `json:"started_ms"`
	SavedMs   int64  `json:"saved_ms"`
}

// CheckDraft looks at the draft on disk. A draft that was already exported and not
// edited since is not offered: it is deleted quietly. So is an unreadable one (kept as .bad).
func (j *Journal) CheckDraft() DraftInfo {
	j.mu.Lock()
	defer j.mu.Unlock()
	s, err := j.loadDraftLocked()
	if err != nil {
		return DraftInfo{}
	}
	if s.Exported || s.Phase == PhaseIdle {
		j.removeDraftLocked()
		return DraftInfo{}
	}
	return DraftInfo{Found: true, Name: s.Name, Activity: s.Activity, Phase: s.Phase,
		Steps: len(s.Steps), StartedMs: s.StartedMs, SavedMs: s.SavedMs}
}

func (j *Journal) loadDraftLocked() (Session, error) {
	if j.draftPath == "" {
		return Session{}, os.ErrNotExist
	}
	b, err := os.ReadFile(j.draftPath)
	if err != nil {
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil || s.Schema != 1 ||
		(s.Phase != PhaseRunning && s.Phase != PhaseStopped && s.Phase != PhaseIdle) {
		os.Rename(j.draftPath, j.draftPath+".bad") // kept for inspection, not offered again
		return Session{}, errors.New("unreadable draft")
	}
	if s.Targets == nil {
		s.Targets = []int{}
	}
	if s.Steps == nil {
		s.Steps = []Step{}
	}
	if s.NextID < 1 {
		s.NextID = 1
	}
	for _, st := range s.Steps {
		if st.ID >= s.NextID {
			s.NextID = st.ID + 1
		}
	}
	return s, nil
}

// Recover loads the draft. A session that was running keeps running
// (its Stop takes the time at which the person presses it).
func (j *Journal) Recover() (State, error) {
	j.mu.Lock()
	s, err := j.loadDraftLocked()
	if err != nil {
		j.mu.Unlock()
		return State{}, err
	}
	j.s = s
	j.exportDir = ""
	st := j.stateLocked()
	j.mu.Unlock()
	if j.onChange != nil {
		j.onChange(st)
	}
	return st, nil
}

// DiscardDraft deletes the draft without loading it.
func (j *Journal) DiscardDraft() {
	j.mu.Lock()
	j.removeDraftLocked()
	j.mu.Unlock()
}

func mustJSON(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err) // our own types: this cannot fail
	}
	return append(b, '\n')
}

// writeFileAtomic writes to a temporary file and renames it over the target.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
