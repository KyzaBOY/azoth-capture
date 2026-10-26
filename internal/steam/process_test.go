//go:build linux

package steam

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeProc builds a /proc/<pid> folder for a game started with plain Wine,
// outside any container. The game file is named name.
func fakeProc(t *testing.T, name string, env []string, mnt string) (procDir, exe, wine, prefix string) {
	t.Helper()
	root := t.TempDir()
	exe = filepath.Join(root, "Games", "New World", "Bin64", name)
	wineBin := filepath.Join(root, "wine", "bin")
	prefix = filepath.Join(root, "prefix")
	for _, d := range []string{filepath.Dir(exe), wineBin, prefix} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{exe, filepath.Join(wineBin, "wine"), filepath.Join(wineBin, "wine64-preloader")} {
		if err := os.WriteFile(f, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	procDir = filepath.Join(root, "proc", "4242")
	if err := os.MkdirAll(filepath.Join(procDir, "ns"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmdline := `Z:` + strings.ReplaceAll(exe, "/", `\`) + "\x00-launcherargs\x00"
	os.WriteFile(filepath.Join(procDir, "cmdline"), []byte(cmdline), 0o644)
	comm := name
	if len(comm) > commLen {
		comm = comm[:commLen]
	}
	os.WriteFile(filepath.Join(procDir, "comm"), []byte(comm+"\n"), 0o644)
	env = append(env, "WINEPREFIX="+prefix)
	os.WriteFile(filepath.Join(procDir, "environ"), []byte(strings.Join(env, "\x00")+"\x00"), 0o644)
	os.Symlink(filepath.Join(wineBin, "wine64-preloader"), filepath.Join(procDir, "exe"))
	os.Symlink(mnt, filepath.Join(procDir, "ns", "mnt"))
	return procDir, exe, filepath.Join(wineBin, "wine"), prefix
}

func TestInspectGameOutsideSteam(t *testing.T) {
	ours, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		t.Skip("no mount namespace info here")
	}
	procDir, exe, wine, prefix := fakeProc(t, GameProcess, []string{"WINEFSYNC=1", "HOME=/home/x"}, ours)
	g, err := inspect(procDir, 4242)
	if err != nil {
		t.Fatal(err)
	}
	if g.Exe != exe || g.Wine != wine || g.Prefix != prefix || g.Contained {
		t.Errorf("game = %+v", g)
	}
	if !slices.Contains(g.Env, "WINEFSYNC=1") {
		t.Errorf("env = %v", g.Env)
	}
}

func TestInspectPrefersWineLoader(t *testing.T) {
	ours, _ := os.Readlink("/proc/self/ns/mnt")
	loader := filepath.Join(t.TempDir(), "proton-wine")
	os.WriteFile(loader, nil, 0o755)
	procDir, _, _, _ := fakeProc(t, GameProcess, []string{"WINELOADER=" + loader}, ours)
	if g, err := inspect(procDir, 4242); err != nil || g.Wine != loader {
		t.Errorf("wine = %q, %v; want WINELOADER", g.Wine, err)
	}
}

// A game in a container (Steam Linux Runtime) has its own mount namespace,
// and paths only the container sees are not reported.
func TestInspectContainedGame(t *testing.T) {
	procDir, _, _, _ := fakeProc(t, GameProcess, []string{"WINELOADER=/run/pressure-vessel/wine"}, "mnt:[1]")
	os.Remove(filepath.Join(procDir, "exe"))
	os.Symlink("/run/pressure-vessel/pv-from-host/bin/wine64-preloader", filepath.Join(procDir, "exe"))
	g, err := inspect(procDir, 4242)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Contained || g.Wine != "" {
		t.Errorf("game = %+v: contained, and no Wine this app could use", g)
	}
}

func TestLinuxPath(t *testing.T) {
	for in, want := range map[string]string{
		`Z:\mnt\Games\New World\Bin64\NewWorld.exe`: "/mnt/Games/New World/Bin64/NewWorld.exe",
		`z:\home\me\NewWorld.exe`:                   "/home/me/NewWorld.exe",
	} {
		if got, ok := linuxPath(in); !ok || got != want {
			t.Errorf("linuxPath(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := linuxPath(`C:\Program Files\NewWorld.exe`); ok {
		t.Error("a path on a Wine-only drive has no Linux path")
	}
}

func TestFindGameByPickedFile(t *testing.T) {
	ours, _ := os.Readlink("/proc/self/ns/mnt")
	procDir, exe, _, _ := fakeProc(t, "nw-client-renamed.exe", nil, ours)
	procRoot := filepath.Dir(procDir)

	g, err := findGame(procRoot, exe)
	if err != nil || g.Exe != exe || g.PID != 4242 {
		t.Fatalf("picked file: game = %+v, %v", g, err)
	}
	if _, err := findGame(procRoot, ""); err == nil {
		t.Error("a renamed game was found without a picked file")
	}
}

func TestFindGameFallsBackToTheName(t *testing.T) {
	ours, _ := os.Readlink("/proc/self/ns/mnt")
	procDir, exe, _, _ := fakeProc(t, GameProcess, nil, ours)
	stale := filepath.Join(t.TempDir(), "old.exe")
	os.WriteFile(stale, []byte("an older build"), 0o644)
	if g, err := findGame(filepath.Dir(procDir), stale); err != nil || g.Exe != exe {
		t.Errorf("game = %+v, %v; want the one named %s", g, err, GameProcess)
	}
}

// A launcher whose command line names the game file, but which is not the
// game itself, is not taken for the game.
func TestFindGameSkipsALauncher(t *testing.T) {
	ours, _ := os.Readlink("/proc/self/ns/mnt")
	procDir, exe, _, _ := fakeProc(t, GameProcess, nil, ours)
	os.WriteFile(filepath.Join(procDir, "comm"), []byte("launcher.exe\n"), 0o644)
	if _, err := findGame(filepath.Dir(procDir), exe); err == nil {
		t.Error("the launcher was taken for the game")
	}
}

func TestFindGameRefusesTwoCopies(t *testing.T) {
	ours, _ := os.Readlink("/proc/self/ns/mnt")
	procDir, exe, _, _ := fakeProc(t, GameProcess, nil, ours)
	second := filepath.Join(filepath.Dir(procDir), "4343")
	os.MkdirAll(second, 0o755)
	for _, f := range []string{"cmdline", "comm"} {
		b, _ := os.ReadFile(filepath.Join(procDir, f))
		os.WriteFile(filepath.Join(second, f), b, 0o644)
	}
	if _, err := findGame(filepath.Dir(procDir), exe); err == nil {
		t.Error("two copies of the game did not stop the capture")
	}
}
