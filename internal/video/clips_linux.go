package video

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	clipPrefix = "gameplay-"
	clipExt    = ".mkv"
	maxClips   = 999
	// timestampExt is what GPU Screen Recorder appends to the video file name
	// for its first-frame timestamp file.
	timestampExt = ".ts"
	usPerMs      = 1000
)

// nextClipPath returns the first free gameplay-NNN.mkv path in dir.
func nextClipPath(dir string) (string, error) {
	for n := 1; n <= maxClips; n++ {
		p := filepath.Join(dir, fmt.Sprintf("%s%03d%s", clipPrefix, n, clipExt))
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p, nil
		}
	}
	return "", fmt.Errorf("this session already has %d video clips", maxClips)
}

// parseFirstFrame reads a GPU Screen Recorder timestamp file: a header line
// ("monotonic_microsec<TAB>realtime_microsec") and a line of two numbers. It
// returns the realtime value in Unix milliseconds.
func parseFirstFrame(text string) (int64, error) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("timestamp file has no value line")
	}
	fields := strings.Fields(lines[1])
	if len(fields) != 2 {
		return 0, fmt.Errorf("timestamp line %q does not have two values", lines[1])
	}
	realtimeUs, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || realtimeUs <= 0 {
		return 0, fmt.Errorf("bad realtime value %q", fields[1])
	}
	return realtimeUs / usPerMs, nil
}

// firstField returns each line's text before the first "|", the format of the
// recorder's --list-* commands ("DP-1|1920x1080").
func firstField(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		name, _, _ := strings.Cut(strings.TrimSpace(line), "|")
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}
