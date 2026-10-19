package main

import "azoth/capture/internal/steam"

const platformName = "linux"

// detectGameExe tries to locate NewWorld.exe through Steam, so the user does not
// have to type the path. An empty result means "not found; ask the user".
func detectGameExe() string {
	g, err := steam.Discover()
	if err != nil {
		return ""
	}
	return g.ExePath
}
