package aimarks

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

// A local game data folder has the layout of an nwdb.info copy:
//
//	catalog.json                  [{"id", "name", "type"}, ...]
//	records/<type>/<id>.json      {"success": true, "data": {..., "spawns": {...}}}
//
// spawns maps a map name to {"coordinates": [[x, y, z?], ...]}. Only the open
// world map is indexed: expeditions use their own coordinates.
const (
	catalogFile   = "catalog.json"
	recordsDir    = "records"
	openWorldMap  = "newworld_vitaeeterna"
	nearbyCell    = 64.0 // meters per grid cell of the spawn index
	indexFileName = "nearby-index.gob"
)

// spawnTypes are the record types whose spawn positions are indexed.
var spawnTypes = []string{"gatherable", "creature", "npc"}

// Local reads a game data folder on this PC.
type Local struct {
	dir      string
	cacheDir string

	catalogOnce sync.Once
	catalog     []Entry
	catalogErr  error

	indexOnce sync.Once
	index     *spawnIndex
	indexErr  error
}

// NewLocal checks that dir looks like a game data folder. cacheDir keeps the
// spawn index between runs, because building it reads every record.
func NewLocal(dir, cacheDir string) (*Local, error) {
	if _, err := os.Stat(filepath.Join(dir, catalogFile)); err != nil {
		return nil, fmt.Errorf("%s has no %s; pick the folder of an nwdb.info copy", dir, catalogFile)
	}
	return &Local{dir: dir, cacheDir: cacheDir}, nil
}

// Search matches every query word against the catalog names.
func (l *Local) Search(_ context.Context, query string) ([]Entry, error) {
	l.catalogOnce.Do(func() {
		b, err := os.ReadFile(filepath.Join(l.dir, catalogFile))
		if err != nil {
			l.catalogErr = err
			return
		}
		l.catalogErr = json.Unmarshal(b, &l.catalog)
	})
	if l.catalogErr != nil {
		return nil, fmt.Errorf("game data catalog: %w", l.catalogErr)
	}
	words := queryWords(query)
	var hits []Entry
	for _, e := range l.catalog {
		if searchTypes[e.Type] && nameMatches(e.Name, words) {
			hits = append(hits, e)
		}
	}
	// The shortest names first: "Iron Vein" before "Iron Vein (Expedition)".
	sort.SliceStable(hits, func(a, b int) bool { return len(hits[a].Name) < len(hits[b].Name) })
	return uniqueNames(hits, maxSearchResults), nil
}

// Nearby lists what spawns within nearbyRadius of (x, y), the nearest first.
func (l *Local) Nearby(ctx context.Context, x, y float64) ([]Entry, error) {
	l.indexOnce.Do(func() { l.index, l.indexErr = l.loadIndex(ctx) })
	if l.indexErr != nil {
		return nil, l.indexErr
	}
	return l.index.near(x, y, nearbyRadius, maxNearbyResults), nil
}

// spawnIndex is a grid of spawn points. It is saved with gob.
type spawnIndex struct {
	Key     string // which folder and catalog it was built from
	Entries []Entry
	Cells   map[[2]int32][]spawnPoint
}

type spawnPoint struct {
	X, Y  float32
	Entry int32
}

func cellOf(x, y float64) [2]int32 {
	return [2]int32{int32(math.Floor(x / nearbyCell)), int32(math.Floor(y / nearbyCell))}
}

func (ix *spawnIndex) near(x, y, radius float64, limit int) []Entry {
	type hit struct {
		d float64
		e int32
	}
	var hits []hit
	lo, hi := cellOf(x-radius, y-radius), cellOf(x+radius, y+radius)
	for cx := lo[0]; cx <= hi[0]; cx++ {
		for cy := lo[1]; cy <= hi[1]; cy++ {
			for _, p := range ix.Cells[[2]int32{cx, cy}] {
				if d := math.Hypot(float64(p.X)-x, float64(p.Y)-y); d <= radius {
					hits = append(hits, hit{d, p.Entry})
				}
			}
		}
	}
	sort.Slice(hits, func(a, b int) bool { return hits[a].d < hits[b].d })
	out := make([]Entry, 0, len(hits))
	for _, h := range hits {
		out = append(out, ix.Entries[h.e])
	}
	return uniqueNames(out, limit)
}

