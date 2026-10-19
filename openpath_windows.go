package main

import "os/exec"

// openPath opens a folder in Windows Explorer.
func openPath(dir string) error {
	return exec.Command("explorer.exe", dir).Start()
}
