package aimarks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConfigCheck(t *testing.T) {
	for _, c := range []Config{
		{},
		{Connector: ConnectorOpenAI, Model: "m"},
		{Connector: ConnectorAnthropic, Model: "m"},
		{Connector: ConnectorCLI, CLI: "nope", Model: "m"},
		{Connector: ConnectorCLI, CLI: CLICodex},
	} {
		if c.Check() == nil {
			t.Errorf("%+v passed the check", c)
		}
	}
	ok := Config{Connector: ConnectorOpenAI, BaseURL: "http://127.0.0.1:1/v1", Model: "llava"}
	if err := ok.Check(); err != nil {
		t.Errorf("a local server without a key must pass: %v", err)
	}
	if got := (Config{Connector: ConnectorCLI, CLI: CLICodex, Model: "gpt-6-sol", Effort: "low"}).Label(); got != "codex: gpt-6-sol @ low" {
		t.Errorf("label = %q", got)
	}
}

func TestOpenAIAskSendsImagesAndEffort(t *testing.T) {
	img := filepath.Join(t.TempDir(), "f.jpg")
	if err := os.WriteFile(img, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/models":
			w.Write([]byte(`{"data":[{"id":"llava"},{"id":"qwen-vl"}]}`))
		case "/v1/chat/completions":
			var req openAIChat
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			parts := req.Messages[0].Content
			if req.Model != "llava" || req.ReasoningEffort != "low" || len(parts) != 2 ||
				parts[1].ImageURL.URL != "data:image/jpeg;base64,anBlZw==" {
				t.Errorf("request = %+v", req)
			}
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"events\":[]}"}}]}`))
		default:
			t.Errorf("path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c := Config{Connector: ConnectorOpenAI, BaseURL: srv.URL + "/v1/", APIKey: "k", Model: "llava", Effort: "low"}

	models, err := ListModels(context.Background(), c)
	if err != nil || len(models) != 2 || !slices.Equal(models[0].Efforts, openAIEfforts) {
		t.Fatalf("models = %+v, %v", models, err)
	}
	m, err := NewModel(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Ask(context.Background(), "what happens?", []string{img})
	if err != nil || got != `{"events":[]}` {
		t.Errorf("answer = %q, %v", got, err)
	}
}

func TestOpenAIErrorShowsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model does not support images", http.StatusBadRequest)
	}))
	defer srv.Close()
	_, err := ListModels(context.Background(), Config{Connector: ConnectorOpenAI, BaseURL: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "does not support images") {
		t.Errorf("err = %v", err)
	}
}

func TestParseCodexModelsKeepsListedImageModels(t *testing.T) {
	got := parseCodexModels([]byte(`{"models":[
		{"slug":"gpt-6-sol","visibility":"list","input_modalities":["text","image"],
		 "supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"}]},
		{"slug":"hidden","visibility":"hide","input_modalities":["text","image"]},
		{"slug":"text-only","visibility":"list","input_modalities":["text"]}]}`))
	if len(got) != 1 || got[0].ID != "gpt-6-sol" || !slices.Equal(got[0].Efforts, []string{"low", "medium"}) {
		t.Errorf("codex models = %+v", got)
	}
}

func TestParseCLIModelLists(t *testing.T) {
	grok := parseGrokModels([]byte("You are not authenticated.\n\nDefault model: grok-4.6\n\nAvailable models:\n  * grok-4.6 (default)\n  - grok-4.5\n"))
	devin := parseDevinModels([]byte("Available models (44 families)\n\nSWE-2 (swe-2)\n  aliases: swe\n" +
		"  swe-2-high                      SWE-2 High  [262K context, Free]\n" +
		"  swe-2-medium                    SWE-2 Medium  [262K context, Free]\n"))
	open := parseLineModels([]byte("opencode/big-pickle\nollama-cloud/gpt-oss:120b\n\nnot a model id\n"))
	ids := func(ms []ModelInfo) []string {
		var out []string
		for _, m := range ms {
			out = append(out, m.ID)
		}
		return out
	}
	for name, c := range map[string]struct{ got, want []string }{
		"grok":     {ids(grok), []string{"grok-4.6", "grok-4.5"}},
		"devin":    {ids(devin), []string{"swe-2-high", "swe-2-medium"}},
		"opencode": {ids(open), []string{"opencode/big-pickle", "ollama-cloud/gpt-oss:120b"}},
	} {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("%s = %v, want %v", name, c.got, c.want)
		}
	}
}

func TestParsePiFamilyAndCursorModels(t *testing.T) {
	pi := parsePiModels([]byte("provider      model             context  max-out  thinking  images\n" +
		"anthropic     claude-haiku-5-5  1M       128K     yes       yes   \n" +
		"ollama-cloud  beam              1.0M     32.8K    yes       no    \n" +
		"openai        gpt-mini          400K     128K     no        yes   \n"))
	if len(pi) != 2 || pi[0].ID != "anthropic/claude-haiku-5-5" || !slices.Equal(pi[0].Efforts, piThinking) || len(pi[1].Efforts) != 0 {
		t.Errorf("pi models = %+v: only image models, thinking levels only where thinking is yes", pi)
	}
	omp := parseOhMyPiModels([]byte(`{"models":[
		{"kind":"chat","selector":"ollama-cloud/clef:27b","thinking":null,"input":["text","image"]},
		{"kind":"chat","selector":"ollama-cloud/beam","thinking":["low"],"input":["text"]},
		{"kind":"embedding","selector":"x/embed","input":["text","image"]},
		{"kind":"chat","selector":"anthropic/claude-sonnet-5-5","thinking":["low","high"],"input":["text","image"]}]}`))
	if len(omp) != 2 || omp[1].ID != "anthropic/claude-sonnet-5-5" || !slices.Equal(omp[1].Efforts, []string{"low", "high"}) {
		t.Errorf("omp models = %+v", omp)
	}
	cursor := parseCursorModels([]byte("Available models\n\nauto - Auto\ngpt-5 - GPT-5\nsonnet-4-thinking - Sonnet 4 Thinking\n"))
	if len(cursor) != 3 || cursor[2].ID != "sonnet-4-thinking" {
		t.Errorf("cursor models = %+v", cursor)
	}
}

func TestPiFamilyAndCursorArgs(t *testing.T) {
	c := Config{Model: "anthropic/claude-haiku-5-5", Effort: "low"}
	pi := cliSpecs[CLIPi].args(c, "q", []string{"/f/a.jpg"}, "/f", "")
	want := []string{"-p", "--no-session", "--no-tools", "--no-context-files", "--model", "anthropic/claude-haiku-5-5", "--thinking", "low", "@/f/a.jpg", "q"}
	if !slices.Equal(pi, want) {
		t.Errorf("pi args = %q", pi)
	}
	cursor := cliSpecs[CLICursor].args(Config{Model: "gpt-5.3-codex-low"}, "q", nil, "/f", "")
	if !slices.Contains(cursor, "gpt-5.3-codex-low") || !slices.Contains(cursor, "ask") {
		t.Errorf("cursor args = %q: read-only ask mode", cursor)
	}
	if got := tail("first line error\n"+strings.Repeat("stack ", 200)+"end", 40); !strings.HasPrefix(got, "first line") || !strings.HasSuffix(got, "end") {
		t.Errorf("tail = %q: keep the start and the end", got)
	}
}

func TestClaudeListIsStatic(t *testing.T) {
	got, err := ListModels(context.Background(), Config{Connector: ConnectorCLI, CLI: CLIClaude})
	if err != nil || len(got) != 4 || got[1].ID != "claude-sonnet-5-5" || !slices.Equal(got[0].Efforts, anthropicEfforts) {
		t.Errorf("claude models = %+v, %v", got, err)
	}
}

func TestCLIArgs(t *testing.T) {
	c := Config{Model: "m", Effort: "low"}
	codex := cliSpecs[CLICodex].args(c, "p", []string{"/f/a.jpg", "/f/b.jpg"}, "/f", "/tmp/o")
	want := []string{"exec", "--skip-git-repo-check", "-s", "read-only", "-C", "/f", "-m", "m", "-o", "/tmp/o",
		"-c", `model_reasoning_effort="low"`, "-i", "/f/a.jpg", "-i", "/f/b.jpg", "--", "-"}
	if !slices.Equal(codex, want) {
		t.Errorf("codex args = %q", codex)
	}
	claude := cliSpecs[CLIClaude].args(Config{Model: "claude-sonnet-5-5"}, "p", nil, "/f", "")
	if slices.Contains(claude, "--effort") || !slices.Contains(claude, "Read") {
		t.Errorf("claude args = %q: no effort when none is set, and only the Read tool", claude)
	}
	if p := withImagePaths("question", []string{"/f/a.jpg"}); !strings.Contains(p, "Image 1: /f/a.jpg") || !strings.HasSuffix(p, "question") {
		t.Errorf("prompt = %q", p)
	}
}
