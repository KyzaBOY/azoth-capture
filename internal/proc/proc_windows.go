// Package proc prepares commands the app runs in the background.
package proc

import (
	"os/exec"
	"syscall"
)

// createNoWindow keeps a console program from opening a console window.
const createNoWindow = 0x08000000

// Hidden runs cmd without a console window, so background tools (ffmpeg, AI
// CLIs) do not flash a window over the game.
func Hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}
