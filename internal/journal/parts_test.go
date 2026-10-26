package journal

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"azoth/capture/internal/ledger"
)

const (
	recMagic  = 0
	recTime   = 4
	recSSL    = 16
	recLength = 24
)

func ledgerRec(ts int64, ssl uint64, payload string) []byte {
	b := make([]byte, ledger.HeaderSize, ledger.HeaderSize+len(payload))
	binary.LittleEndian.PutUint32(b[recMagic:], ledger.Magic)
	binary.LittleEndian.PutUint64(b[recTime:], uint64(ts))
	binary.LittleEndian.PutUint64(b[recSSL:], ssl)
	binary.LittleEndian.PutUint32(b[recLength:], uint32(len(payload)))
	return append(b, payload...)
}

// session writes an exported session folder with a ledger in send/.
func session(t *testing.T, steps []StepsEntry, recs ...[]byte) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, sendDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sendDirName, ledgerName), bytes.Join(recs, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	f := StepsFile{Schema: 1, App: "azoth-capture test", Session: "relog test", Activity: "Other", StartedMs: 0, StoppedMs: 9000, Steps: steps}
	if err := writeFileAtomic(filepath.Join(dir, stepsName), mustJSON(f)); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readZip(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = b
	}
	return out
}

func TestSplitByConnectionOnePartPerLogin(t *testing.T) {
	first := [][]byte{ledgerRec(1000, 0xA, "a"), ledgerRec(2000, 0xA, "b")}
	second := ledgerRec(5000, 0xB, "c")
	dir := session(t, []StepsEntry{
		{TMs: 500, Text: "before the first login", Via: ViaButton},
		{TMs: 1500, Text: "in the first", Via: ViaHotkey},
		{TMs: 3000, Text: "while reconnecting", Via: ViaHotkey},
		{TMs: 9000, Text: "after the last", Via: ViaAI},
	}, first[0], first[1], second)

	res, err := SplitByConnection(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Parts) != 2 {
		t.Fatalf("parts = %+v", res.Parts)
	}
	wantSteps := [][]string{{"before the first login", "in the first"}, {"while reconnecting", "after the last"}}
	wantLedger := [][]byte{bytes.Join(first, nil), second}
	for i, p := range res.Parts {
		if filepath.Base(p.Zip) != "relog test-part-"+string(rune('1'+i))+".zip" || p.Problem != "" {
			t.Errorf("part %d = %+v", i+1, p)
		}
		files := readZip(t, p.Zip)
		if len(files) != 2 || !bytes.Equal(files[ledgerName], wantLedger[i]) {
			t.Errorf("part %d zip holds %d files; ledger matches = %v", i+1, len(files), bytes.Equal(files[ledgerName], wantLedger[i]))
		}
		var sf StepsFile
		if err := json.Unmarshal(files[stepsName], &sf); err != nil {
			t.Fatal(err)
		}
		var texts []string
		for _, s := range sf.Steps {
			texts = append(texts, s.Text)
		}
		if !slices.Equal(texts, wantSteps[i]) || sf.Session != "relog test part "+string(rune('1'+i))+" of 2" {
			t.Errorf("part %d steps.json = %+v", i+1, sf)
		}
		if sf.StartedMs > sf.Steps[0].TMs || sf.StoppedMs < sf.Steps[len(sf.Steps)-1].TMs {
			t.Errorf("part %d time range %d-%d does not cover its steps", i+1, sf.StartedMs, sf.StoppedMs)
		}
	}
}

func TestMakeUploadZip(t *testing.T) {
	recs := [][]byte{ledgerRec(1000, 0xA, "a"), ledgerRec(2000, 0xA, "b")}
	dir := session(t, []StepsEntry{{TMs: 1500, Text: "mine", Via: ViaHotkey}}, recs...)
	// MakeUploadZip zips the send folder's steps.json, as Stop writes it.
	b, _ := os.ReadFile(filepath.Join(dir, stepsName))
	if err := os.WriteFile(filepath.Join(dir, sendDirName, stepsName), b, 0o644); err != nil {
		t.Fatal(err)
	}
	up, err := MakeUploadZip(dir)
	if err != nil {
		t.Fatal(err)
	}
	if up.Path != filepath.Clean(dir)+".zip" || len(up.Problems) != 0 {
		t.Errorf("upload = %+v", up)
	}
	files := readZip(t, up.Path)
	if len(files) != 2 || !bytes.Equal(files[ledgerName], bytes.Join(recs, nil)) || !bytes.Equal(files[stepsName], b) {
		t.Errorf("zip holds %d files", len(files))
	}
}

func TestMakeUploadZipFlagsWhatTheSiteRejects(t *testing.T) {
	dir := session(t, nil, ledgerRec(1000, 0xA, "a"), ledgerRec(5000, 0xB, "b"))
	b, _ := os.ReadFile(filepath.Join(dir, stepsName))
	os.WriteFile(filepath.Join(dir, sendDirName, stepsName), b, 0o644)
	up, err := MakeUploadZip(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(up.Problems) != 2 {
		t.Errorf("problems = %q, want no steps and two logins", up.Problems)
	}
}

func TestSplitFlagsPartWithoutSteps(t *testing.T) {
	dir := session(t, []StepsEntry{{TMs: 6000, Text: "only in the second", Via: ViaHotkey}},
		ledgerRec(1000, 0xA, "a"), ledgerRec(5000, 0xB, "b"))
	res, err := SplitByConnection(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Parts[0].Problem == "" || res.Parts[1].Problem != "" {
		t.Errorf("parts = %+v: the part without steps must be flagged", res.Parts)
	}
}

func TestSplitRefusesOneOrOverlappingConnections(t *testing.T) {
	one := session(t, nil, ledgerRec(1000, 0xA, "a"), ledgerRec(2000, 0xA, "b"))
	if _, err := SplitByConnection(one); !errors.Is(err, ErrOneConnection) {
		t.Errorf("one connection: %v", err)
	}
	overlap := session(t, nil, ledgerRec(1000, 0xA, "a"), ledgerRec(1500, 0xB, "b"), ledgerRec(2000, 0xA, "c"))
	if _, err := SplitByConnection(overlap); !errors.Is(err, ErrOverlap) {
		t.Errorf("overlap: %v", err)
	}
	if _, err := SplitByConnection(t.TempDir()); !errors.Is(err, ErrNoLedger) {
		t.Errorf("no ledger: %v", err)
	}
}
