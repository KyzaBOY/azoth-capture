//go:build !windows

// Package proc prepares commands the app runs in the background.
package proc

import "os/exec"

// Hidden returns cmd unchanged: only Windows opens a console window for a
// background tool.
func Hidden(cmd *exec.Cmd) *exec.Cmd { return cmd }
