package aimarks

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Connector kinds, as stored in the settings.
const (
	ConnectorOpenAI    = "openai"    // any OpenAI-style API: Ollama, LM Studio, OpenAI, OpenRouter, xAI, ...
	ConnectorAnthropic = "anthropic" // the Anthropic API
	ConnectorCLI       = "cli"       // a logged-in CLI that uses the player's subscription
)

// Config picks the model. The player sets every field; nothing is chosen for
// them except the lowest effort when they leave it empty.
type Config struct {
	Connector string `json:"connector"`
	BaseURL   string `json:"base_url"` // OpenAI-style only
	APIKey    string `json:"api_key"`  // API connectors; empty for a local server
	CLI       string `json:"cli"`      // CLI connector: one of CLINames
	Model     string `json:"model"`
	Effort    string `json:"effort"`
}

// Label names the model and effort, for the hit rate and the run summary.
func (c Config) Label() string {
	via := c.Connector
	if c.Connector == ConnectorCLI {
		via = c.CLI
	}
	l := via + ": " + c.Model
	if c.Effort != "" {
		l += " @ " + c.Effort
	}
	return l
}

// ModelInfo is one model the player can pick, with the efforts it accepts,
// lowest first. Efforts is empty when the provider does not say.
type ModelInfo struct {
	ID      string   `json:"id"`
	Efforts []string `json:"efforts"`
}

// Model answers one question about some frames.
type Model interface {
	Ask(ctx context.Context, prompt string, images []string) (string, error)
}

var errNoModel = errors.New("pick a model first")

// Check reports what is missing before a run can start.
func (c Config) Check() error {
	switch c.Connector {
	case ConnectorOpenAI:
		if c.BaseURL == "" {
			return errors.New("enter the API address, for example http://127.0.0.1:11434/v1 for Ollama")
		}
	case ConnectorAnthropic:
		if c.APIKey == "" {
			return errors.New("enter an Anthropic API key")
		}
	case ConnectorCLI:
		if _, ok := cliSpecs[c.CLI]; !ok {
			return fmt.Errorf("pick a CLI: %s", strings.Join(CLINames(), ", "))
		}
	default:
		return errors.New("pick how to reach the model")
	}
	if c.Model == "" {
		return errNoModel
	}
	return nil
}

// NewModel returns the model c describes.
func NewModel(c Config) (Model, error) {
	if err := c.Check(); err != nil {
		return nil, err
	}
	switch c.Connector {
	case ConnectorOpenAI:
		return newOpenAI(c), nil
	case ConnectorAnthropic:
		return newAnthropic(c), nil
	default:
		return newCLI(c), nil
	}
}

// ListModels asks the provider which models the player can use.
func ListModels(ctx context.Context, c Config) ([]ModelInfo, error) {
	probe := c
	probe.Model = "-" // listing needs no model
	if err := probe.Check(); err != nil {
		return nil, err
	}
	switch c.Connector {
	case ConnectorOpenAI:
		return listOpenAI(ctx, c)
	case ConnectorAnthropic:
		return listAnthropic(ctx, c)
	default:
		return listCLI(ctx, c)
	}
}
