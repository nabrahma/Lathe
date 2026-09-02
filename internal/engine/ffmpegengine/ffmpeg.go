// Package ffmpegengine performs media tasks by driving FFmpeg as a
// subprocess.
//
// FFmpeg reports elapsed output time on stderr, so progress here is real
// rather than a guess. That matters: a bar that jumps to 90% and stops teaches
// people to distrust every bar the app ever shows them.
package ffmpegengine

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nabrahma/lathe/internal/deps"
	"github.com/nabrahma/lathe/internal/engine"
	lexec "github.com/nabrahma/lathe/internal/exec"
	"github.com/nabrahma/lathe/internal/usererr"
)

// componentID names the manifest entry providing ffmpeg and ffprobe.
const componentID = "ffmpeg"

// Engine performs media tasks.
type Engine struct {
	deps   deps.Manager
	runner lexec.Runner
}

// New returns the media engine backed by the given component manager.
func New(m deps.Manager) *Engine {
	return &Engine{deps: m, runner: lexec.New()}
}

// ID identifies the engine in the task registry.
func (e *Engine) ID() string { return "ffmpeg" }

// Available reports whether the media component is installed and runs.
func (e *Engine) Available() bool {
	return e.deps != nil && e.deps.Available(componentID)
}

// Execute converts each input in turn.
func (e *Engine) Execute(ctx context.Context, req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	bin, err := e.deps.BinaryPath(componentID, "ffmpeg")
	if err != nil {
		return nil, usererr.Wrap(err, usererr.CodeComponentMissing,
			"Video support isn't installed yet.", usererr.ActionDownload)
	}

	var outputs []string
	for i, in := range req.Inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		out, args, err := e.plan(req, in)
		if err != nil {
			return nil, err
		}

		total := e.duration(ctx, in)
		label := filepath.Base(in)
		if len(req.Inputs) > 1 {
			label = fmt.Sprintf("%s (%d of %d)", label, i+1, len(req.Inputs))
		}

		if err := e.run(ctx, bin, args, total, label, progress); err != nil {
			return nil, err
		}
		outputs = append(outputs, out)
	}
	return &engine.Response{Outputs: outputs}, nil
}

// plan turns a task request into an FFmpeg argument list. Arguments are always
// a slice, never a formatted string, so a filename cannot become a flag.
func (e *Engine) plan(req engine.Request, in string) (out string, args []string, err error) {
	base := filepath.Base(in)
	base = strings.TrimSuffix(base, filepath.Ext(base))

	// -y is safe here: the output always lands in a private workspace, never
	// on a file the user owns. A fresh slice per call, because appending to a
	// shared one would let two inputs alias the same backing array.
	common := func() []string {
		return []string{"-hide_banner", "-nostdin", "-y", "-i", in}
	}

	switch req.Task.ID {
	case "media.convert-video", "media.compress-video":
		format := req.Options.String("format", "mp4")
		out = req.Workspace.UniqueName(base + "." + format)

		args = append(common(), videoArgs(format, req.Options)...)
		if h := req.Options.Int("maxHeight", 0); h > 0 {
			// Scale down only, and keep the width even, which most encoders
			// require.
			args = append(args, "-vf", fmt.Sprintf("scale=-2:'min(%d,ih)'", h))
		}
		args = append(args, out)

	case "media.extract-audio", "media.convert-audio":
		format := req.Options.String("format", "mp3")
		out = req.Workspace.UniqueName(base + "." + format)

		args = append(common(), "-vn")
		args = append(args, audioArgs(format, req.Options.Int("bitrate", 192))...)
		args = append(args, out)

	case "media.video-to-gif":
		out = req.Workspace.UniqueName(base + ".gif")
		width := req.Options.Int("width", 480)

		args = append([]string{"-hide_banner", "-nostdin", "-y"},
			"-ss", req.Options.String("start", "0"),
			"-t", strconv.Itoa(req.Options.Int("duration", 5)),
			"-i", in,
			// One shared palette gives a far better GIF than the default
			// 216-colour web palette, at no extra cost.
			"-vf", fmt.Sprintf("fps=12,scale=%d:-1:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse", width),
			"-loop", "0",
			out)

	default:
		return "", nil, fmt.Errorf("media engine cannot handle task %q", req.Task.ID)
	}
	return out, args, nil
}

