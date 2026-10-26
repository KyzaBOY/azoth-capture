package aimarks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"azoth/capture/internal/video"
)

// Pipeline settings.
const (
	FramesDirName  = "ai-frames" // in the session folder, next to the clips
	mergeGapMs     = 6000        // the same action seen again within this time is one action
	unsureBelow    = 0.6         // drafts below this confidence start unchecked
	DefaultWorkers = 2           // questions asked at the same time
)

// Action kinds the model may report.
var kinds = []string{"gather", "loot", "quest", "craft", "menu", "combat", "travel", "other"}

// Job is one run over a session's video.
type Job struct {
	SessionDir  string
	FFmpeg      string // the ffmpeg that reads the frames
	Clips       []video.Clip
	Model       Model
	Label       string // Config.Label of the model
	Data        GameData
	SampleEvery int // seconds between frames
	Workers     int
	Now         func() time.Time
	Progress    func(string)
}

// Result is what a run found. It is saved as DraftsFile.
type Result struct {
	Model      string   `json:"model"`
	StartedMs  int64    `json:"started_ms"`
	FinishedMs int64    `json:"finished_ms"`
	Frames     int      `json:"frames"`
	Parts      int      `json:"parts"`  // windows the model looked at
	Failed     int      `json:"failed"` // questions that got no usable answer
	Notes      []string `json:"notes"`
	Drafts     []Draft  `json:"drafts"`
}

// Draft is a proposed step. Frame is relative to the session folder.
type Draft struct {
	ID         int      `json:"id"`
	TMs        int64    `json:"t_ms"`
	Text       string   `json:"text"`
	Kind       string   `json:"kind"`
	ScreenText string   `json:"screen_text"`
	Candidates []string `json:"candidates"`
	Confidence float64  `json:"confidence"`
	Unsure     bool     `json:"unsure"`
	Frame      string   `json:"frame"`
	Status     string   `json:"status"`
}

// Draft states.
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

// seen is one action in the model's first answer.
type seen struct {
	Frame      int       `json:"frame"`
	Kind       string    `json:"kind"`
	ScreenText string    `json:"screen_text"`
	Summary    string    `json:"summary"`
	Position   []float64 `json:"position"`
	Confidence float64   `json:"confidence"`
}

// event is a seen action tied to its frame.
type event struct {
	seen
	clip  []Frame
	frame Frame
}

// verdict is the model's answer in the check pass.
type verdict struct {
	Keep       bool    `json:"keep"`
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
}

const findPrompt = `You are labelling a New World (MMO) gameplay recording. The %d images are frames taken every %d seconds, numbered 1 to %d in time order.

List each action the player STARTS in these frames. Kinds:
- gather: mining, logging, harvesting, skinning, fishing
- loot: opening a chest or container, picking up items
- quest: accepting, progressing or turning in a quest, talking to a quest NPC
- craft: using a crafting station or the crafting menu
- menu: opening the inventory, map, trading post, storage, settings or another game menu
- combat: starting a fight with a mob or a player
- travel: fast travel, mounting, dismounting, entering a town or settlement, respawning
- other: any other clear action

Rules:
- Loading screens, the login screen, and walking or running with no other action are not actions.
- screen_text: copy the exact name shown on screen for the thing acted on: a node name, a mob nameplate, a quest title, a station or menu title. Leave it empty if no name is shown. Never guess a name.
- position: if the screen shows the player's world coordinates as numbers, give [x, y]. Otherwise null.
- confidence: 0 to 1, how sure you are that the action happened.
- frame: the number of the first frame that shows the action.

Answer with JSON only, no other text:
{"events":[{"frame":1,"kind":"gather","screen_text":"","summary":"short description","position":null,"confidence":0.0}]}
If nothing happens, answer {"events":[]}`

const checkPrompt = `You are checking one label for a New World (MMO) gameplay recording. The %d images are frames %d seconds apart, in time order. Image %d is the moment of the action.

Proposed action: %s, "%s". Name read on screen: "%s".
Real game names that match it or spawn near the player: %s.

Decide if the proposed action really happens at that moment. If it does, write the step text: one short line in plain English that starts with a verb, for example "Mine Iron Vein (Medium)" or "Accept quest Soul Survivor". When one of the real game names fits what you see, use that exact name. Do not use a name that you cannot see and that is not in the list.

Answer with JSON only, no other text:
{"keep":true,"text":"...","confidence":0.0}`

