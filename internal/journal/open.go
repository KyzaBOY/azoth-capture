package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// OpenExported loads a stopped session back from its exported steps.json,
// for example to review its AI marks after the app was restarted. It works
// only while no session runs. The session comes back stopped and exported.
func (j *Journal) OpenExported(sessionDir string) (State, error) {
	b, err := os.ReadFile(filepath.Join(sessionDir, stepsName))
	if err != nil {
		return State{}, fmt.Errorf("%s has no %s: %w", sessionDir, stepsName, err)
	}
	var f StepsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return State{}, fmt.Errorf("%s is not readable: %w", stepsName, err)
	}
	s := emptySession()
	s.Phase, s.Name, s.Activity = PhaseStopped, f.Session, f.Activity
	s.StartedMs, s.StoppedMs = f.StartedMs, f.StoppedMs
	s.Targets = append([]int{}, f.Targets...)
	for _, e := range f.Steps {
		s.Steps = append(s.Steps, Step{ID: s.NextID, TMs: e.TMs, Text: e.Text, Target: e.Target, Via: e.Via})
		s.NextID++
	}

	j.mu.Lock()
	if j.s.Phase == PhaseRunning {
		j.mu.Unlock()
		return State{}, ErrAlreadyRunning
	}
	j.s = s
	j.mu.Unlock()
	return j.markExported(sessionDir), nil
}
