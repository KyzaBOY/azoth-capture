package main

import "os/exec"

// openPath opens a folder in the user's file manager via the freedesktop
// xdg-open helper, which every desktop environment provides.
func openPath(dir string) error {
	return exec.Command("xdg-open", dir).Start()
}
