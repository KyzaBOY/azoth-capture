package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"azoth/capture/internal/aimarks"
	"azoth/capture/internal/config"
	"azoth/capture/internal/journal"
	"azoth/capture/internal/video"
)

// ---- AI marks (opt-in; needs video) -----------------------------------------
//
// AI marks are off until the player turns them on and picks a model. Then,
// after Stop, the app reads the session's video, asks the model what the
// player did, and shows drafts. A draft becomes a step in steps.json only when
// the player accepts it.

var (
	errAINeedsVideo  = errors.New("turn on video first; AI marks read the session's video")
	errAINoSession   = errors.New("stop a session with video first")
	errAIRunning     = errors.New("AI marks are already running")
	errAINoResult    = errors.New("no AI marks for this session")
	errAINoLocalData = errors.New("pick the local game data folder first")
	errAIStopped     = errors.New("stopped before it finished; no drafts were kept")
	errAINeedsFFmpeg = errors.New("AI marks need ffmpeg to read the video: install it from your distribution on Linux, or press Install in the Video row on Windows")
)

// listModelsTimeout bounds a model list request, including a CLI start.
const listModelsTimeout = 60 * time.Second

// aiState is the AI part of App, behind its own lock: a run takes minutes.
type aiState struct {
	mu         sync.Mutex
	cancel     context.CancelFunc
	progress   string
	err        string
	sessionDir string
	result     *aimarks.Result
}

// AIStatus is what the window needs to draw the AI controls and drafts.
type AIStatus struct {
	Settings config.AISettings `json:"settings"` // the API key is never sent to the window
	HasKey   bool              `json:"has_key"`
	CLIs     []string          `json:"clis"`
	Running  bool              `json:"running"`
	Progress string            `json:"progress"`
	Error    string            `json:"error"`
	Result   *aimarks.Result   `json:"result"`
	Stats    []aimarks.HitRate `json:"stats"`
}

func (a *App) aiConfig(s config.AISettings) aimarks.Config {
	return aimarks.Config{Connector: s.Connector, BaseURL: s.BaseURL, APIKey: s.APIKey, CLI: s.CLI, Model: s.Model, Effort: s.Effort}
}

// AIState returns the current AI status.
func (a *App) AIState() AIStatus {
	a.mu.Lock()
	set := a.set.AI
	a.mu.Unlock()
	stats, err := aimarks.LoadStats(config.DataDir())
	a.ai.mu.Lock()
	defer a.ai.mu.Unlock()
	st := AIStatus{
		HasKey: set.APIKey != "", CLIs: aimarks.InstalledCLIs(), Running: a.ai.cancel != nil,
		Progress: a.ai.progress, Error: a.ai.err, Result: a.ai.result, Stats: stats,
	}
	if err != nil && st.Error == "" {
		st.Error = err.Error()
	}
	set.APIKey = ""
	st.Settings = set
	return st
}

func (a *App) emitAI() { a.emit("ai", a.AIState()) }

// SetAISettings saves the AI settings. An empty API key keeps the saved one;
// clearKey forgets it.
func (a *App) SetAISettings(s config.AISettings, clearKey bool) (AIStatus, error) {
	a.mu.Lock()
	if s.Enabled && !a.set.VideoAllowed {
		a.mu.Unlock()
		return a.AIState(), errAINeedsVideo
	}
	if s.APIKey == "" && !clearKey {
		s.APIKey = a.set.AI.APIKey
	}
	s.SampleEvery = min(max(s.SampleEvery, aimarks.MinSampleEvery), aimarks.MaxSampleEvery)
	if s.DataSource != config.AIDataLocal {
		s.DataSource = config.AIDataOnline
	}
	a.set.AI = s
	err := a.saveLocked()
	a.mu.Unlock()
	a.emitAI()
	return a.AIState(), err
}

// ListAIModels asks the saved provider which models the player can use.
func (a *App) ListAIModels() ([]aimarks.ModelInfo, error) {
	a.mu.Lock()
	cfg := a.aiConfig(a.set.AI)
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(a.ctx, listModelsTimeout)
	defer cancel()
	return aimarks.ListModels(ctx, cfg)
}

// PickAIDataDir lets the player choose a local game data folder.
func (a *App) PickAIDataDir() (AIStatus, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Game data folder (an nwdb.info copy)"})
	if err != nil || dir == "" {
		return a.AIState(), err
	}
	if _, err := aimarks.NewLocal(dir, config.DataDir()); err != nil {
		return a.AIState(), err
	}
	a.mu.Lock()
	a.set.AI.DataDir = dir
	a.set.AI.DataSource = config.AIDataLocal
	err = a.saveLocked()
	a.mu.Unlock()
	a.emitAI()
	return a.AIState(), err
}

