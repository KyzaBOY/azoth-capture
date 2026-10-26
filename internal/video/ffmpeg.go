package video

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ffmpeg is used on both systems: AI marks read frames with it, and on
// Windows it also records the clips. The app looks for its own downloaded
// copy first, then for one on PATH.

// ToolsDirName is the folder, inside the app's data folder, for tools the app
// downloaded.
const ToolsDirName = "tools"

// ErrNoFFmpeg means no ffmpeg was found.
var ErrNoFFmpeg = errors.New("ffmpeg is not installed")

func ffmpegFile() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

// FFmpegPath returns the ffmpeg in toolsDir, or else the one on PATH.
func FFmpegPath(toolsDir string) (string, error) {
	own := filepath.Join(toolsDir, ffmpegFile())
	if fi, err := os.Stat(own); err == nil && !fi.IsDir() {
		return own, nil
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	return "", ErrNoFFmpeg
}

// verifySHA256 checks a downloaded file against its published hash.
func verifySHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("the download is damaged or changed (SHA-256 %s, expected %s); nothing was installed", got, want)
	}
	return nil
}

// extractFromZip copies the zip entry whose path ends in suffix to dst.
func extractFromZip(zipPath, suffix, dst string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, suffix) {
			continue
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		defer src.Close()
		tmp := dst + ".tmp"
		out, err := os.Create(tmp)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, src); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return os.Rename(tmp, dst)
	}
	return fmt.Errorf("%s is not in the download", suffix)
}

// progress is what ffmpeg's -progress output says so far.
type progress struct {
	frames    int64
	outTimeUs int64
}

// readProgress reads "key=value" lines of ffmpeg's -progress output and calls
// onFirstFrame once, when the first frame has been encoded, with how much
// video was encoded at that moment. Other lines go to logf.
func readProgress(r io.Reader, onFirstFrame func(outTimeUs int64), logf func(string)) {
	var p progress
	seen := false
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			logf(sc.Text())
			continue
		}
		switch key {
		case "frame":
			p.frames, _ = strconv.ParseInt(value, 10, 64)
		case "out_time_us":
			p.outTimeUs, _ = strconv.ParseInt(value, 10, 64)
		case "progress": // the last line of each report
			if !seen && p.frames > 0 {
				seen = true
				onFirstFrame(max(p.outTimeUs, 0))
			}
		}
	}
}
