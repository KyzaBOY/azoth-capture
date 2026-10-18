package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// StepsFile é o steps.json (CONTRATOS.md §9). A ordem dos campos é a do contrato.
type StepsFile struct {
	Schema      int          `json:"schema"`
	App         string       `json:"app"`
	Session     string       `json:"session"`
	Activity    string       `json:"activity"`
	StartedMs   int64        `json:"started_ms"`
	StoppedMs   int64        `json:"stopped_ms"`
	TzOffsetMin int          `json:"tz_offset_min"`
	Targets     []int        `json:"targets,omitempty"`
	Steps       []StepsEntry `json:"steps"`
}

// StepsEntry é um passo no steps.json (sem o ID interno).
type StepsEntry struct {
	TMs    int64  `json:"t_ms"`
	Text   string `json:"text"`
	Target *int   `json:"target,omitempty"`
	Via    string `json:"via"`
}

// BuildStepsFile monta o steps.json de uma sessão. Se ela ainda roda, o
// stopped_ms é `now` (export parcial). tz_offset_min é o fuso do PC na hora do
// início, em minutos a leste de UTC (UTC-3 = -180, como no exemplo do contrato).
func BuildStepsFile(s Session, now time.Time, loc *time.Location) StepsFile {
	stopped := s.StoppedMs
	if s.Phase == PhaseRunning || stopped == 0 {
		stopped = now.UnixMilli()
	}
	_, off := time.UnixMilli(s.StartedMs).In(loc).Zone()
	f := StepsFile{
		Schema:      1,
		App:         AppName + " " + Version,
		Session:     s.Name,
		Activity:    s.Activity,
		StartedMs:   s.StartedMs,
		StoppedMs:   stopped,
		TzOffsetMin: off / 60,
		Steps:       make([]StepsEntry, 0, len(s.Steps)),
	}
	if len(s.Targets) > 0 {
		f.Targets = append([]int{}, s.Targets...)
	}
	for _, st := range s.Steps {
		f.Steps = append(f.Steps, StepsEntry{TMs: st.TMs, Text: st.Text, Target: st.Target, Via: st.Via})
	}
	return f
}

// FormatClock formata um instante como HH:MM:SS.mmm no fuso dado.
func FormatClock(ms int64, loc *time.Location) string {
	return time.UnixMilli(ms).In(loc).Format("15:04:05.000")
}

// FormatRel formata uma duração como +MM:SS.mmm (ou +H:MM:SS.mmm a partir de 1 h).
// Negativo (relógio andou pra trás) sai com "-".
func FormatRel(ms int64) string {
	sign := "+"
	if ms < 0 {
		sign, ms = "-", -ms
	}
	h, m, s, mm := ms/3600000, ms/60000%60, ms/1000%60, ms%1000
	if h > 0 {
		return fmt.Sprintf("%s%d:%02d:%02d.%03d", sign, h, m, s, mm)
	}
	return fmt.Sprintf("%s%02d:%02d.%03d", sign, m, s, mm)
}

// BuildStepsText monta o steps.txt legível (uma linha por passo).
func BuildStepsText(s Session, now time.Time, loc *time.Location) string {
	f := BuildStepsFile(s, now, loc)
	var b strings.Builder
	line := func(k, v string) { fmt.Fprintf(&b, "%-10s%s\n", k, v) }
	fmt.Fprintf(&b, "Azoth Capture Journal %s\n\n", Version)
	line("Session:", s.Name)
	line("Activity:", s.Activity)
	line("Started:", time.UnixMilli(f.StartedMs).In(loc).Format("2006-01-02 15:04:05.000")+" ("+tzLabel(f.TzOffsetMin)+")")
	line("Stopped:", time.UnixMilli(f.StoppedMs).In(loc).Format("2006-01-02 15:04:05.000"))
	if len(f.Targets) > 0 {
		ids := make([]string, len(f.Targets))
		for i, t := range f.Targets {
			ids[i] = fmt.Sprint(t)
		}
		line("Targets:", strings.Join(ids, ", "))
	}
	line("Clock:", "PC local time, the same clock as ts_ms in ledger.bin")
	b.WriteString("\n")
	if len(f.Steps) == 0 {
		b.WriteString("(no steps)\n")
		return b.String()
	}
	fmt.Fprintf(&b, "%3s  %-12s  %-12s  %-9s  %s\n", "#", "Local time", "Since start", "Via", "Text")
	for i, st := range f.Steps {
		text := st.Text
		if st.Target != nil {
			text += fmt.Sprintf("  [packet %d]", *st.Target)
		}
		fmt.Fprintf(&b, "%3d  %-12s  %-12s  %-9s  %s\n", i+1, FormatClock(st.TMs, loc),
			FormatRel(st.TMs-f.StartedMs), st.Via, text)
	}
	return b.String()
}

func tzLabel(offMin int) string {
	sign := "+"
	if offMin < 0 {
		sign, offMin = "-", -offMin
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, offMin/60, offMin%60)
}

var badNameChars = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)

// FolderName torna o nome da sessão um nome de pasta válido no Windows.
func FolderName(name string) string {
	n := strings.TrimSpace(badNameChars.ReplaceAllString(name, "-"))
	n = strings.TrimRight(n, ". ")
	if n == "" {
		return "session"
	}
	switch strings.ToUpper(strings.SplitN(n, ".", 2)[0]) { // nomes reservados do Windows
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		n = "_" + n
	}
	return n
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// SuggestName devolve "<atividade>-NN" com o primeiro NN livre em outDir.
func SuggestName(outDir, activity string) string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(activity), "-"), "-")
	if slug == "" {
		slug = "activity"
	}
	for n := 1; n < 1000; n++ {
		name := fmt.Sprintf("%s-%02d", slug, n)
		if _, err := os.Stat(filepath.Join(outDir, name)); os.IsNotExist(err) {
			return name
		}
	}
	return slug + "-1000"
}

// Export grava steps.json e steps.txt em <outDir>\<sessão>\ e devolve a pasta.
// Sessão parada e exportada vira "exportada" no rascunho (não é mais oferecida).
func (j *Journal) Export(outDir string) (string, error) {
	j.mu.Lock()
	if j.s.Phase == PhaseIdle {
		j.mu.Unlock()
		return "", ErrNotRunning
	}
	s := j.stateLocked().Session
	j.mu.Unlock()

	now := j.now()
	dir := filepath.Join(outDir, FolderName(s.Name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := writeFileAtomic(filepath.Join(dir, "steps.json"), mustJSON(BuildStepsFile(s, now, now.Location()))); err != nil {
		return "", err
	}
	if err := writeFileAtomic(filepath.Join(dir, "steps.txt"), []byte(BuildStepsText(s, now, now.Location()))); err != nil {
		return "", err
	}
	if s.Phase == PhaseStopped {
		j.markExported(dir)
	}
	return dir, nil
}