func (a *App) gameData(s config.AISettings) (aimarks.GameData, error) {
	if s.DataSource == config.AIDataLocal {
		if s.DataDir == "" {
			return nil, errAINoLocalData
		}
		return aimarks.NewLocal(s.DataDir, config.DataDir())
	}
	return aimarks.NewOnline(aimarks.DefaultOnlineURL, journal.AppName+"/"+journal.Version, nil), nil
}

// startAIAfterStop runs AI marks after Stop when the player turned them on.
// A setup problem is shown in the AI panel instead of failing the stop.
func (a *App) startAIAfterStop(sessionDir string) {
	a.mu.Lock()
	on := a.set.AI.Enabled
	a.mu.Unlock()
	if !on {
		return
	}
	if err := a.startAI(sessionDir); err != nil {
		a.ai.mu.Lock()
		a.ai.err = err.Error()
		a.ai.mu.Unlock()
		a.emitAI()
	}
}

// RunAIMarks runs AI marks again on the stopped session, for example with
// another model.
func (a *App) RunAIMarks() (AIStatus, error) {
	st := a.j.State()
	if st.Phase != journal.PhaseStopped || st.ExportDir == "" {
		return a.AIState(), errAINoSession
	}
	err := a.startAI(st.ExportDir)
	return a.AIState(), err
}

