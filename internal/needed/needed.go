// Package needed reads the list of packets the community still needs recorded
// (GET /api/needed on the Azoth site). It is the app's only network call; without
// it the journal works the same way, just without the missions.
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

// URL is the public endpoint.
const URL = "https://projectazoth.com/api/needed"

const maxBody = 16 << 20

// Packet is one packet of the list, in the /api/needed format.
type Packet struct {
	Idx             int      `json:"idx"`
	Name            string   `json:"name"`
	Short           string   `json:"short"`
	Status          string   `json:"status"` // wanted | captured
	What            string   `json:"what"`
	Confidence      string   `json:"confidence"`
	ConfidenceLabel string   `json:"confidence_label"` // computed here, not sent by the API
	HowToCapture    []string `json:"how_to_capture"`
	Notes           string   `json:"notes"`
}

// UnmarshalJSON accepts "notes": null and a missing how_to_capture.
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

// Activity groups the packets of one game activity.
type Activity struct {
	Activity string   `json:"activity"`
	Packets  []Packet `json:"packets"`
}

// Catalog is the whole response.
type Catalog struct {
	GeneratedAt string     `json:"generated_at"`
	Activities  []Activity `json:"activities"`
}

// ConfidenceLabel is the confidence label the screen shows.
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

// Parse reads the /api/needed JSON. The "Developer-only" activity is dropped
// (a test command the normal game never sends, so there is nothing to record),
// and so is any activity without packets.
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

// Fetch downloads and reads the list. Any failure (network, status, JSON) comes back
// as an error; the caller treats it as "offline".
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
