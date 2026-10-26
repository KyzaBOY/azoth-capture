// Package aimarks drafts step marks from a session's video with a model the
// player picks. It samples frames, asks the model what the player did, checks
// the names the model read against New World game data, and keeps the result
// as drafts. A draft becomes a step only when the player accepts it.
package aimarks

import (
	"context"
	"errors"
	"strings"
)

// Entry is one thing in the game data: a node, a mob, an NPC, a quest.
type Entry struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GameData answers the two questions the pipeline asks about the game.
type GameData interface {
	// Search finds entries whose name matches what the model read on screen.
	Search(ctx context.Context, query string) ([]Entry, error)
	// Nearby lists entries that spawn close to a world position.
	Nearby(ctx context.Context, x, y float64) ([]Entry, error)
}

// ErrNoNearby is returned by a source that has no spawn positions.
var ErrNoNearby = errors.New("this game data source has no spawn positions")

// maxSearchResults and maxNearbyResults keep the candidate list short, so the
// check pass gets a clear choice.
const (
	maxSearchResults = 8
	maxNearbyResults = 12
	nearbyRadius     = 40.0 // meters around the player
)

// searchTypes are the record types a step can be about. Items, perks and the
// like are not things the player walks up to.
var searchTypes = map[string]bool{
	"gatherable": true, "creature": true, "npc": true, "quest": true, "recipe": true, "zone": true,
}

// queryWords splits a query into lower-case words for matching.
func queryWords(q string) []string {
	return strings.Fields(strings.ToLower(q))
}

// nameMatches reports whether every query word is in the name.
func nameMatches(name string, words []string) bool {
	if len(words) == 0 {
		return false
	}
	n := strings.ToLower(name)
	for _, w := range words {
		if !strings.Contains(n, w) {
			return false
		}
	}
	return true
}

// uniqueNames drops entries whose type and name were already listed.
func uniqueNames(in []Entry, limit int) []Entry {
	seen := map[string]bool{}
	var out []Entry
	for _, e := range in {
		k := e.Type + "\x00" + e.Name
		if e.Name == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
		if len(out) == limit {
			break
		}
	}
	return out
}
