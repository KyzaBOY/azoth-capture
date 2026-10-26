package capture

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"azoth/capture/internal/hooks"
	"azoth/capture/internal/steam"
)

// attachHelper attaches to the running game on both systems: always on Linux,
// and on Windows when the player started the game before pressing Start.
//
//go:embed embed/azoth_attach.py
var attachHelper []byte

const attachHelperName = "azoth_attach.py"

// stopGrace is how long the attach helper gets, after Stop, to flush the last
// data, detach from the game and stop its Frida server, if it has one.
const stopGrace = 15 * time.Second

// hookSite is one function the agent will hook, with the bytes the executable
// file has at its start. The attach helper checks the running game's memory
// against these before any hook is placed.
type hookSite struct {
	Name  string `json:"name"`
	VA    string `json:"va"`
	Bytes string `json:"bytes"`
}

// hookSitesJSON describes the three hook sites for the attach helper.
func hookSitesJSON(exePath string, a hooks.Addrs) (string, error) {
	p, err := hooks.ReadPrologues(exePath, a)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal([]hookSite{
		{"SSL_read", fmt.Sprintf("0x%X", a.SSLRead), p.SSLRead},
		{"SSL_write", fmt.Sprintf("0x%X", a.SSLWrite), p.SSLWrite},
		{"nss_keylog", fmt.Sprintf("0x%X", a.Keylog), p.Keylog},
	})
	return string(b), err
}

// attachArgs is the attach helper's command line after the Python program.
// exePath is the running game's file; the helper looks for a program of that
// name. platformArgs are the options only one system uses.
func (r *Runner) attachArgs(exePath, session string, timeout int, setup hookSetup, platformArgs ...string) ([]string, error) {
	sites, err := hookSitesJSON(exePath, setup.addrs)
	if err != nil {
		return nil, fmt.Errorf("Cannot read the hook sites from the game executable: %w", err)
	}
	args := []string{
		// -u: the helper's output goes through a pipe to the Activity Log, and
		// Python holds piped output back in a buffer; unbuffered, each line
		// shows up as soon as it is printed.
		"-u",
		r.helperPath(attachHelperName),
		"--common-js", setup.common,
		"--ledger-js", setup.patched,
		"--process", steam.GameFileName(exePath),
		"--sites", sites,
		"--session", session,
		"--captures-root", filepath.Join(r.captureDir(), "captures"),
		"--timeout", fmt.Sprint(timeout),
	}
	return append(args, platformArgs...), nil
}

// fail logs a reason the capture could not run and marks it as failed.
func (r *Runner) fail(msg string) {
	r.log(msg)
	r.setStatus(StatusError)
}
