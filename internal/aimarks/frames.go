package aimarks

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"

	"azoth/capture/internal/proc"
)

// Frame sampling. Frames are small JPEGs: big enough to read on-screen text,
// small enough to send many of them.
const (
	frameWidth     = 960
	frameQuality   = "4" // ffmpeg -q:v, 2 (best) to 31
	framePattern   = "f%06d.jpg"
	changeGridW    = 32 // the change score compares a 32x18 grid of pixels
	changeGridH    = 18
	idleChange     = 2.0 // below this mean difference (0-255) nothing moved
	windowFrames   = 10  // frames the model sees per question
	MinSampleEvery = 1
	MaxSampleEvery = 10
)

// Frame is one sampled picture of a clip.
type Frame struct {
	Path   string // absolute path of the JPEG
	ClipMs int64  // offset from the clip's first frame
	UnixMs int64  // the same moment on the PC clock
	Change float64
	Index  int // position in the clip's frame list
}

// SampleClip writes one frame every everySec seconds of clip into outDir
// with the given ffmpeg. firstFrameUnixMs is when the clip's first frame was
// captured.
func SampleClip(ctx context.Context, ffmpeg, clip, outDir string, everySec int, firstFrameUnixMs int64) ([]Frame, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	cmd := proc.Hidden(exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", clip,
		"-vf", "fps=1/"+strconv.Itoa(everySec)+",scale="+strconv.Itoa(frameWidth)+":-2",
		"-q:v", frameQuality,
		filepath.Join(outDir, framePattern)))
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg could not read %s: %v: %s", filepath.Base(clip), err, out)
	}
	paths, err := filepath.Glob(filepath.Join(outDir, "f*.jpg"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	frames := make([]Frame, len(paths))
	var prev []float64
	for i, p := range paths {
		ms := int64(i) * int64(everySec) * 1000
		frames[i] = Frame{Path: p, ClipMs: ms, UnixMs: firstFrameUnixMs + ms, Index: i}
		grid, err := grayGrid(p)
		if err != nil {
			return nil, err
		}
		if prev != nil {
			frames[i].Change = meanDiff(prev, grid)
		}
		prev = grid
	}
	return frames, nil
}

// grayGrid shrinks a frame to a small grid of gray values.
func grayGrid(path string) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("frame %s: %w", filepath.Base(path), err)
	}
	return sampleGray(img), nil
}

func sampleGray(img image.Image) []float64 {
	b := img.Bounds()
	grid := make([]float64, 0, changeGridW*changeGridH)
	for gy := range changeGridH {
		for gx := range changeGridW {
			x := b.Min.X + (2*gx+1)*b.Dx()/(2*changeGridW)
			y := b.Min.Y + (2*gy+1)*b.Dy()/(2*changeGridH)
			r, g, bl, _ := img.At(x, y).RGBA()
			grid = append(grid, (0.299*float64(r)+0.587*float64(g)+0.114*float64(bl))/257)
		}
	}
	return grid
}

func meanDiff(a, b []float64) float64 {
	var sum float64
	for i := range a {
		d := a[i] - b[i]
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return sum / float64(len(a))
}

// Window is a run of frames the model sees in one question.
type Window struct {
	Frames []Frame
}

// Windows cuts frames into windows of windowFrames and drops windows where
// nothing moved, for example while the player is away from the keyboard.
func Windows(frames []Frame) []Window {
	var out []Window
	for start := 0; start < len(frames); start += windowFrames {
		end := min(start+windowFrames, len(frames))
		active := false
		for _, f := range frames[start:end] {
			if f.Change >= idleChange {
				active = true
				break
			}
		}
		if active {
			out = append(out, Window{Frames: frames[start:end]})
		}
	}
	return out
}
