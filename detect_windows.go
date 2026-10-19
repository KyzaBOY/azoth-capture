package main

const platformName = "windows"

// detectGameExe is Windows's no-op: the user points at NewWorld.exe with the
// Browse button, as before.
func detectGameExe() string { return "" }