// Run samples the clips, asks the model, checks names and returns drafts.
func Run(ctx context.Context, job Job) (Result, error) {
	// Empty lists, not nil: the window reads them as arrays.
	res := Result{Model: job.Label, StartedMs: job.Now().UnixMilli(), Notes: []string{}, Drafts: []Draft{}}
	if len(job.Clips) == 0 {
		return res, errors.New("this session has no video clips")
	}
	if job.Workers < 1 {
		job.Workers = DefaultWorkers
	}

	var windows []Window
	clipOf := map[string][]Frame{} // first frame path -> its clip's frames
	for i, c := range job.Clips {
		job.Progress(fmt.Sprintf("Reading frames from clip %d of %d…", i+1, len(job.Clips)))
		first := c.FirstFrameUnixMs
		if first == 0 {
			first = c.StartedUnixMs
		}
		outDir := filepath.Join(job.SessionDir, FramesDirName, strings.TrimSuffix(c.File, filepath.Ext(c.File)))
		if err := os.RemoveAll(outDir); err != nil { // frames of an earlier run with another interval
			return res, err
		}
		frames, err := SampleClip(ctx, job.FFmpeg, filepath.Join(job.SessionDir, c.File), outDir, job.SampleEvery, first)
		if err != nil {
			return res, err
		}
		res.Frames += len(frames)
		for _, w := range Windows(frames) {
			clipOf[w.Frames[0].Path] = frames
			windows = append(windows, w)
		}
	}
	res.Parts = len(windows)
	if len(windows) == 0 {
		res.Notes = append(res.Notes, "Nothing moved on screen in the video, so the model was not asked.")
		res.FinishedMs = job.Now().UnixMilli()
		return res, nil
	}

	var failed atomic.Int32
	var mu sync.Mutex
	var events []event
	var done atomic.Int32
	parallel(ctx, job.Workers, len(windows), func(i int) {
		w := windows[i]
		found, err := askWindow(ctx, job, w)
		job.Progress(fmt.Sprintf("Looked at part %d of %d…", done.Add(1), len(windows)))
		if err != nil {
			failed.Add(1)
			job.Progress("A question failed: " + err.Error())
			return
		}
		clip := clipOf[w.Frames[0].Path]
		mu.Lock()
		for _, s := range found {
			if s.Frame < 1 || s.Frame > len(w.Frames) {
				continue
			}
			events = append(events, event{seen: s, clip: clip, frame: w.Frames[s.Frame-1]})
		}
		mu.Unlock()
	})
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if int(failed.Load()) == len(windows) {
		return res, errors.New("every question to the model failed; see the activity log")
	}
	events = mergeEvents(events)

	notes := newNoteSet()
	drafts := make([]*Draft, len(events))
	done.Store(0)
	parallel(ctx, job.Workers, len(events), func(i int) {
		d, err := checkEvent(ctx, job, events[i], notes)
		job.Progress(fmt.Sprintf("Checked %d of %d actions…", done.Add(1), len(events)))
		if err != nil {
			failed.Add(1)
			job.Progress("A check failed: " + err.Error())
			return
		}
		drafts[i] = d
	})
	if err := ctx.Err(); err != nil {
		return res, err
	}
	for _, d := range drafts {
		if d != nil {
			d.ID = len(res.Drafts) + 1
			res.Drafts = append(res.Drafts, *d)
		}
	}
	res.Failed = int(failed.Load())
	res.Notes = append(res.Notes, notes.list()...)
	res.FinishedMs = job.Now().UnixMilli()
	return res, nil
}

func askWindow(ctx context.Context, job Job, w Window) ([]seen, error) {
	images := make([]string, len(w.Frames))
	for i, f := range w.Frames {
		images[i] = f.Path
	}
	answer, err := job.Model.Ask(ctx, fmt.Sprintf(findPrompt, len(images), job.SampleEvery, len(images)), images)
	if err != nil {
		return nil, err
	}
	var body struct {
		Events []seen `json:"events"`
	}
	if err := decodeJSON(answer, &body); err != nil {
		return nil, err
	}
	return body.Events, nil
}

