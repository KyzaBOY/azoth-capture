package video

import (
	"strconv"
	"strings"

	"azoth/capture/internal/steam"
)

// The Windows recorder is ffmpeg with its gfxcapture source (Windows Graphics
// Capture, ffmpeg 8.1 and newer). This file builds its command lines; it has
// no build tag so the tests run on every system.

const (
	wgcFPS = 60
	// Clips are scaled to fit 1920x1080, like on Linux.
	wgcMaxW, wgcMaxH = 1920, 1080
	progressPeriod   = "0.5" // seconds between -progress reports
)

// encoder is one ffmpeg video encoder for a format, with the setting that
// sets its picture quality. levels gives that setting's value for each of
// Qualities: lower values mean better pictures and bigger files.
type encoder struct {
	name   string
	codec  string
	base   []string
	qflags []string // the quality setting(s); all get the same value
	levels map[string]string
}

// args are the encoder's settings for a quality level.
func (e encoder) args(quality string) []string {
	a := append([]string{}, e.base...)
	for _, f := range e.qflags {
		a = append(a, f, e.levels[qualityOr(quality)])
	}
	return a
}

func levels(medium, high, veryHigh string) map[string]string {
	return map[string]string{QualityMedium: medium, QualityHigh: high, QualityVeryHigh: veryHigh}
}

// encoders are tried per format, most compact format first (see codecOrder).
// Within a format the GPU encoders come first (NVIDIA, AMD, Intel, then
// Windows' Media Foundation) and the CPU encoder last, so H.264 always has
// one that works. The AV1 encoders use their own quality scales (AMF's goes
// to 255); they are not yet tested on a real Windows PC.
var encoders = []encoder{
	{"av1_nvenc", CodecAV1, []string{"-preset", "p4", "-rc", "vbr"}, []string{"-cq"}, levels("34", "30", "26")},
	{"av1_amf", CodecAV1, []string{"-quality", "balanced", "-rc", "cqp"}, []string{"-qp_i", "-qp_p"}, levels("140", "120", "100")},
	{"hevc_nvenc", CodecHEVC, []string{"-preset", "p4", "-rc", "vbr"}, []string{"-cq"}, levels("28", "25", "22")},
	{"hevc_amf", CodecHEVC, []string{"-quality", "balanced", "-rc", "cqp"}, []string{"-qp_i", "-qp_p"}, levels("28", "25", "22")},
	{"hevc_qsv", CodecHEVC, []string{"-preset", "medium"}, []string{"-global_quality"}, levels("28", "25", "22")},
	{"h264_nvenc", CodecH264, []string{"-preset", "p4", "-rc", "vbr"}, []string{"-cq"}, levels("26", "23", "20")},
	{"h264_amf", CodecH264, []string{"-quality", "balanced", "-rc", "cqp"}, []string{"-qp_i", "-qp_p"}, levels("26", "23", "20")},
	{"h264_qsv", CodecH264, []string{"-preset", "medium"}, []string{"-global_quality"}, levels("26", "23", "20")},
	{"h264_mf", CodecH264, nil, []string{"-b:v"}, levels("8M", "12M", "18M")},
	{"libx264", CodecH264, []string{"-preset", "veryfast"}, []string{"-crf"}, levels("26", "23", "20")},
}

// encodersFor lists the encoders to try for a chosen format, in order.
func encodersFor(codec string) []encoder {
	var out []encoder
	for _, c := range codecOrder(codecOr(codec)) {
		for _, e := range encoders {
			if e.codec == c {
				out = append(out, e)
			}
		}
	}
	return out
}

// encoderTestArgs encode a few frames of a test picture with enc, to learn
// whether this PC can use it.
func encoderTestArgs(enc encoder) []string {
	a := []string{"-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "lavfi", "-i", "color=c=black:s=256x144:r=30:d=0.2", "-pix_fmt", "yuv420p", "-c:v", enc.name}
	a = append(a, enc.args(QualityHigh)...)
	return append(a, "-f", "null", "-")
}

// gfxcaptureSource is the filter graph that captures the window of the game
// in gameExe (the picked file). Its frames live on the GPU; hwdownload brings
// them back for the encoder.
func gfxcaptureSource(gameExe string) string {
	return "gfxcapture=window_exe=" + gameExeMatch(gameExe) + ":max_framerate=" + strconv.Itoa(wgcFPS) +
		",hwdownload,format=bgra"
}

// gameExeMatch finds the game window by its program's file name, so the
// player never picks a window. gfxcapture takes
// an ECMAScript regular expression. Letters, digits, "-" and "_" match
// themselves and "." is escaped; any other character matches any one
// character, so no file name can break the regular expression or the filter
// graph around it.
func gameExeMatch(gameExe string) string {
	var b strings.Builder
	b.WriteString("(?i)^")
	for _, c := range steam.GameFileName(gameExe) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			b.WriteRune(c)
		case c == '.':
			b.WriteString(`\.`)
		default:
			b.WriteByte('.')
		}
	}
	b.WriteString("$")
	return b.String()
}

// recordArgs is the ffmpeg command line for one clip from source (the
// gfxcapture graph; tests pass a test picture) at a quality level. The
// encoder writes to out.
// ffmpeg stops cleanly when it reads "q" on its input.
func recordArgs(source string, enc encoder, quality, out string) []string {
	scale := "scale=w='min(" + strconv.Itoa(wgcMaxW) + ",iw)':h='min(" + strconv.Itoa(wgcMaxH) +
		",ih)':force_original_aspect_ratio=decrease:force_divisible_by=2,format=yuv420p"
	a := []string{"-hide_banner", "-loglevel", "error", "-y",
		"-stats_period", progressPeriod, "-progress", "pipe:1",
		"-f", "lavfi", "-i", source, "-vf", scale, "-r", strconv.Itoa(wgcFPS), "-c:v", enc.name}
	a = append(a, enc.args(quality)...)
	return append(a, out)
}
