package aimarks

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"azoth/capture/internal/proc"
)

// A subscription CLI runs logged in on the player's PC, so the player's plan
// pays for the questions. The app runs it once per question, without a
// session, with tools limited to reading the frames.

// cliSpec describes how to drive one CLI.
type cliSpec struct {
	bin string
	// listArgs and parseList read the model list; nil listArgs means the CLI
	// has no list command and staticModels is offered instead.
	listArgs     []string
	parseList    func([]byte) []ModelInfo
	staticModels []string
	// efforts are offered for every model when the list does not give them.
	efforts []string
	// args builds the command line for one question. dir holds the frames;
	// outFile is a file the CLI may write its final answer to.
	args func(c Config, prompt string, images []string, dir, outFile string) []string
	// stdinPrompt sends the prompt on stdin instead of the command line.
	stdinPrompt bool
	// imagesByPath means the CLI has no image flag: the prompt lists the frame
	// files and the CLI's read tool opens them.
	imagesByPath bool
	// readsOutFile means the answer is in outFile, not on stdout.
	readsOutFile bool
	// attachesFiles means the CLI attaches the files args gets (Pi and Oh My
	// Pi run without tools, so they cannot open a file themselves).
	attachesFiles bool
}

// CLI names, as stored in the settings.
const (
	CLICodex    = "codex"
	CLIClaude   = "claude"
	CLIOpenCode = "opencode"
	CLIGrok     = "grok"
	CLIDevin    = "devin"
	CLICursor   = "cursor"
	CLIPi       = "pi"
	CLIOhMyPi   = "omp"
)

// piThinking are the thinking levels of Pi and Oh My Pi, lowest first.
var piThinking = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

var cliSpecs = map[string]cliSpec{
	CLICodex: {
		bin: "codex", listArgs: []string{"debug", "models"}, parseList: parseCodexModels,
		stdinPrompt: true, readsOutFile: true,
		args: func(c Config, _ string, images []string, dir, outFile string) []string {
			a := []string{"exec", "--skip-git-repo-check", "-s", "read-only", "-C", dir, "-m", c.Model, "-o", outFile}
			if c.Effort != "" {
				a = append(a, "-c", "model_reasoning_effort="+strconv.Quote(c.Effort))
			}
			for _, img := range images {
				a = append(a, "-i", img)
			}
			return append(a, "--", "-")
		},
	},
	CLIClaude: {
		// Claude Code cannot list models, so the current full model names are
		// offered: an alias such as "sonnet" would hide which version runs.
		bin: "claude", staticModels: []string{"claude-haiku-5-5", "claude-sonnet-5-5", "claude-opus-5-5", "claude-fable-5-1"},
		efforts:     anthropicEfforts,
		stdinPrompt: true, imagesByPath: true,
		args: func(c Config, _ string, _ []string, dir, _ string) []string {
			a := []string{"-p", "--model", c.Model, "--allowedTools", "Read", "--add-dir", dir, "--output-format", "text"}
			if c.Effort != "" {
				a = append(a, "--effort", c.Effort)
			}
			return a
		},
	},
	CLIOpenCode: {
		bin: "opencode", listArgs: []string{"models"}, parseList: parseLineModels,
		args: func(c Config, prompt string, images []string, _ string, _ string) []string {
			a := []string{"run", prompt, "-m", c.Model}
			if c.Effort != "" {
				a = append(a, "--variant", c.Effort)
			}
			for _, img := range images {
				a = append(a, "-f", img)
			}
			return a
		},
	},
	CLIGrok: {
		bin: "grok", listArgs: []string{"models"}, parseList: parseGrokModels, efforts: openAIEfforts,
		imagesByPath: true,
		args: func(c Config, prompt string, _ []string, dir, _ string) []string {
			a := []string{"-p", prompt, "-m", c.Model, "--cwd", dir, "--allow", "Read", "--disable-web-search"}
			if c.Effort != "" {
				a = append(a, "--reasoning-effort", c.Effort)
			}
			return a
		},
	},
	CLIDevin: {
		bin: "devin", listArgs: []string{"models", "list"}, parseList: parseDevinModels,
		imagesByPath: true,
		args: func(c Config, prompt string, _ []string, _ string, _ string) []string {
			// Devin's reasoning level is part of the model name (swe-2-high).
			return []string{"--model", c.Model, "--permission-mode", "auto", "-p", prompt}
		},
	},
	CLICursor: {
		bin: "cursor-agent", listArgs: []string{"--list-models"}, parseList: parseCursorModels,
		imagesByPath: true,
		args: func(c Config, prompt string, _ []string, dir, _ string) []string {
			// "ask" mode is read-only. Cursor's effort is part of the model
			// name (gpt-5.3-codex-low), like Devin's.
			return []string{"-p", "--mode", "ask", "--output-format", "text", "--trust", "--workspace", dir, "--model", c.Model, prompt}
		},
	},
	CLIPi: {
		bin: "pi", listArgs: []string{"--list-models"}, parseList: parsePiModels, attachesFiles: true,
		args: func(c Config, prompt string, images []string, _ string, _ string) []string {
			// Extensions stay on: some providers (Ollama Cloud) come from one.
			return piArgs([]string{"-p", "--no-session", "--no-tools", "--no-context-files"}, c, prompt, images)
		},
	},
	CLIOhMyPi: {
		bin: "omp", listArgs: []string{"models", "--json"}, parseList: parseOhMyPiModels, attachesFiles: true,
		args: func(c Config, prompt string, images []string, dir string, _ string) []string {
			return piArgs([]string{"-p", "--no-session", "--no-tools", "--no-rules", "--no-skills", "--cwd", dir}, c, prompt, images)
		},
	},
}

