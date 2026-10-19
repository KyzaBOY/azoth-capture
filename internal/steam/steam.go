//go:build linux

// Package steam finds, at run time, everything the Linux capture needs: the
// Steam install, the New World game folder, the game's Proton prefix, the Proton
// build the game is set to use, and the Steam Linux Runtime that Proton runs
// inside. Nothing is hard-coded to one folder or one Proton version, so the
// capture works wherever the user keeps their library.
package steam

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AppID is New World's Steam application id.
const AppID = "1063730"

const (
	gameExeRel = "Bin64/NewWorld.exe"
	prefixRel  = "pfx"
	// defaultRuntimeAppID is Steam Linux Runtime 3.0 (sniper), used when a
	// Proton build does not say which runtime it needs.
	defaultRuntimeAppID = "1628350"
)

// runtimeDirFallbacks are tried, in order, only when the runtime the Proton
// build asks for is not installed in any library.
var runtimeDirFallbacks = []string{
	"SteamLinuxRuntime_sniper",
	"SteamLinuxRuntime_4",
	"SteamLinuxRuntime_soldier",
}

// Game holds the resolved paths for one New World install.
type Game struct {
	SteamRoot  string // the Steam root that owns the library list
	ExePath    string // .../New World/Bin64/NewWorld.exe
	Prefix     string // .../compatdata/1063730/pfx  (WINEPREFIX)
	ProtonWine string // .../<Proton>/files/bin/wine
	RuntimeRun string // .../SteamLinuxRuntime_*/run
}

// Discover resolves the New World install, or returns an error naming the first
// piece it could not find.
func Discover() (Game, error) {
	root, err := steamRoot()
	if err != nil {
		return Game{}, err
	}

	libs := libraries(root)
	lib, installDir := findAppLibrary(libs, AppID)
	if lib == "" {
		return Game{}, fmt.Errorf("New World (app %s) is not installed in any Steam library; install it through Steam first", AppID)
	}

	g := Game{SteamRoot: root}
	g.ExePath = filepath.Join(lib, "steamapps", "common", installDir, gameExeRel)
	if !fileExists(g.ExePath) {
		return Game{}, fmt.Errorf("found the New World entry but not its executable at %s", g.ExePath)
	}

	g.Prefix = filepath.Join(lib, "steamapps", "compatdata", AppID, prefixRel)
	if !dirExists(g.Prefix) {
		return Game{}, fmt.Errorf("no Proton prefix at %s; start New World through Steam at least once", g.Prefix)
	}

	g.ProtonWine, err = protonWine(lib)
	if err != nil {
		return Game{}, err
	}

	g.RuntimeRun, err = runtimeRun(libs, g.ProtonWine)
	if err != nil {
		return Game{}, err
	}

	return g, nil
}

// steamRoot returns the first Steam root that has a library list.
func steamRoot() (string, error) {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".steam", "root"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
	}
	for _, c := range candidates {
		if fileExists(filepath.Join(c, "steamapps", "libraryfolders.vdf")) {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not find Steam; looked in %s", strings.Join(candidates, ", "))
}

// libraries returns every Steam library path, including the root itself.
func libraries(root string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	add(root)
	data, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
	if err == nil {
		toks := vdfTokens(string(data))
		for i := 0; i+1 < len(toks); i++ {
			if toks[i] == "path" {
				add(toks[i+1])
			}
		}
	}
	return out
}

// findAppLibrary returns the library that has the given app installed and
// the app's install folder name, read from its app manifest. Both are empty
// when no library has it.
func findAppLibrary(libs []string, appID string) (lib, installDir string) {
	for _, l := range libs {
		manifest := filepath.Join(l, "steamapps", "appmanifest_"+appID+".acf")
		data, err := os.ReadFile(manifest)
		if err != nil {
			continue
		}
		if dir := vdfValue(vdfTokens(string(data)), "installdir"); dir != "" {
			return l, dir
		}
	}
	return "", ""
}

// protonWine finds the wine binary of the Proton build the game is set to use.
// It reads compatdata/<app>/config_info, whose path lines point inside the
// Proton "files" folder, so it works for official Proton and for custom builds
// such as GE-Proton without knowing the name in advance.
func protonWine(lib string) (string, error) {
	configInfo := filepath.Join(lib, "steamapps", "compatdata", AppID, "config_info")
	data, err := os.ReadFile(configInfo)
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			marker := "/files/"
			idx := strings.Index(line, marker)
			if idx < 0 {
				continue
			}
			protonFiles := line[:idx+len("/files")]
			wine := filepath.Join(protonFiles, "bin", "wine")
			if fileExists(wine) {
				return wine, nil
			}
		}
	}
	return "", fmt.Errorf("could not find the Proton build for New World; set a Proton version for it in Steam (Properties > Compatibility)")
}

// runtimeRun finds the Steam Linux Runtime "run" script that Proton needs. It
// asks the Proton build which runtime app it wants (toolmanifest.vdf), finds
// that app's install folder through its Steam app manifest, and only falls
// back to well-known folder names when that runtime is not installed.
func runtimeRun(libs []string, protonWine string) (string, error) {
	if lib, dir := findAppLibrary(libs, requiredRuntimeAppID(protonWine)); lib != "" {
		run := filepath.Join(lib, "steamapps", "common", dir, "run")
		if fileExists(run) {
			return run, nil
		}
	}
	for _, dir := range runtimeDirFallbacks {
		for _, l := range libs {
			run := filepath.Join(l, "steamapps", "common", dir, "run")
			if fileExists(run) {
				return run, nil
			}
		}
	}
	return "", fmt.Errorf("could not find the Steam Linux Runtime this Proton build needs; install it through Steam (it installs with Proton)")
}

// requiredRuntimeAppID reads the Proton build's toolmanifest.vdf and returns
// the runtime app id it asks for, or the sniper runtime when it says nothing.
func requiredRuntimeAppID(protonWine string) string {
	// protonWine is .../<Proton>/files/bin/wine; the manifest sits at
	// .../<Proton>/toolmanifest.vdf, three folders up from the wine binary.
	protonDir := filepath.Dir(filepath.Dir(filepath.Dir(protonWine)))
	data, err := os.ReadFile(filepath.Join(protonDir, "toolmanifest.vdf"))
	if err != nil {
		return defaultRuntimeAppID
	}
	if id := vdfValue(vdfTokens(string(data)), "require_tool_appid"); id != "" {
		return id
	}
	return defaultRuntimeAppID
}

// GameProcess is the name of New World's process under Proton.
const GameProcess = "NewWorld.exe"

// GameRunning reports whether a NewWorld.exe process exists.
func GameRunning() bool {
	return exec.Command("pgrep", "-x", GameProcess).Run() == nil
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
