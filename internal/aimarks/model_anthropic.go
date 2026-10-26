package aimarks

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicEfforts are the effort levels of the Anthropic API, lowest first.
var anthropicEfforts = []string{"low", "medium", "high", "xhigh", "max"}

// anthropicMaxTokens is enough for the JSON answers the pipeline asks for.
const anthropicMaxTokens = 16000

type anthropicModel struct {
	c      Config
	client anthropic.Client
}

func newAnthropic(c Config) *anthropicModel {
	return &anthropicModel{c: c, client: anthropic.NewClient(option.WithAPIKey(c.APIKey), option.WithRequestTimeout(apiTimeout))}
}

func (a *anthropicModel) Ask(ctx context.Context, prompt string, images []string) (string, error) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(images)+1)
	for _, p := range images {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, anthropic.NewImageBlockBase64("image/jpeg", base64.StdEncoding.EncodeToString(b)))
	}
	blocks = append(blocks, anthropic.NewTextBlock(prompt))
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.c.Model),
		MaxTokens: anthropicMaxTokens,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(blocks...)},
	}
	if a.c.Effort != "" {
		params.OutputConfig = anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(a.c.Effort)}
	}
	resp, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return "", err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("the model declined to answer (%s)", resp.StopDetails.Category)
	}
	var out strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			out.WriteString(t.Text)
		}
	}
	return out.String(), nil
}

func listAnthropic(ctx context.Context, c Config) ([]ModelInfo, error) {
	client := newAnthropic(c).client
	pager := client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	var out []ModelInfo
	for pager.Next() {
		out = append(out, ModelInfo{ID: pager.Current().ID, Efforts: anthropicEfforts})
	}
	return out, pager.Err()
}
