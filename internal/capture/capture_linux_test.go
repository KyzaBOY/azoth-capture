package capture

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"azoth/capture/internal/hooks"
)

// The same shapes the attach helper accepts (SITE_VA / SITE_BYTES there).
var (
	siteVA    = regexp.MustCompile(`^0x[0-9A-F]+$`)
	siteBytes = regexp.MustCompile(`^[0-9A-F]{2}( [0-9A-F]{2}){15}$`)
)

// Runs against a real game install when AZOTH_NW_EXE points at NewWorld.exe
// (the exe is Amazon's and is not in this repo, so CI skips this).
func TestHookSitesJSONRealGame(t *testing.T) {
	exe := os.Getenv("AZOTH_NW_EXE")
	if exe == "" {
		t.Skip("set AZOTH_NW_EXE to the path of NewWorld.exe to run this")
	}
	a, err := hooks.Find(exe)
	if err != nil {
		t.Fatal(err)
	}
	text, err := hookSitesJSON(exe, a)
	if err != nil {
		t.Fatal(err)
	}
	var sites []hookSite
	if err := json.Unmarshal([]byte(text), &sites); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
	names := []string{"SSL_read", "SSL_write", "nss_keylog"}
	if len(sites) != len(names) {
		t.Fatalf("got %d sites, want %d", len(sites), len(names))
	}
	for i, s := range sites {
		if s.Name != names[i] || !siteVA.MatchString(s.VA) || !siteBytes.MatchString(s.Bytes) {
			t.Errorf("site %d = %+v, not in the shape the attach helper accepts", i, s)
		}
	}
	t.Log(text)
}
