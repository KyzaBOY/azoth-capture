// Package journal é o núcleo do Azoth Capture Journal: a sessão, os passos
// marcados e o rascunho em disco. Não conhece janela nem atalho; quem chama
// (app.go) injeta o relógio e recebe um aviso a cada mudança.
//
// O relógio é o do PC (Unix ms), o mesmo que o app de captura grava no
// ledger.bin, então os horários dos passos casam com os do ledger.
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

// Versão do app, gravada no steps.json ("azoth-capture 0.1.0").
const (
	AppName = "azoth-capture"
	Version = "0.1.0"
)

// Quem gerou o passo.
const (
	ViaButton    = "button"
	ViaHotkey    = "hotkey"
	ViaChecklist = "checklist"
)

// Fases da sessão.
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

// Activities é a lista do §7 do CONTRATOS.md, sem "Developer-only" (comando de
// teste que o jogo normal nunca manda; não há o que gravar).
var Activities = []string{
	"Fishing", "Combat", "Inventory & gear", "Chat", "Trading post", "Housing",
	"Expeditions", "Wars & PvP", "Crafting", "Gathering", "Quests",
	"Login & world entry", "Movement", "Social & groups", "Companies",
	"Progression", "Store & cosmetics", "Camping", "Other",
}

// Step é um passo marcado. ID só existe pra tela editar/apagar; não vai pro steps.json.
type Step struct {
	ID     int    `json:"id"`
	TMs    int64  `json:"t_ms"`
	Text   string `json:"text"`
	Target *int   `json:"target,omitempty"`
	Via    string `json:"via"`
}

// Session é o estado completo da sessão e também o formato do rascunho.
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
	// Exported: o steps.json atual já está em disco; qualquer edição zera.
	Exported bool  `json:"exported"`
	SavedMs  int64 `json:"saved_ms"`
}

// State é o que a tela recebe: a sessão mais o que só existe em memória.
type State struct {
	Session
	ExportDir  string `json:"export_dir"`
	DraftError string `json:"draft_error"`
}

// Journal guarda a sessão atual e salva o rascunho a cada mudança.
type Journal struct {
	mu        sync.Mutex
	now       func() time.Time
	draftPath string
	s         Session
	exportDir string
	draftErr  string
	onChange  func(State)
}

// New cria um Journal vazio. draftPath vazio desliga o rascunho (testes).
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

// State devolve uma cópia do estado atual.
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

// commit salva o rascunho, solta o lock e avisa a tela. Chamar com o lock preso.
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

// Start abre uma sessão nova. Nome ou atividade vazios viram "session" / "Other".
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
	targets := j.s.Targets // alvos escolhidos antes do Start valem pra sessão
	j.s = emptySession()
	j.s.Targets = targets
	j.s.Phase = PhaseRunning
	j.s.Name = name
	j.s.Activity = activity
	j.s.StartedMs = j.now().UnixMilli()
	j.exportDir = ""
	return j.commit(), nil
}

// Stop encerra a sessão. O rascunho fica até o export dar certo.
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

// Mark grava um passo com o horário `at` (o instante do clique ou da tecla,
// tomado por quem chama ANTES de qualquer trabalho). Texto vazio vira "Step N".
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

// EditStep troca o texto de um passo (o horário nunca muda).
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

// DeleteStep apaga um passo.
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

// SetTargets guarda os pacotes escolhidos pra gravar (sem repetição, na ordem).
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

// Reset descarta a sessão (e o rascunho) e volta pra tela inicial.
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

// cleanText tira quebras de linha (o steps.txt é uma linha por passo) e limita o tamanho.
func cleanText(s string) string {
	s = strings.Join(strings.Fields(strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(s)), " ")
	if r := []rune(s); len(r) > maxTextRunes {
		s = string(r[:maxTextRunes])
	}
	return s
}

// ---- rascunho ---------------------------------------------------------------

// persistLocked grava o rascunho de forma atômica (tmp + rename). Falha não
// interrompe a sessão; vira aviso na tela.
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

// markExported grava no rascunho que o steps.json atual já está no disco.
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

// DraftInfo resume um rascunho achado no disco.
type DraftInfo struct {
	Found     bool   `json:"found"`
	Name      string `json:"name"`
	Activity  string `json:"activity"`
	Phase     string `json:"phase"`
	Steps     int    `json:"steps"`
	StartedMs int64  `json:"started_ms"`
	SavedMs   int64  `json:"saved_ms"`
}

// CheckDraft olha o rascunho em disco. Um rascunho já exportado e sem edição
// depois não é oferecido: é apagado em silêncio. Ilegível também (fica .bad).
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
		os.Rename(j.draftPath, j.draftPath+".bad") // guarda pra inspeção, não oferece de novo
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

// Recover carrega o rascunho. Sessão que estava rodando continua rodando
// (o Stop dela vale a hora em que a pessoa apertar).
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

// DiscardDraft apaga o rascunho sem carregar.
func (j *Journal) DiscardDraft() {
	j.mu.Lock()
	j.removeDraftLocked()
	j.mu.Unlock()
}

func mustJSON(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err) // tipos próprios, não falha
	}
	return append(b, '\n')
}

// writeFileAtomic escreve em arquivo temporário e renomeia por cima.
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
