package aimarks

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// writeGameData makes a small folder in the nwdb.info layout.
func writeGameData(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(catalogFile, `[
		{"id":"oreveinsmall_dungeon","name":"Iron Vein (Small) (Expedition)","type":"gatherable"},
		{"id":"oreveinsmall","name":"Iron Vein (Small)","type":"gatherable"},
		{"id":"ironore","name":"Iron Ore","type":"item"},
		{"id":"turkey_53","name":"Turkey","type":"creature"}]`)
	write("records/gatherable/oreveinsmall.json", `{"success":true,"data":{"type":"gatherable","id":"oreveinsmall","name":"Iron Vein (Small)",
		"spawns":{"newworld_vitaeeterna":{"map":"newworld_vitaeeterna","coordinates":[[1000,2000,50],[5000,5000,10]]},
		"dungeon_x":{"coordinates":[[1010,2010]]}}}}`)
	write("records/creature/turkey_53.json", `{"success":true,"data":{"type":"creature","id":"turkey_53","name":"Turkey",
		"spawns":{"newworld_vitaeeterna":{"map":"newworld_vitaeeterna","coordinates":[[1020,2000,50]]}}}}`)
	write("records/npc/broken.json", `{"success":true,"data":{"type":"npc","id":"broken","name":"Broken","spawns":[]}}`)
	return dir
}

func TestLocalSearchMatchesWordsAndSkipsItems(t *testing.T) {
	l, err := NewLocal(writeGameData(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := l.Search(context.Background(), "iron  VEIN")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Iron Vein (Small)" {
		t.Errorf("search = %+v, want the shorter name first and no items", got)
	}
}

func TestLocalNearbyUsesOpenWorldSpawnsAndCachesIndex(t *testing.T) {
	data, cache := writeGameData(t), t.TempDir()
	l, err := NewLocal(data, cache)
	if err != nil {
		t.Fatal(err)
	}
	got, err := l.Nearby(context.Background(), 1015, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Turkey" || got[1].Name != "Iron Vein (Small)" {
		t.Errorf("nearby = %+v, want the turkey (5 m) before the vein (15 m)", got)
	}
	if far, _ := l.Nearby(context.Background(), 3000, 3000); len(far) != 0 {
		t.Errorf("nothing spawns near (3000, 3000), got %+v", far)
	}
	if _, err := os.Stat(filepath.Join(cache, indexFileName)); err != nil {
		t.Fatalf("the spawn index was not saved: %v", err)
	}

	// A second source reads the saved index instead of the records.
	if err := os.RemoveAll(filepath.Join(data, recordsDir)); err != nil {
		t.Fatal(err)
	}
	again, err := NewLocal(data, cache)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := again.Nearby(context.Background(), 1015, 2000); err != nil || len(got) != 2 {
		t.Errorf("cached nearby = %+v, %v", got, err)
	}
}

func TestNewLocalNeedsCatalog(t *testing.T) {
	if _, err := NewLocal(t.TempDir(), t.TempDir()); err == nil {
		t.Error("a folder without catalog.json was accepted")
	}
}

func TestOnlineSearchFiltersAndCaches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/db/search/iron vein" || r.Header.Get("User-Agent") != "test/1" {
			t.Errorf("request %q with agent %q", r.URL.Path, r.Header.Get("User-Agent"))
		}
		w.Write([]byte(`{"success":true,"data":[{"id":"a","name":"Iron Vein (Small)","type":"gatherable"},
			{"id":"b","name":"Iron Ore","type":"item"},{"id":"c","name":"Iron Vein (Small)","type":"gatherable"}]}`))
	}))
	defer srv.Close()
	o := NewOnline(srv.URL, "test/1", srv.Client())
	for range 2 {
		got, err := o.Search(context.Background(), "Iron Vein")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "a" {
			t.Errorf("search = %+v", got)
		}
	}
	if calls != 1 {
		t.Errorf("the site was asked %d times, want 1 (cached)", calls)
	}
	if _, err := o.Nearby(context.Background(), 1, 2); !errors.Is(err, ErrNoNearby) {
		t.Errorf("online nearby = %v", err)
	}
}