// piArgs finishes a Pi or Oh My Pi command line: both attach a file to the
// message with an @ in front of its path.
func piArgs(a []string, c Config, prompt string, images []string) []string {
	a = append(a, "--model", c.Model)
	if c.Effort != "" {
		a = append(a, "--thinking", c.Effort)
	}
	for _, img := range images {
		a = append(a, "@"+img)
	}
	return append(a, prompt)
}

// InstalledCLIs lists the CLIs found on PATH, so the window offers only those.
func InstalledCLIs() []string {
	var out []string
	for _, n := range CLINames() {
		if _, err := exec.LookPath(cliSpecs[n].bin); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// CLINames lists the CLIs the app can drive.
func CLINames() []string {
	names := make([]string, 0, len(cliSpecs))
	for n := range cliSpecs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

type cliModel struct {
	c    Config
	spec cliSpec
}

func newCLI(c Config) *cliModel { return &cliModel{c: c, spec: cliSpecs[c.CLI]} }

// maxCLIErr is how much of a failing CLI's output goes into the message.
const maxCLIErr = 800

func (m *cliModel) Ask(ctx context.Context, prompt string, images []string) (string, error) {
	bin, err := exec.LookPath(m.spec.bin)
	if err != nil {
		return "", fmt.Errorf("%s is not installed or not on PATH", m.spec.bin)
	}
	dir := filepath.Dir(images[0])
	if m.spec.imagesByPath {
		prompt = withImagePaths(prompt, images)
	}
	tmp, err := os.MkdirTemp("", "azoth-ai-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	outFile := filepath.Join(tmp, "answer.txt")

	argPrompt, attach := prompt, images
	if isBatchLauncher(bin) && !m.spec.stdinPrompt {
		// A .cmd or .bat launcher (an npm install on Windows) breaks a
		// command line argument that has line breaks or quotes, so the
		// question goes in a file and the argument is one plain line.
		promptFile, err := writePromptFile(dir, prompt)
		if err != nil {
			return "", err
		}
		defer os.Remove(promptFile)
		if m.spec.attachesFiles {
			argPrompt, attach = attachedPromptLine, append(append([]string{}, images...), promptFile)
		} else {
			argPrompt = fmt.Sprintf(readPromptLine, promptFile)
		}
	}

	cmd := proc.Hidden(exec.CommandContext(ctx, bin, m.spec.args(m.c, argPrompt, attach, dir, outFile)...))
	cmd.Dir = dir
	if m.spec.stdinPrompt {
		cmd.Stdin = strings.NewReader(prompt)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s failed: %v: %s", m.spec.bin, err, tail(stderr.String()+stdout.String(), maxCLIErr))
	}
	if m.spec.readsOutFile {
		b, err := os.ReadFile(outFile)
		if err != nil {
			return "", fmt.Errorf("%s wrote no answer: %w", m.spec.bin, err)
		}
		return string(b), nil
	}
	return stdout.String(), nil
}

// The one-line prompts used with a batch launcher.
const (
	attachedPromptLine = "Follow the instructions in the attached text file exactly."
	readPromptLine     = "Read the file %s with your file read tool and follow its instructions exactly. Do not run commands or change any file."
)

// isBatchLauncher reports whether bin is a Windows .cmd or .bat file.
func isBatchLauncher(bin string) bool {
	ext := strings.ToLower(filepath.Ext(bin))
	return ext == ".cmd" || ext == ".bat"
}

func writePromptFile(dir, prompt string) (string, error) {
	f, err := os.CreateTemp(dir, "prompt-*.txt")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(prompt); err != nil {
		f.Close()
		return "", err
	}
	return f.Name(), f.Close()
}

// withImagePaths tells a CLI without an image flag which files to look at.
func withImagePaths(prompt string, images []string) string {
	var b strings.Builder
	b.WriteString("Open each of these image files with your file read tool and look at them, in this order:\n")
	for i, p := range images {
		fmt.Fprintf(&b, "Image %d: %s\n", i+1, p)
	}
	b.WriteString("Do not run commands or change any file.\n\n")
	b.WriteString(prompt)
	return b.String()
}

// tail shortens s to about n bytes. It keeps the start and the end: a CLI
// often prints its error first and a stack trace after it.
func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n/2] + " … " + s[len(s)-n/2:]
	}
	return s
}

func listCLI(ctx context.Context, c Config) ([]ModelInfo, error) {
	spec := cliSpecs[c.CLI]
	if spec.listArgs == nil {
		out := make([]ModelInfo, 0, len(spec.staticModels))
		for _, id := range spec.staticModels {
			out = append(out, ModelInfo{ID: id, Efforts: spec.efforts})
		}
		return out, nil
	}
	bin, err := exec.LookPath(spec.bin)
	if err != nil {
		return nil, fmt.Errorf("%s is not installed or not on PATH", spec.bin)
	}
	cmd := proc.Hidden(exec.CommandContext(ctx, bin, spec.listArgs...))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %v: %s", spec.bin, strings.Join(spec.listArgs, " "), err, tail(stderr.String()+string(out), maxCLIErr))
	}
	models := spec.parseList(out)
	if len(models) == 0 {
		return nil, fmt.Errorf("%s listed no models; is it logged in? %s", spec.bin, tail(string(out), maxCLIErr))
	}
	for i := range models {
		if models[i].Efforts == nil {
			models[i].Efforts = spec.efforts
		}
	}
	return models, nil
}