func (a *App) startAI(sessionDir string) error {
	a.mu.Lock()
	set := a.set.AI
	videoOn := a.set.VideoAllowed
	a.mu.Unlock()
	if !videoOn {
		return errAINeedsVideo
	}
	cfg := a.aiConfig(set)
	model, err := aimarks.NewModel(cfg)
	if err != nil {
		return err
	}
	data, err := a.gameData(set)
	if err != nil {
		return err
	}
	clips, err := video.ReadIndex(sessionDir)
	if err != nil {
		return err
	}
	ffmpeg, err := video.FFmpegPath(toolsDir())
	if err != nil {
		return errAINeedsFFmpeg
	}

	a.ai.mu.Lock()
	if a.ai.cancel != nil {
		a.ai.mu.Unlock()
		return errAIRunning
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.ai.cancel, a.ai.err, a.ai.progress, a.ai.sessionDir, a.ai.result = cancel, "", "Starting…", sessionDir, nil
	a.ai.mu.Unlock()
	a.emitAI()
	a.emit("caplog", "AI marks: started with "+cfg.Label()+".")

	go func() {
		res, err := aimarks.Run(ctx, aimarks.Job{
			SessionDir: sessionDir, FFmpeg: ffmpeg, Clips: clips, Model: model, Label: cfg.Label(), Data: data,
			SampleEvery: set.SampleEvery, Workers: aimarks.DefaultWorkers, Now: time.Now,
			Progress: func(msg string) {
				a.ai.mu.Lock()
				a.ai.progress = msg
				a.ai.mu.Unlock()
				a.emitAI()
			},
		})
		if err == nil {
			err = aimarks.SaveResult(sessionDir, res)
		}
		if err == nil {
			a.pruneFrames(sessionDir, res)
		}
		if errors.Is(err, context.Canceled) {
			err = errAIStopped
		}
		a.ai.mu.Lock()
		a.ai.cancel, a.ai.progress = nil, ""
		if a.ai.sessionDir != sessionDir { // a new session started meanwhile
			a.ai.mu.Unlock()
			cancel()
			return
		}
		if err != nil {
			a.ai.err = err.Error()
		} else {
			a.ai.result = &res
		}
		a.ai.mu.Unlock()
		cancel()
		if err != nil {
			a.emit("caplog", "AI marks: "+err.Error())
		} else {
			a.emit("caplog", fmt.Sprintf("AI marks: %d draft(s) from %d part(s) of video.", len(res.Drafts), res.Parts))
		}
		a.emitAI()
	}()
	return nil
}

// CancelAIMarks stops a running run.
func (a *App) CancelAIMarks() AIStatus {
	a.ai.mu.Lock()
	if a.ai.cancel != nil {
		a.ai.cancel()
	}
	a.ai.mu.Unlock()
	return a.AIState()
}

// resetAI stops a run and forgets the drafts, for a new session.
func (a *App) resetAI() {
	a.ai.mu.Lock()
	if a.ai.cancel != nil {
		a.ai.cancel()
	}
	a.ai.err, a.ai.progress, a.ai.sessionDir, a.ai.result = "", "", "", nil
	a.ai.mu.Unlock()
	a.emitAI()
}

// LoadAIMarks shows the saved drafts of the stopped session, for example after
// the app was restarted and the session recovered.
func (a *App) LoadAIMarks() (AIStatus, error) {
	st := a.j.State()
	if st.ExportDir == "" {
		return a.AIState(), errAINoSession
	}
	res, found, err := aimarks.LoadResult(st.ExportDir)
	if err != nil || !found {
		return a.AIState(), err
	}
	a.pruneFrames(st.ExportDir, res)
	a.ai.mu.Lock()
	a.ai.sessionDir, a.ai.result = st.ExportDir, &res
	a.ai.mu.Unlock()
	return a.AIState(), nil
}

// pruneFrames deletes the frames the review no longer needs. A failure only
// leaves files behind, so it is logged, not returned.
func (a *App) pruneFrames(sessionDir string, res aimarks.Result) {
	if err := aimarks.PruneFrames(sessionDir, res); err != nil {
		a.emit("caplog", "AI marks: could not delete old frames: "+err.Error())
	}
}

// OpenSession loads a stopped session from the captures folder, with its AI
// marks, so it can be reviewed after the app was restarted.
func (a *App) OpenSession() (journal.State, error) {
	out := a.outputDir()
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Open a saved session", DefaultDirectory: out})
	if err != nil || dir == "" {
		return a.j.State(), err
	}
	if filepath.Dir(filepath.Clean(dir)) != filepath.Clean(out) {
		return a.j.State(), fmt.Errorf("pick a session folder inside %s", out)
	}
	a.resetAI()
	st, err := a.j.OpenExported(dir)
	if err != nil {
		return a.j.State(), err
	}
	if _, err := a.LoadAIMarks(); err != nil {
		a.emit("caplog", "AI marks: "+err.Error())
	}
	a.emitAI()
	a.refreshUpload(dir)
	return st, nil
}

// AIDecision is the player's verdict on one draft. Text is the step text
// to use when accepting; empty keeps the draft's own text.
type AIDecision struct {
	ID     int    `json:"id"`
	Accept bool   `json:"accept"`
	Text   string `json:"text"`
}

// DecideAIDrafts accepts or rejects drafts, one or many at once. Accepted
// drafts become steps; steps.json, the send folder and the upload zip are
// then written once for the whole batch.
func (a *App) DecideAIDrafts(decisions []AIDecision) (AIStatus, error) {
	a.ai.mu.Lock()
	res, dir := a.ai.result, a.ai.sessionDir
	if res == nil {
		a.ai.mu.Unlock()
		return a.AIState(), errAINoResult
	}
	var accepted []aimarks.Draft
	var rejected int
	var err error
	for _, dec := range decisions {
		status := aimarks.StatusRejected
		if dec.Accept {
			status = aimarks.StatusAccepted
		}
		d, changed, derr := res.Decide(dec.ID, status)
		if derr != nil {
			err = derr
			break
		}
		if !changed {
			continue
		}
		if dec.Accept {
			if dec.Text != "" {
				d.Text = dec.Text
			}
			accepted = append(accepted, d)
		} else {
			rejected++
		}
	}
	changed := len(accepted)+rejected > 0
	if changed {
		if serr := aimarks.SaveResult(dir, *res); err == nil {
			err = serr
		}
		a.pruneFrames(dir, *res)
	}
	model := res.Model
	a.ai.mu.Unlock()
	if !changed {
		return a.AIState(), err
	}
	if cerr := aimarks.CountDecisions(config.DataDir(), model, len(accepted), rejected); cerr != nil {
		a.emit("caplog", "AI marks: could not save the hit rate: "+cerr.Error())
	}
	for _, d := range accepted {
		if _, aerr := a.j.AddReviewedStep(time.UnixMilli(d.TMs), d.Text); aerr != nil {
			return a.AIState(), aerr
		}
	}
	if len(accepted) > 0 {
		exportDir, eerr := a.j.Export(a.outputDir())
		if eerr != nil {
			return a.AIState(), fmt.Errorf("the steps were added, but steps.json was not written: %w", eerr)
		}
		a.refreshUpload(exportDir) // the window hears the result as "upload"
	}
	a.emitAI()
	return a.AIState(), err
}

// AIFrame returns a draft's frame as a data URL for the window, or "" when
// the frame was already cleaned up (the draft was decided).
func (a *App) AIFrame(id int) (string, error) {
	a.ai.mu.Lock()
	res, dir := a.ai.result, a.ai.sessionDir
	a.ai.mu.Unlock()
	if res == nil {
		return "", errAINoResult
	}
	for _, d := range res.Drafts {
		if d.ID == id {
			b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(d.Frame)))
			if errors.Is(err, os.ErrNotExist) {
				return "", nil
			}
			if err != nil {
				return "", err
			}
			return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b), nil
		}
	}
	return "", fmt.Errorf("draft %d not found", id)
}
