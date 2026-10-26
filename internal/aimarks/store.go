package aimarks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// DraftsFile is the result of the last run, in the session folder.
const DraftsFile = "ai-marks.json"

// statsFile counts accepted and rejected drafts per model, in the app's data
// folder, so the player sees each model's real hit rate.
const statsFile = "ai-stats.json"

// LoadResult reads the drafts of a session. A session that was never run has
// none.
func LoadResult(sessionDir string) (Result, bool, error) {
	var res Result
	b, err := os.ReadFile(filepath.Join(sessionDir, DraftsFile))
	if errors.Is(err, os.ErrNotExist) {
		return res, false, nil
	}
	if err != nil {
		return res, false, err
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return res, false, fmt.Errorf("%s is not readable: %w", DraftsFile, err)
	}
	return res, true, nil
}

// SaveResult writes the drafts of a session.
func SaveResult(sessionDir string, res Result) error {
	return writeJSON(filepath.Join(sessionDir, DraftsFile), res)
}

// Decide marks a draft accepted or rejected and returns it. A draft decided
// before keeps its first decision, so the hit rate counts it once.
func (r *Result) Decide(id int, status string) (Draft, bool, error) {
	if status != StatusAccepted && status != StatusRejected {
		return Draft{}, false, fmt.Errorf("unknown decision %q", status)
	}
	for i := range r.Drafts {
		if r.Drafts[i].ID != id {
			continue
		}
		if r.Drafts[i].Status != StatusPending {
			return r.Drafts[i], false, nil
		}
		r.Drafts[i].Status = status
		return r.Drafts[i], true, nil
	}
	return Draft{}, false, fmt.Errorf("draft %d not found", id)
}

// Frames are needed only while the player reviews the drafts: each draft
// shows its frame. A new run makes fresh frames from the video.

// PruneFrames deletes the frames no draft shows, and the whole frames folder
// once every draft is accepted or rejected.
func PruneFrames(sessionDir string, res Result) error {
	dir := filepath.Join(sessionDir, FramesDirName)
	keep := map[string]bool{}
	for _, d := range res.Drafts {
		if d.Status == StatusPending {
			keep[filepath.Join(sessionDir, filepath.FromSlash(d.Frame))] = true
		}
	}
	if len(keep) == 0 {
		return os.RemoveAll(dir)
	}
	return filepath.WalkDir(dir, func(path string, e os.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return filepath.SkipDir
		}
		if err != nil || e.IsDir() || keep[path] {
			return err
		}
		return os.Remove(path)
	})
}

// HitRate is one model's record.
type HitRate struct {
	Model    string `json:"model"`
	Accepted int    `json:"accepted"`
	Rejected int    `json:"rejected"`
}

// LoadStats reads the hit rates, best first.
func LoadStats(dataDir string) ([]HitRate, error) {
	m, err := readStats(dataDir)
	if err != nil {
		return nil, err
	}
	out := make([]HitRate, 0, len(m))
	for _, h := range m {
		out = append(out, h)
	}
	sort.Slice(out, func(a, b int) bool { return rate(out[a]) > rate(out[b]) })
	return out, nil
}

func rate(h HitRate) float64 {
	if n := h.Accepted + h.Rejected; n > 0 {
		return float64(h.Accepted) / float64(n)
	}
	return 0
}

// CountDecisions adds accepted and rejected drafts to a model's record.
func CountDecisions(dataDir, model string, accepted, rejected int) error {
	m, err := readStats(dataDir)
	if err != nil {
		return err
	}
	h := m[model]
	h.Model = model
	h.Accepted += accepted
	h.Rejected += rejected
	m[model] = h
	return writeJSON(filepath.Join(dataDir, statsFile), m)
}

func readStats(dataDir string) (map[string]HitRate, error) {
	m := map[string]HitRate{}
	b, err := os.ReadFile(filepath.Join(dataDir, statsFile))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s is not readable: %w", statsFile, err)
	}
	return m, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
