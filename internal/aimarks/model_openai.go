package aimarks

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// openAIEfforts are the reasoning_effort values of the OpenAI-style API. A
// server that does not know the field ignores it or says so; the player then
// leaves the effort empty.
var openAIEfforts = []string{"low", "medium", "high"}

// apiTimeout covers one question with ten frames on a slow local model.
const apiTimeout = 10 * time.Minute

// maxErrorBody is how much of an error response goes into the message.
const maxErrorBody = 600

type openAI struct {
	c      Config
	client *http.Client
}

func newOpenAI(c Config) *openAI {
	return &openAI{c: c, client: &http.Client{Timeout: apiTimeout}}
}

func (o *openAI) url(path string) string { return strings.TrimRight(o.c.BaseURL, "/") + path }

func (o *openAI) do(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, o.url(path), rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.c.APIKey)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type openAIPart struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	ImageURL *openAIImageID `json:"image_url,omitempty"`
}

type openAIImageID struct {
	URL string `json:"url"`
}

type openAIChat struct {
	Model           string          `json:"model"`
	Messages        []openAIMessage `json:"messages"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
}

type openAIMessage struct {
	Role    string       `json:"role"`
	Content []openAIPart `json:"content"`
}

type openAIReply struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (o *openAI) Ask(ctx context.Context, prompt string, images []string) (string, error) {
	parts := []openAIPart{{Type: "text", Text: prompt}}
	for _, p := range images {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		parts = append(parts, openAIPart{Type: "image_url",
			ImageURL: &openAIImageID{URL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b)}})
	}
	var reply openAIReply
	err := o.do(ctx, http.MethodPost, "/chat/completions", openAIChat{
		Model: o.c.Model, ReasoningEffort: o.c.Effort,
		Messages: []openAIMessage{{Role: "user", Content: parts}},
	}, &reply)
	if err != nil {
		return "", err
	}
	if len(reply.Choices) == 0 {
		return "", fmt.Errorf("the model gave no answer")
	}
	return reply.Choices[0].Message.Content, nil
}

type openAIModels struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func listOpenAI(ctx context.Context, c Config) ([]ModelInfo, error) {
	var list openAIModels
	if err := newOpenAI(c).do(ctx, http.MethodGet, "/models", nil, &list); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(list.Data))
	for _, m := range list.Data {
		out = append(out, ModelInfo{ID: m.ID, Efforts: openAIEfforts})
	}
	return out, nil
}