// indexKey changes when the folder or its catalog changes.
func (l *Local) indexKey() (string, error) {
	fi, err := os.Stat(filepath.Join(l.dir, catalogFile))
	if err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(l.dir)
	return fmt.Sprintf("%s|%d|%d", abs, fi.Size(), fi.ModTime().UnixNano()), nil
}

func (l *Local) loadIndex(ctx context.Context) (*spawnIndex, error) {
	key, err := l.indexKey()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(l.cacheDir, indexFileName)
	if f, err := os.Open(path); err == nil {
		var ix spawnIndex
		decErr := gob.NewDecoder(f).Decode(&ix)
		f.Close()
		if decErr == nil && ix.Key == key {
			return &ix, nil
		}
	}
	ix, err := buildIndex(ctx, l.dir)
	if err != nil {
		return nil, err
	}
	ix.Key = key
	if err := saveIndex(path, ix); err != nil {
		return nil, fmt.Errorf("save the spawn index: %w", err)
	}
	return ix, nil
}

func saveIndex(path string, ix *spawnIndex) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(ix); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// spawnRecord is the part of a record the index needs.
type spawnRecord struct {
	Data struct {
		Type   string                     `json:"type"`
		ID     string                     `json:"id"`
		Name   string                     `json:"name"`
		Spawns map[string]json.RawMessage `json:"spawns"`
	} `json:"data"`
}

type spawnCoords struct {
	Coordinates [][]float64 `json:"coordinates"`
}

// readSpawns returns a record's entry and its open world positions. A record
// without readable spawns gives no positions.
func readSpawns(path string) (Entry, [][2]float64) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, nil
	}
	var r spawnRecord
	if json.Unmarshal(b, &r) != nil || r.Data.Name == "" {
		return Entry{}, nil
	}
	e := Entry{Type: r.Data.Type, ID: r.Data.ID, Name: r.Data.Name}
	raw, ok := r.Data.Spawns[openWorldMap]
	if !ok {
		return e, nil
	}
	var c spawnCoords
	if json.Unmarshal(raw, &c) != nil {
		return e, nil
	}
	pos := make([][2]float64, 0, len(c.Coordinates))
	for _, xy := range c.Coordinates {
		if len(xy) >= 2 {
			pos = append(pos, [2]float64{xy[0], xy[1]})
		}
	}
	return e, pos
}

// buildIndex reads the spawn records with one worker per CPU.
func buildIndex(ctx context.Context, dir string) (*spawnIndex, error) {
	var paths []string
	for _, t := range spawnTypes {
		found, err := filepath.Glob(filepath.Join(dir, recordsDir, t, "*.json"))
		if err != nil {
			return nil, err
		}
		paths = append(paths, found...)
	}
	if len(paths) == 0 {
		return nil, errors.New("the game data folder has no gatherable, creature or npc records")
	}

	type result struct {
		e   Entry
		pos [][2]float64
	}
	jobs := make(chan string)
	results := make(chan result)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				e, pos := readSpawns(p)
				results <- result{e, pos}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, p := range paths {
			select {
			case jobs <- p:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()

	ix := &spawnIndex{Cells: map[[2]int32][]spawnPoint{}}
	for r := range results {
		if len(r.pos) == 0 {
			continue
		}
		id := int32(len(ix.Entries))
		ix.Entries = append(ix.Entries, r.e)
		for _, p := range r.pos {
			c := cellOf(p[0], p[1])
			ix.Cells[c] = append(ix.Cells[c], spawnPoint{X: float32(p[0]), Y: float32(p[1]), Entry: id})
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ix, nil
}
