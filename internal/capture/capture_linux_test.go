package capture

import (
	"slices"
	"testing"

	"azoth/capture/internal/steam"
)

// A game started outside any container is joined with its own Wine and its
// own environment, so the capture server lands in the game's Wine prefix.
func TestServerLaunchOutsideContainer(t *testing.T) {
	r := New(t.TempDir(), func(string) {}, func(Status) {})
	g := steam.Running{Exe: "/games/NewWorld.exe", Wine: "/opt/wine/bin/wine", Prefix: "/games/prefix",
		Env: []string{"WINEPREFIX=/games/prefix", "WINEFSYNC=1"}}
	launch, err := r.serverLaunch(g)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(launch.argv, []string{"/opt/wine/bin/wine"}) {
		t.Errorf("argv = %q", launch.argv)
	}
	for _, want := range []string{"WINEPREFIX=/games/prefix", "WINEFSYNC=1", "WINEDEBUG=-all"} {
		if !slices.Contains(launch.env, want) {
			t.Errorf("env %q lacks %q", launch.env, want)
		}
	}
}

// A game in a container needs Steam's runtime; without Steam that is a clear
// error, not a silent attach to the wrong prefix.
func TestServerLaunchContainedNeedsSteam(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r := New(t.TempDir(), func(string) {}, func(Status) {})
	if _, err := r.serverLaunch(steam.Running{Contained: true}); err == nil {
		t.Error("a contained game without Steam's runtime did not report an error")
	}
}
