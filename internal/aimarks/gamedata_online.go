package aimarks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultOnlineURL is nwdb.info. Its robots.txt allows reference use by AI
// tools ("use=reference") and forbids training, so the app only looks up a
// few names per session, one request at a time, and keeps the answers.
const DefaultOnlineURL = "https://nwdb.info"

// onlineMinGap is the least time between two requests to the site.
const onlineMinGap = time.Second

// Online looks names up on nwdb.info. It has no spawn positions.
type Online struct {
	base   string
	agent  string
	client *http.Client

	mu    sync.Mutex
	last  time.Time
	cache map[string][]Entry
}

// NewOnline returns a source that asks base (normally DefaultOnlineURL).
func NewOnline(base, userAgent string, client *http.Client) *Online {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Online{base: strings.TrimRight(base, "/"), agent: userAgent, client: client, cache: map[string][]Entry{}}
}

type onlineSearch struct {
	Success bool    `json:"success"`
	Data    []Entry `json:"data"`
}

// Search asks the site's search for query.
func (o *Online) Search(ctx context.Context, query string) ([]Entry, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if hit, ok := o.cache[q]; ok {
		return hit, nil
	}
	if wait := onlineMinGap - time.Since(o.last); wait > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	o.last = time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.base+"/db/search/"+url.PathEscape(q), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", o.agent)
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("game data search: %s", resp.Status)
	}
	var body onlineSearch
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("game data search: %w", err)
	}
	var kept []Entry
	for _, e := range body.Data {
		if searchTypes[e.Type] {
			kept = append(kept, e)
		}
	}
	kept = uniqueNames(kept, maxSearchResults)
	o.cache[q] = kept
	return kept, nil
}

// Nearby is not possible online: the site cannot list what spawns near a
// position.
func (o *Online) Nearby(context.Context, float64, float64) ([]Entry, error) {
	return nil, ErrNoNearby
}
