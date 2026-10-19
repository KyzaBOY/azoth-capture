//go:build linux

package steam

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeSteam is a Steam tree in a temporary home. The game lives in a SECOND
// library and the Proton build is a custom one in compatibilitytools.d.
type fakeSteam struct {
	root, gameLib, exe, wine string
}

func newFakeSteam(t *testing.T, requiredRuntime string) fakeSteam {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	f := fakeSteam{
		root:    filepath.Join(home, ".local", "share", "Steam"),
		gameLib: t.TempDir(),
	}
	proton := filepath.Join(f.root, "compatibilitytools.d", "Proton-test")
	f.wine = filepath.Join(proton, "files", "bin", "wine")
	f.exe = filepath.Join(f.gameLib, "steamapps", "common", "New World", "Bin64", "NewWorld.exe")

	writeFile(t, filepath.Join(f.root, "steamapps", "libraryfolders.vdf"), `
"libraryfolders"
{
	"0" { "path" "`+f.root+`" }
	"1" { "path" "`+f.gameLib+`" }
}
`)
	writeFile(t, filepath.Join(f.gameLib, "steamapps", "appmanifest_1063730.acf"),
		`"AppState" { "appid" "1063730" "installdir" "New World" }`)
	writeFile(t, f.exe, "MZ")
	compat := filepath.Join(f.gameLib, "steamapps", "compatdata", "1063730")
	writeFile(t, filepath.Join(compat, "pfx", "system.reg"), "")
	writeFile(t, filepath.Join(compat, "config_info"),
		"Proton-test\n"+filepath.Join(proton, "files", "share", "fonts")+"/\n")
	writeFile(t, f.wine, "#!/bin/true\n")
	writeFile(t, filepath.Join(proton, "toolmanifest.vdf"),
		`"manifest" { "require_tool_appid" "`+requiredRuntime+`" }`)
	return f
}

// addRuntime installs a runtime app in the root library, with its manifest.
func (f fakeSteam) addRuntime(t *testing.T, appID, dir string) string {
	t.Helper()
	writeFile(t, filepath.Join(f.root, "steamapps", "appmanifest_"+appID+".acf"),
		`"AppState" { "appid" "`+appID+`" "installdir" "`+dir+`" }`)
	run := filepath.Join(f.root, "steamapps", "common", dir, "run")
	writeFile(t, run, "#!/bin/true\n")
	return run
}

func TestDiscover(t *testing.T) {
	f := newFakeSteam(t, "1628350")
	run := f.addRuntime(t, "1628350", "SteamLinuxRuntime_sniper")

	g, err := Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if g.ExePath != f.exe {
		t.Errorf("ExePath = %q, want %q", g.ExePath, f.exe)
	}
	if g.ProtonWine != f.wine {
		t.Errorf("ProtonWine = %q, want %q", g.ProtonWine, f.wine)
	}
	if g.RuntimeRun != run {
		t.Errorf("RuntimeRun = %q, want %q", g.RuntimeRun, run)
	}
	wantPrefix := filepath.Join(f.gameLib, "steamapps", "compatdata", "1063730", "pfx")
	if g.Prefix != wantPrefix {
		t.Errorf("Prefix = %q, want %q", g.Prefix, wantPrefix)
	}
}

// Proton 11 and Proton Experimental ask for a newer runtime (app 4183110,
// SteamLinuxRuntime_4). It must be chosen over sniper even though both exist.
func TestDiscoverPicksRuntimeTheProtonAsksFor(t *testing.T) {
	f := newFakeSteam(t, "4183110")
	f.addRuntime(t, "1628350", "SteamLinuxRuntime_sniper")
	want := f.addRuntime(t, "4183110", "SteamLinuxRuntime_4")

	g, err := Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if g.RuntimeRun != want {
		t.Errorf("RuntimeRun = %q, want %q", g.RuntimeRun, want)
	}
}

// Without a manifest for the requested runtime, a well-known folder is used.
func TestDiscoverRuntimeFallback(t *testing.T) {
	f := newFakeSteam(t, "999999")
	run := filepath.Join(f.root, "steamapps", "common", "SteamLinuxRuntime_sniper", "run")
	writeFile(t, run, "#!/bin/true\n")

	g, err := Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if g.RuntimeRun != run {
		t.Errorf("RuntimeRun = %q, want %q", g.RuntimeRun, run)
	}
}

func TestDiscoverNoSteam(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := Discover(); err == nil {
		t.Fatal("expected an error when Steam is absent")
	}
}
