// Package needed lê a lista de pacotes que a comunidade ainda precisa gravar
// (GET /api/needed do site do Azoth). É a única chamada de rede do app; sem ela
// o diário funciona do mesmo jeito, só sem as missões.
package needed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// URL é o endpoint público.
const URL = "https://projectazoth.com/api/needed"

const maxBody = 16 << 20

// Packet é um pacote da lista, no formato de /api/needed.
type Packet struct {
	Idx             int      `json:"idx"`
	Name            string   `json:"name"`
	Short           string   `json:"short"`
	Status          string   `json:"status"` // wanted | captured
	What            string   `json:"what"`
	Confidence      string   `json:"confidence"`
	ConfidenceLabel string   `json:"confidence_label"` // calculado aqui, não vem da API
	HowToCapture    []string `json:"how_to_capture"`
	Notes           string   `json:"notes"`
}

// UnmarshalJSON aceita "notes": null e how_to_capture ausente.
func (p *Packet) UnmarshalJSON(b []byte) error {
	type plain Packet
	aux := struct {
		*plain
		Notes *string `json:"notes"`
	}{plain: (*plain)(p)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	p.Notes = ""
	if aux.Notes != nil {
		p.Notes = *aux.Notes
	}
	if p.HowToCapture == nil {
		p.HowToCapture = []string{}
	}
	p.ConfidenceLabel = ConfidenceLabel(p.Confidence)
	return nil
}

// Activity agrupa os pacotes de uma atividade do jogo.
type Activity struct {
	Activity string   `json:"activity"`
	Packets  []Packet `json:"packets"`
}

// Catalog é a resposta inteira.
type Catalog struct {
	GeneratedAt string     `json:"generated_at"`
	Activities  []Activity `json:"activities"`
}

// ConfidenceLabel é o rótulo de certeza que a tela mostra.
func ConfidenceLabel(c string) string {
	switch c {
	case "known":
		return "We know what this does"
	case "likely":
		return "Very likely"
	case "guess":
		return "Our best guess"
	default:
		return "We don't know what this does yet"
	}
}

// Parse lê o JSON de /api/needed. Atividade "Developer-only" é descartada
// (comando de teste que o jogo normal não manda: não há o que gravar), assim
// como atividade sem pacotes.
func Parse(r io.Reader) (*Catalog, error) {
	var c Catalog
	if err := json.NewDecoder(io.LimitReader(r, maxBody)).Decode(&c); err != nil {
		return nil, fmt.Errorf("invalid needed list: %w", err)
	}
	kept := make([]Activity, 0, len(c.Activities))
	for _, a := range c.Activities {
		if strings.EqualFold(strings.TrimSpace(a.Activity), "Developer-only") || len(a.Packets) == 0 {
			continue
		}
		kept = append(kept, a)
	}
	c.Activities = kept
	return &c, nil
}

// Fetch baixa e lê a lista. Qualquer falha (rede, status, JSON) volta como erro;
// quem chama trata como "offline".
func Fetch(ctx context.Context, client *http.Client, url, userAgent string) (*Catalog, error) {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the server answered %s", resp.Status)
	}
	return Parse(resp.Body)
}