// videoArgs maps a plain quality word onto encoder settings. The user never
// sees a CRF number; they see "Balanced".
func videoArgs(format string, opts engine.Options) []string {
	crf, preset := 23, "medium"
	switch opts.String("quality", "medium") {
	case "low":
		crf, preset = 30, "faster"
	case "high":
		crf, preset = 18, "slow"
	}

	switch format {
	case "webm":
		return []string{"-c:v", "libvpx-vp9", "-crf", strconv.Itoa(crf), "-b:v", "0", "-c:a", "libopus"}
	default:
		return []string{
			"-c:v", "libx264", "-crf", strconv.Itoa(crf), "-preset", preset,
			// yuv420p and faststart are what make the result play in browsers
			// and on phones rather than only in VLC.
			"-pix_fmt", "yuv420p", "-movflags", "+faststart",
			"-c:a", "aac", "-b:a", "160k",
		}
	}
}

func audioArgs(format string, bitrate int) []string {
	if bitrate < 32 || bitrate > 512 {
		bitrate = 192
	}
	rate := strconv.Itoa(bitrate) + "k"

	switch format {
	case "wav":
		return []string{"-c:a", "pcm_s16le"}
	case "flac":
		return []string{"-c:a", "flac"}
	case "m4a":
		return []string{"-c:a", "aac", "-b:a", rate}
	default:
		return []string{"-c:a", "libmp3lame", "-b:a", rate}
	}
}

// duration asks ffprobe how long the input is, so progress can be a real
// percentage. A failure here is not fatal: the job runs with an indeterminate
// bar instead.
func (e *Engine) duration(ctx context.Context, in string) time.Duration {
	probe, err := e.deps.BinaryPath(componentID, "ffprobe")
	if err != nil {
		return 0
	}

	res, err := e.runner.Run(ctx, probe, []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		in,
	}, lexec.Options{Timeout: 30 * time.Second})
	if err != nil {
		return 0
	}

	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(res.Stdout)), 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

func (e *Engine) run(ctx context.Context, bin string, args []string,
	total time.Duration, label string, progress func(engine.Progress),
) error {
	_, err := e.runner.RunStreaming(ctx, bin, args, lexec.Options{
		// Long conversions are normal; the job is cancellable, so a short
		// timeout would only punish people with large files.
		Timeout: 4 * time.Hour,
	}, func(_ lexec.Stream, line string) {
		at, ok := parseTime(line)
		if !ok {
			return
		}
		if total <= 0 {
			progress(engine.Progress{Fraction: -1, Stage: fmt.Sprintf("Converting %s", label)})
			return
		}
		frac := float64(at) / float64(total)
		if frac > 1 {
			frac = 1
		}
		progress(engine.Progress{
			Fraction: frac,
			Stage:    fmt.Sprintf("Converting %s — %s of %s", label, short(at), short(total)),
		})
	})
	return err
}

// parseTime pulls the "time=00:01:23.45" field out of an FFmpeg status line.
func parseTime(line string) (time.Duration, bool) {
	i := strings.Index(line, "time=")
	if i < 0 {
		return 0, false
	}
	field := strings.Fields(line[i+len("time="):])
	if len(field) == 0 {
		return 0, false
	}
	return parseClock(field[0])
}

// parseClock reads FFmpeg's HH:MM:SS.mmm form. "N/A" appears at the very start
// of a run and simply means "not yet known".
func parseClock(s string) (time.Duration, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, false
	}

	hours, err1 := strconv.Atoi(parts[0])
	minutes, err2 := strconv.Atoi(parts[1])
	seconds, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	if hours < 0 || minutes < 0 || seconds < 0 {
		return 0, false
	}
	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds*float64(time.Second)), true
}

func short(d time.Duration) string {
	d = d.Round(time.Second)
	if h := int(d.Hours()); h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, int(d.Minutes())%60, int(d.Seconds())%60)
	}
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

var _ engine.Engine = (*Engine)(nil)