func checkEvent(ctx context.Context, job Job, e event, notes *noteSet) (*Draft, error) {
	candidates := lookup(ctx, job.Data, e.seen, notes)
	lo, hi := max(e.frame.Index-1, 0), min(e.frame.Index+2, len(e.clip))
	images := make([]string, 0, hi-lo)
	for _, f := range e.clip[lo:hi] {
		images = append(images, f.Path)
	}
	list := "none found"
	if len(candidates) > 0 {
		list = strings.Join(candidates, "; ")
	}
	prompt := fmt.Sprintf(checkPrompt, len(images), job.SampleEvery, e.frame.Index-lo+1,
		e.Kind, e.Summary, e.ScreenText, list)
	answer, err := job.Model.Ask(ctx, prompt, images)
	if err != nil {
		return nil, err
	}
	var v verdict
	if err := decodeJSON(answer, &v); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(v.Text)
	if !v.Keep || text == "" {
		return nil, nil
	}
	rel, err := filepath.Rel(job.SessionDir, e.frame.Path)
	if err != nil {
		return nil, err
	}
	return &Draft{
		TMs: e.frame.UnixMs, Text: text, Kind: e.Kind, ScreenText: e.ScreenText, Candidates: candidates,
		Confidence: v.Confidence, Unsure: v.Confidence < unsureBelow, Frame: filepath.ToSlash(rel), Status: StatusPending,
	}, nil
}

// lookup lists real game names for what the model read and where the player
// stands. A failing source is noted once and does not stop the run.
func lookup(ctx context.Context, data GameData, s seen, notes *noteSet) []string {
	var found []Entry
	if s.ScreenText != "" {
		hits, err := data.Search(ctx, s.ScreenText)
		if err != nil {
			notes.add("Game data search failed: " + err.Error())
		}
		found = append(found, hits...)
	}
	if len(s.Position) == 2 {
		hits, err := data.Nearby(ctx, s.Position[0], s.Position[1])
		switch {
		case errors.Is(err, ErrNoNearby):
			notes.add("The screen showed coordinates, but online game data cannot list what spawns there. Pick a local game data folder for that.")
		case err != nil:
			notes.add("Game data nearby lookup failed: " + err.Error())
		}
		found = append(found, hits...)
	}
	names := make([]string, 0, len(found))
	for _, e := range uniqueNames(found, maxSearchResults+maxNearbyResults) {
		names = append(names, e.Name+" ("+e.Type+")")
	}
	return names
}

// mergeEvents keeps one event per action: the same kind and name seen again
// within mergeGapMs (often across two windows) is the same action.
func mergeEvents(in []event) []event {
	sort.SliceStable(in, func(a, b int) bool { return in[a].frame.UnixMs < in[b].frame.UnixMs })
	var out []event
	for _, e := range in {
		if !validKind(e.Kind) {
			e.Kind = "other"
		}
		if n := len(out); n > 0 {
			last := &out[n-1]
			if last.Kind == e.Kind && strings.EqualFold(last.ScreenText, e.ScreenText) &&
				e.frame.UnixMs-last.frame.UnixMs <= mergeGapMs {
				last.Confidence = max(last.Confidence, e.Confidence)
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

func validKind(k string) bool {
	for _, v := range kinds {
		if v == k {
			return true
		}
	}
	return false
}

// decodeJSON reads the first JSON object in a model's answer. Models often
// wrap it in a code fence or a sentence.
func decodeJSON(answer string, out any) error {
	start, end := strings.Index(answer, "{"), strings.LastIndex(answer, "}")
	if start < 0 || end < start {
		return fmt.Errorf("the model did not answer with JSON: %s", tail(answer, maxCLIErr))
	}
	if err := json.Unmarshal([]byte(answer[start:end+1]), out); err != nil {
		return fmt.Errorf("the model's JSON is not readable: %w", err)
	}
	return nil
}

// parallel runs fn(0..count-1) on n goroutines and stops starting new calls
// once ctx is done.
func parallel(ctx context.Context, n, count int, fn func(int)) {
	sem := make(chan struct{}, n)
	var wg sync.WaitGroup
	for i := range count {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			fn(i)
		}()
	}
	wg.Wait()
}

// noteSet collects messages once each, from many goroutines.
type noteSet struct {
	mu    sync.Mutex
	seen  map[string]bool
	order []string
}

func newNoteSet() *noteSet { return &noteSet{seen: map[string]bool{}} }

func (n *noteSet) add(msg string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.seen[msg] {
		n.seen[msg] = true
		n.order = append(n.order, msg)
	}
}

func (n *noteSet) list() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.order...)
}