// parseCodexModels reads `codex debug models`: the listed models that take
// images, with their reasoning levels.
func parseCodexModels(b []byte) []ModelInfo {
	var cat struct {
		Models []struct {
			Slug            string   `json:"slug"`
			Visibility      string   `json:"visibility"`
			InputModalities []string `json:"input_modalities"`
			Levels          []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if json.Unmarshal(b, &cat) != nil {
		return nil
	}
	var out []ModelInfo
	for _, m := range cat.Models {
		if m.Visibility != "list" || !slices.Contains(m.InputModalities, "image") {
			continue
		}
		info := ModelInfo{ID: m.Slug, Efforts: []string{}}
		for _, l := range m.Levels {
			info.Efforts = append(info.Efforts, l.Effort)
		}
		out = append(out, info)
	}
	return out
}

// parseLineModels reads one model id per line (`opencode models`).
func parseLineModels(b []byte) []ModelInfo {
	var out []ModelInfo
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if id := strings.TrimSpace(sc.Text()); id != "" && !strings.ContainsAny(id, " \t") {
			out = append(out, ModelInfo{ID: id})
		}
	}
	return out
}

var grokModelLine = regexp.MustCompile(`^\s+[*-]\s+(\S+)`)

// parseGrokModels reads `grok models`: "  * grok-4.6 (default)", "  - grok-4.5".
func parseGrokModels(b []byte) []ModelInfo {
	var out []ModelInfo
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if m := grokModelLine.FindStringSubmatch(sc.Text()); m != nil {
			out = append(out, ModelInfo{ID: m[1]})
		}
	}
	return out
}

// parsePiModels reads `pi --list-models`: a table with the columns provider,
// model, context, max-out, thinking (yes/no) and images (yes/no).
func parsePiModels(b []byte) []ModelInfo {
	const (
		colProvider, colModel, colThinking, colImages = 0, 1, 4, 5
		columns                                       = 6
	)
	var out []ModelInfo
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != columns || f[colImages] != "yes" {
			continue // the header, or a model that cannot see the frames
		}
		info := ModelInfo{ID: f[colProvider] + "/" + f[colModel], Efforts: []string{}}
		if f[colThinking] == "yes" {
			info.Efforts = piThinking
		}
		out = append(out, info)
	}
	return out
}

// parseOhMyPiModels reads `omp models --json`: the chat models that take
// images, with their own thinking levels.
func parseOhMyPiModels(b []byte) []ModelInfo {
	var cat struct {
		Models []struct {
			Kind     string   `json:"kind"`
			Selector string   `json:"selector"`
			Thinking []string `json:"thinking"`
			Input    []string `json:"input"`
		} `json:"models"`
	}
	if json.Unmarshal(b, &cat) != nil {
		return nil
	}
	var out []ModelInfo
	for _, m := range cat.Models {
		if m.Kind != "chat" || !slices.Contains(m.Input, "image") {
			continue
		}
		out = append(out, ModelInfo{ID: m.Selector, Efforts: append([]string{}, m.Thinking...)})
	}
	return out
}

// cursorModelLine matches "gpt-5 - GPT-5": a model id, a dash, its name.
var cursorModelLine = regexp.MustCompile(`^\s*(\S+)\s+-\s+\S`)

// parseCursorModels reads `cursor-agent --list-models`.
func parseCursorModels(b []byte) []ModelInfo {
	var out []ModelInfo
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if m := cursorModelLine.FindStringSubmatch(sc.Text()); m != nil {
			out = append(out, ModelInfo{ID: m[1]})
		}
	}
	return out
}

// devinModelLine matches "  swe-2-high      SWE-2 High  [...]": a model id
// indented by two spaces and followed by its display name. Family lines are
// not indented, and alias lines have one space after "aliases:".
var devinModelLine = regexp.MustCompile(`^  (\S+)\s{2,}\S`)

// parseDevinModels reads `devin models list`.
func parseDevinModels(b []byte) []ModelInfo {
	var out []ModelInfo
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if m := devinModelLine.FindStringSubmatch(sc.Text()); m != nil {
			out = append(out, ModelInfo{ID: m[1]})
		}
	}
	return out
}
