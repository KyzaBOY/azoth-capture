package journal

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"azoth/capture/internal/ledger"
)

// Splitting a session into one submission per connection. The upload site
// takes one zip per recording with ledger.bin and steps.json at its root.
// A part holds one whole connection (one login): cutting one connection into
// time slices makes uploads that overlap, so it is never done.
const (
	PartsDirName = "parts"
	sendDirName  = "send"
	ledgerName   = "ledger.bin"
	stepsName    = "steps.json"
)

var (
	ErrOneConnection = errors.New("this session has one connection, so there is nothing to split; send it as it is")
	ErrOverlap       = errors.New("the connections in this session overlap in time, so they cannot be sent as separate parts")
	ErrNoLedger      = errors.New("the send folder has no ledger.bin; stop the session first")
)

// Part is one upload-ready submission.
type Part struct {
	Number  int    `json:"number"`
	Zip     string `json:"zip"`
	FirstMs int64  `json:"first_ms"`
	LastMs  int64  `json:"last_ms"`
	Steps   int    `json:"steps"`
	Bytes   int64  `json:"bytes"`
	Problem string `json:"problem"` // why the site would reject it, or ""
}

// SplitResult lists the parts written into <session>/parts.
type SplitResult struct {
	Dir   string `json:"dir"`
	Parts []Part `json:"parts"`
}

// SplitByConnection writes one zip per connection into <sessionDir>/parts,
// from the send folder's ledger.bin and the session's steps.json. A step
// marked between two connections (while the game reconnected) goes with the
// connection that follows it; one marked after the last goes with the last.
func SplitByConnection(sessionDir string) (SplitResult, error) {
	ledgerPath := filepath.Join(sessionDir, sendDirName, ledgerName)
	if _, err := os.Stat(ledgerPath); err != nil {
		return SplitResult{}, ErrNoLedger
	}
	conns, _, err := ledger.Scan(ledgerPath)
	if err != nil {
		return SplitResult{}, err
	}
	if len(conns) < 2 {
		return SplitResult{}, ErrOneConnection
	}
	if !ledger.Sequential(conns) {
		return SplitResult{}, ErrOverlap
	}
	var steps StepsFile
	b, err := os.ReadFile(filepath.Join(sessionDir, stepsName))
	if err != nil {
		return SplitResult{}, err
	}
	if err := json.Unmarshal(b, &steps); err != nil {
		return SplitResult{}, fmt.Errorf("%s is not readable: %w", stepsName, err)
	}

	dir := filepath.Join(sessionDir, PartsDirName)
	if err := os.RemoveAll(dir); err != nil { // parts of an earlier split
		return SplitResult{}, err
	}
	ledgers := make([]string, len(conns))
	for i := range conns {
		partDir := filepath.Join(dir, fmt.Sprintf("part-%d", i+1))
		if err := os.MkdirAll(partDir, 0o755); err != nil {
			return SplitResult{}, err
		}
		ledgers[i] = filepath.Join(partDir, ledgerName)
	}
	if err := ledger.Split(ledgerPath, conns, ledgers); err != nil {
		return SplitResult{}, err
	}

	byPart := assignSteps(steps.Steps, conns)
	res := SplitResult{Dir: dir}
	for i, c := range conns {
		pf := steps
		pf.Session = fmt.Sprintf("%s part %d of %d", steps.Session, i+1, len(conns))
		pf.StartedMs, pf.StoppedMs = c.FirstMs, c.LastMs
		pf.Steps = byPart[i]
		for _, s := range pf.Steps {
			pf.StartedMs, pf.StoppedMs = min(pf.StartedMs, s.TMs), max(pf.StoppedMs, s.TMs)
		}
		if pf.Steps == nil {
			pf.Steps = []StepsEntry{}
		}
		stepsPath := filepath.Join(filepath.Dir(ledgers[i]), stepsName)
		if err := writeFileAtomic(stepsPath, mustJSON(pf)); err != nil {
			return SplitResult{}, err
		}
		zipPath := filepath.Join(dir, fmt.Sprintf("%s-part-%d.zip", FolderName(steps.Session), i+1))
		if err := writeZip(zipPath, ledgers[i], stepsPath); err != nil {
			return SplitResult{}, err
		}
		p := Part{Number: i + 1, Zip: zipPath, FirstMs: c.FirstMs, LastMs: c.LastMs, Steps: len(pf.Steps), Bytes: c.Bytes}
		if p.Steps == 0 {
			p.Problem = "no steps: the upload site rejects a part without steps"
		}
		res.Parts = append(res.Parts, p)
	}
	return res, nil
}

// UploadZip is the upload-ready zip of a whole session.
type UploadZip struct {
	Path     string   `json:"path"`
	Problems []string `json:"problems"` // why the site would reject it; empty when it looks fine
}

// Reasons the upload site rejects a recording, checked before zipping.
const (
	problemNoSteps   = "steps.json has no steps: the upload site rejects it. Mark or accept at least one step."
	problemNoLedger  = "ledger.bin is empty: nothing was recorded in the game world."
	problemManyLogin = "this recording has %d logins: press Split by connection and upload the parts instead."
)

// MakeUploadZip writes <session>.zip next to the session folder, with the
// send folder's ledger.bin and steps.json at its root: the form the upload
// site takes. It is written even when it has problems, so the player can see
// them and decide.
func MakeUploadZip(sessionDir string) (UploadZip, error) {
	ledgerPath := filepath.Join(sessionDir, sendDirName, ledgerName)
	stepsPath := filepath.Join(sessionDir, sendDirName, stepsName)
	var problems []string

	var steps StepsFile
	b, err := os.ReadFile(stepsPath)
	if err != nil {
		return UploadZip{}, err
	}
	if err := json.Unmarshal(b, &steps); err != nil {
		return UploadZip{}, fmt.Errorf("%s is not readable: %w", stepsName, err)
	}
	if len(steps.Steps) == 0 {
		problems = append(problems, problemNoSteps)
	}
	conns, _, err := ledger.Scan(ledgerPath)
	if err != nil {
		return UploadZip{}, err
	}
	switch {
	case len(conns) == 0:
		problems = append(problems, problemNoLedger)
	case len(conns) > 1:
		problems = append(problems, fmt.Sprintf(problemManyLogin, len(conns)))
	}

	zipPath := filepath.Clean(sessionDir) + ".zip"
	if err := writeZip(zipPath, ledgerPath, stepsPath); err != nil {
		return UploadZip{}, err
	}
	return UploadZip{Path: zipPath, Problems: problems}, nil
}

// assignSteps gives each step to the connection it happened in, the next
// connection when it happened between two, or the last one after the end.
func assignSteps(steps []StepsEntry, conns []ledger.Connection) [][]StepsEntry {
	out := make([][]StepsEntry, len(conns))
	for _, s := range steps {
		i := len(conns) - 1
		for j, c := range conns {
			if s.TMs <= c.LastMs {
				i = j
				break
			}
		}
		out[i] = append(out[i], s)
	}
	return out
}

// writeZip puts the files at the root of a new zip.
func writeZip(path string, files ...string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for _, name := range files {
		if err := addToZip(zw, name); err != nil {
			f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func addToZip(zw *zip.Writer, name string) error {
	src, err := os.Open(name)
	if err != nil {
		return err
	}
	defer src.Close()
	w, err := zw.Create(filepath.Base(name))
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	return err
}
