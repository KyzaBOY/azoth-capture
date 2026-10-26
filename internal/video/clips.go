package video

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	clipPrefix = "gameplay-"
	clipExt    = ".mkv"
	maxClips   = 999
	usPerMs    = 1000
	noAudio    = "none" // Clip.Audio of a clip without sound
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
